package model

type RecognizeObjectRequest struct {
	ImageDataURL   string
	Crops          *ImageCrops
	MIMEType       string
	CropMIMETypes  map[string]string
	Language       string
	VisualEvidence *VisualEvidence
}

type ExtractEvidenceRequest struct {
	ImageDataURL  string
	Crops         *ImageCrops
	MIMEType      string
	CropMIMETypes map[string]string
}

type ExtractEvidenceResponse struct {
	Evidence VisualEvidence
	Provider string
}

type RecognizeObjectResponse struct {
	Object RecognizedObject
	Model  string
}

type SummarizeSearchResultsRequest struct {
	Language         string
	RecognizedObject RecognizedObject
	Results          []NormalizedSearchResult
}

type SummarizeSearchResultsResponse struct {
	Text  string
	Model string
}

type SearchRequest struct {
	Query      string
	Language   string
	MaxResults int
}

type SearchResponse struct {
	Provider string
	Query    string
	Results  []NormalizedSearchResult
}
