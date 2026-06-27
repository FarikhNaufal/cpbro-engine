package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/entity"
	"cpbro-engine/internal/modules/cryptobroV3/service"
	"cpbro-engine/internal/modules/cryptobroV3/usecase"
	"github.com/gin-gonic/gin"
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

func TestBuildHealthLatestSnapshot_MapsLatestResultFields(t *testing.T) {
	now := time.Date(2026, 6, 27, 10, 15, 0, 0, time.UTC)
	snapshot := buildHealthLatestSnapshot(&entity.LatestResult{
		GeneratedAt:                     now,
		ScanID:                          "scheduled_20260627101500",
		MarketRegime:                    "HIGH_VOL",
		MarketPolicy:                    "HIGH_VOL active - strict risk reduction mode",
		MacroVolatility:                 "HIGH",
		MarketBreadth:                   0.81,
		ActivePolicyLongMode:            "NORMAL",
		ActivePolicyShortMode:           "NORMAL",
		ActivePolicyRequireAIConfidence: "HIGH",
		ActivePolicyRequireFreshEntry:   true,
		ActivePolicyAllowedPlaybooks:    []string{"LIQUIDITY_SWEEP_REVERSAL", "TREND_PULLBACK"},
	})

	if snapshot == nil {
		t.Fatal("expected latest snapshot")
	}
	if snapshot.MarketRegime != "HIGH_VOL" {
		t.Fatalf("expected market regime HIGH_VOL, got %s", snapshot.MarketRegime)
	}
	if snapshot.GeneratedAt != now.Format(time.RFC3339) {
		t.Fatalf("expected generated_at %s, got %s", now.Format(time.RFC3339), snapshot.GeneratedAt)
	}
	if len(snapshot.ActivePolicyAllowedPlaybooks) != 2 {
		t.Fatalf("expected allowed playbooks to be copied, got %+v", snapshot.ActivePolicyAllowedPlaybooks)
	}
}

func TestHealthEndpoint_IncludesLatestSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	st, err := service.NewJSONStorageService(dir)
	if err != nil {
		t.Fatalf("NewJSONStorageService: %v", err)
	}
	if err := st.SaveLatestResult(&entity.LatestResult{
		GeneratedAt:                     time.Date(2026, 6, 27, 10, 15, 0, 0, time.UTC),
		ScanID:                          "scheduled_20260627101500",
		MarketRegime:                    "HIGH_VOL",
		MarketPolicy:                    "HIGH_VOL active - strict risk reduction mode",
		MacroVolatility:                 "HIGH",
		MarketBreadth:                   0.81,
		ActivePolicyRequireAIConfidence: "HIGH",
		ActivePolicyRequireFreshEntry:   true,
		ActivePolicyAllowedPlaybooks:    []string{"LIQUIDITY_SWEEP_REVERSAL", "TREND_PULLBACK"},
	}); err != nil {
		t.Fatalf("SaveLatestResult: %v", err)
	}

	h := &Handler{
		storageUC:       usecase.NewStorageUsecase(st),
		observabilityUC: &usecase.ObservabilityUsecase{},
		startTime:       time.Now().Add(-5 * time.Second),
	}
	orig := healthAuditFn
	healthAuditFn = func(uc *usecase.ObservabilityUsecase, ctx context.Context) (usecase.HealthStatus, error) {
		return usecase.HealthStatus{
			Status:              "UP",
			Mode:                "alert-only",
			BinanceConnectivity: "OK",
			GeminiAvailability:  "OK",
			StorageWritable:     "OK",
		}, nil
	}
	defer func() { healthAuditFn = orig }()

	r := gin.New()
	r.GET("/health", h.GetHealth)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp APIResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	dataBytes, _ := json.Marshal(resp.Data)
	var health map[string]any
	_ = json.Unmarshal(dataBytes, &health)
	latestSnapshot, ok := health["latest_snapshot"].(map[string]any)
	if !ok {
		t.Fatalf("expected latest_snapshot object, got %T", health["latest_snapshot"])
	}
	if latestSnapshot["market_regime"] != "HIGH_VOL" {
		t.Fatalf("expected market_regime HIGH_VOL, got %v", latestSnapshot["market_regime"])
	}
}
