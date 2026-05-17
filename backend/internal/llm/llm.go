package llm

import (
	"context"

	"object-lens-search-demo/backend/internal/model"
)

type VisionLLM interface {
	RecognizeObject(ctx context.Context, req model.RecognizeObjectRequest) (*model.RecognizeObjectResponse, error)
	SummarizeSearchResults(ctx context.Context, req model.SummarizeSearchResultsRequest) (*model.SummarizeSearchResultsResponse, error)
}

type HypothesisLLM interface {
	HypothesizeObject(ctx context.Context, req model.HypothesizeObjectRequest) (*model.HypothesizeObjectResponse, error)
}
