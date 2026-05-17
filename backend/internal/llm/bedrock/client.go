package bedrock

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"

	"object-lens-search-demo/backend/internal/model"
)

type RuntimeAPI interface {
	InvokeModel(ctx context.Context, params *bedrockruntime.InvokeModelInput, optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error)
}

type Client struct {
	Runtime      RuntimeAPI
	ModelID      string
	LightModelID string
}

func New(runtime RuntimeAPI, modelID string) *Client {
	return &Client{Runtime: runtime, ModelID: modelID}
}

func NewWithLightModel(runtime RuntimeAPI, modelID string, lightModelID string) *Client {
	return &Client{Runtime: runtime, ModelID: modelID, LightModelID: lightModelID}
}

func (c *Client) RecognizeObject(ctx context.Context, req model.RecognizeObjectRequest) (*model.RecognizeObjectResponse, error) {
	if c.Runtime == nil || strings.TrimSpace(c.ModelID) == "" {
		return nil, errors.New("bedrock runtime and model ID are required")
	}
	mediaType, data, err := parseDataURL(req.ImageDataURL)
	if err != nil {
		return nil, err
	}
	prompt := recognitionPrompt(req.Language, req.VisualEvidence)
	body := anthropicRequest{
		AnthropicVersion: "bedrock-2023-05-31",
		MaxTokens:        700,
		Temperature:      aws.Float64(0.2),
		Messages: []anthropicMessage{{Role: "user", Content: []anthropicContent{
			{Type: "image", Source: &anthropicImageSource{Type: "base64", MediaType: mediaType, Data: data}},
			{Type: "text", Text: prompt},
		}}},
	}
	var parsed model.RecognizedObject
	if err := c.invokeJSON(ctx, body, &parsed); err != nil {
		return nil, err
	}
	if parsed.Confidence == "" {
		parsed.Confidence = "medium"
	}
	return &model.RecognizeObjectResponse{Object: parsed, Model: c.ModelID}, nil
}

func (c *Client) SummarizeSearchResults(ctx context.Context, req model.SummarizeSearchResultsRequest) (*model.SummarizeSearchResultsResponse, error) {
	if c.Runtime == nil || strings.TrimSpace(c.ModelID) == "" {
		return nil, errors.New("bedrock runtime and model ID are required")
	}
	compact, err := json.Marshal(compactSearchResults(req.Results))
	if err != nil {
		return nil, fmt.Errorf("marshal search results: %w", err)
	}
	prompt := summarizePrompt(req.Language, req.RecognizedObject, string(compact))
	body := anthropicRequest{
		AnthropicVersion: "bedrock-2023-05-31",
		MaxTokens:        500,
		Temperature:      aws.Float64(0.2),
		Messages:         []anthropicMessage{{Role: "user", Content: []anthropicContent{{Type: "text", Text: prompt}}}},
	}
	var parsed struct {
		Text string `json:"text"`
	}
	if err := c.invokeJSON(ctx, body, &parsed); err != nil {
		return nil, err
	}
	return &model.SummarizeSearchResultsResponse{Text: parsed.Text, Model: c.ModelID}, nil
}

func (c *Client) HypothesizeObject(ctx context.Context, req model.HypothesizeObjectRequest) (*model.HypothesizeObjectResponse, error) {
	modelID := strings.TrimSpace(c.LightModelID)
	if modelID == "" {
		modelID = strings.TrimSpace(c.ModelID)
	}
	if c.Runtime == nil || modelID == "" {
		return nil, errors.New("bedrock runtime and light model ID are required")
	}
	mediaType, data, err := parseDataURL(req.ImageDataURL)
	if err != nil {
		return nil, err
	}
	prompt := hypothesisPrompt(req.Language, req.VisualEvidence)
	body := anthropicRequest{
		AnthropicVersion: "bedrock-2023-05-31",
		MaxTokens:        350,
		Temperature:      aws.Float64(0.1),
		Messages: []anthropicMessage{{Role: "user", Content: []anthropicContent{
			{Type: "image", Source: &anthropicImageSource{Type: "base64", MediaType: mediaType, Data: data}},
			{Type: "text", Text: prompt},
		}}},
	}
	var parsed model.RecognizedObject
	if err := c.invokeJSONWithModel(ctx, modelID, body, &parsed); err != nil {
		return nil, err
	}
	if parsed.Confidence == "" {
		parsed.Confidence = "low"
	}
	return &model.HypothesizeObjectResponse{Object: parsed, Model: modelID}, nil
}

