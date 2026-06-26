package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

// Mock SRE dependencies for health checking
type mockObsMarketDataProvider struct {
	failPrice bool
}

func (m *mockObsMarketDataProvider) FetchClosedCandles(ctx context.Context, symbol string, interval string, limit int) ([]dto.Candle, error) {
	return []dto.Candle{}, nil
}

func (m *mockObsMarketDataProvider) FetchLatestPrice(ctx context.Context, symbol string) (float64, error) {
	if m.failPrice {
		return 0, errors.New("binance connection reset")
	}
	return 67000.0, nil
}

func (m *mockObsMarketDataProvider) FetchAllFuturesTickers24h(ctx context.Context) ([]dto.Ticker24h, error) {
	return []dto.Ticker24h{}, nil
}

func (m *mockObsMarketDataProvider) FetchPremiumFundingRates(ctx context.Context) (map[string]float64, error) {
	return nil, nil
}

func (m *mockObsMarketDataProvider) FetchOpenInterest(ctx context.Context, symbol string) (float64, error) {
	return 0, nil
}

func (m *mockObsMarketDataProvider) FetchHistoricalCandles(ctx context.Context, symbol string, interval string, startTime time.Time, endTime time.Time) ([]dto.Candle, error) {
	return []dto.Candle{}, nil
}

type mockObsAIAuditor struct {
	failPing bool
}

func (m *mockObsAIAuditor) AuditCandidate(ctx context.Context, req dto.AIAuditRequest) (*dto.AIAuditResponse, error) {
	return nil, nil
}

func (m *mockObsAIAuditor) Ping(ctx context.Context) error {
	if m.failPing {
		return errors.New("gemini quota exceeded")
	}
	return nil
}

type mockObsNotifier struct {
	failPing bool
}

type mockObsRealtimeStatus struct {
	status RealtimePriceStatus
}

func (m *mockObsNotifier) SendFinalExecuteAlert(ctx context.Context, signal dto.SignalResponse) error {
	return nil
}

func (m *mockObsNotifier) SendTelegramMessage(ctx context.Context, msg string) error {
	return nil
}

func (m *mockObsNotifier) Ping(ctx context.Context) error {
	if m.failPing {
		return errors.New("telegram status 401")
	}
	return nil
}

func (m *mockObsRealtimeStatus) RealtimeStatus() RealtimePriceStatus {
	return m.status
}

func TestObservability_MetricsRegistry(t *testing.T) {
	reg := GetGlobalMetrics()

	// Initial metrics checks
	reg.IncrementScanSuccess()
	reg.IncrementScanFail()
	reg.SetLastScanDuration(150 * time.Millisecond)
	reg.AddTotalTickers(100)
	reg.AddUniversePass(10)
	reg.AddUniverseReject(90)
	reg.IncrementMarketDataError()
	reg.AddAICandidateCount(5)
	reg.IncrementAITimeoutCount()
	reg.AddAILatency(100 * time.Millisecond)
	reg.AddAILatency(200 * time.Millisecond)
	reg.AddStalenessChecked(10)
	reg.AddStalenessCount(2)
	reg.IncrementStorageWriteFail()
	reg.SetEvalMetrics(3, 1)

	if reg.ScanSuccessCount != 1 {
		t.Errorf("Expected ScanSuccessCount=1, got %d", reg.ScanSuccessCount)
	}
	if reg.ScanFailCount != 1 {
		t.Errorf("Expected ScanFailCount=1, got %d", reg.ScanFailCount)
	}
	if reg.LastScanDurationMs != 150 {
		t.Errorf("Expected LastScanDurationMs=150, got %d", reg.LastScanDurationMs)
	}
	if reg.TotalTickers != 100 {
		t.Errorf("Expected TotalTickers=100, got %d", reg.TotalTickers)
	}

	avgLat := reg.GetAverageAILatencyMs()
	if avgLat != 150.0 {
		t.Errorf("Expected average AI latency 150ms, got %f", avgLat)
	}

	staleRate := reg.GetStalenessRate()
	if staleRate != 0.2 {
		t.Errorf("Expected staleness rate 0.2, got %f", staleRate)
	}

	if reg.StorageWriteFail != 1 {
		t.Errorf("Expected StorageWriteFail=1, got %d", reg.StorageWriteFail)
	}

	if reg.EvalRecCount != 3 || reg.GateBugCount != 1 {
		t.Errorf("Expected rec=3 bug=1, got rec=%d bug=%d", reg.EvalRecCount, reg.GateBugCount)
	}
}

