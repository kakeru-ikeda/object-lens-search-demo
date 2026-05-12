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

	normalized, mimeType, errCode, errMsg := validateRequest(req, h.MaxRequestBytes)
	if errCode != "" {
		status := http.StatusBadRequest
		if errCode == model.ErrImageTooLarge {
			status = http.StatusRequestEntityTooLarge
		}
		writeError(w, status, errCode, errMsg, requestID)
		return
	}

	resp, err := h.Usecase.Execute(ctx, usecase.ExecuteRequest{RequestID: requestID, Request: normalized, MIMEType: mimeType})
	if err != nil {
		status, code := mapUsecaseError(err)
		writeError(w, status, code, publicUsecaseErrorMessage(code), requestID)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func validateRequest(req model.RecognizeSearchRequest, maxBytes int64) (model.RecognizeSearchRequest, string, string, string) {
	req.Language = strings.TrimSpace(req.Language)
	if req.Language == "" {
		req.Language = "ja"
	}
	if req.Language != "ja" && req.Language != "en" {
		return req, "", model.ErrInvalidRequest, "language must be ja or en"
	}
	if req.Options.MaxSearchResults == 0 {
		req.Options.MaxSearchResults = 5
	}
	if req.Options.MaxSearchResults < 1 || req.Options.MaxSearchResults > 5 {
		return req, "", model.ErrInvalidRequest, "options.maxSearchResults must be between 1 and 5"
	}
	if strings.TrimSpace(req.ImageBase64) == "" {
		return req, "", model.ErrInvalidRequest, "imageBase64 is required"
	}
	mimeType, payload, ok := strings.Cut(req.ImageBase64, ";base64,")
	if !ok || !strings.HasPrefix(mimeType, "data:") {
		return req, "", model.ErrUnsupportedImageType, "imageBase64 must be a jpeg, png, or webp data URL"
	}
	mimeType = strings.TrimPrefix(mimeType, "data:")
	if mimeType != "image/jpeg" && mimeType != "image/png" && mimeType != "image/webp" {
		return req, "", model.ErrUnsupportedImageType, "unsupported image MIME type"
	}
	decodedLen := base64.StdEncoding.DecodedLen(len(payload))
	if int64(decodedLen) > maxBytes {
		return req, "", model.ErrImageTooLarge, fmt.Sprintf("decoded image exceeds %d bytes", maxBytes)
	}
	if _, err := base64.StdEncoding.DecodeString(payload); err != nil {
		return req, "", model.ErrInvalidRequest, "imageBase64 contains invalid base64"
	}
	return req, mimeType, "", ""
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
