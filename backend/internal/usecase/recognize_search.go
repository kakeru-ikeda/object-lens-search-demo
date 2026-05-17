package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"object-lens-search-demo/backend/internal/llm"
	"object-lens-search-demo/backend/internal/model"
	"object-lens-search-demo/backend/internal/search"
	"object-lens-search-demo/backend/internal/vision"
)

var (
	ErrLLM     = errors.New("llm error")
	ErrSearch  = errors.New("search error")
	ErrTimeout = errors.New("timeout")
)

type RecognizeSearchUsecase struct {
	LLM                 llm.VisionLLM
	Searcher            search.WebSearcher
	Vision              vision.EvidenceExtractor
	LLMProvider         string
	SearchProvider      string
	CloudVisionProvider string
	Logger              *slog.Logger
}

type ExecuteRequest struct {
	RequestID     string
	Request       model.RecognizeSearchRequest
	MIMEType      string
	CropMIMETypes map[string]string
}

type EventSink interface {
	Emit(ctx context.Context, event StreamEvent) error
}

type StreamEvent struct {
	RequestID string                 `json:"requestId"`
	Sequence  int                    `json:"sequence"`
	Revision  int                    `json:"revision"`
	Source    string                 `json:"source"`
	Stage     string                 `json:"stage"`
	Status    string                 `json:"status"`
	ElapsedMs int64                  `json:"elapsedMs"`
	ImageID   string                 `json:"imageId,omitempty"`
	Message   string                 `json:"message"`
	Payload   map[string]interface{} `json:"payload,omitempty"`
}

type noopSink struct{}

func (noopSink) Emit(context.Context, StreamEvent) error { return nil }

type eventEmitter struct {
	mu        sync.Mutex
	requestID string
	start     time.Time
	sequence  int
	sink      EventSink
}

func newEventEmitter(requestID string, start time.Time, sink EventSink) *eventEmitter {
	if sink == nil {
		sink = noopSink{}
	}
	return &eventEmitter{requestID: requestID, start: start, sink: sink}
}

func (e *eventEmitter) emit(ctx context.Context, stage string, status string, imageID string, message string, payload map[string]interface{}) error {
	return e.emitFrom(ctx, "backend", stage, status, imageID, message, payload)
}

func (e *eventEmitter) emitFrom(ctx context.Context, source string, stage string, status string, imageID string, message string, payload map[string]interface{}) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.sequence++
	if source == "" {
		source = "backend"
	}
	return e.sink.Emit(ctx, StreamEvent{RequestID: e.requestID, Sequence: e.sequence, Revision: e.sequence, Source: source, Stage: stage, Status: status, ElapsedMs: time.Since(e.start).Milliseconds(), ImageID: imageID, Message: message, Payload: payload})
}

func (u *RecognizeSearchUsecase) Execute(ctx context.Context, req ExecuteRequest) (*model.RecognizeSearchResponse, error) {
	return u.execute(ctx, req, noopSink{})
}

func (u *RecognizeSearchUsecase) ExecuteWithEvents(ctx context.Context, req ExecuteRequest, sink EventSink) (*model.RecognizeSearchResponse, error) {
	return u.execute(ctx, req, sink)
}