func (c *Client) invokeJSON(ctx context.Context, request anthropicRequest, target any) error {
	return c.invokeJSONWithModel(ctx, c.ModelID, request, target)
}

func (c *Client) invokeJSONWithModel(ctx context.Context, modelID string, request anthropicRequest, target any) error {
	body, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("marshal bedrock request: %w", err)
	}
	out, err := c.Runtime.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{ModelId: aws.String(modelID), ContentType: aws.String("application/json"), Accept: aws.String("application/json"), Body: body})
	if err != nil {
		return fmt.Errorf("invoke bedrock model: %w", err)
	}
	var resp anthropicResponse
	if err := json.Unmarshal(out.Body, &resp); err != nil {
		return fmt.Errorf("decode bedrock response: %w", err)
	}
	text := strings.TrimSpace(resp.firstText())
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)
	if err := json.Unmarshal([]byte(text), target); err != nil {
		return fmt.Errorf("decode model JSON: %w", err)
	}
	return nil
}

type anthropicRequest struct {
	AnthropicVersion string             `json:"anthropic_version"`
	MaxTokens        int                `json:"max_tokens"`
	Temperature      *float64           `json:"temperature,omitempty"`
	Messages         []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string             `json:"role"`
	Content []anthropicContent `json:"content"`
}

type anthropicContent struct {
	Type   string                `json:"type"`
	Text   string                `json:"text,omitempty"`
	Source *anthropicImageSource `json:"source,omitempty"`
}

type anthropicImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

func (r anthropicResponse) firstText() string {
	for _, item := range r.Content {
		if item.Type == "text" && item.Text != "" {
			return item.Text
		}
	}
	return ""
}

func parseDataURL(value string) (string, string, error) {
	meta, payload, ok := strings.Cut(value, ";base64,")
	if !ok || !strings.HasPrefix(meta, "data:") {
		return "", "", errors.New("invalid data URL")
	}
	if _, err := base64.StdEncoding.DecodeString(payload); err != nil {
		return "", "", fmt.Errorf("invalid image base64: %w", err)
	}
	return strings.TrimPrefix(meta, "data:"), payload, nil
}

type compactSearchResult struct {
	Title       string  `json:"title"`
	URL         string  `json:"url"`
	Snippet     string  `json:"snippet"`
	Source      string  `json:"source"`
	Rank        int     `json:"rank"`
	Score       float64 `json:"score"`
	ContentType string  `json:"contentType,omitempty"`
}

func compactSearchResults(results []model.NormalizedSearchResult) []compactSearchResult {
	compact := make([]compactSearchResult, 0, len(results))
	for _, result := range results {
		compact = append(compact, compactSearchResult{
			Title:       result.Title,
			URL:         result.URL,
			Snippet:     result.Snippet,
			Source:      result.Source,
			Rank:        result.Rank,
			Score:       result.Score,
			ContentType: result.ContentType,
		})
	}
	return compact
}

