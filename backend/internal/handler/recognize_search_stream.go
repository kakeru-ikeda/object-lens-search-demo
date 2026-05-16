package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"object-lens-search-demo/backend/internal/middleware"
	"object-lens-search-demo/backend/internal/model"
	"object-lens-search-demo/backend/internal/usecase"
)

type RecognizeSearchStreamHandler struct {
	Usecase              *usecase.RecognizeSearchUsecase
	MaxRequestBytes      int64
	MaxImageBytes        int64
	MaxTotalImageBytes   int64
	StreamRequestTimeout time.Duration
}

func (h *RecognizeSearchStreamHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.RequestID(r.Context())
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, model.ErrInvalidRequest, "method not allowed", requestID)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming_not_supported", "streaming is not supported by this server", requestID)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.StreamRequestTimeout)
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
	req.Options.Stream = true

	normalized, mimeType, cropMIMETypes, errCode, errMsg := validateRequestWithLimits(req, validationLimits{MaxImageBytes: effectiveMaxImageBytes(h.MaxImageBytes, h.MaxRequestBytes), MaxTotalImageBytes: effectiveMaxTotalImageBytes(h.MaxTotalImageBytes, h.MaxRequestBytes)})
	if errCode != "" {
		status := http.StatusBadRequest
		if errCode == model.ErrImageTooLarge {
			status = http.StatusRequestEntityTooLarge
		}
		writeError(w, status, errCode, errMsg, requestID)
		return
	}

	setSSEHeaders(w)
	flusher.Flush()

	sink := newSSEEventSink(w, flusher)
	heartbeatCtx, stopHeartbeat := context.WithCancel(ctx)
	heartbeatDone := make(chan struct{})
	go sink.heartbeat(heartbeatCtx, heartbeatDone)
	defer func() {
		stopHeartbeat()
		<-heartbeatDone
	}()

	_, err := h.Usecase.ExecuteWithEvents(ctx, usecase.ExecuteRequest{RequestID: requestID, Request: normalized, MIMEType: mimeType, CropMIMETypes: cropMIMETypes}, sink)
	if err != nil && !errors.Is(err, context.Canceled) {
		_, code := mapUsecaseError(err)
		_ = sink.Emit(ctx, usecase.StreamEvent{RequestID: requestID, Stage: "error", Status: "error", Message: publicUsecaseErrorMessage(code), Payload: map[string]interface{}{"code": code}})
	}
}

func setSSEHeaders(w http.ResponseWriter) {
	header := w.Header()
	header.Set("Content-Type", "text/event-stream; charset=utf-8")
	header.Set("Cache-Control", "no-cache, no-transform")
	header.Set("X-Accel-Buffering", "no")
}

type sseEventSink struct {
	w       http.ResponseWriter
	flusher http.Flusher
	mu      sync.Mutex
}

func newSSEEventSink(w http.ResponseWriter, flusher http.Flusher) *sseEventSink {
	return &sseEventSink{w: w, flusher: flusher}
}

func (s *sseEventSink) Emit(ctx context.Context, event usecase.StreamEvent) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := fmt.Fprintf(s.w, "event: %s\n", event.Stage); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(s.w, "data: %s\n\n", payload); err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}

func (s *sseEventSink) heartbeat(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			_, _ = io.WriteString(s.w, ": heartbeat\n\n")
			s.flusher.Flush()
			s.mu.Unlock()
		}
	}
}
