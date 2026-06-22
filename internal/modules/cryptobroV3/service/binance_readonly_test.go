package service

import (
	"math"
	"testing"
	"time"

	"github.com/adshao/go-binance/v2/futures"
)

func TestBuildTicker24h_SkipsMalformedNonUSDTQuoteVolume(t *testing.T) {
	ticker, ok := buildTicker24h(&futures.PriceChangeStats{
		Symbol:             "ETHUSD_PERP",
		PriceChangePercent: "1.25",
		LastPrice:          "3500",
		Volume:             "1234",
		QuoteVolume:        "",
	})
	if ok {
		t.Fatalf("expected malformed non-USDT ticker to be skipped, got %+v", ticker)
	}
}

func TestBuildTicker24h_FallsBackQuoteVolumeForUSDT(t *testing.T) {
	ticker, ok := buildTicker24h(&futures.PriceChangeStats{
		Symbol:             "ETHUSDT",
		PriceChangePercent: "1.25",
		LastPrice:          "3500",
		Volume:             "2",
		QuoteVolume:        "",
	})
	if !ok {
		t.Fatal("expected USDT ticker with blank quote volume to use fallback")
	}
	if math.Abs(ticker.QuoteVolume-7000) > 1e-9 {
		t.Fatalf("expected fallback quote volume 7000, got %f", ticker.QuoteVolume)
	}
}

func TestBuildTicker24h_ParsesValidTicker(t *testing.T) {
	ticker, ok := buildTicker24h(&futures.PriceChangeStats{
		Symbol:             "SOLUSDT",
		PriceChangePercent: "-3.5",
		LastPrice:          "150.5",
		Volume:             "100",
		QuoteVolume:        "15050",
	})
	if !ok {
		t.Fatal("expected valid ticker to parse")
	}
	if ticker.Symbol != "SOLUSDT" || ticker.LastPrice != 150.5 || ticker.QuoteVolume != 15050 {
		t.Fatalf("unexpected parsed ticker: %+v", ticker)
	}
}

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
