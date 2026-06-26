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

func TestHealthResponse_IncludesWebsocketStatus(t *testing.T) {
	h := &Handler{
		startTime: time.Now().Add(-5 * time.Second),
	}

	resp := h.mapHealthResponse(usecase.HealthStatus{
		Status:              "UP",
		Mode:                "alert-only",
		BinanceConnectivity: "OK",
		GeminiAvailability:  "OK",
		StorageWritable:     "OK",
		RealtimePrice: usecase.RealtimePriceStatus{
			Enabled:         true,
			Connected:       true,
			ActiveSymbols:   3,
			LastMessageTime: time.Date(2026, 6, 3, 14, 40, 0, 0, time.UTC),
		},
	})

	if !resp.WebsocketEnabled || !resp.WebsocketConnected {
		t.Fatalf("expected websocket flags to be true: %+v", resp)
	}
	if resp.WebsocketActiveSymbols != 3 {
		t.Fatalf("expected active symbols=3, got %d", resp.WebsocketActiveSymbols)
	}
	if resp.WebsocketLastMessageTime == "" {
		t.Fatalf("expected websocket last message time to be populated")
	}
}

func TestHealthResponse_IncludesRolloutReadiness(t *testing.T) {
	h := &Handler{
		startTime: time.Now().Add(-5 * time.Second),
	}

	resp := h.mapHealthResponse(usecase.HealthStatus{
		Status:              "UP",
		Mode:                "alert-only",
		BinanceConnectivity: "OK",
		GeminiAvailability:  "OK",
		StorageWritable:     "OK",
		RolloutReadiness: usecase.RolloutReadinessStatus{
			Ready:            true,
			RecommendedPhase: "PHASE_1_EXPAND",
			Blockers:         nil,
			RollbackCriteria: []string{"rollback if health degrades"},
		},
	})

	if !resp.RolloutReadiness.Ready {
		t.Fatal("expected rollout readiness ready=true")
	}
	if resp.RolloutReadiness.RecommendedPhase != "PHASE_1_EXPAND" {
		t.Fatalf("unexpected rollout phase %s", resp.RolloutReadiness.RecommendedPhase)
	}
	if len(resp.RolloutReadiness.RollbackCriteria) != 1 {
		t.Fatalf("expected rollback criteria to be mapped, got %+v", resp.RolloutReadiness)
	}
}