func (u *RecognizeSearchUsecase) execute(ctx context.Context, req ExecuteRequest, sink EventSink) (*model.RecognizeSearchResponse, error) {
	start := time.Now()
	emitter := newEventEmitter(req.RequestID, start, sink)
	if err := emitter.emit(ctx, "request_received", "completed", "", "Request accepted", map[string]interface{}{"imageCount": inputImageCount(req.Request)}); err != nil {
		return nil, err
	}
	stageLatency := model.StageLatency{}
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	evidenceCh := make(chan evidenceResult, 1)
	hypothesisCh := make(chan hypothesisResult, 1)
	recognitionCh := make(chan recognitionResult, 1)
	speculative := newSpeculativeSearches(u.Searcher, req.Request.Language, req.Request.Options.MaxSearchResults, emitter, u.Logger)
	defer speculative.cancel()

	go func() {
		_ = emitter.emit(workCtx, "vision_started", "started", "", "Visual evidence extraction started", nil)
		stageStart := time.Now()
		evidence, status := u.extractEvidence(workCtx, req)
		elapsed := time.Since(stageStart).Milliseconds()
		evidenceCh <- evidenceResult{evidence: evidence, status: status, elapsedMs: elapsed}
	}()
	go func() {
		_ = emitter.emitFrom(workCtx, "bedrock_light", "llm_hypothesis_started", "started", "", "Interim hypothesis started", map[string]interface{}{"maxQuerySources": maxSpeculativeQuerySources})
		stageStart := time.Now()
		resp, err := u.hypothesize(workCtx, req, nil)
		hypothesisCh <- hypothesisResult{resp: resp, err: err, elapsedMs: time.Since(stageStart).Milliseconds()}
	}()

	if err := emitter.emit(ctx, "recognition_started", "started", "", "Integrated recognition started", nil); err != nil {
		return nil, err
	}
	go func() {
		stageStart := time.Now()
		recognized, err := u.LLM.RecognizeObject(workCtx, recognizeObjectRequest(req, nil))
		recognitionCh <- recognitionResult{resp: recognized, err: err, elapsedMs: time.Since(stageStart).Milliseconds()}
	}()

	var recognized *model.RecognizeObjectResponse
	var evidence *model.VisualEvidence
	var evidenceStatus string
	for pending := 3; pending > 0; pending-- {
		select {
		case res := <-hypothesisCh:
			if res.err != nil {
				if u.Logger != nil {
					u.Logger.Warn("interim hypothesis failed", "error", res.err)
				}
				if err := emitter.emitFrom(ctx, "bedrock_light", "llm_hypothesis_completed", "warning", "", "Interim hypothesis unavailable", map[string]interface{}{"error": res.err.Error(), "elapsedMs": res.elapsedMs}); err != nil {
					return nil, err
				}
				continue
			}
			if err := emitter.emitFrom(ctx, "bedrock_light", "llm_hypothesis_completed", "completed", "", "Interim hypothesis completed", map[string]interface{}{"model": res.resp.Model, "objectName": res.resp.Object.ObjectName, "description": res.resp.Object.Description, "searchQuery": res.resp.Object.SearchQuery, "query": res.resp.Object.SearchQuery, "hypothesis": interimHypothesisText(res.resp.Object), "confidence": res.resp.Object.Confidence, "elapsedMs": res.elapsedMs}); err != nil {
				return nil, err
			}
			if err := emitter.emitFrom(ctx, "bedrock_light", "query_generated", "completed", "", "Interim query generated", map[string]interface{}{"source": "raw_llm_hypothesis_query", "query": res.resp.Object.SearchQuery, "confidence": res.resp.Object.Confidence}); err != nil {
				return nil, err
			}
			speculative.Start(ctx, "raw_llm_hypothesis_query", res.resp.Object.SearchQuery)
		case res := <-evidenceCh:
			evidence = res.evidence
			evidenceStatus = res.status
			stageLatency.CloudVisionMs = res.elapsedMs
			payload := map[string]interface{}{"status": res.status, "elapsedMs": res.elapsedMs}
			if res.evidence != nil && !res.evidence.Empty() {
				payload["evidenceTypes"] = res.evidence.EvidenceTypes()
			}
			if err := emitter.emitFrom(ctx, "cloud_vision", "vision_completed", "completed", primaryImageID(req.Request), "Visual evidence extraction completed", payload); err != nil {
				return nil, err
			}
			for _, source := range evidenceQuerySources(res.evidence, req.Request) {
				if err := emitter.emitFrom(ctx, "cloud_vision", "query_generated", "completed", "", "Vision query generated", map[string]interface{}{"source": source.source, "query": source.query, "confidence": source.confidence}); err != nil {
					return nil, err
				}
				speculative.Start(ctx, source.source, source.query)
			}
		case res := <-recognitionCh:
			stageLatency.RecognizeMs = res.elapsedMs
			if res.err != nil {
				cancel()
				return nil, fmt.Errorf("%w: recognize object: %v", ErrLLM, res.err)
			}
			recognized = res.resp
			if err := emitter.emit(ctx, "recognition_completed", "completed", "", "Recognition completed", map[string]interface{}{"objectName": recognized.Object.ObjectName, "searchQuery": recognized.Object.SearchQuery, "elapsedMs": res.elapsedMs}); err != nil {
				return nil, err
			}
		case <-ctx.Done():
			cancel()
			return nil, ctx.Err()
		}
	}
	if recognized == nil {
		cancel()
		return nil, fmt.Errorf("%w: recognize object: empty response", ErrLLM)
	}
	recognized.Object = finalizeRecognizedObject(recognized.Object, nil)
	if evidence != nil && !evidence.Empty() {
		recognized.Object.VisualEvidence = evidence
		if shouldRefineRecognition(recognized.Object, evidence) {
			if err := emitter.emit(ctx, "recognition_refinement_started", "started", "", "Evidence-backed recognition started", map[string]interface{}{"evidenceTypes": evidence.EvidenceTypes()}); err != nil {
				return nil, err
			}
			stageStart := time.Now()
			refined, err := u.LLM.RecognizeObject(ctx, recognizeObjectRequest(req, evidence))
			elapsed := time.Since(stageStart).Milliseconds()
			stageLatency.RecognizeMs += elapsed
			if err != nil {
				if u.Logger != nil {
					u.Logger.Warn("evidence-backed recognition failed; keeping initial recognition", "error", err)
				}
				if err := emitter.emit(ctx, "recognition_refinement_completed", "warning", "", "Evidence-backed recognition unavailable", map[string]interface{}{"error": err.Error(), "elapsedMs": elapsed}); err != nil {
					return nil, err
				}
			} else if refined != nil {
				recognized.Object = mergeRecognizedObject(recognized.Object, refined.Object, evidence)
				if err := emitter.emit(ctx, "recognition_refinement_completed", "completed", "", "Evidence-backed recognition completed", map[string]interface{}{"objectName": recognized.Object.ObjectName, "displayName": recognized.Object.DisplayName, "category": recognized.Object.Category, "finalObjectName": recognized.Object.FinalObjectName, "searchQuery": recognized.Object.SearchQuery, "elapsedMs": elapsed}); err != nil {
					return nil, err
				}
			}
		}
		recognized.Object.VisualEvidence = evidence
		recognized.Object.SearchQuery = enrichSearchQuery(recognized.Object.SearchQuery, evidence, req.Request)
	}
	recognized.Object = finalizeRecognizedObject(recognized.Object, nil)
	if err := emitter.emitFrom(ctx, "bedrock", "query_generated", "completed", "", "Recognition query generated", map[string]interface{}{"source": "merged_evidence_query", "query": recognized.Object.SearchQuery, "confidence": recognized.Object.Confidence}); err != nil {
		return nil, err
	}
	adoptedQuerySources := append(speculative.Sources(), "merged_evidence_query")

	stageStart := time.Now()
	var searchResp *model.SearchResponse
	var specResults []model.NormalizedSearchResult
	if speculative.HasQuery(recognized.Object.SearchQuery) {
		specResults = speculative.Wait()
		matching := speculative.ResultsFor(recognized.Object.SearchQuery)
		if len(matching) > 0 {
			searchResp = &model.SearchResponse{Provider: u.SearchProvider, Query: recognized.Object.SearchQuery, Results: matching}
		}
	}
	if searchResp == nil {
		if err := emitter.emit(ctx, "search_started", "started", "", "Web search started", map[string]interface{}{"query": recognized.Object.SearchQuery, "adoptedQuerySources": adoptedQuerySources}); err != nil {
			return nil, err
		}
		var err error
		searchResp, err = u.Searcher.Search(ctx, model.SearchRequest{Query: recognized.Object.SearchQuery, Language: req.Request.Language, MaxResults: req.Request.Options.MaxSearchResults})
		if err != nil {
			if u.Logger != nil {
				u.Logger.Warn("primary web search failed; continuing with degraded final response", "query", recognized.Object.SearchQuery, "error", err)
			}
			searchResp = &model.SearchResponse{Provider: u.SearchProvider, Query: recognized.Object.SearchQuery, Results: nil}
			if err := emitter.emit(ctx, "search_completed", "warning", "", "Web search failed; continuing with visual and LLM evidence", map[string]interface{}{"code": "search_error", "query": recognized.Object.SearchQuery, "error": err.Error(), "searchStatus": "degraded", "adoptedQuerySources": adoptedQuerySources}); err != nil {
				return nil, err
			}
		}
	}
	stageLatency.SearchMs = time.Since(stageStart).Milliseconds()
	if specResults == nil {
		specResults = speculative.Wait()
	}
	adoptedResults := mergeSearchResults(searchResp.Results, specResults)
	searchStatus := "ok"
	searchEventStatus := "completed"
	if len(adoptedResults) == 0 {
		searchStatus = "degraded"
		searchEventStatus = "warning"
	}
	if err := emitter.emit(ctx, "search_results", searchEventStatus, "", "Search results available", map[string]interface{}{"searchResults": adoptedResults, "resultCount": len(adoptedResults), "primaryResultCount": len(searchResp.Results), "speculativeResultCount": len(specResults), "query": searchResp.Query, "searchStatus": searchStatus, "adoptedQuerySources": adoptedQuerySources}); err != nil {
		return nil, err
	}
	if err := emitter.emit(ctx, "search_completed", searchEventStatus, "", "Web search completed", map[string]interface{}{"searchResults": adoptedResults, "resultCount": len(adoptedResults), "primaryResultCount": len(searchResp.Results), "speculativeResultCount": len(specResults), "query": searchResp.Query, "searchStatus": searchStatus, "adoptedQuerySources": adoptedQuerySources}); err != nil {
		return nil, err
	}

	if err := emitter.emit(ctx, "summary_started", "started", "", "Summary generation started", nil); err != nil {
		return nil, err
	}
	recognized.Object = finalizeRecognizedObject(recognized.Object, adoptedResults)
	stageStart = time.Now()
	summary, err := u.LLM.SummarizeSearchResults(ctx, model.SummarizeSearchResultsRequest{Language: req.Request.Language, RecognizedObject: recognized.Object, Results: adoptedResults})
	stageLatency.SummarizeMs = time.Since(stageStart).Milliseconds()
	if err != nil {
		return nil, fmt.Errorf("%w: summarize search results: %v", ErrLLM, err)
	}
	recognized.Object = finalizeRecognizedObject(applySummaryIdentity(recognized.Object, summary), adoptedResults)
	if err := emitter.emit(ctx, "summary_completed", "completed", "", "Summary completed", map[string]interface{}{"model": summary.Model}); err != nil {
		return nil, err
	}
	resp := &model.RecognizeSearchResponse{
		RequestID:        req.RequestID,
		ResponseVersion:  responseVersion(req.Request),
		QueryQuality:     queryQuality(req.Request, evidence, evidenceStatus),
		RecognizedObject: recognized.Object,
		Ambiguity:        ambiguity(recognized.Object),
		Search: model.SearchSection{
			Provider: searchResp.Provider,
			Query:    searchResp.Query,
			Results:  adoptedResults,
		},
		Summary:        model.Summary{Text: summary.Text, LLMProvider: u.LLMProvider, Model: summary.Model},
		Meta:           model.Meta{LLMProvider: u.LLMProvider, SearchProvider: u.SearchProvider, CloudVisionProvider: u.cloudVisionProvider(), ElapsedMs: time.Since(start).Milliseconds(), StageLatency: stageLatency},
		InputSummary:   inputSummary(req.Request),
		ImageAnalyses:  imageAnalyses(req.Request, evidence, evidenceStatus),
		EvidenceFusion: evidenceFusion(req.Request, evidence),
	}
	if err := emitter.emit(ctx, "final", "completed", "", "Final response ready", map[string]interface{}{"response": resp}); err != nil {
		return nil, err
	}
	return resp, nil
}

