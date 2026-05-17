package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
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

const refinedPackageName = "『サンプル映画タイトル』Blu-ray/DVDパッケージ"

type evidenceRefiningLLM struct {
	mu                     sync.Mutex
	recognizedWithEvidence []bool
}

func (l *evidenceRefiningLLM) RecognizeObject(ctx context.Context, req model.RecognizeObjectRequest) (*model.RecognizeObjectResponse, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	withEvidence := req.VisualEvidence != nil && !req.VisualEvidence.Empty()
	l.mu.Lock()
	l.recognizedWithEvidence = append(l.recognizedWithEvidence, withEvidence)
	l.mu.Unlock()
	if withEvidence {
		return &model.RecognizeObjectResponse{Object: model.RecognizedObject{ObjectName: "サンプル映画タイトル", DisplayName: "『サンプル映画タイトル』", Category: "Blu-ray/DVDパッケージ", FinalObjectName: refinedPackageName, Description: "OCRと検索証拠で特定した映像パッケージです。", SearchQuery: refinedPackageName, Confidence: "high", NeedsMoreContext: false}, Model: "refiner"}, nil
	}
	return &model.RecognizeObjectResponse{Object: model.RecognizedObject{ObjectName: "アニメCD/音楽アルバム", Description: "アニメキャラクターが描かれた音楽CDまたはアルバムのパッケージ。", SearchQuery: "アニメ CD 音楽アルバム", Confidence: "medium", NeedsMoreContext: true}, Model: "refiner"}, nil
}

func (l *evidenceRefiningLLM) HypothesizeObject(ctx context.Context, req model.HypothesizeObjectRequest) (*model.HypothesizeObjectResponse, error) {
	return &model.HypothesizeObjectResponse{Object: model.RecognizedObject{ObjectName: "アニメCD/音楽アルバム", Description: "暫定仮説です。", SearchQuery: "アニメ CD 音楽アルバム", Confidence: "low", NeedsMoreContext: true}, Model: "refiner-light"}, nil
}

func (l *evidenceRefiningLLM) SummarizeSearchResults(ctx context.Context, req model.SummarizeSearchResultsRequest) (*model.SummarizeSearchResultsResponse, error) {
	return &model.SummarizeSearchResultsResponse{Text: "検索結果とOCRからサンプル映画タイトルのBlu-ray/DVDパッケージです。", DisplayName: req.RecognizedObject.DisplayName, Category: req.RecognizedObject.Category, FinalObjectName: refinedPackageName, Model: "refiner"}, nil
}

func (l *evidenceRefiningLLM) calls() []bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]bool(nil), l.recognizedWithEvidence...)
}

func TestExecuteRefinesRecognitionWithCloudVisionEvidenceAndFinalObjectName(t *testing.T) {
	llm := &evidenceRefiningLLM{}
	uc := &RecognizeSearchUsecase{
		LLM:            llm,
		Searcher:       &mocksearch.Client{},
		Vision:         stubVision{evidence: model.VisualEvidence{OCR: []model.EvidenceItem{{Text: refinedPackageName, Score: 0.99}}, WebEntities: []model.EvidenceItem{{Text: "Sample Movie Blu-ray", Score: 0.92}}}},
		LLMProvider:    "refiner",
		SearchProvider: "mock",
	}
	resp, err := uc.Execute(context.Background(), ExecuteRequest{RequestID: "req", Request: model.RecognizeSearchRequest{ImageBase64: "data:image/jpeg;base64,aW1hZ2U=", Language: "ja", Options: model.RequestOptions{MaxSearchResults: 1}}, MIMEType: "image/jpeg"})
	if err != nil {
		t.Fatalf("unexpected execute error: %v", err)
	}
	if resp.RecognizedObject.FinalObjectName != refinedPackageName {
		t.Fatalf("expected final object name %q, got %#v", refinedPackageName, resp.RecognizedObject)
	}
	if resp.RecognizedObject.Category != "Blu-ray/DVDパッケージ" || resp.RecognizedObject.DisplayName == "" {
		t.Fatalf("expected separated display/category fields, got %#v", resp.RecognizedObject)
	}
	calls := llm.calls()
	if len(calls) != 2 || calls[0] || !calls[1] {
		t.Fatalf("expected initial recognition without evidence then refinement with evidence, got %#v", calls)
	}
}

