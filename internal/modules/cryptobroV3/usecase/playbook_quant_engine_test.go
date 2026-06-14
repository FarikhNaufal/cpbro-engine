package usecase

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

func buildQuantEngineTestMarketData() MarketData {
	now := time.Now()
	m15Candles := make([]dto.Candle, 30)
	for i := 0; i < 30; i++ {
		m15Candles[i] = dto.Candle{
			Time:  now.Add(-time.Duration(30-i) * 15 * time.Minute),
			Open:  100.0,
			High:  101.0,
			Low:   99.0,
			Close: 100.0,
			Vol:   100.0,
		}
	}

	h1Candles := make([]dto.Candle, 60)
	h4Candles := make([]dto.Candle, 60)
	for i := range h1Candles {
		h1Candles[i] = dto.Candle{Time: now.Add(-time.Duration(60-i) * time.Hour), Close: 100.0, Vol: 1000.0}
		h4Candles[i] = dto.Candle{Time: now.Add(-time.Duration(60-i) * 4 * time.Hour), Close: 100.0, Vol: 1000.0}
	}

	return MarketData{
		Symbol:      "TESTUSDT",
		M15Candles:  m15Candles,
		H1Candles:   h1Candles,
		H4Candles:   h4Candles,
		LatestPrice: 100.0,
	}
}

func TestQuantEngineSafetyChecks(t *testing.T) {
	engine := NewPlaybookQuantEngineUsecase()

	// 1. Check H4 Trend closed candle vs EMA H4
	h4Candles := make([]dto.Candle, 201)
	for i := 0; i < 201; i++ {
		h4Candles[i] = dto.Candle{
			Close: 100.0,
			Vol:   1000.0,
		}
	}
	// Make the last closed candle above EMA
	h4Candles[200].Close = 105.0
	trend := CalculateH4Trend(h4Candles, 200)
	if trend != "BULLISH" {
		t.Errorf("Expected H4 Trend to be BULLISH, got %s", trend)
	}

	// 2. SHORT during BTC Bullish restriction
	policy := MarketPolicy{
		AllowShort:   true,
		ShortMode:    NORMAL,
		BtcTrend:     "BULLISH",
		AllowedTiers: []Tier{TierA},
	}

	m15Candles := make([]dto.Candle, 30)
	for i := 0; i < 30; i++ {
		m15Candles[i] = dto.Candle{
			Open:  100.0,
			Close: 100.0,
			High:  100.0,
			Low:   100.0,
			Vol:   100.0,
		}
	}

	data := MarketData{
		Symbol:      "SOLUSDT",
		M15Candles:  m15Candles,
		H1Candles:   h4Candles[:50],
		H4Candles:   h4Candles,
		LatestPrice: 100.0,
	}

	// When BTC (or asset H4) is bullish, short trend pullback should be rejected
	res := engine.RunEngine(TREND_PULLBACK, SHORT, data, policy)
	if res.Status != PLAYBOOK_REJECTED || res.Reason != "SHORT direction rejected by BTC bullish safety helper rules" {
		t.Errorf("Expected short pullback to be rejected under bullish trend, got status: %s, reason: %s", res.Status, res.Reason)
	}

	// 3. Test that Latest Price is excluded from indicator calculations
	// The in-progress M15 candle (open kline) must not be used for indicator calculations.
	// GetClosedCandlesOnly should drop it based on candle open-time + timeframe > now.
	dataWithPollution := MarketData{
		Symbol: "SOLUSDT",
		M15Candles: append(m15Candles, dto.Candle{
			Time:  time.Now(),
			Close: 125.0, // Active open candle close-like value
		}),
		H1Candles:   h4Candles[:50],
		H4Candles:   h4Candles,
		LatestPrice: 125.0,
	}
	resPollution := engine.RunEngine(TREND_PULLBACK, LONG, dataWithPollution, policy)
	if resPollution.TriggerPrice != 100.0 {
		t.Errorf("Expected trigger price to be 100.0 (excluding the polluted latest price candle), got %f", resPollution.TriggerPrice)
	}
}

