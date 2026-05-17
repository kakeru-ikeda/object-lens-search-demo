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
		recognized, err := u.LLM.RecognizeObject(workCtx, model.RecognizeObjectRequest{ImageDataURL: req.Request.ImageBase64, Crops: req.Request.Crops, Images: recognizeImages(req), MIMEType: req.MIMEType, CropMIMETypes: req.CropMIMETypes, Language: req.Request.Language})
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
			for _, source := range evidenceQuerySources(res.evidence) {
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
			// Start speculative searches for each multi-intent query candidate from the LLM.
			for _, candidate := range recognized.Object.SearchQueries {
				if candidate.Query == "" || candidate.Query == recognized.Object.SearchQuery {
					continue
				}
				_ = emitter.emitFrom(workCtx, "bedrock", "query_generated", "completed", "", "Multi-intent query generated", map[string]interface{}{"source": "recognition_" + candidate.Intent, "query": candidate.Query, "intent": candidate.Intent})
				speculative.Start(ctx, "recognition_"+candidate.Intent, candidate.Query)
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
	if evidence != nil && !evidence.Empty() {
		recognized.Object.VisualEvidence = evidence
		recognized.Object.SearchQuery = enrichSearchQuery(recognized.Object.SearchQuery, evidence)
	}
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
	stageStart = time.Now()
	summary, err := u.LLM.SummarizeSearchResults(ctx, model.SummarizeSearchResultsRequest{Language: req.Request.Language, RecognizedObject: recognized.Object, Results: adoptedResults})
	stageLatency.SummarizeMs = time.Since(stageStart).Milliseconds()
	if err != nil {
		return nil, fmt.Errorf("%w: summarize search results: %v", ErrLLM, err)
	}
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

const maxSpeculativeQuerySources = 5

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

func evidenceQuerySources(evidence *model.VisualEvidence) []querySource {
	if evidence == nil || evidence.Empty() {
		return nil
	}
	queries := make([]querySource, 0, 4)
	// 1. BestGuessLabels — Google's direct product hypothesis, highest value
	for _, label := range evidence.BestGuessLabels {
		queries = appendUniqueQuery(queries, "vision_best_guess", label, "high")
	}
	// 2. WebEntities — often contain specific product/brand names
	for _, entity := range evidence.WebEntities {
		if entity.Score >= 0.5 {
			queries = appendUniqueQuery(queries, "vision_web_entity", entity.Text, confidenceFromScore(entity.Score))
		}
	}
	// 3. Logos — brand names
	for _, logo := range evidence.Logos {
		queries = appendUniqueQuery(queries, "vision_logo", logo.Text, confidenceFromScore(logo.Score))
	}
	// 4. OCR — only if it looks like a model number (alphanumeric, short)
	for _, ocr := range evidence.OCR {
		if isLikelyModelNumber(ocr.Text) {
			queries = appendUniqueQuery(queries, "vision_ocr_model", ocr.Text, confidenceFromScore(ocr.Score))
		}
	}
	if len(queries) > 3 {
		return queries[:3]
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

func enrichSearchQuery(query string, evidence *model.VisualEvidence) string {
	if evidence == nil || evidence.Empty() {
		return query
	}
	queryLower := strings.ToLower(query)
	terms := make([]string, 0, 4)
	// Logos: brand names are high-signal — add only if not already in the query
	for _, logo := range evidence.Logos {
		term := strings.TrimSpace(logo.Text)
		if term != "" && !strings.Contains(queryLower, strings.ToLower(term)) {
			terms = appendEvidenceTerm(terms, term)
		}
	}
	// WebEntities: product/brand identifiers — add high-confidence ones not already in query
	for _, entity := range evidence.WebEntities {
		term := strings.TrimSpace(entity.Text)
		if term != "" && entity.Score >= 0.5 && !strings.Contains(queryLower, strings.ToLower(term)) {
			terms = appendEvidenceTerm(terms, term)
		}
	}
	// BestGuessLabels: Google's product hypothesis — add if not already in query
	for _, label := range evidence.BestGuessLabels {
		term := strings.TrimSpace(label)
		if term != "" && !strings.Contains(queryLower, strings.ToLower(term)) {
			terms = appendEvidenceTerm(terms, term)
		}
	}
	// Skip raw OCR text — noisy for search queries; model numbers are handled via speculative searches
	if len(terms) == 0 {
		return query
	}
	return strings.TrimSpace(query + " " + strings.Join(terms, " "))
}

// isLikelyModelNumber returns true when text looks like a product model number
// (3–25 chars, contains both letters and digits).
func isLikelyModelNumber(text string) bool {
	text = strings.TrimSpace(text)
	if len(text) < 3 || len(text) > 25 {
		return false
	}
	hasLetter, hasDigit := false, false
	for _, r := range text {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			hasLetter = true
		}
		if r >= '0' && r <= '9' {
			hasDigit = true
		}
	}
	return hasLetter && hasDigit
}

func appendEvidenceTerm(terms []string, value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || len(terms) >= 6 {
		return terms
	}
	lower := strings.ToLower(trimmed)
	for _, term := range terms {
		if strings.ToLower(term) == lower {
			return terms
		}
	}
	return append(terms, trimmed)
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
