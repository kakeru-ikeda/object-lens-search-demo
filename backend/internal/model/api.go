package model

import "encoding/json"

type RecognizeSearchRequest struct {
	ImageBase64 string         `json:"imageBase64,omitempty"`
	Crops       *ImageCrops    `json:"crops,omitempty"`
	Images      []ImageInput   `json:"images,omitempty"`
	Language    string         `json:"language,omitempty"`
	Options     RequestOptions `json:"options,omitempty"`
}

type ImageInput struct {
	ID          string      `json:"id,omitempty"`
	Role        string      `json:"role,omitempty"`
	ImageBase64 string      `json:"imageBase64,omitempty"`
	Crops       *ImageCrops `json:"crops,omitempty"`
}

type ImageCrops struct {
	TightCrop        string `json:"tightCrop"`
	ContextCrop      string `json:"contextCrop"`
	TextEnhancedCrop string `json:"textEnhancedCrop,omitempty"`
}

type RequestOptions struct {
	MaxSearchResults int  `json:"maxSearchResults,omitempty"`
	EnableMultiCrop  bool `json:"enableMultiCrop,omitempty"`
	MaxImages        int  `json:"maxImages,omitempty"`
	Stream           bool `json:"stream,omitempty"`
}

type EvidenceItem struct {
	Text  string  `json:"text"`
	Score float64 `json:"score,omitempty"`
}

type VisualEvidence struct {
	OCR               []EvidenceItem `json:"ocr,omitempty"`
	Logos             []EvidenceItem `json:"logos,omitempty"`
	WebEntities       []EvidenceItem `json:"webEntities,omitempty"`
	BestGuessLabels   []string       `json:"bestGuessLabels,omitempty"`
	Labels            []EvidenceItem `json:"labels,omitempty"`
	MatchingImageURLs []string       `json:"matchingImageUrls,omitempty"`
}

func (e VisualEvidence) EvidenceTypes() []string {
	types := make([]string, 0, 4)
	if len(e.OCR) > 0 {
		types = append(types, "ocr")
	}
	if len(e.Logos) > 0 {
		types = append(types, "logo")
	}
	if len(e.WebEntities) > 0 || len(e.BestGuessLabels) > 0 || len(e.MatchingImageURLs) > 0 {
		types = append(types, "web")
	}
	if len(e.Labels) > 0 {
		types = append(types, "label")
	}
	return types
}

func (e VisualEvidence) Empty() bool {
	return len(e.EvidenceTypes()) == 0
}

type RecognizedObject struct {
	ObjectName       string          `json:"objectName"`
	Description      string          `json:"description"`
	SearchQuery      string          `json:"searchQuery"`
	Confidence       string          `json:"confidence"`
	NeedsMoreContext bool            `json:"needsMoreContext"`
	VisualEvidence   *VisualEvidence `json:"visualEvidence,omitempty"`
}

type NormalizedSearchResult struct {
	ID          string          `json:"id"`
	Title       string          `json:"title"`
	URL         string          `json:"url"`
	DisplayURL  string          `json:"displayUrl"`
	Snippet     string          `json:"snippet"`
	Source      string          `json:"source"`
	PublishedAt *string         `json:"publishedAt"`
	Language    string          `json:"language"`
	Rank        int             `json:"rank"`
	Score       float64         `json:"score"`
	ContentType string          `json:"contentType"`
	Provider    string          `json:"provider"`
	Raw         json.RawMessage `json:"raw"`
}

type RecognizeSearchResponse struct {
	RequestID        string           `json:"requestId"`
	ResponseVersion  int              `json:"responseVersion,omitempty"`
	QueryQuality     QueryQuality     `json:"queryQuality"`
	RecognizedObject RecognizedObject `json:"recognizedObject"`
	Ambiguity        Ambiguity        `json:"ambiguity"`
	Search           SearchSection    `json:"search"`
	Summary          Summary          `json:"summary"`
	Meta             Meta             `json:"meta"`
	InputSummary     *InputSummary    `json:"inputSummary,omitempty"`
	ImageAnalyses    []ImageAnalysis  `json:"imageAnalyses,omitempty"`
	EvidenceFusion   *EvidenceFusion  `json:"evidenceFusion,omitempty"`
}

type InputSummary struct {
	ImageCount     int      `json:"imageCount"`
	PrimaryImageID string   `json:"primaryImageId"`
	ImageIDs       []string `json:"imageIds"`
	Roles          []string `json:"roles,omitempty"`
	Mode           string   `json:"mode"`
}

type ImageAnalysis struct {
	ImageID       string          `json:"imageId"`
	Role          string          `json:"role,omitempty"`
	EvidenceTypes []string        `json:"evidenceTypes,omitempty"`
	Status        string          `json:"status"`
	Evidence      *VisualEvidence `json:"evidence,omitempty"`
}

type EvidenceFusion struct {
	Coverage       string   `json:"coverage"`
	Agreement      string   `json:"agreement"`
	Signals        []string `json:"signals,omitempty"`
	PrimaryImageID string   `json:"primaryImageId"`
}

type QueryQuality struct {
	Blur           string   `json:"blur"`
	CropConfidence string   `json:"cropConfidence"`
	TextVisibility string   `json:"textVisibility"`
	Status         string   `json:"status"`
	EvidenceTypes  []string `json:"evidenceTypes,omitempty"`
}

type Ambiguity struct {
	IsAmbiguous bool   `json:"isAmbiguous"`
	Reason      string `json:"reason"`
}

type SearchSection struct {
	Provider string                   `json:"provider"`
	Query    string                   `json:"query"`
	Results  []NormalizedSearchResult `json:"results"`
}

type Summary struct {
	Text        string `json:"text"`
	LLMProvider string `json:"llmProvider"`
	Model       string `json:"model"`
}

type StageLatency struct {
	CloudVisionMs int64 `json:"cloudVisionMs"`
	RecognizeMs   int64 `json:"recognizeMs"`
	SearchMs      int64 `json:"searchMs"`
	SummarizeMs   int64 `json:"summarizeMs"`
}

type Meta struct {
	LLMProvider         string       `json:"llmProvider"`
	SearchProvider      string       `json:"searchProvider"`
	CloudVisionProvider string       `json:"cloudVisionProvider"`
	ElapsedMs           int64        `json:"elapsedMs"`
	StageLatency        StageLatency `json:"stageLatency"`
}

type ErrorResponse struct {
	Error APIError `json:"error"`
}

type APIError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestId"`
}

const (
	ErrInvalidRequest       = "invalid_request"
	ErrImageTooLarge        = "image_too_large"
	ErrUnsupportedImageType = "unsupported_image_type"
	ErrLLM                  = "llm_error"
	ErrSearch               = "search_error"
	ErrTimeout              = "timeout"
	ErrRateLimited          = "rate_limited"
	ErrInternal             = "internal_error"
)
