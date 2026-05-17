package bedrock

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"

	"object-lens-search-demo/backend/internal/model"
)

type captureRuntime struct {
	modelIDs []string
	bodies   [][]byte
}

func (r *captureRuntime) InvokeModel(ctx context.Context, params *bedrockruntime.InvokeModelInput, optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error) {
	if params.ModelId != nil {
		r.modelIDs = append(r.modelIDs, *params.ModelId)
	}
	r.bodies = append(r.bodies, append([]byte(nil), params.Body...))
	body, _ := json.Marshal(anthropicResponse{Content: []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}{{Type: "text", Text: `{"objectName":"sample","description":"sample","searchQuery":"sample query","confidence":"low","needsMoreContext":false}`}}})
	return &bedrockruntime.InvokeModelOutput{Body: body}, nil
}

func TestCompactSearchResultsDropsRawPayload(t *testing.T) {
	results := []model.NormalizedSearchResult{{
		Title:       "Product page",
		URL:         "https://example.com/product",
		Snippet:     "Useful product snippet",
		Source:      "example.com",
		Rank:        2,
		Score:       0.75,
		ContentType: "web_page",
		Raw:         []byte(`{"large":"payload"}`),
		Provider:    "tavily",
	}}
	compact := compactSearchResults(results)
	if len(compact) != 1 {
		t.Fatalf("expected one compact result, got %d", len(compact))
	}
	got := compact[0]
	if got.Title != results[0].Title || got.URL != results[0].URL || got.Snippet != results[0].Snippet || got.Source != results[0].Source {
		t.Fatalf("compact result lost summary fields: %#v", got)
	}
	if got.Rank != results[0].Rank || got.Score != results[0].Score || got.ContentType != results[0].ContentType {
		t.Fatalf("compact result lost ranking fields: %#v", got)
	}
}

func TestHypothesizeObjectUsesConfiguredLightModel(t *testing.T) {
	runtime := &captureRuntime{}
	client := NewWithLightModel(runtime, "main-model", "light-model")
	resp, err := client.HypothesizeObject(context.Background(), model.HypothesizeObjectRequest{ImageDataURL: "data:image/jpeg;base64,aW1hZ2U=", Language: "en"})
	if err != nil {
		t.Fatalf("unexpected hypothesis error: %v", err)
	}
	if resp.Model != "light-model" || resp.Object.SearchQuery != "sample query" {
		t.Fatalf("unexpected hypothesis response: %#v", resp)
	}
	if len(runtime.modelIDs) != 1 || runtime.modelIDs[0] != "light-model" {
		t.Fatalf("expected light model invocation, got %#v", runtime.modelIDs)
	}
}

func TestRecognizeObjectIncludesMultipleImages(t *testing.T) {
	runtime := &captureRuntime{}
	client := New(runtime, "main-model")
	_, err := client.RecognizeObject(context.Background(), model.RecognizeObjectRequest{
		ImageDataURL: "data:image/jpeg;base64,aW1hZ2Ux",
		Images: []model.RecognizeImageInput{
			{ID: "front", Role: "primary", ImageDataURL: "data:image/jpeg;base64,aW1hZ2Ux"},
			{ID: "label", Role: "supporting", ImageDataURL: "data:image/jpeg;base64,aW1hZ2Uy"},
		},
		Language: "en",
	})
	if err != nil {
		t.Fatalf("unexpected recognize error: %v", err)
	}
	if len(runtime.bodies) != 1 {
		t.Fatalf("expected one request body, got %d", len(runtime.bodies))
	}
	var request anthropicRequest
	if err := json.Unmarshal(runtime.bodies[0], &request); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	imageCount := 0
	for _, content := range request.Messages[0].Content {
		if content.Type == "image" {
			imageCount++
		}
	}
	if imageCount != 2 {
		t.Fatalf("expected two image contents, got %d in %#v", imageCount, request.Messages[0].Content)
	}
}