func recognizeObjectRequest(req ExecuteRequest, evidence *model.VisualEvidence) model.RecognizeObjectRequest {
	return model.RecognizeObjectRequest{ImageDataURL: req.Request.ImageBase64, Crops: req.Request.Crops, Images: recognizeImages(req), MIMEType: req.MIMEType, CropMIMETypes: req.CropMIMETypes, Language: req.Request.Language, VisualEvidence: evidence}
}

func recognizeImages(req ExecuteRequest) []model.RecognizeImageInput {
	if len(req.Request.Images) == 0 {
		return nil
	}
	out := make([]model.RecognizeImageInput, 0, len(req.Request.Images))
	for _, image := range req.Request.Images {
		mimeType := req.MIMEType
		cropMIMETypes := map[string]string(nil)
		if image.ID == primaryImageID(req.Request) {
			cropMIMETypes = req.CropMIMETypes
		}
		out = append(out, model.RecognizeImageInput{ID: image.ID, Role: image.Role, ImageDataURL: image.ImageBase64, Crops: image.Crops, MIMEType: mimeType, CropMIMETypes: cropMIMETypes})
	}
	return out
}

func inputImageCount(req model.RecognizeSearchRequest) int {
	if len(req.Images) > 0 {
		return len(req.Images)
	}
	return 1
}

