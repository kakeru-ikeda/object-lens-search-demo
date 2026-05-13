package tavily

import (
	"testing"
	"time"
)

func TestDisplayURL(t *testing.T) {
	got := displayURL("https://example.com/path?q=1")
	if got != "example.com/path" {
		t.Fatalf("unexpected display URL: %q", got)
	}
}

func TestSourceHost(t *testing.T) {
	got := sourceHost("https://example.com/path")
	if got != "example.com" {
		t.Fatalf("unexpected source host: %q", got)
	}
}

func TestDefaultHTTPClientHasTimeout(t *testing.T) {
	if defaultHTTPClient.Timeout != 20*time.Second {
		t.Fatalf("expected default timeout 20s, got %s", defaultHTTPClient.Timeout)
	}
}