func TestEvidenceQuerySourcesPreferOCR(t *testing.T) {
	sources := evidenceQuerySources(&model.VisualEvidence{Logos: []model.EvidenceItem{{Text: "Coarse Logo", Score: 0.99}}, BestGuessLabels: []string{"coarse guess"}, WebEntities: []model.EvidenceItem{{Text: "Web Entity", Score: 0.91}}, OCR: []model.EvidenceItem{{Text: "label: Exact Product Title Blu-ray", Score: 0.97}}}, model.RecognizeSearchRequest{Images: []model.ImageInput{{ID: "label"}}})
	if len(sources) == 0 {
		t.Fatal("expected evidence query sources")
	}
	if sources[0].source != "vision_ocr_query" || sources[0].query != "Exact Product Title Blu-ray" {
		t.Fatalf("expected OCR query first without image prefix, got %#v", sources)
	}
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

func (s slowLLM) HypothesizeObject(ctx context.Context, req model.HypothesizeObjectRequest) (*model.HypothesizeObjectResponse, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(s.delay):
	}
	return &model.HypothesizeObjectResponse{Object: model.RecognizedObject{ObjectName: "hypothesis", Description: "hypothesis", SearchQuery: "hypothesis product", Confidence: "low"}, Model: "slow-light"}, nil
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
	if len(resp.Search.Results) != 1 {
		t.Fatalf("expected mock search duplicate URLs to dedupe to one result, got %d", len(resp.Search.Results))
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

func TestExecuteMultiImageContributesToMockResult(t *testing.T) {
	uc := &RecognizeSearchUsecase{LLM: &mock.Client{Model: "mock"}, Searcher: &mocksearch.Client{}, LLMProvider: "mock", SearchProvider: "mock"}
	req := model.RecognizeSearchRequest{
		Images: []model.ImageInput{
			{ID: "front", Role: "primary", ImageBase64: "data:image/jpeg;base64,aW1hZ2Ux"},
			{ID: "label", Role: "supporting", ImageBase64: "data:image/jpeg;base64,aW1hZ2Uy"},
		},
		Language: "ja",
		Options:  model.RequestOptions{Stream: true, MaxSearchResults: 1},
	}
	resp, err := uc.Execute(context.Background(), ExecuteRequest{RequestID: "req", Request: req, MIMEType: "image/jpeg"})
	if err != nil {
		t.Fatalf("unexpected execute error: %v", err)
	}
	if resp.ResponseVersion != 2 {
		t.Fatalf("expected v2 response, got %d", resp.ResponseVersion)
	}
	if resp.InputSummary == nil || resp.InputSummary.ImageCount != 2 || resp.InputSummary.Mode != "multi_image_stream" {
		t.Fatalf("unexpected input summary: %#v", resp.InputSummary)
	}
	if resp.RecognizedObject.ObjectName != "2枚のサンプル物体" {
		t.Fatalf("expected mock LLM to use image count, got %q", resp.RecognizedObject.ObjectName)
	}
	if resp.EvidenceFusion == nil || resp.EvidenceFusion.PrimaryImageID != "front" {
		t.Fatalf("unexpected evidence fusion: %#v", resp.EvidenceFusion)
	}
}

type imageAwareVision struct{}

func (imageAwareVision) ExtractEvidence(ctx context.Context, req model.ExtractEvidenceRequest) (*model.ExtractEvidenceResponse, error) {
	label := "unknown"
	imageURL := "https://example.com/unknown.jpg"
	if strings.Contains(req.ImageDataURL, "aW1hZ2Ux") {
		label = "front signal"
		imageURL = "https://example.com/front.jpg"
	}
	if strings.Contains(req.ImageDataURL, "aW1hZ2Uy") {
		label = "label signal"
		imageURL = "https://example.com/label.jpg"
	}
	return &model.ExtractEvidenceResponse{Evidence: model.VisualEvidence{Labels: []model.EvidenceItem{{Text: label, Score: 0.9}}, RelatedImages: []model.RelatedImage{{URL: imageURL, MatchType: "visually_similar", Score: 0.8}}}, Provider: "stub"}, nil
}

func (imageAwareVision) Close() error { return nil }

func TestExecuteMultiImageMergesEvidenceFromAllImages(t *testing.T) {
	uc := &RecognizeSearchUsecase{LLM: &mock.Client{Model: "mock"}, Searcher: &mocksearch.Client{}, Vision: imageAwareVision{}, LLMProvider: "mock", SearchProvider: "mock"}
	req := model.RecognizeSearchRequest{
		Images: []model.ImageInput{
			{ID: "front", Role: "primary", ImageBase64: "data:image/jpeg;base64,aW1hZ2Ux"},
			{ID: "label", Role: "supporting", ImageBase64: "data:image/jpeg;base64,aW1hZ2Uy"},
		},
		Language: "en",
		Options:  model.RequestOptions{Stream: true, MaxSearchResults: 1},
	}
	resp, err := uc.Execute(context.Background(), ExecuteRequest{RequestID: "req", Request: req, MIMEType: "image/jpeg"})
	if err != nil {
		t.Fatalf("unexpected execute error: %v", err)
	}
	if resp.RecognizedObject.VisualEvidence == nil || len(resp.RecognizedObject.VisualEvidence.Labels) != 2 {
		t.Fatalf("expected merged labels from both images: %#v", resp.RecognizedObject.VisualEvidence)
	}
	texts := []string{resp.RecognizedObject.VisualEvidence.Labels[0].Text, resp.RecognizedObject.VisualEvidence.Labels[1].Text}
	if texts[0] != "front: front signal" || texts[1] != "label: label signal" {
		t.Fatalf("unexpected merged evidence texts: %#v", texts)
	}
	if len(resp.RecognizedObject.VisualEvidence.RelatedImages) != 2 {
		t.Fatalf("expected related images from both images: %#v", resp.RecognizedObject.VisualEvidence.RelatedImages)
	}
	if resp.RecognizedObject.VisualEvidence.RelatedImages[0].SourceImageID != "front" || resp.RecognizedObject.VisualEvidence.RelatedImages[1].SourceImageID != "label" {
		t.Fatalf("expected related image source IDs, got %#v", resp.RecognizedObject.VisualEvidence.RelatedImages)
	}
	if len(resp.ImageAnalyses) != 2 {
		t.Fatalf("expected per-image analyses, got %#v", resp.ImageAnalyses)
	}
}

type recordingSink struct {
	mu     sync.Mutex
	events []StreamEvent
}

func (s *recordingSink) Emit(ctx context.Context, event StreamEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

func (s *recordingSink) stages() map[string]StreamEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]StreamEvent, len(s.events))
	for _, event := range s.events {
		out[event.Stage] = event
	}
	return out
}