func TestQuantEngine_CompressionBreakoutRetest_SetsSetupTypeToRetest(t *testing.T) {
	engine := NewPlaybookQuantEngineUsecase()

	// Build M15 candles where the last candles retest a prior range high and hold above it.
	m15Candles := make([]dto.Candle, 30)
	for i := 0; i < 30; i++ {
		m15Candles[i] = dto.Candle{
			Time:  time.Now().Add(-time.Duration(30-i) * 15 * time.Minute),
			Open:  100.0,
			High:  110.0,
			Low:   99.0,
			Close: 100.0,
			Vol:   10.0,
		}
	}
	// Define a clear range high on the prior 20 candles (excluding the last candle).
	// HighestHigh(m15Closed[:len-1], 20) will see 110.0 as the level.
	for i := 0; i < 29; i++ {
		m15Candles[i].High = 110.0
		m15Candles[i].Low = 99.0
		m15Candles[i].Close = 100.0
	}
	// Retest/hold in the last candle: dip to level and close back above.
	m15Candles[29].Low = 109.5
	m15Candles[29].Close = 110.5

	// Minimal HTF candles (trend is not used to gate COMPRESSION inside RunEngine).
	h1Candles := make([]dto.Candle, 60)
	for i := range h1Candles {
		h1Candles[i] = dto.Candle{Close: 100.0}
	}
	h4Candles := make([]dto.Candle, 60)
	for i := range h4Candles {
		h4Candles[i] = dto.Candle{Close: 100.0}
	}

	data := MarketData{
		Symbol:      "TESTUSDT",
		M15Candles:  m15Candles,
		H1Candles:   h1Candles,
		H4Candles:   h4Candles,
		LatestPrice: m15Candles[29].Close,
	}
	policy := MarketPolicy{
		AllowLong:  true,
		AllowShort: true,
	}

	res := engine.RunEngine(COMPRESSION_BREAKOUT_RETEST, LONG, data, policy)
	if res.SetupType != "BREAKOUT_RETEST" {
		t.Fatalf("expected SetupType=BREAKOUT_RETEST when retest evidence exists, got %q", res.SetupType)
	}
	if res.TechnicalSnapshot.IndicatorValues[IndicatorRetestHold] != 1.0 {
		t.Fatalf("expected IndicatorRetestHold=1.0, got %v", res.TechnicalSnapshot.IndicatorValues[IndicatorRetestHold])
	}
}

func TestQuantEngine_RunEngineWithPreparedContext_DoesNotMutateBaseSnapshot(t *testing.T) {
	engine := NewPlaybookQuantEngineUsecase()

	m15Candles := make([]dto.Candle, 30)
	for i := 0; i < 30; i++ {
		m15Candles[i] = dto.Candle{
			Time:  time.Now().Add(-time.Duration(30-i) * 15 * time.Minute),
			Open:  100.0,
			High:  110.0,
			Low:   99.0,
			Close: 100.0,
			Vol:   10.0,
		}
	}
	m15Candles[29].Low = 109.5
	m15Candles[29].Close = 110.5

	h1Candles := make([]dto.Candle, 60)
	h4Candles := make([]dto.Candle, 60)
	for i := range h1Candles {
		h1Candles[i] = dto.Candle{Time: time.Now().Add(-time.Duration(60-i) * time.Hour), Close: 100.0}
		h4Candles[i] = dto.Candle{Time: time.Now().Add(-time.Duration(60-i) * 4 * time.Hour), Close: 100.0}
	}

	data := MarketData{
		Symbol:      "TESTUSDT",
		M15Candles:  m15Candles,
		H1Candles:   h1Candles,
		H4Candles:   h4Candles,
		LatestPrice: m15Candles[29].Close,
	}
	policy := MarketPolicy{
		AllowLong:  true,
		AllowShort: true,
	}

	prepared, ok := engine.prepareContext(data)
	if !ok {
		t.Fatalf("expected prepared context to succeed")
	}
	if _, exists := prepared.technicalSnapshot.IndicatorValues[IndicatorRetestHold]; exists {
		t.Fatalf("expected base prepared snapshot to not contain retest hold before quant run")
	}

	res := engine.RunEngineWithPreparedContext(COMPRESSION_BREAKOUT_RETEST, LONG, data, policy, prepared)
	if res.TechnicalSnapshot.IndicatorValues[IndicatorRetestHold] != 1.0 {
		t.Fatalf("expected quant result to contain retest hold signal")
	}
	if _, exists := prepared.technicalSnapshot.IndicatorValues[IndicatorRetestHold]; exists {
		t.Fatalf("expected prepared snapshot to remain immutable after quant run")
	}
}

func TestQuantEngine_LiquiditySweep_UsesConfiguredVolumeRatio(t *testing.T) {
	engine := NewPlaybookQuantEngineUsecase()
	now := time.Now()

	m15Candles := make([]dto.Candle, 30)
	for i := 0; i < 30; i++ {
		m15Candles[i] = dto.Candle{
			Time:  now.Add(-time.Duration(30-i) * 15 * time.Minute),
			Open:  100.0,
			High:  105.0,
			Low:   95.0,
			Close: 100.0,
			Vol:   100.0,
		}
	}
	m15Candles[29].Low = 90.0
	m15Candles[29].Close = 98.0
	m15Candles[29].Vol = 135.0 // above 1.3x profile threshold, below old 1.5x hardcode

	h1Candles := make([]dto.Candle, 60)
	h4Candles := make([]dto.Candle, 220)
	for i := range h1Candles {
		h1Candles[i] = dto.Candle{Time: now.Add(-time.Duration(60-i) * time.Hour), Close: 100.0, Vol: 1000.0}
	}
	for i := range h4Candles {
		h4Candles[i] = dto.Candle{Time: now.Add(-time.Duration(220-i) * 4 * time.Hour), Close: 95.0, Vol: 1000.0}
	}
	h4Candles[len(h4Candles)-1].Close = 90.0 // bearish enough so SHORT safety does not interfere if direction changes

	data := MarketData{
		Symbol:      "TESTUSDT",
		M15Candles:  m15Candles,
		H1Candles:   h1Candles,
		H4Candles:   h4Candles,
		LatestPrice: m15Candles[29].Close,
	}
	policy := MarketPolicy{
		AllowLong:        true,
		AllowShort:       true,
		LongMode:         NORMAL,
		ShortMode:        NORMAL,
		AllowedTiers:     []Tier{TierA},
		AllowedPlaybooks: []Playbook{LIQUIDITY_SWEEP_REVERSAL},
	}

	res := engine.RunEngine(LIQUIDITY_SWEEP_REVERSAL, LONG, data, policy)
	if !res.IndicatorMet {
		t.Fatalf("expected sweep to pass with profile-based volume threshold, got reason: %s", res.Reason)
	}
	if res.TechnicalSnapshot.IndicatorValues[IndicatorVolumeSpike] != 1.0 {
		t.Fatalf("expected quant engine to mark volume spike using configured ratio, got %v", res.TechnicalSnapshot.IndicatorValues[IndicatorVolumeSpike])
	}
}

