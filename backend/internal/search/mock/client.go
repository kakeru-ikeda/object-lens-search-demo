package mock

import (
	"context"
	"encoding/json"
	"fmt"

	"object-lens-search-demo/backend/internal/model"
)

type Client struct{}

func (c *Client) Search(ctx context.Context, req model.SearchRequest) (*model.SearchResponse, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	count := req.MaxResults
	if count <= 0 {
		count = 5
	}
	results := make([]model.NormalizedSearchResult, 0, count)
	for i := 1; i <= count; i++ {
		raw, _ := json.Marshal(map[string]any{"mock": true, "rank": i})
		results = append(results, model.NormalizedSearchResult{
			ID:          resultID(i),
			Title:       req.Query,
			URL:         "https://example.com/mock-result",
			DisplayURL:  "example.com/mock-result",
			Snippet:     "Mock search result for local development.",
			Source:      "example.com",
			PublishedAt: nil,
			Language:    req.Language,
			Rank:        i,
			Score:       1 / float64(i),
			ContentType: "web_page",
			Provider:    "tavily",
			Raw:         raw,
		})
	}
	return &model.SearchResponse{Provider: "tavily", Query: req.Query, Results: results}, nil
}

func resultID(rank int) string {
	return fmt.Sprintf("sr_mock_%03d", rank)
}