func TestObservability_PerformHealthAudit(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "health_audit_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	provider := &mockObsMarketDataProvider{failPrice: false}
	aiService := &mockObsAIAuditor{failPing: false}
	notifier := &mockObsNotifier{failPing: false}

	uc := NewObservabilityUsecase(provider, aiService, notifier, tmpDir)
	uc.SetStorageFiles("signal_health_test.json", "watch_health_test.json", "custom_health_snapshot.json")
	uc.SetRealtimeStatusProvider(&mockObsRealtimeStatus{
		status: RealtimePriceStatus{
			Enabled:         true,
			Connected:       true,
			ActiveSymbols:   2,
			LastMessageTime: time.Now(),
		},
	})

	// Set running statuses
	ScanWorkerRunning.Store(true)
	defer ScanWorkerRunning.Store(false)

	original := getRuntimeSettings()
	defer SetRuntimeSettings(original)
	settings := original
	settings.HealthStorageCheck = true
	settings.WatchRecheckBoundaryMinutes = 5
	settings.WatchRecheckMaxAgeMinutes = 12
	settings.WatchRecheckAllowedPlaybooks = []string{"TREND_PULLBACK", "LIQUIDITY_SWEEP_REVERSAL"}
	SetRuntimeSettings(settings)

	ctx := context.Background()
	status, err := uc.PerformHealthAudit(ctx)
	if err != nil {
		t.Fatalf("PerformHealthAudit failed: %v", err)
	}

	if status.BinanceConnectivity != "OK" {
		t.Errorf("Expected Binance OK, got %s", status.BinanceConnectivity)
	}
	if status.GeminiAvailability != "OK" {
		t.Errorf("Expected Gemini OK, got %s", status.GeminiAvailability)
	}
	if status.TelegramAvailability != "OK" {
		t.Errorf("Expected Telegram OK, got %s", status.TelegramAvailability)
	}
	if status.StorageWritable != "OK" {
		t.Errorf("Expected Storage OK, got %s", status.StorageWritable)
	}
	if !status.ScanWorkerRunning {
		t.Error("Expected ScanWorkerRunning=true")
	}
	if !status.RealtimePrice.Enabled || !status.RealtimePrice.Connected || status.RealtimePrice.ActiveSymbols != 2 {
		t.Errorf("unexpected realtime status: %+v", status.RealtimePrice)
	}
	if status.RecheckCadence.BoundaryMinutes != 5 || status.RecheckCadence.MaxAgeMinutes != 12 {
		t.Errorf("unexpected recheck cadence: %+v", status.RecheckCadence)
	}
	if !status.RecheckCadence.BoundaryCloseAware || status.RecheckCadence.PrimaryGuardSeconds < 0 {
		t.Errorf("expected boundary-close-aware recheck cadence, got %+v", status.RecheckCadence)
	}
	if len(status.RecheckCadence.AllowedPlaybooks) != 2 {
		t.Errorf("expected allowed playbooks in health, got %+v", status.RecheckCadence)
	}
	if !status.RolloutReadiness.Ready {
		t.Fatalf("expected rollout readiness true, got blockers=%v", status.RolloutReadiness.Blockers)
	}
	if status.RolloutReadiness.RecommendedPhase == "" {
		t.Fatal("expected rollout recommended phase to be populated")
	}
	if len(status.RolloutReadiness.RollbackCriteria) == 0 {
		t.Fatal("expected rollback criteria to be populated")
	}

	// Verify health snapshot file creation
	snapFile := filepath.Join(tmpDir, "custom_health_snapshot.json")
	data, err := os.ReadFile(snapFile)
	if err != nil {
		t.Fatalf("failed to read health snapshot file: %v", err)
	}

	var parsed HealthStatus
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to parse health snapshot JSON: %v", err)
	}

	if parsed.BinanceConnectivity != "OK" || parsed.StorageWritable != "OK" {
		t.Errorf("Invalid snapshot contents: %+v", parsed)
	}

	// Test failures
	provider.failPrice = true
	aiService.failPing = true
	notifier.failPing = true

	failedStatus, err := uc.PerformHealthAudit(ctx)
	if err != nil {
		t.Fatalf("PerformHealthAudit failed on subcomponent errors: %v", err)
	}

	if failedStatus.BinanceConnectivity == "OK" || !strings.Contains(failedStatus.BinanceConnectivity, "ERROR") {
		t.Errorf("Expected Binance error, got %s", failedStatus.BinanceConnectivity)
	}
	if failedStatus.GeminiAvailability == "OK" || !strings.Contains(failedStatus.GeminiAvailability, "ERROR") {
		t.Errorf("Expected Gemini error, got %s", failedStatus.GeminiAvailability)
	}
	if failedStatus.TelegramAvailability == "OK" || !strings.Contains(failedStatus.TelegramAvailability, "ERROR") {
		t.Errorf("Expected Telegram error, got %s", failedStatus.TelegramAvailability)
	}
	if failedStatus.RolloutReadiness.Ready {
		t.Fatalf("expected rollout readiness false on failed dependencies, got %+v", failedStatus.RolloutReadiness)
	}
	if len(failedStatus.RolloutReadiness.Blockers) == 0 {
		t.Fatal("expected rollout blockers on failed health")
	}
}
