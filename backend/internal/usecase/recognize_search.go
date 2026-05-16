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
	e.mu.Lock()
	defer e.mu.Unlock()
	e.sequence++
	return e.sink.Emit(ctx, StreamEvent{RequestID: e.requestID, Sequence: e.sequence, Stage: stage, Status: status, ElapsedMs: time.Since(e.start).Milliseconds(), ImageID: imageID, Message: message, Payload: payload})
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

	go func() {
		_ = emitter.emit(workCtx, "vision_started", "started", "", "Visual evidence extraction started", nil)
		stageStart := time.Now()
		evidence, status := u.extractEvidence(workCtx, req)
		elapsed := time.Since(stageStart).Milliseconds()
		_ = emitter.emit(workCtx, "vision_completed", "completed", primaryImageID(req.Request), "Visual evidence extraction completed", map[string]interface{}{"status": status, "elapsedMs": elapsed})
		evidenceCh <- evidenceResult{evidence: evidence, status: status, elapsedMs: elapsed}
	}()

	if err := emitter.emit(ctx, "recognition_started", "started", "", "Integrated recognition started", nil); err != nil {
		return nil, err
	}
	stageStart := time.Now()
	recognized, err := u.LLM.RecognizeObject(workCtx, model.RecognizeObjectRequest{ImageDataURL: req.Request.ImageBase64, Crops: req.Request.Crops, Images: recognizeImages(req), MIMEType: req.MIMEType, CropMIMETypes: req.CropMIMETypes, Language: req.Request.Language})
	stageLatency.RecognizeMs = time.Since(stageStart).Milliseconds()
	if err != nil {
		cancel()
		<-evidenceCh
		return nil, fmt.Errorf("%w: recognize object: %v", ErrLLM, err)
	}
	if err := emitter.emit(ctx, "recognition_completed", "completed", "", "Recognition completed", map[string]interface{}{"objectName": recognized.Object.ObjectName, "searchQuery": recognized.Object.SearchQuery}); err != nil {
		return nil, err
	}

	evidenceRes := <-evidenceCh
	evidence := evidenceRes.evidence
	evidenceStatus := evidenceRes.status
	stageLatency.CloudVisionMs = evidenceRes.elapsedMs
	if evidence != nil && !evidence.Empty() {
		recognized.Object.VisualEvidence = evidence
		recognized.Object.SearchQuery = enrichSearchQuery(recognized.Object.SearchQuery, evidence)
	}

	if err := emitter.emit(ctx, "search_started", "started", "", "Web search started", map[string]interface{}{"query": recognized.Object.SearchQuery}); err != nil {
		return nil, err
	}
	stageStart = time.Now()
	searchResp, err := u.Searcher.Search(ctx, model.SearchRequest{Query: recognized.Object.SearchQuery, Language: req.Request.Language, MaxResults: req.Request.Options.MaxSearchResults})
	stageLatency.SearchMs = time.Since(stageStart).Milliseconds()
	if err != nil {
		return nil, fmt.Errorf("%w: search: %v", ErrSearch, err)
	}
	if err := emitter.emit(ctx, "search_completed", "completed", "", "Web search completed", map[string]interface{}{"resultCount": len(searchResp.Results), "query": searchResp.Query}); err != nil {
		return nil, err
	}

	if err := emitter.emit(ctx, "summary_started", "started", "", "Summary generation started", nil); err != nil {
		return nil, err
	}
	stageStart = time.Now()
	summary, err := u.LLM.SummarizeSearchResults(ctx, model.SummarizeSearchResultsRequest{Language: req.Request.Language, RecognizedObject: recognized.Object, Results: searchResp.Results})
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
			Results:  searchResp.Results,
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

func enrichSearchQuery(query string, evidence *model.VisualEvidence) string {
	if evidence == nil || evidence.Empty() {
		return query
	}
	terms := make([]string, 0, 6)
	for _, logo := range evidence.Logos {
		terms = appendEvidenceTerm(terms, logo.Text)
	}
	for _, text := range evidence.OCR {
		terms = appendEvidenceTerm(terms, text.Text)
	}
	for _, label := range evidence.BestGuessLabels {
		terms = appendEvidenceTerm(terms, label)
	}
	for _, entity := range evidence.WebEntities {
		terms = appendEvidenceTerm(terms, entity.Text)
	}
	if len(terms) == 0 {
		return query
	}
	return strings.TrimSpace(query + " " + strings.Join(terms, " "))
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