func TestExecuteWithEventsEmitsRichPartialPayloads(t *testing.T) {
	sink := &recordingSink{}
	uc := &RecognizeSearchUsecase{LLM: &mock.Client{Model: "mock"}, Searcher: &mocksearch.Client{}, Vision: stubVision{evidence: model.VisualEvidence{Logos: []model.EvidenceItem{{Text: "Acme", Score: 0.9}}}}, LLMProvider: "mock", SearchProvider: "mock"}
	_, err := uc.ExecuteWithEvents(context.Background(), ExecuteRequest{RequestID: "req", Request: model.RecognizeSearchRequest{ImageBase64: "data:image/jpeg;base64,aW1hZ2U=", Language: "en", Options: model.RequestOptions{MaxSearchResults: 1, Stream: true}}, MIMEType: "image/jpeg"}, sink)
	if err != nil {
		t.Fatalf("unexpected execute error: %v", err)
	}
	stages := sink.stages()
	for _, stage := range []string{"llm_hypothesis_completed", "vision_completed", "query_generated", "search_results", "search_completed", "final"} {
		if _, ok := stages[stage]; !ok {
			t.Fatalf("missing event stage %q from %#v", stage, stages)
		}
	}
	if stages["llm_hypothesis_completed"].Source != "bedrock_light" || stages["llm_hypothesis_completed"].Revision == 0 || stages["llm_hypothesis_completed"].Payload["query"] == "" || stages["llm_hypothesis_completed"].Payload["hypothesis"] == "" {
		t.Fatalf("hypothesis event missing source/revision/payload: %#v", stages["llm_hypothesis_completed"])
	}
	if stages["vision_completed"].Source != "cloud_vision" || stages["vision_completed"].Payload["status"] != "measured" {
		t.Fatalf("vision event missing source/status: %#v", stages["vision_completed"])
	}
	if stages["search_completed"].Payload["speculativeResultCount"] == nil {
		t.Fatalf("search_completed missing speculative count: %#v", stages["search_completed"].Payload)
	}
	if stages["search_results"].Payload["searchResults"] == nil {
		t.Fatalf("search_results missing partial results: %#v", stages["search_results"].Payload)
	}
	if _, err := json.Marshal(stages["final"]); err != nil {
		t.Fatalf("final event must marshal: %v", err)
	}
}

