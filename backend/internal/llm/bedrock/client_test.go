package bedrock

import (
	"testing"

	"object-lens-search-demo/backend/internal/model"
)

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
