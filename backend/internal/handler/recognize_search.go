package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"object-lens-search-demo/backend/internal/middleware"
	"object-lens-search-demo/backend/internal/model"
	"object-lens-search-demo/backend/internal/usecase"
)

type RecognizeSearchHandler struct {
	Usecase         *usecase.RecognizeSearchUsecase
	MaxRequestBytes int64
	RequestTimeout  time.Duration
}

func (h *RecognizeSearchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.RequestID(r.Context())
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, model.ErrInvalidRequest, "method not allowed", requestID)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.RequestTimeout)
	defer cancel()

	r.Body = http.MaxBytesReader(w, r.Body, h.MaxRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var req model.RecognizeSearchRequest
	if err := decoder.Decode(&req); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) || strings.Contains(err.Error(), "http: request body too large") {
			writeError(w, http.StatusRequestEntityTooLarge, model.ErrImageTooLarge, "request body too large", requestID)
			return
		}
		writeError(w, http.StatusBadRequest, model.ErrInvalidRequest, "invalid JSON request", requestID)
		return
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		writeError(w, http.StatusBadRequest, model.ErrInvalidRequest, "request body must contain a single JSON object", requestID)
		return
	}

	normalized, mimeType, cropMIMETypes, errCode, errMsg := validateRequest(req, h.MaxRequestBytes)
	if errCode != "" {
		status := http.StatusBadRequest
		if errCode == model.ErrImageTooLarge {
			status = http.StatusRequestEntityTooLarge
		}
		writeError(w, status, errCode, errMsg, requestID)
		return
	}

	resp, err := h.Usecase.Execute(ctx, usecase.ExecuteRequest{RequestID: requestID, Request: normalized, MIMEType: mimeType, CropMIMETypes: cropMIMETypes})
	if err != nil {
		status, code := mapUsecaseError(err)
		writeError(w, status, code, publicUsecaseErrorMessage(code), requestID)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func validateRequest(req model.RecognizeSearchRequest, maxBytes int64) (model.RecognizeSearchRequest, string, map[string]string, string, string) {
	req.Language = strings.TrimSpace(req.Language)
	if req.Language == "" {
		req.Language = "ja"
	}
	if req.Language != "ja" && req.Language != "en" {
		return req, "", nil, model.ErrInvalidRequest, "language must be ja or en"
	}
	if req.Options.MaxSearchResults == 0 {
		req.Options.MaxSearchResults = 5
	}
	if req.Options.MaxSearchResults < 1 || req.Options.MaxSearchResults > 5 {
		return req, "", nil, model.ErrInvalidRequest, "options.maxSearchResults must be between 1 and 5"
	}
	hasImage := strings.TrimSpace(req.ImageBase64) != ""
	hasCrops := req.Crops != nil
	if hasImage == hasCrops {
		return req, "", nil, model.ErrInvalidRequest, "provide exactly one of imageBase64 or crops"
	}
	if hasCrops {
		cropMIMETypes, code, msg := validateCrops(req.Crops, maxBytes)
		if code != "" {
			return req, "", nil, code, msg
		}
		if req.ImageBase64 == "" {
			req.ImageBase64 = req.Crops.TightCrop
		}
		return req, cropMIMETypes["tightCrop"], cropMIMETypes, "", ""
	}
	mimeType, code, msg := validateImageDataURL(req.ImageBase64, "imageBase64", maxBytes)
	if code != "" {
		return req, "", nil, code, msg
	}
	return req, mimeType, nil, "", ""
}

func validateCrops(crops *model.ImageCrops, maxBytes int64) (map[string]string, string, string) {
	if crops == nil {
		return nil, model.ErrInvalidRequest, "crops is required"
	}
	crops.TightCrop = strings.TrimSpace(crops.TightCrop)
	crops.ContextCrop = strings.TrimSpace(crops.ContextCrop)
	crops.TextEnhancedCrop = strings.TrimSpace(crops.TextEnhancedCrop)
	if crops.TightCrop == "" || crops.ContextCrop == "" {
		return nil, model.ErrInvalidRequest, "crops.tightCrop and crops.contextCrop are required"
	}
	cropMIMETypes := make(map[string]string, 3)
	for name, value := range map[string]string{
		"tightCrop":   crops.TightCrop,
		"contextCrop": crops.ContextCrop,
	} {
		mimeType, code, msg := validateImageDataURL(value, "crops."+name, maxBytes)
		if code != "" {
			return nil, code, msg
		}
		cropMIMETypes[name] = mimeType
	}
	if crops.TextEnhancedCrop != "" {
		mimeType, code, msg := validateImageDataURL(crops.TextEnhancedCrop, "crops.textEnhancedCrop", maxBytes)
		if code != "" {
			return nil, code, msg
		}
		cropMIMETypes["textEnhancedCrop"] = mimeType
	}
	return cropMIMETypes, "", ""
}

func validateImageDataURL(imageDataURL string, fieldName string, maxBytes int64) (string, string, string) {
	mimeType, payload, ok := strings.Cut(imageDataURL, ";base64,")
	if !ok || !strings.HasPrefix(mimeType, "data:") {
		return "", model.ErrUnsupportedImageType, fieldName + " must be a jpeg, png, or webp data URL"
	}
	mimeType = strings.TrimPrefix(mimeType, "data:")
	if mimeType != "image/jpeg" && mimeType != "image/png" && mimeType != "image/webp" {
		return "", model.ErrUnsupportedImageType, "unsupported image MIME type"
	}
	decodedLen := base64.StdEncoding.DecodedLen(len(payload))
	if int64(decodedLen) > maxBytes {
		return "", model.ErrImageTooLarge, fmt.Sprintf("decoded image exceeds %d bytes", maxBytes)
	}
	if _, err := base64.StdEncoding.DecodeString(payload); err != nil {
		return "", model.ErrInvalidRequest, fieldName + " contains invalid base64"
	}
	return mimeType, "", ""
}

func mapUsecaseError(err error) (int, string) {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, usecase.ErrTimeout) {
		return http.StatusGatewayTimeout, model.ErrTimeout
	}
	if errors.Is(err, usecase.ErrLLM) {
		return http.StatusBadGateway, model.ErrLLM
	}
	if errors.Is(err, usecase.ErrSearch) {
		return http.StatusBadGateway, model.ErrSearch
	}
	return http.StatusInternalServerError, model.ErrInternal
}

func publicUsecaseErrorMessage(code string) string {
	switch code {
	case model.ErrLLM:
		return "failed to recognize image"
	case model.ErrSearch:
		return "failed to search web results"
	case model.ErrTimeout:
		return "request timed out"
	default:
		return "internal server error"
	}
}
