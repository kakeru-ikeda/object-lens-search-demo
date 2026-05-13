package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"object-lens-search-demo/backend/internal/llm/mock"
	"object-lens-search-demo/backend/internal/model"
	mocksearch "object-lens-search-demo/backend/internal/search/mock"
)

type stubVision struct {
	evidence model.VisualEvidence
	err      error
}

func (s stubVision) ExtractEvidence(ctx context.Context, req model.ExtractEvidenceRequest) (*model.ExtractEvidenceResponse, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &model.ExtractEvidenceResponse{Evidence: s.evidence, Provider: "stub"}, nil
}

func (s stubVision) Close() error {
	return nil
}

func TestExecuteAttachesCloudVisionEvidence(t *testing.T) {
	uc := &RecognizeSearchUsecase{
		LLM:            &mock.Client{Model: "mock"},
		Searcher:       &mocksearch.Client{},
		Vision:         stubVision{evidence: model.VisualEvidence{OCR: []model.EvidenceItem{{Text: "Coca-Cola", Score: 0.99}}, Logos: []model.EvidenceItem{{Text: "Coca-Cola", Score: 0.98}}}},
		LLMProvider:    "mock",
		SearchProvider: "mock",
	}
	resp, err := uc.Execute(context.Background(), ExecuteRequest{RequestID: "req", Request: model.RecognizeSearchRequest{ImageBase64: "data:image/jpeg;base64,aW1hZ2U=", Language: "ja"}, MIMEType: "image/jpeg"})
	if err != nil {
		t.Fatalf("unexpected execute error: %v", err)
	}
	if resp.QueryQuality.Status != "measured" {
		t.Fatalf("expected measured status, got %q", resp.QueryQuality.Status)
	}
	if len(resp.QueryQuality.EvidenceTypes) != 2 || resp.QueryQuality.EvidenceTypes[0] != "ocr" || resp.QueryQuality.EvidenceTypes[1] != "logo" {
		t.Fatalf("unexpected evidence types: %#v", resp.QueryQuality.EvidenceTypes)
	}
	if resp.RecognizedObject.VisualEvidence == nil || len(resp.RecognizedObject.VisualEvidence.OCR) != 1 {
		t.Fatalf("expected visual evidence in response: %#v", resp.RecognizedObject.VisualEvidence)
	}
}

func TestExecuteMarksCloudVisionDisabled(t *testing.T) {
	uc := &RecognizeSearchUsecase{LLM: &mock.Client{Model: "mock"}, Searcher: &mocksearch.Client{}, LLMProvider: "mock", SearchProvider: "mock"}
	resp, err := uc.Execute(context.Background(), ExecuteRequest{RequestID: "req", Request: model.RecognizeSearchRequest{ImageBase64: "data:image/jpeg;base64,aW1hZ2U=", Language: "en"}, MIMEType: "image/jpeg"})
	if err != nil {
		t.Fatalf("unexpected execute error: %v", err)
	}
	if resp.QueryQuality.Status != "cloud_vision_disabled" {
		t.Fatalf("expected disabled status, got %q", resp.QueryQuality.Status)
	}
}

func TestExecuteIncludesStageLatency(t *testing.T) {
	uc := &RecognizeSearchUsecase{LLM: &mock.Client{Model: "mock"}, Searcher: &mocksearch.Client{}, LLMProvider: "mock", SearchProvider: "mock"}
	resp, err := uc.Execute(context.Background(), ExecuteRequest{RequestID: "req", Request: model.RecognizeSearchRequest{ImageBase64: "data:image/jpeg;base64,aW1hZ2U=", Language: "en"}, MIMEType: "image/jpeg"})
	if err != nil {
		t.Fatalf("unexpected execute error: %v", err)
	}
	if resp.Meta.StageLatency.CloudVisionMs < 0 || resp.Meta.StageLatency.RecognizeMs < 0 || resp.Meta.StageLatency.SearchMs < 0 || resp.Meta.StageLatency.SummarizeMs < 0 {
		t.Fatalf("stage latency must be non-negative: %#v", resp.Meta.StageLatency)
	}
	if resp.Meta.CloudVisionProvider != "disabled" {
		t.Fatalf("expected disabled cloud vision provider, got %q", resp.Meta.CloudVisionProvider)
	}
}

type slowVision struct {
	delay time.Duration
}

