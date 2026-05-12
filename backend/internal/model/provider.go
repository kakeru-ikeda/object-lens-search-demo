package model

type RecognizeObjectRequest struct {
	ImageDataURL string
	MIMEType     string
	Language     string
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
