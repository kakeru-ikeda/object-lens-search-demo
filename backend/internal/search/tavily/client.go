package tavily

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"object-lens-search-demo/backend/internal/model"
)

var defaultHTTPClient = &http.Client{Timeout: 20 * time.Second}

type Client struct {
	APIKey     string
	Endpoint   string
	HTTPClient *http.Client
}

type requestBody struct {
	APIKey        string `json:"api_key"`
	Query         string `json:"query"`
	SearchDepth   string `json:"search_depth,omitempty"`
	MaxResults    int    `json:"max_results"`
	IncludeAnswer bool   `json:"include_answer"`
}

type responseBody struct {
	Query   string         `json:"query"`
	Answer  string         `json:"answer"`
	Results []tavilyResult `json:"results"`
}

type tavilyResult struct {
	Title        string   `json:"title"`
	URL          string   `json:"url"`
	Content      string   `json:"content"`
	RawContent   string   `json:"raw_content"`
	Score        *float64 `json:"score"`
	PublishedAt  string   `json:"published_at"`
	ContentType  string   `json:"content_type"`
	ProviderRank int      `json:"rank"`
}

func (c *Client) Search(ctx context.Context, req model.SearchRequest) (*model.SearchResponse, error) {
	if strings.TrimSpace(c.APIKey) == "" {
		return nil, errors.New("TAVILY_API_KEY is required")
	}
	endpoint := c.Endpoint
	if endpoint == "" {
		endpoint = "https://api.tavily.com/search"
	}
	body, err := json.Marshal(requestBody{APIKey: c.APIKey, Query: req.Query, SearchDepth: "basic", MaxResults: req.MaxResults, IncludeAnswer: false})
	if err != nil {
		return nil, fmt.Errorf("marshal tavily request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create tavily request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	client := c.HTTPClient
	if client == nil {
		client = defaultHTTPClient
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call tavily: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("tavily returned status %d", resp.StatusCode)
	}
	var parsed responseBody
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode tavily response: %w", err)
	}
	results := make([]model.NormalizedSearchResult, 0, len(parsed.Results))
	for i, item := range parsed.Results {
		rank := i + 1
		if item.ProviderRank > 0 {
			rank = item.ProviderRank
		}
		score := 1 / float64(rank)
		if item.Score != nil {
			score = *item.Score
		}
		raw, err := json.Marshal(item)
		if err != nil {
			return nil, fmt.Errorf("marshal tavily raw result: %w", err)
		}
		publishedAt := normalizePublishedAt(item.PublishedAt)
		results = append(results, model.NormalizedSearchResult{
			ID:          fmt.Sprintf("sr_%03d", rank),
			Title:       item.Title,
			URL:         item.URL,
			DisplayURL:  displayURL(item.URL),
			Snippet:     firstNonEmpty(item.Content, item.RawContent),
			Source:      sourceHost(item.URL),
			PublishedAt: publishedAt,
			Language:    req.Language,
			Rank:        rank,
			Score:       score,
			ContentType: firstNonEmpty(item.ContentType, "web_page"),
			Provider:    "tavily",
			Raw:         raw,
		})
	}
	return &model.SearchResponse{Provider: "tavily", Query: firstNonEmpty(parsed.Query, req.Query), Results: results}, nil
}

func normalizePublishedAt(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		v := t.UTC().Format(time.RFC3339)
		return &v
	}
	if t, err := time.Parse("2006-01-02", value); err == nil {
		v := t.UTC().Format(time.RFC3339)
		return &v
	}
	return &value
}

func displayURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return raw
	}
	return parsed.Host + parsed.EscapedPath()
}

func sourceHost(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "unknown"
	}
	return parsed.Host
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