func responseVersion(req model.RecognizeSearchRequest) int {
	if len(req.Images) > 0 || req.Options.Stream {
		return 2
	}
	return 0
}

func primaryImageID(req model.RecognizeSearchRequest) string {
	if len(req.Images) == 0 {
		return ""
	}
	for _, image := range req.Images {
		if image.Role == "primary" {
			return image.ID
		}
	}
	return req.Images[0].ID
}

func inputSummary(req model.RecognizeSearchRequest) *model.InputSummary {
	if len(req.Images) == 0 {
		return nil
	}
	ids := make([]string, 0, len(req.Images))
	roles := make([]string, 0, len(req.Images))
	for _, image := range req.Images {
		ids = append(ids, image.ID)
		if image.Role != "" {
			roles = append(roles, image.Role)
		}
	}
	mode := "multi_image"
	if req.Options.Stream {
		mode = "multi_image_stream"
	}
	return &model.InputSummary{ImageCount: len(req.Images), PrimaryImageID: primaryImageID(req), ImageIDs: ids, Roles: roles, Mode: mode}
}

func imageAnalyses(req model.RecognizeSearchRequest, evidence *model.VisualEvidence, status string) []model.ImageAnalysis {
	if len(req.Images) == 0 {
		return nil
	}
	analyses := make([]model.ImageAnalysis, 0, len(req.Images))
	primaryID := primaryImageID(req)
	for _, image := range req.Images {
		analysis := model.ImageAnalysis{ImageID: image.ID, Role: image.Role, Status: "received"}
		if image.ID == primaryID {
			analysis.Status = status
			if evidence != nil {
				analysis.EvidenceTypes = evidence.EvidenceTypes()
				analysis.Evidence = evidence
			}
		}
		analyses = append(analyses, analysis)
	}
	return analyses
}

func evidenceFusion(req model.RecognizeSearchRequest, evidence *model.VisualEvidence) *model.EvidenceFusion {
	if len(req.Images) == 0 {
		return nil
	}
	signals := []string{"image count increased"}
	coverage := "image coverage increased"
	agreement := "primary image selected"
	if evidence != nil && !evidence.Empty() {
		signals = append(signals, evidence.EvidenceTypes()...)
		agreement = "visual evidence attached to primary image"
	}
	return &model.EvidenceFusion{Coverage: coverage, Agreement: agreement, Signals: signals, PrimaryImageID: primaryImageID(req)}
}

type evidenceResult struct {
	evidence  *model.VisualEvidence
	status    string
	elapsedMs int64
}

type hypothesisResult struct {
	resp      *model.HypothesizeObjectResponse
	err       error
	elapsedMs int64
}

type recognitionResult struct {
	resp      *model.RecognizeObjectResponse
	err       error
	elapsedMs int64
}

