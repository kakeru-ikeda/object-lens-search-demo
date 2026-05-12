package middleware

import (
	"net/http"
	"sync"
	"time"

	"object-lens-search-demo/backend/internal/model"
)

type RateLimiter struct {
	limit       int
	mu          sync.Mutex
	buckets     map[string]*bucket
	now         func() time.Time
	lastCleanup time.Time
}

type bucket struct {
	windowStart time.Time
	count       int
}

func NewRateLimiter(limit int) *RateLimiter {
	return &RateLimiter{limit: limit, buckets: make(map[string]*bucket), now: time.Now}
}

func (r *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !r.allow(ClientIP(req)) {
			WriteJSONError(w, http.StatusTooManyRequests, model.ErrRateLimited, "rate limit exceeded", RequestID(req.Context()))
			return
		}
		next.ServeHTTP(w, req)
	})
}

func (r *RateLimiter) allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	r.cleanupExpired(now)
	b, ok := r.buckets[key]
	if !ok || now.Sub(b.windowStart) >= time.Minute {
		r.buckets[key] = &bucket{windowStart: now, count: 1}
		return true
	}
	if b.count >= r.limit {
		return false
	}
	b.count++
	return true
}

func (r *RateLimiter) cleanupExpired(now time.Time) {
	if !r.lastCleanup.IsZero() && now.Sub(r.lastCleanup) < time.Minute {
		return
	}
	for key, b := range r.buckets {
		if now.Sub(b.windowStart) >= 2*time.Minute {
			delete(r.buckets, key)
		}
	}
	r.lastCleanup = now
}

func WriteJSONError(w http.ResponseWriter, status int, code, message, requestID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = jsonEncoder(w).Encode(model.ErrorResponse{Error: model.APIError{Code: code, Message: message, RequestID: requestID}})
}

type encodeWriter interface {
	Encode(v any) error
}

func jsonEncoder(w http.ResponseWriter) encodeWriter {
	return encoder{w: w}
}

type encoder struct{ w http.ResponseWriter }

func (e encoder) Encode(v any) error {
	return jsonNewEncoder(e.w).Encode(v)
}
