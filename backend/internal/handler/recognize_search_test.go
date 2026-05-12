package handler

import (
	"encoding/base64"
	"strings"
	"testing"

	"object-lens-search-demo/backend/internal/model"
)

func TestValidateRequestDefaults(t *testing.T) {
	img := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString([]byte("image"))
	req, mimeType, code, msg := validateRequest(model.RecognizeSearchRequest{ImageBase64: img}, 1024)
	if code != "" || msg != "" {
		t.Fatalf("unexpected validation error: %s %s", code, msg)
	}
	if req.Language != "ja" {
		t.Fatalf("expected default ja, got %q", req.Language)
	}
	if req.Options.MaxSearchResults != 5 {
		t.Fatalf("expected default max results 5, got %d", req.Options.MaxSearchResults)
	}
	if mimeType != "image/jpeg" {
		t.Fatalf("expected jpeg mime, got %q", mimeType)
	}
}

func TestValidateRequestRejectsUnsupportedMime(t *testing.T) {
	img := "data:image/gif;base64," + base64.StdEncoding.EncodeToString([]byte("image"))
	_, _, code, _ := validateRequest(model.RecognizeSearchRequest{ImageBase64: img}, 1024)
	if code != model.ErrUnsupportedImageType {
		t.Fatalf("expected unsupported image type, got %q", code)
	}
}

func TestValidateRequestRejectsLargeImage(t *testing.T) {
	img := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 20)))
	_, _, code, _ := validateRequest(model.RecognizeSearchRequest{ImageBase64: img}, 10)
	if code != model.ErrImageTooLarge {
		t.Fatalf("expected image too large, got %q", code)
	}
}


func TestPublicUsecaseErrorMessageDoesNotExposeDetails(t *testing.T) {
	if got := publicUsecaseErrorMessage(model.ErrLLM); got != "failed to recognize image" {
		t.Fatalf("unexpected llm public message: %q", got)
	}
	if got := publicUsecaseErrorMessage(model.ErrSearch); got != "failed to search web results" {
		t.Fatalf("unexpected search public message: %q", got)
	}
	if got := publicUsecaseErrorMessage(model.ErrInternal); got != "internal server error" {
		t.Fatalf("unexpected internal public message: %q", got)
	}
}