const maxSpeculativeQuerySources = 3

type speculativeSearches struct {
	searcher   search.WebSearcher
	language   string
	maxResults int
	emitter    *eventEmitter
	logger     *slog.Logger
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.Mutex
	sources    []string
	queries    map[string]struct{}
	results    []speculativeSearchResult
	wg         sync.WaitGroup
}

type speculativeSearchResult struct {
	query   string
	results []model.NormalizedSearchResult
}

func newSpeculativeSearches(searcher search.WebSearcher, language string, maxResults int, emitter *eventEmitter, logger *slog.Logger) *speculativeSearches {
	ctx, cancel := context.WithCancel(context.Background())
	return &speculativeSearches{searcher: searcher, language: language, maxResults: maxResults, emitter: emitter, logger: logger, ctx: ctx, cancel: cancel, queries: make(map[string]struct{})}
}

func (s *speculativeSearches) Start(parent context.Context, source string, query string) {
	query = strings.TrimSpace(query)
	if s == nil || s.searcher == nil || query == "" {
		return
	}
	s.mu.Lock()
	if len(s.sources) >= maxSpeculativeQuerySources {
		s.mu.Unlock()
		return
	}
	key := strings.ToLower(query)
	if _, exists := s.queries[key]; exists {
		s.mu.Unlock()
		return
	}
	s.queries[key] = struct{}{}
	s.sources = append(s.sources, source)
	s.mu.Unlock()
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ctx, cancel := context.WithCancel(parent)
		defer cancel()
		go func() {
			select {
			case <-s.ctx.Done():
				cancel()
			case <-ctx.Done():
			}
		}()
		_ = s.emitter.emitFrom(parent, "speculative_search", "search_started", "started", "", "Speculative web search started", map[string]interface{}{"source": source, "query": query, "speculative": true})
		resp, err := s.searcher.Search(ctx, model.SearchRequest{Query: query, Language: s.language, MaxResults: s.maxResults})
		if err != nil {
			if s.logger != nil {
				s.logger.Warn("speculative search failed", "source", source, "query", query, "error", err)
			}
			_ = s.emitter.emitFrom(parent, "speculative_search", "search_completed", "warning", "", "Speculative web search failed", map[string]interface{}{"source": source, "query": query, "error": err.Error(), "speculative": true})
			return
		}
		_ = s.emitter.emitFrom(parent, "speculative_search", "search_results", "completed", "", "Speculative search results available", map[string]interface{}{"source": source, "query": resp.Query, "resultCount": len(resp.Results), "searchResults": resp.Results, "speculative": true})
		s.mu.Lock()
		s.results = append(s.results, speculativeSearchResult{query: resp.Query, results: resp.Results})
		s.mu.Unlock()
	}()
}

func (s *speculativeSearches) HasQuery(query string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, exists := s.queries[strings.ToLower(strings.TrimSpace(query))]
	return exists
}

func (s *speculativeSearches) Wait() []model.NormalizedSearchResult {
	if s == nil {
		return nil
	}
	s.wg.Wait()
	s.cancel()
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]model.NormalizedSearchResult, 0)
	for _, result := range s.results {
		out = append(out, result.results...)
	}
	return out
}

func (s *speculativeSearches) ResultsFor(query string) []model.NormalizedSearchResult {
	if s == nil {
		return nil
	}
	query = strings.ToLower(strings.TrimSpace(query))
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, result := range s.results {
		if strings.ToLower(strings.TrimSpace(result.query)) == query {
			out := make([]model.NormalizedSearchResult, len(result.results))
			copy(out, result.results)
			return out
		}
	}
	return nil
}

func (s *speculativeSearches) Sources() []string {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.sources))
	copy(out, s.sources)
	return out
}

func (u *RecognizeSearchUsecase) hypothesize(ctx context.Context, req ExecuteRequest, evidence *model.VisualEvidence) (*model.HypothesizeObjectResponse, error) {
	hypothesisLLM, ok := u.LLM.(llm.HypothesisLLM)
	if !ok {
		return nil, errors.New("interim hypothesis unsupported")
	}
	return hypothesisLLM.HypothesizeObject(ctx, model.HypothesizeObjectRequest{ImageDataURL: req.Request.ImageBase64, Crops: req.Request.Crops, Images: recognizeImages(req), MIMEType: req.MIMEType, CropMIMETypes: req.CropMIMETypes, Language: req.Request.Language, VisualEvidence: evidence})
}

type querySource struct {
	source     string
	query      string
	confidence string
}

func evidenceQuerySources(evidence *model.VisualEvidence, req model.RecognizeSearchRequest) []querySource {
	if evidence == nil || evidence.Empty() {
		return nil
	}
	queries := make([]querySource, 0, 2)
	for _, ocr := range evidence.OCR {
		queries = appendUniqueQuery(queries, "vision_ocr_query", evidenceSearchText(ocr.Text, req), confidenceFromScore(ocr.Score))
	}
	for _, entity := range evidence.WebEntities {
		queries = appendUniqueQuery(queries, "vision_web_query", evidenceSearchText(entity.Text, req), confidenceFromScore(entity.Score))
	}
	for _, logo := range evidence.Logos {
		queries = appendUniqueQuery(queries, "vision_logo_query", evidenceSearchText(logo.Text, req), confidenceFromScore(logo.Score))
	}
	for _, label := range evidence.BestGuessLabels {
		queries = appendUniqueQuery(queries, "vision_labels_query", evidenceSearchText(label, req), "medium")
	}
	if len(queries) > 2 {
		return queries[:2]
	}
	return queries
}