type failingHypothesisLLM struct {
	base mock.Client
}

func (f failingHypothesisLLM) RecognizeObject(ctx context.Context, req model.RecognizeObjectRequest) (*model.RecognizeObjectResponse, error) {
	return f.base.RecognizeObject(ctx, req)
}

func (f failingHypothesisLLM) SummarizeSearchResults(ctx context.Context, req model.SummarizeSearchResultsRequest) (*model.SummarizeSearchResultsResponse, error) {
	return f.base.SummarizeSearchResults(ctx, req)
}

func (f failingHypothesisLLM) HypothesizeObject(ctx context.Context, req model.HypothesizeObjectRequest) (*model.HypothesizeObjectResponse, error) {
	return nil, errors.New("light model unavailable")
}

func TestExecuteContinuesWhenLightHypothesisFails(t *testing.T) {
	sink := &recordingSink{}
	uc := &RecognizeSearchUsecase{LLM: failingHypothesisLLM{base: mock.Client{Model: "mock"}}, Searcher: &mocksearch.Client{}, LLMProvider: "mock", SearchProvider: "mock"}
	resp, err := uc.ExecuteWithEvents(context.Background(), ExecuteRequest{RequestID: "req", Request: model.RecognizeSearchRequest{ImageBase64: "data:image/jpeg;base64,aW1hZ2U=", Language: "en", Options: model.RequestOptions{MaxSearchResults: 1}}, MIMEType: "image/jpeg"}, sink)
	if err != nil {
		t.Fatalf("expected final flow to succeed despite hypothesis failure: %v", err)
	}
	if resp.Search.Query == "" || resp.Summary.Text == "" {
		t.Fatalf("expected complete response: %#v", resp)
	}
	stage := sink.stages()["llm_hypothesis_completed"]
	if stage.Status != "warning" {
		t.Fatalf("expected warning hypothesis event, got %#v", stage)
	}
}

type uniqueSearchClient struct {
	mu      sync.Mutex
	queries []string
}

