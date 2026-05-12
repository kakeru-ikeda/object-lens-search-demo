package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"object-lens-search-demo/backend/internal/llm"
	"object-lens-search-demo/backend/internal/model"
	"object-lens-search-demo/backend/internal/search"
)

var (
	ErrLLM     = errors.New("llm error")
	ErrSearch  = errors.New("search error")
	ErrTimeout = errors.New("timeout")
)

type RecognizeSearchUsecase struct {
	LLM            llm.VisionLLM
	Searcher       search.WebSearcher
	LLMProvider    string
	SearchProvider string
}

type ExecuteRequest struct {
	RequestID string
	Request   model.RecognizeSearchRequest
	MIMEType  string
}

func (u *RecognizeSearchUsecase) Execute(ctx context.Context, req ExecuteRequest) (*model.RecognizeSearchResponse, error) {
	start := time.Now()
	recognized, err := u.LLM.RecognizeObject(ctx, model.RecognizeObjectRequest{ImageDataURL: req.Request.ImageBase64, MIMEType: req.MIMEType, Language: req.Request.Language})
	if err != nil {
		return nil, fmt.Errorf("%w: recognize object: %v", ErrLLM, err)
	}
	searchResp, err := u.Searcher.Search(ctx, model.SearchRequest{Query: recognized.Object.SearchQuery, Language: req.Request.Language, MaxResults: req.Request.Options.MaxSearchResults})
	if err != nil {
		return nil, fmt.Errorf("%w: search: %v", ErrSearch, err)
	}
	summary, err := u.LLM.SummarizeSearchResults(ctx, model.SummarizeSearchResultsRequest{Language: req.Request.Language, RecognizedObject: recognized.Object, Results: searchResp.Results})
	if err != nil {
		return nil, fmt.Errorf("%w: summarize search results: %v", ErrLLM, err)
	}
	return &model.RecognizeSearchResponse{
		RequestID:        req.RequestID,
		RecognizedObject: recognized.Object,
		Search: model.SearchSection{
			Provider: searchResp.Provider,
			Query:    searchResp.Query,
			Results:  searchResp.Results,
		},
		Summary: model.Summary{Text: summary.Text, LLMProvider: u.LLMProvider, Model: summary.Model},
		Meta:    model.Meta{LLMProvider: u.LLMProvider, SearchProvider: u.SearchProvider, ElapsedMs: time.Since(start).Milliseconds()},
	}, nil
}
