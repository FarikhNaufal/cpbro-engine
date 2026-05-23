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

	// Set running statuses
	ScanWorkerRunning.Store(true)
	defer ScanWorkerRunning.Store(false)

	os.Setenv("HEALTH_STORAGE_CHECK", "true")
	defer os.Unsetenv("HEALTH_STORAGE_CHECK")

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

	// Verify health snapshot file creation
	snapFile := filepath.Join(tmpDir, "health_snapshot.json")
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
}