func (c *uniqueSearchClient) Search(ctx context.Context, req model.SearchRequest) (*model.SearchResponse, error) {
	c.mu.Lock()
	c.queries = append(c.queries, req.Query)
	c.mu.Unlock()
	return &model.SearchResponse{Provider: "tavily", Query: req.Query, Results: []model.NormalizedSearchResult{{ID: "id-" + strings.ReplaceAll(req.Query, " ", "-"), Title: req.Query, URL: "https://example.com/" + strings.ReplaceAll(req.Query, " ", "-"), DisplayURL: "example.com", Snippet: "snippet", Source: "example.com", Language: req.Language, Rank: 1, Score: 1, ContentType: "web_page", Provider: "tavily"}}}, nil
}

type failingSearchClient struct{}

func (f failingSearchClient) Search(ctx context.Context, req model.SearchRequest) (*model.SearchResponse, error) {
	return nil, errors.New("tavily unavailable")
}

func TestExecuteReturnsDegradedFinalWhenSearchFails(t *testing.T) {
	sink := &recordingSink{}
	uc := &RecognizeSearchUsecase{LLM: failingHypothesisLLM{base: mock.Client{Model: "mock"}}, Searcher: failingSearchClient{}, LLMProvider: "mock", SearchProvider: "tavily"}
	resp, err := uc.ExecuteWithEvents(context.Background(), ExecuteRequest{RequestID: "req", Request: model.RecognizeSearchRequest{ImageBase64: "data:image/jpeg;base64,aW1hZ2U=", Language: "en", Options: model.RequestOptions{MaxSearchResults: 1, Stream: true}}, MIMEType: "image/jpeg"}, sink)
	if err != nil {
		t.Fatalf("expected degraded final response instead of fatal search error: %v", err)
	}
	if resp.Search.Query == "" {
		t.Fatalf("expected final response to keep search query: %#v", resp.Search)
	}
	if len(resp.Search.Results) != 0 {
		t.Fatalf("expected no search results in degraded response, got %#v", resp.Search.Results)
	}
	stages := sink.stages()
	if stages["final"].Stage != "final" {
		t.Fatalf("expected final event after search failure, got stages %#v", stages)
	}
	searchCompleted := stages["search_completed"]
	if searchCompleted.Status != "warning" || searchCompleted.Payload["searchStatus"] != "degraded" {
		t.Fatalf("expected degraded search warning event, got %#v", searchCompleted)
	}
}

func TestSpeculativeSearchSourcesAreBounded(t *testing.T) {
	searcher := &uniqueSearchClient{}
	uc := &RecognizeSearchUsecase{LLM: &mock.Client{Model: "mock"}, Searcher: searcher, Vision: stubVision{evidence: model.VisualEvidence{Logos: []model.EvidenceItem{{Text: "Logo"}}, BestGuessLabels: []string{"Guess"}, WebEntities: []model.EvidenceItem{{Text: "Entity"}}, OCR: []model.EvidenceItem{{Text: "Text"}}}}, LLMProvider: "mock", SearchProvider: "mock"}
	resp, err := uc.Execute(context.Background(), ExecuteRequest{RequestID: "req", Request: model.RecognizeSearchRequest{ImageBase64: "data:image/jpeg;base64,aW1hZ2U=", Language: "en", Options: model.RequestOptions{MaxSearchResults: 1}}, MIMEType: "image/jpeg"})
	if err != nil {
		t.Fatalf("unexpected execute error: %v", err)
	}
	searcher.mu.Lock()
	queryCount := len(searcher.queries)
	queries := append([]string(nil), searcher.queries...)
	searcher.mu.Unlock()
	if queryCount > maxSpeculativeQuerySources+1 {
		t.Fatalf("expected at most %d speculative plus primary searches, got %d", maxSpeculativeQuerySources, queryCount)
	}
	seen := make(map[string]int, len(queries))
	for _, query := range queries {
		seen[query]++
	}
	for query, count := range seen {
		if count > 1 {
			t.Fatalf("query %q was searched %d times; primary query must not duplicate speculative search list %#v", query, count, queries)
		}
	}
	if len(resp.Search.Results) != queryCount {
		t.Fatalf("expected adopted deduped results for all successful searches, got results=%d queries=%d", len(resp.Search.Results), queryCount)
	}
}
