package middleware

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientIPNormalizesRemoteAddrPort(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "203.0.113.10:54321"
	if got := ClientIP(req); got != "203.0.113.10" {
		t.Fatalf("expected host without port, got %q", got)
	}
}

func TestClientIPNormalizesForwardedFor(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", " 2001:db8::1 , 10.0.0.1")
	if got := ClientIP(req); got != "2001:db8::1" {
		t.Fatalf("expected first forwarded IP, got %q", got)
	}
}

func TestRateLimiterUsesNormalizedIPAndPrunesExpiredBuckets(t *testing.T) {
	start := time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)
	current := start
	limiter := NewRateLimiter(1)
	limiter.now = func() time.Time { return current }

	if !limiter.allow("203.0.113.10") {
		t.Fatal("first request should pass")
	}
	if limiter.allow("203.0.113.10") {
		t.Fatal("second request in same window should be limited")
	}

	current = start.Add(3 * time.Minute)
	if !limiter.allow("198.51.100.20") {
		t.Fatal("new IP should pass")
	}
	if _, ok := limiter.buckets["203.0.113.10"]; ok {
		t.Fatal("expired bucket should be pruned")
	}
}