func recognitionPrompt(language string, evidence *model.VisualEvidence) string {
	base := `You are a product identification expert. Your goal is to determine the EXACT commercial product shown so a user can find it on an e-commerce or manufacturer website.

Return ONLY valid JSON (no markdown, no explanation) with these fields:
- objectName: exact product name including brand and model (e.g. "Nike Air Max 270 React")
- description: one-sentence description in language ` + language + `
- searchQuery: the single best query to find the official product listing page. Follow this priority:
    1. "<Brand> <ModelNumber/SKU>" — HIGHEST PRIORITY if a model number is detected (see below)
    2. "<Brand> <ModelName> <ModelNumber/SKU>" — use if both name and number are identifiable
    3. "<Brand> <ProductCategory> <KeyDistinguishingFeature>"
    4. Descriptive fallback (last resort only)
  IMPORTANT: Do NOT describe visual appearance (color, shape). Generate the query a buyer would type to find THIS specific product.
- searchQueries: array of up to 3 additional query objects, each with "query" (string) and "intent" (string). Use these intents:
    - "product_page": query targeting the official product or purchase page
    - "model_lookup": query to identify the exact model/variant/SKU — MUST include the model number if one is detected
    - "price_comparison": query to compare prices across sellers
- confidence: "low"|"medium"|"high"
- needsMoreContext: true only if brand and model cannot be determined at all

Use language ` + language + ` for objectName and description.`
	if evidence == nil || evidence.Empty() {
		return base
	}
	modelNumbers := extractModelNumbers(evidence)
	modelNumberHint := ""
	if len(modelNumbers) > 0 {
		modelNumberHint = "\n\n⚠️  DETECTED MODEL NUMBER(S) in OCR text: " + strings.Join(modelNumbers, ", ") + `
These look like product model numbers or SKUs. If they match the product in the image:
- Use the model number directly in searchQuery (e.g. "<Brand> ` + strings.Join(modelNumbers[:1], "") + `")
- Include it in the "model_lookup" entry of searchQueries
- Do NOT ignore them in favour of a generic description`
	}
	compact, err := json.Marshal(evidence)
	if err != nil {
		return base + modelNumberHint
	}
	return base + modelNumberHint + `

Google Cloud Vision evidence below — treat OCR, logo, and webEntities as strong signals; labels as weak signals. Use them to sharpen the product identification, not to override your visual analysis.
Evidence JSON: ` + string(compact)
}

// extractModelNumbers returns OCR strings that look like product model numbers
// (alphanumeric, 3–25 characters, contains both letters and digits).
func extractModelNumbers(evidence *model.VisualEvidence) []string {
	if evidence == nil {
		return nil
	}
	var out []string
	seen := make(map[string]struct{})
	for _, item := range evidence.OCR {
		text := strings.TrimSpace(item.Text)
		if !likelyModelNumber(text) {
			continue
		}
		key := strings.ToLower(text)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, text)
	}
	return out
}

// likelyModelNumber returns true when s looks like a product model number:
// 3–25 chars, contains both letters and digits.
func likelyModelNumber(s string) bool {
	if len(s) < 3 || len(s) > 25 {
		return false
	}
	hasLetter, hasDigit := false, false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			hasLetter = true
		case r >= '0' && r <= '9':
			hasDigit = true
		}
	}
	return hasLetter && hasDigit
}

func summarizePrompt(language string, object model.RecognizedObject, results string) string {
	evidence := ""
	if object.VisualEvidence != nil && !object.VisualEvidence.Empty() {
		compact, err := json.Marshal(object.VisualEvidence)
		if err == nil {
			evidence = " Cloud Vision evidence JSON: " + string(compact)
		}
	}
	return fmt.Sprintf("Summarize these search results for object %q. Return only valid JSON {\"text\":\"...\"}. Use language %s.%s Results JSON: %s", object.ObjectName, language, evidence, results)
}

func hypothesisPrompt(language string, evidence *model.VisualEvidence) string {
	base := `You are a product identification expert. Quickly identify the most likely commercial product shown.

Return ONLY valid JSON (no markdown) with these fields:
- objectName: brand + model name if possible (e.g. "Nike Air Max 270")
- description: one-line description in language ` + language + `
- searchQuery: a web search query that would find the official product page — if a model number is detected (see below), use "<Brand> <ModelNumber>" as the query
- confidence: "low"|"medium"|"high"
- needsMoreContext: true if product cannot be identified

Use language ` + language + `.`
	if evidence == nil || evidence.Empty() {
		return base
	}
	modelNumbers := extractModelNumbers(evidence)
	modelNumberHint := ""
	if len(modelNumbers) > 0 {
		modelNumberHint = "\n\n⚠️  DETECTED MODEL NUMBER(S) in OCR: " + strings.Join(modelNumbers, ", ") + " — prioritize these in searchQuery."
	}
	compact, err := json.Marshal(evidence)
	if err != nil {
		return base + modelNumberHint
	}
	return base + modelNumberHint + `

Cloud Vision signals (treat as supporting evidence — OCR/logo/webEntities are strongest):
` + string(compact)
}
