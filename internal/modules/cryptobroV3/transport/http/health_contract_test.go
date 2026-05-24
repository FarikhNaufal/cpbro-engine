package http

import (
	"encoding/json"
	"testing"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/usecase"
)

func TestHealthResponse_DegradedWhenStorageUnavailable(t *testing.T) {
	h := &Handler{
		startTime: time.Now().Add(-10 * time.Second),
	}

	// Call mapper directly with a degraded HealthStatus snapshot.
	resp := h.mapHealthResponse(usecase.HealthStatus{
		Status:              "UP",
		Mode:                "alert-only",
		BinanceConnectivity: "OK",
		GeminiAvailability:  "OK",
		StorageWritable:     "ERROR: no permission",
	})

	if resp.Status != "degraded" {
		t.Fatalf("expected status=degraded, got %s", resp.Status)
	}
	if resp.StorageAvailable {
		t.Fatalf("expected storage_available=false")
	}
	if resp.Warnings == nil || len(resp.Warnings) == 0 {
		t.Fatalf("expected warnings to be non-empty")
	}
}

func TestHealthEndpoint_ResponseShape(t *testing.T) {
	h := &Handler{
		startTime: time.Now().Add(-5 * time.Second),
	}

	// Build APIResponse manually (shape test).
	apiResp := ok("ok", h.mapHealthResponse(usecase.HealthStatus{
		Status:              "UP",
		Mode:                "alert-only",
		BinanceConnectivity: "OK",
		GeminiAvailability:  "OK",
		StorageWritable:     "OK",
	}))

	b, _ := json.Marshal(apiResp)
	var decoded map[string]any
	_ = json.Unmarshal(b, &decoded)
	if decoded["success"] != true {
		t.Fatalf("expected success=true")
	}
	if decoded["data"] == nil {
		t.Fatalf("expected data object")
	}
}