func appendUniqueQuery(queries []querySource, source string, value string, confidence string) []querySource {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return queries
	}
	lower := strings.ToLower(trimmed)
	for _, query := range queries {
		if strings.ToLower(query.query) == lower {
			return queries
		}
	}
	return append(queries, querySource{source: source, query: trimmed, confidence: confidence})
}

func confidenceFromScore(score float64) string {
	if score >= 0.85 {
		return "high"
	}
	if score >= 0.5 {
		return "medium"
	}
	return "low"
}

func interimHypothesisText(object model.RecognizedObject) string {
	parts := []string{strings.TrimSpace(object.ObjectName)}
	if desc := strings.TrimSpace(object.Description); desc != "" {
		parts = append(parts, desc)
	}
	return strings.Join(parts, " — ")
}

func mergeSearchResults(primary []model.NormalizedSearchResult, speculative []model.NormalizedSearchResult) []model.NormalizedSearchResult {
	merged := make([]model.NormalizedSearchResult, 0, len(primary)+len(speculative))
	seen := make(map[string]struct{}, len(primary)+len(speculative))
	for _, result := range primary {
		seen[searchResultKey(result)] = struct{}{}
		result.Rank = len(merged) + 1
		merged = append(merged, result)
	}
	for _, result := range speculative {
		key := searchResultKey(result)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result.Rank = len(merged) + 1
		merged = append(merged, result)
	}
	return merged
}

func searchResultKey(result model.NormalizedSearchResult) string {
	key := strings.ToLower(strings.TrimSpace(result.URL))
	if key == "" {
		key = strings.ToLower(strings.TrimSpace(result.Title + "|" + result.Snippet))
	}
	return key
}

func enrichSearchQuery(query string, evidence *model.VisualEvidence, req model.RecognizeSearchRequest) string {
	if evidence == nil || evidence.Empty() {
		return query
	}
	terms := make([]string, 0, 6)
	for _, text := range evidence.OCR {
		terms = appendEvidenceTerm(terms, query, text.Text, req)
	}
	for _, entity := range evidence.WebEntities {
		terms = appendEvidenceTerm(terms, query, entity.Text, req)
	}
	for _, logo := range evidence.Logos {
		terms = appendEvidenceTerm(terms, query, logo.Text, req)
	}
	for _, label := range evidence.BestGuessLabels {
		terms = appendEvidenceTerm(terms, query, label, req)
	}
	if len(terms) == 0 {
		return query
	}
	return strings.TrimSpace(query + " " + strings.Join(terms, " "))
}

func appendEvidenceTerm(terms []string, baseQuery string, value string, req model.RecognizeSearchRequest) []string {
	trimmed := evidenceSearchText(value, req)
	if trimmed == "" || len(terms) >= 6 {
		return terms
	}
	lower := strings.ToLower(trimmed)
	if strings.Contains(strings.ToLower(baseQuery), lower) {
		return terms
	}
	for _, term := range terms {
		if strings.ToLower(term) == lower {
			return terms
		}
	}
	return append(terms, trimmed)
}

func evidenceSearchText(value string, req model.RecognizeSearchRequest) string {
	trimmed := stripEvidenceImagePrefix(value, req)
	trimmed = strings.Join(strings.Fields(trimmed), " ")
	return truncateRunes(trimmed, 240)
}

func stripEvidenceImagePrefix(value string, req model.RecognizeSearchRequest) string {
	trimmed := strings.TrimSpace(value)
	prefix, rest, ok := strings.Cut(trimmed, ": ")
	if !ok || len(req.Images) == 0 {
		return trimmed
	}
	for _, image := range req.Images {
		if prefix == image.ID {
			return strings.TrimSpace(rest)
		}
	}
	return trimmed
}

func shouldRefineRecognition(object model.RecognizedObject, evidence *model.VisualEvidence) bool {
	if evidence == nil || evidence.Empty() {
		return false
	}
	if object.NeedsMoreContext || object.Confidence == "low" {
		return true
	}
	return len(evidence.OCR) > 0 || len(evidence.WebEntities) > 0 || len(evidence.BestGuessLabels) > 0
}

