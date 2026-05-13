package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"

	"object-lens-search-demo/backend/internal/config"
	"object-lens-search-demo/backend/internal/handler"
	"object-lens-search-demo/backend/internal/llm"
	bedrockllm "object-lens-search-demo/backend/internal/llm/bedrock"
	mockllm "object-lens-search-demo/backend/internal/llm/mock"
	"object-lens-search-demo/backend/internal/middleware"
	"object-lens-search-demo/backend/internal/search"
	mocksearch "object-lens-search-demo/backend/internal/search/mock"
	"object-lens-search-demo/backend/internal/search/tavily"
	"object-lens-search-demo/backend/internal/usecase"
	"object-lens-search-demo/backend/internal/vision"
	cloudvision "object-lens-search-demo/backend/internal/vision/cloudvision"
	mockvision "object-lens-search-demo/backend/internal/vision/mock"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}
	vision, llmProvider, err := buildLLM(context.Background(), cfg, logger)
	if err != nil {
		logger.Error("build llm", "error", err)
		os.Exit(1)
	}
	webSearcher, searchProvider, err := buildSearcher(cfg, logger)
	if err != nil {
		logger.Error("build searcher", "error", err)
		os.Exit(1)
	}
	visionEvidence, cloudVisionProvider, err := buildCloudVision(context.Background(), cfg, logger)
	if err != nil {
		logger.Error("build cloud vision", "error", err)
		os.Exit(1)
	}
	if visionEvidence != nil {
		defer visionEvidence.Close()
	}
	uc := &usecase.RecognizeSearchUsecase{LLM: vision, Searcher: webSearcher, Vision: visionEvidence, LLMProvider: llmProvider, SearchProvider: searchProvider, CloudVisionProvider: cloudVisionProvider, Logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", handler.Health)
	mux.Handle("/api/recognize-search", &handler.RecognizeSearchHandler{Usecase: uc, MaxRequestBytes: cfg.MaxRequestBytes, RequestTimeout: cfg.RequestTimeout})

	var app http.Handler = mux
	app = middleware.NewRateLimiter(cfg.RateLimitPerMinute).Middleware(app)
	app = middleware.LoggingMiddleware(logger)(app)
	app = middleware.CORSMiddleware(cfg.AllowedOrigins)(app)
	app = middleware.RequestIDMiddleware(app)

	server := &http.Server{Addr: ":" + cfg.Port, Handler: app, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		logger.Info("server starting", "port", cfg.Port, "llmProvider", llmProvider, "searchProvider", searchProvider, "cloudVisionProvider", cloudVisionProvider)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("server shutdown", "error", err)
	}
}

func buildLLM(ctx context.Context, cfg config.Config, logger *slog.Logger) (llm.VisionLLM, string, error) {
	if cfg.LLMProvider == "mock" {
		return &mockllm.Client{Model: "mock-vision-llm"}, "bedrock", nil
	}
	if strings.TrimSpace(cfg.AWSRegion) == "" || strings.TrimSpace(cfg.BedrockModelID) == "" {
		if cfg.AllowMockFallback {
			logger.Warn("bedrock config missing; using mock llm outside production")
			return &mockllm.Client{Model: "mock-vision-llm"}, "bedrock", nil
		}
		return nil, "", errors.New("AWS_REGION and BEDROCK_MODEL_ID are required")
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.AWSRegion))
	if err != nil {
		if cfg.AllowMockFallback {
			logger.Warn("aws config unavailable; using mock llm outside production", "error", err)
			return &mockllm.Client{Model: "mock-vision-llm"}, "bedrock", nil
		}
		return nil, "", err
	}
	return bedrockllm.New(bedrockruntime.NewFromConfig(awsCfg), cfg.BedrockModelID), "bedrock", nil
}

func buildSearcher(cfg config.Config, logger *slog.Logger) (search.WebSearcher, string, error) {
	if cfg.SearchProvider == "mock" {
		return &mocksearch.Client{}, "tavily", nil
	}
	if strings.TrimSpace(cfg.TavilyAPIKey) == "" {
		if cfg.AllowMockFallback {
			logger.Warn("tavily api key missing; using mock search outside production")
			return &mocksearch.Client{}, "tavily", nil
		}
		return nil, "", errors.New("TAVILY_API_KEY is required")
	}
	return &tavily.Client{APIKey: cfg.TavilyAPIKey, Endpoint: cfg.TavilyEndpoint}, "tavily", nil
}

func buildCloudVision(ctx context.Context, cfg config.Config, logger *slog.Logger) (vision.EvidenceExtractor, string, error) {
	if !cfg.CloudVisionEnabled {
		logger.Info("cloud vision disabled")
		return nil, "disabled", nil
	}
	if cfg.CloudVisionProvider == "mock" {
		return &mockvision.Client{}, "cloud-vision-mock", nil
	}
	client, err := cloudvision.New(ctx)
	if err != nil {
		if cfg.AllowMockFallback {
			logger.Warn("cloud vision unavailable; disabling evidence extraction outside production", "error", err)
			return nil, "disabled", nil
		}
		return nil, "", err
	}
	return client, "cloud-vision", nil
}
