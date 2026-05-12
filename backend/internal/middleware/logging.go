package middleware

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

func LoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			recorder := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(recorder, r)
			if recorder.status == 0 {
				recorder.status = http.StatusOK
			}
			entry := map[string]any{
				"requestId": RequestID(r.Context()),
				"method":    r.Method,
				"path":      r.URL.Path,
				"status":    recorder.status,
				"elapsedMs": time.Since(start).Milliseconds(),
				"bytes":     recorder.bytes,
				"remoteIp":  ClientIP(r),
			}
			payload, err := json.Marshal(entry)
			if err != nil {
				logger.Error("failed to marshal access log", "error", err)
				return
			}
			logger.Info(string(payload))
		})
	}
}

func ClientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		first := strings.TrimSpace(strings.Split(forwarded, ",")[0])
		if parsed := normalizeHost(first); parsed != "" {
			return parsed
		}
	}
	if parsed := normalizeHost(r.RemoteAddr); parsed != "" {
		return parsed
	}
	return "unknown"
}

func normalizeHost(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(trimmed); err == nil {
		return strings.Trim(host, "[]")
	}
	return strings.Trim(trimmed, "[]")
}
