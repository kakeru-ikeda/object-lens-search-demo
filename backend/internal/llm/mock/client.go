package mock

import (
	"context"
	"fmt"
	"strings"

	"object-lens-search-demo/backend/internal/model"
)

type Client struct {
	Model string
}

func (c *Client) RecognizeObject(ctx context.Context, req model.RecognizeObjectRequest) (*model.RecognizeObjectResponse, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	imageCount := len(req.Images)
	objectName := "sample object"
	displayName := "sample object"
	category := "sample object"
	description := "The image appears to contain a sample object."
	query := "sample object overview"
	confidence := "medium"
	if imageCount > 1 {
		objectName = fmt.Sprintf("sample object from %d images", imageCount)
		displayName = objectName
		description = fmt.Sprintf("The %d supplied images are combined into one sample object result.", imageCount)
		query = fmt.Sprintf("sample object overview %d images", imageCount)
	}
	if req.Language == "ja" {
		objectName = "サンプル物体"
		displayName = "サンプル物体"
		category = "サンプル物体"
		description = "画像内の主対象物はサンプル物体のように見えます。"
		query = "サンプル物体 特徴 使い方"
		if imageCount > 1 {
			objectName = fmt.Sprintf("%d枚のサンプル物体", imageCount)
			displayName = objectName
			description = fmt.Sprintf("%d枚の画像シグナルを統合したサンプル物体の結果です。", imageCount)
			query = fmt.Sprintf("サンプル物体 %d枚 統合 特徴", imageCount)
		}
	}
	if req.VisualEvidence != nil && !req.VisualEvidence.Empty() {
		if value := evidenceDisplayName(req.VisualEvidence); value != "" {
			objectName = value
			displayName = value
			query = value
			confidence = "high"
		}
		if value := evidenceCategory(req.VisualEvidence, req.Language); value != "" {
			category = value
		}
	}
	return &model.RecognizeObjectResponse{Object: model.RecognizedObject{ObjectName: objectName, DisplayName: displayName, Category: category, FinalObjectName: composeFinalName(displayName, category), Description: description, SearchQuery: query, Confidence: confidence, NeedsMoreContext: false}, Model: c.modelName()}, nil
}

func (c *Client) HypothesizeObject(ctx context.Context, req model.HypothesizeObjectRequest) (*model.HypothesizeObjectResponse, error) {
	resp, err := c.RecognizeObject(ctx, model.RecognizeObjectRequest{ImageDataURL: req.ImageDataURL, Crops: req.Crops, Images: req.Images, MIMEType: req.MIMEType, CropMIMETypes: req.CropMIMETypes, Language: req.Language, VisualEvidence: req.VisualEvidence})
	if err != nil {
		return nil, err
	}
	object := resp.Object
	if req.Language == "ja" {
		object.Description = "軽量モデルによる暫定仮説です。"
	} else {
		object.Description = "Interim hypothesis from lightweight model."
	}
	return &model.HypothesizeObjectResponse{Object: object, Model: c.modelName() + "-light"}, nil
}

func (c *Client) SummarizeSearchResults(ctx context.Context, req model.SummarizeSearchResultsRequest) (*model.SummarizeSearchResultsResponse, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	text := fmt.Sprintf("Found %d search results related to %s.", len(req.Results), req.RecognizedObject.ObjectName)
	if req.Language == "ja" {
		text = fmt.Sprintf("%sに関連する検索結果が%d件見つかりました。", req.RecognizedObject.ObjectName, len(req.Results))
	}
	displayName := firstNonEmpty(req.RecognizedObject.DisplayName, req.RecognizedObject.ObjectName)
	category := req.RecognizedObject.Category
	finalObjectName := firstNonEmpty(req.RecognizedObject.FinalObjectName, composeFinalName(displayName, category))
	return &model.SummarizeSearchResultsResponse{Text: text, DisplayName: displayName, Category: category, FinalObjectName: finalObjectName, Model: c.modelName()}, nil
}

func (c *Client) modelName() string {
	if c.Model != "" {
		return c.Model
	}
	return "mock-vision-llm"
}

func evidenceDisplayName(evidence *model.VisualEvidence) string {
	for _, item := range evidence.OCR {
		if value := strings.TrimSpace(item.Text); value != "" {
			return value
		}
	}
	for _, item := range evidence.WebEntities {
		if value := strings.TrimSpace(item.Text); value != "" {
			return value
		}
	}
	for _, label := range evidence.BestGuessLabels {
		if value := strings.TrimSpace(label); value != "" {
			return value
		}
	}
	return ""
}

func evidenceCategory(evidence *model.VisualEvidence, language string) string {
	texts := make([]string, 0, len(evidence.OCR)+len(evidence.WebEntities)+len(evidence.BestGuessLabels))
	for _, item := range evidence.OCR {
		texts = append(texts, item.Text)
	}
	for _, item := range evidence.WebEntities {
		texts = append(texts, item.Text)
	}
	texts = append(texts, evidence.BestGuessLabels...)
	text := strings.ToLower(strings.Join(texts, " "))
	if strings.Contains(text, "blu-ray") || strings.Contains(text, "dvd") || strings.Contains(text, "ブルーレイ") {
		if language == "ja" {
			return "Blu-ray/DVDパッケージ"
		}
		return "Blu-ray/DVD package"
	}
	return ""
}

func composeFinalName(displayName string, category string) string {
	displayName = strings.TrimSpace(displayName)
	category = strings.TrimSpace(category)
	if displayName != "" && category != "" && !strings.Contains(strings.ToLower(displayName), strings.ToLower(category)) {
		separator := " "
		if containsJapanese(displayName) || containsJapanese(category) {
			separator = ""
		}
		return displayName + separator + category
	}
	return firstNonEmpty(displayName, category)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func containsJapanese(value string) bool {
	for _, r := range value {
		if (r >= '\u3040' && r <= '\u30ff') || (r >= '\u4e00' && r <= '\u9fff') || r == '『' || r == '』' {
			return true
		}
	}
	return false
}