func TestQuantEngine_DebugSaveRawKlines_DefaultDisabled(t *testing.T) {
	t.Setenv("DEBUG_SAVE_RAW_KLINES", "false")
	debugDir := t.TempDir()
	t.Setenv("RAW_KLINES_DEBUG_DIR", debugDir)

	engine := NewPlaybookQuantEngineUsecase()
	engine.saveM15RawKlines("BTCUSDT", []dto.Candle{{Time: time.Now().Add(-30 * time.Minute), Close: 100, Vol: 1}})

	_, err := os.Stat(filepath.Join(debugDir, "raw_klines_BTCUSDT.json"))
	if err == nil {
		t.Fatalf("expected no debug raw klines file when disabled")
	}
}

func TestQuantEngine_DebugSaveRawKlines_EnabledWrites(t *testing.T) {
	t.Setenv("DEBUG_SAVE_RAW_KLINES", "true")
	debugDir := t.TempDir()
	t.Setenv("RAW_KLINES_DEBUG_DIR", debugDir)

	engine := NewPlaybookQuantEngineUsecase()
	engine.saveM15RawKlines("BTCUSDT", []dto.Candle{{Time: time.Now().Add(-30 * time.Minute), Close: 100, Vol: 1}})

	_, err := os.Stat(filepath.Join(debugDir, "raw_klines_BTCUSDT.json"))
	if err != nil {
		t.Fatalf("expected debug raw klines file to be written, got err: %v", err)
	}
}

func TestQuantEngine_DebugSaveRawKlines_WriteErrorDoesNotPanic(t *testing.T) {
	t.Setenv("DEBUG_SAVE_RAW_KLINES", "true")

	// Set RAW_KLINES_DEBUG_DIR to a file path to force mkdir error.
	filePath := filepath.Join(t.TempDir(), "not_a_dir")
	if err := os.WriteFile(filePath, []byte("x"), 0644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	t.Setenv("RAW_KLINES_DEBUG_DIR", filePath)

	engine := NewPlaybookQuantEngineUsecase()
	engine.saveM15RawKlines("BTCUSDT", []dto.Candle{{Time: time.Now().Add(-30 * time.Minute), Close: 100, Vol: 1}})
}

func TestQuantEngine_TrendPullbackTradePlanRespectsPolicyRR(t *testing.T) {
	engine := NewPlaybookQuantEngineUsecase()
	data := buildQuantEngineTestMarketData()
	policy := MarketPolicy{
		AllowLong:    true,
		AllowShort:   true,
		MinRRExecute: 1.7,
	}

	res := engine.RunEngine(TREND_PULLBACK, LONG, data, policy)
	risk := res.TriggerPrice - res.StopLoss
	reward := res.TakeProfit - res.TriggerPrice
	rr := reward / risk

	if math.Abs(rr-1.8) > 0.01 {
		t.Fatalf("expected TREND_PULLBACK RR to target policy minimum + buffer (1.8), got %.4f", rr)
	}
}

func TestQuantEngine_CrowdedSqueezeTradePlanRespectsPolicyRR(t *testing.T) {
	engine := NewPlaybookQuantEngineUsecase()
	data := buildQuantEngineTestMarketData()
	policy := MarketPolicy{
		AllowLong:    true,
		AllowShort:   true,
		MinRRExecute: 2.0,
	}

	res := engine.RunEngine(CROWDED_POSITIONING_SQUEEZE, LONG, data, policy)
	risk := res.TriggerPrice - res.StopLoss
	reward := res.TakeProfit - res.TriggerPrice
	rr := reward / risk

	if math.Abs(rr-2.1) > 0.01 {
		t.Fatalf("expected CROWDED_POSITIONING_SQUEEZE RR to target policy minimum + buffer (2.1), got %.4f", rr)
	}
}