func (s slowVision) ExtractEvidence(ctx context.Context, req model.ExtractEvidenceRequest) (*model.ExtractEvidenceResponse, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(s.delay):
	}
	return &model.ExtractEvidenceResponse{Evidence: model.VisualEvidence{Logos: []model.EvidenceItem{{Text: "Acme", Score: 0.9}}}, Provider: "stub"}, nil
}

func (s slowVision) Close() error {
	return nil
}

type slowLLM struct {
	delay time.Duration
}

func (s slowLLM) RecognizeObject(ctx context.Context, req model.RecognizeObjectRequest) (*model.RecognizeObjectResponse, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(s.delay):
	}
	return &model.RecognizeObjectResponse{Object: model.RecognizedObject{ObjectName: "sample", Description: "sample", SearchQuery: "sample product", Confidence: "medium"}, Model: "slow"}, nil
}

func (s slowLLM) SummarizeSearchResults(ctx context.Context, req model.SummarizeSearchResultsRequest) (*model.SummarizeSearchResultsResponse, error) {
	return (&mock.Client{Model: "slow"}).SummarizeSearchResults(ctx, req)
}

func TestExecuteRunsVisionAndRecognitionInParallel(t *testing.T) {
	uc := &RecognizeSearchUsecase{
		LLM:            slowLLM{delay: 80 * time.Millisecond},
		Searcher:       &mocksearch.Client{},
		Vision:         slowVision{delay: 80 * time.Millisecond},
		LLMProvider:    "slow",
		SearchProvider: "mock",
	}
	start := time.Now()
	resp, err := uc.Execute(context.Background(), ExecuteRequest{RequestID: "req", Request: model.RecognizeSearchRequest{ImageBase64: "data:image/jpeg;base64,aW1hZ2U=", Language: "en", Options: model.RequestOptions{MaxSearchResults: 1}}, MIMEType: "image/jpeg"})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected execute error: %v", err)
	}
	if elapsed >= 145*time.Millisecond {
		t.Fatalf("expected vision and recognition to overlap, elapsed=%s", elapsed)
	}
	if resp.RecognizedObject.VisualEvidence == nil || len(resp.RecognizedObject.VisualEvidence.Logos) != 1 {
		t.Fatalf("expected visual evidence to be attached: %#v", resp.RecognizedObject.VisualEvidence)
	}
	if resp.Search.Query != "sample product Acme" {
		t.Fatalf("expected evidence-enriched query, got %q", resp.Search.Query)
	}
	if resp.Meta.StageLatency.CloudVisionMs < 70 || resp.Meta.StageLatency.RecognizeMs < 70 {
		t.Fatalf("expected both stage latencies recorded, got %#v", resp.Meta.StageLatency)
	}
}

type failingLLM struct{}

func (f failingLLM) RecognizeObject(ctx context.Context, req model.RecognizeObjectRequest) (*model.RecognizeObjectResponse, error) {
	return nil, errors.New("recognize failed")
}

func (f failingLLM) SummarizeSearchResults(ctx context.Context, req model.SummarizeSearchResultsRequest) (*model.SummarizeSearchResultsResponse, error) {
	return nil, errors.New("not called")
}

type cancellableVision struct {
	cancelled chan struct{}
}

func (v cancellableVision) ExtractEvidence(ctx context.Context, req model.ExtractEvidenceRequest) (*model.ExtractEvidenceResponse, error) {
	<-ctx.Done()
	close(v.cancelled)
	return nil, ctx.Err()
}

func (v cancellableVision) Close() error {
	return nil
}

func TestExecuteCancelsVisionWhenRecognitionFails(t *testing.T) {
	cancelled := make(chan struct{})
	uc := &RecognizeSearchUsecase{
		LLM:            failingLLM{},
		Searcher:       &mocksearch.Client{},
		Vision:         cancellableVision{cancelled: cancelled},
		LLMProvider:    "failing",
		SearchProvider: "mock",
	}
	_, err := uc.Execute(context.Background(), ExecuteRequest{RequestID: "req", Request: model.RecognizeSearchRequest{ImageBase64: "data:image/jpeg;base64,aW1hZ2U=", Language: "en"}, MIMEType: "image/jpeg"})
	if err == nil {
		t.Fatal("expected recognition error")
	}
	select {
	case <-cancelled:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected vision extraction to be cancelled and drained")
	}
}
