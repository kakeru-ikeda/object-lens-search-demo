package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv              string
	Port                string
	AllowedOrigins      []string
	RateLimitPerMinute  int
	MaxRequestBytes     int64
	RequestTimeout      time.Duration
	LLMProvider         string
	SearchProvider      string
	AWSRegion           string
	BedrockModelID      string
	TavilyAPIKey        string
	TavilyEndpoint      string
	AllowMockFallback   bool
	EffectiveLLMMode    string
	EffectiveSearchMode string
}

func Load() (Config, error) {
	cfg := Config{
		AppEnv:             getEnv("APP_ENV", "development"),
		Port:               getEnv("PORT", "8080"),
		AllowedOrigins:     splitCSV(getEnv("ALLOWED_ORIGINS", "http://localhost:5173")),
		RateLimitPerMinute: getEnvInt("RATE_LIMIT_PER_MINUTE", 30),
		MaxRequestBytes:    int64(getEnvInt("MAX_REQUEST_BYTES", 5*1024*1024)),
		RequestTimeout:     time.Duration(getEnvInt("REQUEST_TIMEOUT_SECONDS", 30)) * time.Second,
		LLMProvider:        strings.ToLower(getEnv("LLM_PROVIDER", "bedrock")),
		SearchProvider:     strings.ToLower(getEnv("SEARCH_PROVIDER", "tavily")),
		AWSRegion:          getEnv("AWS_REGION", ""),
		BedrockModelID:     getEnv("BEDROCK_MODEL_ID", ""),
		TavilyAPIKey:       getEnv("TAVILY_API_KEY", ""),
		TavilyEndpoint:     getEnv("TAVILY_ENDPOINT", "https://api.tavily.com/search"),
	}
	cfg.AllowMockFallback = cfg.AppEnv != "production"
	cfg.EffectiveLLMMode = cfg.LLMProvider
	cfg.EffectiveSearchMode = cfg.SearchProvider

	if cfg.RateLimitPerMinute <= 0 {
		return cfg, errors.New("RATE_LIMIT_PER_MINUTE must be positive")
	}
	if cfg.MaxRequestBytes <= 0 {
		return cfg, errors.New("MAX_REQUEST_BYTES must be positive")
	}
	if cfg.RequestTimeout <= 0 {
		return cfg, errors.New("REQUEST_TIMEOUT_SECONDS must be positive")
	}
	if cfg.LLMProvider != "bedrock" && cfg.LLMProvider != "mock" {
		return cfg, errors.New("LLM_PROVIDER must be bedrock or mock")
	}
	if cfg.SearchProvider != "tavily" && cfg.SearchProvider != "mock" {
		return cfg, errors.New("SEARCH_PROVIDER must be tavily or mock")
	}
	if cfg.AppEnv == "production" {
		if cfg.LLMProvider == "mock" || cfg.SearchProvider == "mock" {
			return cfg, errors.New("mock providers are not allowed in production")
		}
		if cfg.AWSRegion == "" || cfg.BedrockModelID == "" {
			return cfg, errors.New("AWS_REGION and BEDROCK_MODEL_ID are required in production")
		}
		if cfg.TavilyAPIKey == "" {
			return cfg, errors.New("TAVILY_API_KEY is required in production")
		}
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return parsed
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
