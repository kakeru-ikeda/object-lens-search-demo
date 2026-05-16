package handler

import (
	"encoding/base64"
	"strings"
	"testing"

	"object-lens-search-demo/backend/internal/model"
)

func dataURL(mimeType string, payload string) string {
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString([]byte(payload))
}

func TestValidateRequestDefaults(t *testing.T) {
	req, mimeType, cropMIMETypes, code, msg := validateRequest(model.RecognizeSearchRequest{ImageBase64: dataURL("image/jpeg", "image")}, 1024)
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
	if cropMIMETypes != nil {
		t.Fatalf("expected nil crop mime types, got %#v", cropMIMETypes)
	}
}

func TestValidateRequestAcceptsCrops(t *testing.T) {
	req, mimeType, cropMIMETypes, code, msg := validateRequest(model.RecognizeSearchRequest{Crops: &model.ImageCrops{TightCrop: dataURL("image/jpeg", "tight"), ContextCrop: dataURL("image/png", "context")}}, 1024)
	if code != "" || msg != "" {
		t.Fatalf("unexpected validation error: %s %s", code, msg)
	}
	if mimeType != "image/jpeg" {
		t.Fatalf("expected tight crop mime, got %q", mimeType)
	}
	if req.ImageBase64 == "" {
		t.Fatal("expected tight crop copied to imageBase64 for backward-compatible providers")
	}
	if cropMIMETypes["tightCrop"] != "image/jpeg" || cropMIMETypes["contextCrop"] != "image/png" {
		t.Fatalf("unexpected crop mime types: %#v", cropMIMETypes)
	}
}

func TestValidateRequestRejectsImageAndCropsTogether(t *testing.T) {
	_, _, _, code, _ := validateRequest(model.RecognizeSearchRequest{ImageBase64: dataURL("image/jpeg", "image"), Crops: &model.ImageCrops{TightCrop: dataURL("image/jpeg", "tight"), ContextCrop: dataURL("image/jpeg", "context")}}, 1024)
	if code != model.ErrInvalidRequest {
		t.Fatalf("expected invalid request, got %q", code)
	}
}

func TestValidateRequestRejectsMissingCrop(t *testing.T) {
	_, _, _, code, _ := validateRequest(model.RecognizeSearchRequest{Crops: &model.ImageCrops{TightCrop: dataURL("image/jpeg", "tight")}}, 1024)
	if code != model.ErrInvalidRequest {
		t.Fatalf("expected invalid request, got %q", code)
	}
}

func TestValidateRequestRejectsInvalidCropBase64(t *testing.T) {
	_, _, _, code, _ := validateRequest(model.RecognizeSearchRequest{Crops: &model.ImageCrops{TightCrop: "data:image/jpeg;base64,%%%", ContextCrop: dataURL("image/jpeg", "context")}}, 1024)
	if code != model.ErrInvalidRequest {
		t.Fatalf("expected invalid request, got %q", code)
	}
}

func TestValidateRequestAcceptsTextEnhancedCrop(t *testing.T) {
	_, _, cropMIMETypes, code, msg := validateRequest(model.RecognizeSearchRequest{Crops: &model.ImageCrops{TightCrop: dataURL("image/jpeg", "tight"), ContextCrop: dataURL("image/png", "context"), TextEnhancedCrop: dataURL("image/webp", "text")}}, 1024)
	if code != "" || msg != "" {
		t.Fatalf("unexpected validation error: %s %s", code, msg)
	}
	if cropMIMETypes["textEnhancedCrop"] != "image/webp" {
		t.Fatalf("expected textEnhancedCrop webp mime, got %#v", cropMIMETypes)
	}
}

func TestValidateRequestRejectsUnsupportedMime(t *testing.T) {
	_, _, _, code, _ := validateRequest(model.RecognizeSearchRequest{ImageBase64: dataURL("image/gif", "image")}, 1024)
	if code != model.ErrUnsupportedImageType {
		t.Fatalf("expected unsupported image type, got %q", code)
	}
}

func TestValidateRequestRejectsLargeImage(t *testing.T) {
	_, _, _, code, _ := validateRequest(model.RecognizeSearchRequest{ImageBase64: dataURL("image/png", strings.Repeat("x", 20))}, 10)
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
