package mock

import (
	"context"
	"fmt"

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
	description := "The image appears to contain a sample object."
	query := "sample object overview"
	if imageCount > 1 {
		objectName = fmt.Sprintf("sample object from %d images", imageCount)
		description = fmt.Sprintf("The %d supplied images are combined into one sample object result.", imageCount)
		query = fmt.Sprintf("sample object overview %d images", imageCount)
	}
	if req.Language == "ja" {
		objectName = "サンプル物体"
		description = "画像内の主対象物はサンプル物体のように見えます。"
		query = "サンプル物体 特徴 使い方"
		if imageCount > 1 {
			objectName = fmt.Sprintf("%d枚のサンプル物体", imageCount)
			description = fmt.Sprintf("%d枚の画像シグナルを統合したサンプル物体の結果です。", imageCount)
			query = fmt.Sprintf("サンプル物体 %d枚 統合 特徴", imageCount)
		}
	}
	return &model.RecognizeObjectResponse{Object: model.RecognizedObject{ObjectName: objectName, Description: description, SearchQuery: query, Confidence: "medium", NeedsMoreContext: false}, Model: c.modelName()}, nil
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
	return &model.SummarizeSearchResultsResponse{Text: text, Model: c.modelName()}, nil
}

func (c *Client) modelName() string {
	if c.Model != "" {
		return c.Model
	}
	return "mock-vision-llm"
}
