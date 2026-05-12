package model

import "encoding/json"

type RecognizeSearchRequest struct {
	ImageBase64 string         `json:"imageBase64"`
	Language    string         `json:"language,omitempty"`
	Options     RequestOptions `json:"options,omitempty"`
}

type RequestOptions struct {
	MaxSearchResults int `json:"maxSearchResults,omitempty"`
}

type RecognizedObject struct {
	ObjectName       string `json:"objectName"`
	Description      string `json:"description"`
	SearchQuery      string `json:"searchQuery"`
	Confidence       string `json:"confidence"`
	NeedsMoreContext bool   `json:"needsMoreContext"`
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
	RecognizedObject RecognizedObject `json:"recognizedObject"`
	Search           SearchSection    `json:"search"`
	Summary          Summary          `json:"summary"`
	Meta             Meta             `json:"meta"`
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

type Meta struct {
	LLMProvider    string `json:"llmProvider"`
	SearchProvider string `json:"searchProvider"`
	ElapsedMs      int64  `json:"elapsedMs"`
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