func mergeRecognizedObject(base model.RecognizedObject, refined model.RecognizedObject, evidence *model.VisualEvidence) model.RecognizedObject {
	merged := base
	if value := strings.TrimSpace(refined.ObjectName); value != "" {
		merged.ObjectName = value
	}
	if value := strings.TrimSpace(refined.DisplayName); value != "" {
		merged.DisplayName = value
	}
	if value := strings.TrimSpace(refined.Category); value != "" {
		merged.Category = value
	}
	if value := strings.TrimSpace(refined.FinalObjectName); value != "" {
		merged.FinalObjectName = value
	}
	if value := strings.TrimSpace(refined.Description); value != "" {
		merged.Description = value
	}
	if value := strings.TrimSpace(refined.SearchQuery); value != "" {
		merged.SearchQuery = value
	}
	if value := strings.TrimSpace(refined.Confidence); value != "" {
		merged.Confidence = value
	}
	merged.NeedsMoreContext = refined.NeedsMoreContext
	merged.VisualEvidence = evidence
	return finalizeRecognizedObject(merged, nil)
}

func applySummaryIdentity(object model.RecognizedObject, summary *model.SummarizeSearchResultsResponse) model.RecognizedObject {
	if summary == nil {
		return object
	}
	if value := strings.TrimSpace(summary.DisplayName); value != "" {
		object.DisplayName = value
	}
	if value := strings.TrimSpace(summary.Category); value != "" {
		object.Category = value
	}
	if value := strings.TrimSpace(summary.FinalObjectName); value != "" {
		object.FinalObjectName = value
	}
	return object
}

func finalizeRecognizedObject(object model.RecognizedObject, results []model.NormalizedSearchResult) model.RecognizedObject {
	object.ObjectName = strings.TrimSpace(object.ObjectName)
	object.DisplayName = strings.TrimSpace(object.DisplayName)
	object.Category = strings.TrimSpace(object.Category)
	object.FinalObjectName = strings.TrimSpace(object.FinalObjectName)
	object.Description = strings.TrimSpace(object.Description)
	object.SearchQuery = strings.TrimSpace(object.SearchQuery)
	object.Confidence = strings.TrimSpace(object.Confidence)
	bestDisplayName := bestDisplayNameFromResults(results)
	displayNameUpdated := false
	if bestDisplayName != "" && (object.DisplayName == "" || identityLooksGeneric(object.DisplayName)) {
		object.DisplayName = bestDisplayName
		displayNameUpdated = true
	}
	if object.Category == "" {
		object.Category = inferCategory(object, results)
	}
	if object.FinalObjectName == "" || identityLooksGeneric(object.FinalObjectName) || displayNameUpdated {
		object.FinalObjectName = composeFinalObjectName(object.DisplayName, object.Category, object.ObjectName)
	}
	if object.ObjectName == "" {
		object.ObjectName = object.FinalObjectName
	}
	if object.DisplayName == "" && object.FinalObjectName != object.Category {
		object.DisplayName = object.FinalObjectName
	}
	return object
}

func bestDisplayNameFromResults(results []model.NormalizedSearchResult) string {
	for _, result := range results {
		candidate := strings.TrimSpace(result.Title)
		if candidate == "" || identityLooksGeneric(candidate) {
			continue
		}
		return truncateRunes(candidate, 120)
	}
	return ""
}

func inferCategory(object model.RecognizedObject, results []model.NormalizedSearchResult) string {
	text := strings.ToLower(strings.Join(objectIdentityTexts(object, results), " "))
	if strings.Contains(text, "blu-ray") || strings.Contains(text, "bluray") || strings.Contains(text, "ブルーレイ") || strings.Contains(text, "dvd") {
		if containsJapanese(text) {
			return "Blu-ray/DVDパッケージ"
		}
		return "Blu-ray/DVD package"
	}
	return ""
}

func objectIdentityTexts(object model.RecognizedObject, results []model.NormalizedSearchResult) []string {
	texts := []string{object.ObjectName, object.DisplayName, object.Category, object.FinalObjectName, object.Description, object.SearchQuery}
	if object.VisualEvidence != nil {
		for _, item := range object.VisualEvidence.OCR {
			texts = append(texts, item.Text)
		}
		for _, item := range object.VisualEvidence.WebEntities {
			texts = append(texts, item.Text)
		}
		texts = append(texts, object.VisualEvidence.BestGuessLabels...)
	}
	for _, result := range results {
		texts = append(texts, result.Title, result.Snippet)
	}
	return texts
}

func composeFinalObjectName(displayName string, category string, objectName string) string {
	displayName = strings.TrimSpace(displayName)
	category = strings.TrimSpace(category)
	objectName = strings.TrimSpace(objectName)
	if displayName != "" && category != "" {
		if strings.Contains(strings.ToLower(displayName), strings.ToLower(category)) {
			return displayName
		}
		separator := " "
		if containsJapanese(displayName) || containsJapanese(category) {
			separator = ""
		}
		return displayName + separator + category
	}
	if displayName != "" {
		return displayName
	}
	if objectName != "" {
		return objectName
	}
	return category
}

func identityLooksGeneric(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || runeLen(trimmed) > 40 {
		return false
	}
	lower := strings.ToLower(trimmed)
	return strings.Contains(lower, "album") || strings.Contains(lower, "cd") || strings.Contains(lower, "package") || strings.Contains(lower, "パッケージ") || strings.Contains(lower, "アルバム")
}

