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
	prompt := recognitionPrompt(req.Language, req.VisualEvidence)
	content, err := imageRequestContent(req.ImageDataURL, req.Crops, req.Images, prompt)
	if err != nil {
		return nil, err
	}
	body := anthropicRequest{
		AnthropicVersion: "bedrock-2023-05-31",
		MaxTokens:        700,
		Temperature:      aws.Float64(0.2),
		Messages:         []anthropicMessage{{Role: "user", Content: content}},
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
		Text            string `json:"text"`
		DisplayName     string `json:"displayName"`
		Category        string `json:"category"`
		FinalObjectName string `json:"finalObjectName"`
	}
	if err := c.invokeJSON(ctx, body, &parsed); err != nil {
		return nil, err
	}
	return &model.SummarizeSearchResultsResponse{Text: parsed.Text, DisplayName: parsed.DisplayName, Category: parsed.Category, FinalObjectName: parsed.FinalObjectName, Model: c.ModelID}, nil
}

func (c *Client) HypothesizeObject(ctx context.Context, req model.HypothesizeObjectRequest) (*model.HypothesizeObjectResponse, error) {
	modelID := strings.TrimSpace(c.LightModelID)
	if modelID == "" {
		modelID = strings.TrimSpace(c.ModelID)
	}
	if c.Runtime == nil || modelID == "" {
		return nil, errors.New("bedrock runtime and light model ID are required")
	}
	prompt := hypothesisPrompt(req.Language, req.VisualEvidence)
	content, err := imageRequestContent(req.ImageDataURL, req.Crops, req.Images, prompt)
	if err != nil {
		return nil, err
	}
	body := anthropicRequest{
		AnthropicVersion: "bedrock-2023-05-31",
		MaxTokens:        350,
		Temperature:      aws.Float64(0.1),
		Messages:         []anthropicMessage{{Role: "user", Content: content}},
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

const maxBedrockImageInputs = 10

type bedrockImagePart struct {
	label   string
	dataURL string
}

func imageRequestContent(primaryImageDataURL string, crops *model.ImageCrops, images []model.RecognizeImageInput, prompt string) ([]anthropicContent, error) {
	parts := bedrockImageParts(primaryImageDataURL, crops, images)
	if len(parts) == 0 {
		return nil, errors.New("image data URL is required")
	}
	content := make([]anthropicContent, 0, len(parts)*2+1)
	for _, part := range parts {
		mediaType, data, err := parseDataURL(part.dataURL)
		if err != nil {
			return nil, err
		}
		if part.label != "" {
			content = append(content, anthropicContent{Type: "text", Text: part.label})
		}
		content = append(content, anthropicContent{Type: "image", Source: &anthropicImageSource{Type: "base64", MediaType: mediaType, Data: data}})
	}
	content = append(content, anthropicContent{Type: "text", Text: prompt})
	return content, nil
}

func bedrockImageParts(primaryImageDataURL string, crops *model.ImageCrops, images []model.RecognizeImageInput) []bedrockImagePart {
	parts := make([]bedrockImagePart, 0, maxBedrockImageInputs)
	seen := map[string]struct{}{}
	appendPart := func(label string, value string) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" || len(parts) >= maxBedrockImageInputs {
			return
		}
		if _, exists := seen[trimmed]; exists {
			return
		}
		seen[trimmed] = struct{}{}
		parts = append(parts, bedrockImagePart{label: label, dataURL: trimmed})
	}
	if len(images) > 0 {
		for i, image := range images {
			baseLabel := fmt.Sprintf("Image %d", i+1)
			if id := strings.TrimSpace(image.ID); id != "" {
				baseLabel += " id=" + id
			}
			if role := strings.TrimSpace(image.Role); role != "" {
				baseLabel += " role=" + role
			}
			appendPart(baseLabel+" primary crop", image.ImageDataURL)
			if image.Crops != nil {
				appendPart(baseLabel+" tight crop", image.Crops.TightCrop)
				appendPart(baseLabel+" context crop", image.Crops.ContextCrop)
				appendPart(baseLabel+" text enhanced crop", image.Crops.TextEnhancedCrop)
			}
		}
		return parts
	}
	appendPart("Primary image", primaryImageDataURL)
	if crops != nil {
		appendPart("Tight crop", crops.TightCrop)
		appendPart("Context crop", crops.ContextCrop)
		appendPart("Text enhanced crop", crops.TextEnhancedCrop)
	}
	return parts
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
	base := "Identify the main object across the provided image(s). Return only valid JSON with objectName, displayName, category, finalObjectName, description, searchQuery, confidence (low|medium|high), needsMoreContext. objectName is the initial visual label, displayName is the exact product/title/name when visible or web-supported, category is the object/media/product type, and finalObjectName is the best UI headline combining displayName and category. Use language " + language + ". Do not include markdown."
	if evidence == nil || evidence.Empty() {
		return base
	}
	compact, err := json.Marshal(evidence)
	if err != nil {
		return base
	}
	return base + " Use these Google Cloud Vision evidence signals as corroborating evidence, not as absolute truth. Prefer OCR text and catalog/title-like web entities over coarse labels when they agree. Evidence JSON: " + string(compact)
}

func summarizePrompt(language string, object model.RecognizedObject, results string) string {
	evidence := ""
	if object.VisualEvidence != nil && !object.VisualEvidence.Empty() {
		compact, err := json.Marshal(object.VisualEvidence)
		if err == nil {
			evidence = " Cloud Vision evidence JSON: " + string(compact)
		}
	}
	return fmt.Sprintf("Summarize these search results for object %q. Return only valid JSON {\"text\":\"...\",\"displayName\":\"...\",\"category\":\"...\",\"finalObjectName\":\"...\"}. Use language %s. Choose displayName/category/finalObjectName from the same evidence used for the summary: OCR, Cloud Vision web entities, and search result titles/snippets. finalObjectName must be a concrete UI headline, not a broad category, when search/OCR identify a title or product.%s Results JSON: %s", objectIdentityForPrompt(object), language, evidence, results)
}

func objectIdentityForPrompt(object model.RecognizedObject) string {
	for _, value := range []string{object.FinalObjectName, object.DisplayName, object.ObjectName} {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return "unknown object"
}

func hypothesisPrompt(language string, evidence *model.VisualEvidence) string {
	base := "Quickly identify likely main object across the provided image(s). Return only valid JSON with objectName, displayName, category, finalObjectName, description, searchQuery, confidence (low|medium|high), needsMoreContext. Prefer concise searchQuery suitable for web search. Use language " + language + ". Do not include markdown."
	if evidence == nil || evidence.Empty() {
		return base
	}
	compact, err := json.Marshal(evidence)
	if err != nil {
		return base
	}
	return base + " Consider these Cloud Vision signals as weak evidence. Evidence JSON: " + string(compact)
}
