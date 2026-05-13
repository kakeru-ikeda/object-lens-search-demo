package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
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

func (u *RecognizeSearchUsecase) Execute(ctx context.Context, req ExecuteRequest) (*model.RecognizeSearchResponse, error) {
	start := time.Now()
	stageLatency := model.StageLatency{}
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	evidenceCh := make(chan evidenceResult, 1)

	go func() {
		stageStart := time.Now()
		evidence, status := u.extractEvidence(workCtx, req)
		evidenceCh <- evidenceResult{evidence: evidence, status: status, elapsedMs: time.Since(stageStart).Milliseconds()}
	}()

	stageStart := time.Now()
	recognized, err := u.LLM.RecognizeObject(workCtx, model.RecognizeObjectRequest{ImageDataURL: req.Request.ImageBase64, Crops: req.Request.Crops, MIMEType: req.MIMEType, CropMIMETypes: req.CropMIMETypes, Language: req.Request.Language})
	stageLatency.RecognizeMs = time.Since(stageStart).Milliseconds()
	if err != nil {
		cancel()
		<-evidenceCh
		return nil, fmt.Errorf("%w: recognize object: %v", ErrLLM, err)
	}

	evidenceRes := <-evidenceCh
	evidence := evidenceRes.evidence
	evidenceStatus := evidenceRes.status
	stageLatency.CloudVisionMs = evidenceRes.elapsedMs
	if evidence != nil && !evidence.Empty() {
		recognized.Object.VisualEvidence = evidence
		recognized.Object.SearchQuery = enrichSearchQuery(recognized.Object.SearchQuery, evidence)
	}

	stageStart = time.Now()
	searchResp, err := u.Searcher.Search(ctx, model.SearchRequest{Query: recognized.Object.SearchQuery, Language: req.Request.Language, MaxResults: req.Request.Options.MaxSearchResults})
	stageLatency.SearchMs = time.Since(stageStart).Milliseconds()
	if err != nil {
		return nil, fmt.Errorf("%w: search: %v", ErrSearch, err)
	}

	stageStart = time.Now()
	summary, err := u.LLM.SummarizeSearchResults(ctx, model.SummarizeSearchResultsRequest{Language: req.Request.Language, RecognizedObject: recognized.Object, Results: searchResp.Results})
	stageLatency.SummarizeMs = time.Since(stageStart).Milliseconds()
	if err != nil {
		return nil, fmt.Errorf("%w: summarize search results: %v", ErrLLM, err)
	}
	return &model.RecognizeSearchResponse{
		RequestID:        req.RequestID,
		QueryQuality:     queryQuality(req.Request, evidence, evidenceStatus),
		RecognizedObject: recognized.Object,
		Ambiguity:        ambiguity(recognized.Object),
		Search: model.SearchSection{
			Provider: searchResp.Provider,
			Query:    searchResp.Query,
			Results:  searchResp.Results,
		},
		Summary: model.Summary{Text: summary.Text, LLMProvider: u.LLMProvider, Model: summary.Model},
		Meta:    model.Meta{LLMProvider: u.LLMProvider, SearchProvider: u.SearchProvider, CloudVisionProvider: u.cloudVisionProvider(), ElapsedMs: time.Since(start).Milliseconds(), StageLatency: stageLatency},
	}, nil
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
		if req.Request.Crops != nil {
			return nil, "multi_crop_received_cloud_vision_disabled"
		}
		return nil, "cloud_vision_disabled"
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
