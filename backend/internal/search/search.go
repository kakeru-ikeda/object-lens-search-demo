package search

import (
	"context"

	"object-lens-search-demo/backend/internal/model"
)

type WebSearcher interface {
	Search(ctx context.Context, req model.SearchRequest) (*model.SearchResponse, error)
}
