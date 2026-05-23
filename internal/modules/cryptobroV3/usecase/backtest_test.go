package usecase_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"cpbro-engine/internal/modules/cryptobroV3/usecase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockBacktestMarketDataProvider struct {
	candles map[string][]dto.Candle
}

func (m *mockBacktestMarketDataProvider) FetchClosedCandles(ctx context.Context, symbol string, interval string, limit int) ([]dto.Candle, error) {
	return m.candles[symbol], nil
}

func (m *mockBacktestMarketDataProvider) FetchLatestPrice(ctx context.Context, symbol string) (float64, error) {
	cList := m.candles[symbol]
	if len(cList) > 0 {
		return cList[len(cList)-1].Close, nil
	}
	return 100.0, nil
}

func (m *mockBacktestMarketDataProvider) FetchAllFuturesTickers24h(ctx context.Context) ([]dto.Ticker24h, error) {
	return []dto.Ticker24h{
		{Symbol: "BTCUSDT", LastPrice: 50000.0, PriceChangePercent: 0.0},
	}, nil
}

func (m *mockBacktestMarketDataProvider) FetchPremiumFundingRates(ctx context.Context) (map[string]float64, error) {
	return map[string]float64{"BTCUSDT": 0.0001}, nil
}

func (m *mockBacktestMarketDataProvider) FetchOpenInterest(ctx context.Context, symbol string) (float64, error) {
	return 1000000.0, nil
}

func (m *mockBacktestMarketDataProvider) FetchHistoricalCandles(ctx context.Context, symbol string, interval string, startTime time.Time, endTime time.Time) ([]dto.Candle, error) {
	list := m.candles[symbol]
	var res []dto.Candle
	for _, c := range list {
		if (c.Time.After(startTime) || c.Time.Equal(startTime)) && c.Time.Before(endTime) {
			res = append(res, c)
		}
	}
	return res, nil
}

func TestBacktestEngine_Run(t *testing.T) {
	// Create temporary directory for storage
	tmpDir, err := os.MkdirTemp("", "backtest_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create mock klines history (60 candles spaced by 15m)
	startTime := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	var candles []dto.Candle
	for i := 0; i < 60; i++ {
		tTime := startTime.Add(time.Duration(i) * 15 * time.Minute)
		// Simulate price slowly climbing and then dropping
		closePrice := 100.0 + float64(i)*0.1
		candles = append(candles, dto.Candle{
			Time:  tTime,
			Open:  closePrice - 0.05,
			High:  closePrice + 0.1,
			Low:   closePrice - 0.1,
			Close: closePrice,
			Vol:   100.0,
		})
	}

	btcCandles := make([]dto.Candle, len(candles))
	for i, c := range candles {
		btcCandles[i] = dto.Candle{
			Time:  c.Time,
			Open:  50000.0,
			High:  50050.0,
			Low:   49950.0,
			Close: 50000.0,
			Vol:   1000.0,
		}
	}

	mockData := &mockBacktestMarketDataProvider{
		candles: map[string][]dto.Candle{
			"SOLUSDT": candles,
			"BTCUSDT": btcCandles,
		},
	}

	// Instantiate all dependecy modules (mocked repos)
	mockRepo := &mockFeedbackStorageRepo{} // from feedback_test.go
	storageUC := usecase.NewStorageUsecase(mockRepo)
	marketPolicyUC := usecase.NewMarketPolicyUsecase()
	universeUC := usecase.NewUniverseUsecase()
	strategySelectorUC := usecase.NewStrategySelectorUsecase()
	playbookEligibilityUC := usecase.NewPlaybookEligibilityUsecase()
	playbookQuantEngineUC := usecase.NewPlaybookQuantEngineUsecase()
	scoringUC := usecase.NewScoringUsecase()
	candidateArbiterUC := usecase.NewCandidateArbiterUsecase()
	localGateUC := usecase.NewLocalGateUsecase()
	aiCandidateSelectorUC := usecase.NewAICandidateSelectorUsecase(60.0)
	aiAuditorUC := usecase.NewAIAuditorUsecase(&mockAPITestAIAuditor{}, storageUC)
	planReconciliationUC := usecase.NewPlanReconciliationUsecase()
	stalenessUC := usecase.NewStalenessUsecase(30 * time.Minute)
	finalGateUC := usecase.NewFinalGateUsecase()
	conflictResolverUC := usecase.NewConflictResolverUsecase()

	backtestUC := usecase.NewBacktestEngineUsecase(
		mockData,
		marketPolicyUC,
		universeUC,
		strategySelectorUC,
		playbookEligibilityUC,
		playbookQuantEngineUC,
		scoringUC,
		candidateArbiterUC,
		localGateUC,
		aiCandidateSelectorUC,
		aiAuditorUC,
		planReconciliationUC,
		stalenessUC,
		finalGateUC,
		conflictResolverUC,
		storageUC,
		tmpDir,
	)

	// Define Request (start run 10 hours into history, warm up 24h before ensures data coverage)
	req := usecase.BacktestRequest{
		Symbol:    "SOLUSDT",
		StartTime: startTime.Add(10 * time.Hour),
		EndTime:   startTime.Add(14 * time.Hour),
		Playbook:  "ALL",
		Regime:    "DYNAMIC",
		AIMode:    "MOCK",
	}

	report, err := backtestUC.RunBacktest(context.Background(), req)
	assert.NoError(t, err)
	require.NotNil(t, report)

	// General checks
	assert.NotEmpty(t, report.RunID)
	assert.Equal(t, "SOLUSDT", report.Symbol)
	assert.Equal(t, req.StartTime, report.StartTime)
	assert.Equal(t, req.EndTime, report.EndTime)

	// Storage checks: verify directory writes
	summaryFile := filepath.Join(tmpDir, "backtest_report.json")
	assert.FileExists(t, summaryFile)

	runFile := filepath.Join(tmpDir, "backtest_runs", "backtest_"+report.RunID+".json")
	assert.FileExists(t, runFile)

	// Read summary and parse
	summaryBytes, err := os.ReadFile(summaryFile)
	require.NoError(t, err)
	var summaries []usecase.BacktestReportSummary
	err = json.Unmarshal(summaryBytes, &summaries)
	assert.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, report.RunID, summaries[0].RunID)
}

func TestBacktestEngine_LookaheadBiasPrevention(t *testing.T) {
	// Verify that filterClosedCandles hides future candles correctly
	t0 := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	candles := []dto.Candle{
		{Time: t0, Open: 100.0, Close: 101.0},                       // close at 12:15
		{Time: t0.Add(15 * time.Minute), Open: 101.0, Close: 102.0}, // close at 12:30
		{Time: t0.Add(30 * time.Minute), Open: 102.0, Close: 103.0}, // close at 12:45
	}

	// At T = 12:30, only first 2 candles should be returned as closed
	res := usecase.FilterClosedCandles(candles, t0.Add(30*time.Minute), 15*time.Minute)
	require.Len(t, res, 2)
	assert.Equal(t, t0, res[0].Time)
	assert.Equal(t, t0.Add(15*time.Minute), res[1].Time)
}

type mockAPITestAIAuditor struct{}

func (m *mockAPITestAIAuditor) AuditCandidate(ctx context.Context, req dto.AIAuditRequest) (*dto.AIAuditResponse, error) {
	return &dto.AIAuditResponse{Symbol: req.Symbol, IsApproved: true, Decision: "CONFIRM"}, nil
}