func containsJapanese(value string) bool {
	for _, r := range value {
		if (r >= '\u3040' && r <= '\u30ff') || (r >= '\u4e00' && r <= '\u9fff') || r == '『' || r == '』' {
			return true
		}
	}
	return false
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func runeLen(value string) int {
	return len([]rune(value))
}

func (u *RecognizeSearchUsecase) cloudVisionProvider() string {
	if u.CloudVisionProvider != "" {
		return u.CloudVisionProvider
	}
	if u.Vision == nil {
		return "disabled"
	}
	return "cloud-vision"
}

func (u *RecognizeSearchUsecase) extractEvidence(ctx context.Context, req ExecuteRequest) (*model.VisualEvidence, string) {
	if u.Vision == nil {
		if req.Request.Crops != nil || len(req.Request.Images) > 0 {
			return nil, "multi_crop_received_cloud_vision_disabled"
		}
		return nil, "cloud_vision_disabled"
	}
	if len(req.Request.Images) > 0 {
		return u.extractMultiImageEvidence(ctx, req)
	}
	resp, err := u.Vision.ExtractEvidence(ctx, model.ExtractEvidenceRequest{ImageDataURL: req.Request.ImageBase64, Crops: req.Request.Crops, MIMEType: req.MIMEType, CropMIMETypes: req.CropMIMETypes})
	if err != nil {
		if u.Logger != nil {
			u.Logger.Warn("cloud vision evidence extraction failed", "error", err)
		}
		return nil, "cloud_vision_error"
	}
	if resp == nil || resp.Evidence.Empty() {
		return nil, "cloud_vision_no_evidence"
	}
	evidence := resp.Evidence
	return &evidence, "measured"
}

func (u *RecognizeSearchUsecase) extractMultiImageEvidence(ctx context.Context, req ExecuteRequest) (*model.VisualEvidence, string) {
	merged := model.VisualEvidence{}
	measured := 0
	for _, image := range req.Request.Images {
		cropMIMETypes := map[string]string(nil)
		if image.ID == primaryImageID(req.Request) {
			cropMIMETypes = req.CropMIMETypes
		}
		resp, err := u.Vision.ExtractEvidence(ctx, model.ExtractEvidenceRequest{ImageDataURL: image.ImageBase64, Crops: image.Crops, MIMEType: req.MIMEType, CropMIMETypes: cropMIMETypes})
		if err != nil {
			if u.Logger != nil {
				u.Logger.Warn("cloud vision evidence extraction failed", "imageId", image.ID, "error", err)
			}
			continue
		}
		if resp == nil || resp.Evidence.Empty() {
			continue
		}
		measured++
		mergeEvidence(&merged, resp.Evidence, image.ID)
	}
	if measured == 0 {
		return nil, "cloud_vision_no_evidence"
	}
	return &merged, "measured"
}

func mergeEvidence(dst *model.VisualEvidence, src model.VisualEvidence, imageID string) {
	prefix := imageID
	if prefix != "" {
		prefix += ": "
	}
	for _, item := range src.OCR {
		item.Text = prefix + item.Text
		dst.OCR = append(dst.OCR, item)
	}
	for _, item := range src.Logos {
		item.Text = prefix + item.Text
		dst.Logos = append(dst.Logos, item)
	}
	for _, item := range src.WebEntities {
		item.Text = prefix + item.Text
		dst.WebEntities = append(dst.WebEntities, item)
	}
	for _, label := range src.BestGuessLabels {
		dst.BestGuessLabels = append(dst.BestGuessLabels, prefix+label)
	}
	for _, item := range src.Labels {
		item.Text = prefix + item.Text
		dst.Labels = append(dst.Labels, item)
	}
	dst.MatchingImageURLs = append(dst.MatchingImageURLs, src.MatchingImageURLs...)
	for _, image := range src.RelatedImages {
		if image.SourceImageID == "" {
			image.SourceImageID = imageID
		}
		dst.RelatedImages = append(dst.RelatedImages, image)
	}
}

func queryQuality(req model.RecognizeSearchRequest, evidence *model.VisualEvidence, status string) model.QueryQuality {
	quality := model.QueryQuality{Blur: "unknown", CropConfidence: "unknown", TextVisibility: "unknown", Status: status}
	if quality.Status == "" {
		quality.Status = "not_measured"
		if req.Crops != nil {
			quality.Status = "multi_crop_received_not_measured"
		}
	}
	if req.Crops != nil {
		quality.CropConfidence = "received"
	}
	if evidence != nil {
		quality.EvidenceTypes = evidence.EvidenceTypes()
		if len(evidence.OCR) > 0 {
			quality.TextVisibility = "high"
		}
	}
	return quality
}

func ambiguity(object model.RecognizedObject) model.Ambiguity {
	if object.NeedsMoreContext {
		return model.Ambiguity{IsAmbiguous: true, Reason: "recognizer requested more context"}
	}
	if object.Confidence == "low" {
		return model.Ambiguity{IsAmbiguous: true, Reason: "recognizer confidence is low"}
	}
	return model.Ambiguity{IsAmbiguous: false, Reason: "recognizer returned a confident candidate"}
}
