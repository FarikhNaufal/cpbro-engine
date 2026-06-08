package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type hotRankRoundTripper struct {
	calls int
}

func (rt *hotRankRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.calls++

	var (
		body string
		resp any
	)

	switch {
	case strings.Contains(req.URL.Path, "/unified/rank/list/ai"):
		payload := map[string]any{}
		_ = json.NewDecoder(req.Body).Decode(&payload)
		rankType, _ := payload["rankType"].(float64)
		chainID, _ := payload["chainId"].(string)

		tokens := []map[string]any{}
		switch {
		case int(rankType) == 10 && chainID == "1":
			tokens = []map[string]any{{"symbol": "PEPE"}, {"symbol": "DOGE"}}
		case int(rankType) == 11 && chainID == "1":
			tokens = []map[string]any{{"symbol": "PEPE"}, {"symbol": "BONK"}}
		case int(rankType) == 10 && chainID == "56":
			tokens = []map[string]any{{"symbol": "WIF"}}
		case int(rankType) == 11 && chainID == "56":
			tokens = []map[string]any{{"symbol": "DOGE"}}
		}
		resp = map[string]any{
			"code":    "000000",
			"success": true,
			"data": map[string]any{
				"tokens": tokens,
			},
		}

	case strings.Contains(req.URL.Path, "/social/hype/rank/leaderboard"):
		resp = map[string]any{
			"code":    "000000",
			"success": true,
			"data": []map[string]any{
				{
					"metaInfo":       map[string]any{"symbol": "PEPE"},
					"socialHypeInfo": map[string]any{"socialHype": 91.0},
				},
			},
		}

	case strings.Contains(req.URL.Path, "/token/inflow/rank/query/ai"):
		resp = map[string]any{
			"code":    "000000",
			"success": true,
			"data": []map[string]any{
				{"tokenName": "BONK", "inflow": 12345.0},
			},
		}

	default:
		resp = map[string]any{"code": "000000", "success": true, "data": []any{}}
	}

	raw, err := json.Marshal(resp)
	if err != nil {
		return nil, err
	}
	body = string(raw)

	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewBufferString(body)),
		Request:    req,
	}, nil
}

func TestBinanceHotRankService_FetchHotSymbols_MergesAndCaches(t *testing.T) {
	transport := &hotRankRoundTripper{}
	service := &BinanceHotRankService{
		client: &http.Client{
			Timeout:   2 * time.Second,
			Transport: transport,
		},
		cacheTTL: time.Hour,
	}

	first, err := service.FetchHotSymbols(context.Background())
	if err != nil {
		t.Fatalf("FetchHotSymbols returned error: %v", err)
	}
	if len(first) == 0 {
		t.Fatal("expected hot symbols, got none")
	}

	var (
		foundPEPE bool
		foundBONK bool
	)
	for _, item := range first {
		switch item.Symbol {
		case "PEPE":
			foundPEPE = true
			if !strings.Contains(item.Source, "Trending") || !strings.Contains(item.Source, "Top Search") || !strings.Contains(item.Source, "Social Hype") {
				t.Fatalf("expected PEPE merged source, got %q", item.Source)
			}
		case "BONK":
			foundBONK = true
			if !strings.Contains(item.Source, "Top Search") || !strings.Contains(item.Source, "Smart Money Inflow") {
				t.Fatalf("expected BONK merged source, got %q", item.Source)
			}
		}
	}
	if !foundPEPE || !foundBONK {
		t.Fatalf("expected merged symbols PEPE and BONK, got %+v", first)
	}

	firstCalls := transport.calls
	second, err := service.FetchHotSymbols(context.Background())
	if err != nil {
		t.Fatalf("second FetchHotSymbols returned error: %v", err)
	}
	if len(second) != len(first) {
		t.Fatalf("expected cached result length %d, got %d", len(first), len(second))
	}
	if transport.calls != firstCalls {
		t.Fatalf("expected second fetch to hit cache, calls before=%d after=%d", firstCalls, transport.calls)
	}
}
