package service

import (
	"testing"
	"time"

	"github.com/adshao/go-binance/v2/futures"
)

func TestNewBinanceReadonlyServiceWithOptions_UsesDefaultBaseURLWhenEmpty(t *testing.T) {
	svc := NewBinanceReadonlyServiceWithOptions("", "", "", 15*time.Second, 1, 250*time.Millisecond)
	if svc.client.BaseURL != futures.BaseApiMainUrl {
		t.Fatalf("expected default base URL %q, got %q", futures.BaseApiMainUrl, svc.client.BaseURL)
	}
}

func TestNewBinanceReadonlyServiceWithOptions_AppliesCustomBaseURL(t *testing.T) {
	svc := NewBinanceReadonlyServiceWithOptions("", "", "https://example-proxy.binance.local/", 15*time.Second, 1, 250*time.Millisecond)
	if svc.client.BaseURL != "https://example-proxy.binance.local" {
		t.Fatalf("expected trimmed custom base URL, got %q", svc.client.BaseURL)
	}
}
