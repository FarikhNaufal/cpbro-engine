package usecase

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

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
