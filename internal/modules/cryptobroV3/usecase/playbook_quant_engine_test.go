package usecase

import (
	"os"
	"testing"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

func TestQuantEngineSafetyChecks(t *testing.T) {
	// Clean up created directories
	defer os.RemoveAll("storage")

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
	// If the last closed candle's Close is accidentally equal to data.LatestPrice (which is checked via VerifyIndicatorInput),
	// it will slice it properly to ensure the latest active price does not pollute calculations.
	dataWithPollution := MarketData{
		Symbol: "SOLUSDT",
		M15Candles: append(m15Candles, dto.Candle{
			Close: 125.0, // Match the latest price representing active open candle
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
