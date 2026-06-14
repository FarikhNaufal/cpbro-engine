package usecase

import (
	"testing"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"cpbro-engine/internal/modules/cryptobroV3/entity"
)

func TestDeriveMacroMarketState_UsesBroaderMarketVolatility(t *testing.T) {
	tickers := []dto.Ticker24h{
		{Symbol: "BTCUSDT", PriceChangePercent: 0.2},
		{Symbol: "ETHUSDT", PriceChangePercent: 3.2},
		{Symbol: "SOLUSDT", PriceChangePercent: -3.8},
		{Symbol: "DOGEUSDT", PriceChangePercent: 4.1},
		{Symbol: "XRPUSDT", PriceChangePercent: -2.9},
		{Symbol: "ADAUSDT", PriceChangePercent: 3.0},
		{Symbol: "BNBUSDT", PriceChangePercent: -3.4},
		{Symbol: "SUIUSDT", PriceChangePercent: 2.8},
	}

	state := deriveMacroMarketState(tickers)
	if state.Volatility != "HIGH" {
		t.Fatalf("expected HIGH volatility from broad market activity, got %s", state.Volatility)
	}
	if state.BTCChaos >= 0.8 {
		t.Fatalf("expected BTC chaos to stay below chaos threshold when BTC itself is calm, got %0.2f", state.BTCChaos)
	}
}

func TestDeriveMacroMarketState_LowVolRequiresBroadQuiet(t *testing.T) {
	tickers := []dto.Ticker24h{
		{Symbol: "BTCUSDT", PriceChangePercent: 0.1},
		{Symbol: "ETHUSDT", PriceChangePercent: 0.2},
		{Symbol: "SOLUSDT", PriceChangePercent: -0.3},
		{Symbol: "DOGEUSDT", PriceChangePercent: 0.4},
		{Symbol: "XRPUSDT", PriceChangePercent: -0.2},
		{Symbol: "ADAUSDT", PriceChangePercent: 0.3},
		{Symbol: "BNBUSDT", PriceChangePercent: -0.1},
		{Symbol: "SUIUSDT", PriceChangePercent: 0.2},
	}

	state := deriveMacroMarketState(tickers)
	if state.Volatility != "LOW" {
		t.Fatalf("expected LOW volatility when BTC and broader market are both quiet, got %s", state.Volatility)
	}
	if !isCompressionMacroActive(state) {
		t.Fatalf("expected quiet balanced market to qualify as compression macro context")
	}
}

func TestCompressionFallbackHelpers(t *testing.T) {
	previous := &entity.LatestResult{CompressionZeroEligibleStreak: 2}
	macroActive := true

	if !shouldFallbackCompressionToLowVol(previous, macroActive) {
		t.Fatalf("expected compression fallback to activate after streak threshold")
	}
	if streak := nextCompressionZeroEligibleStreak(previous, macroActive, 0); streak != 3 {
		t.Fatalf("expected streak to increment to 3, got %d", streak)
	}
	if streak := nextCompressionZeroEligibleStreak(previous, false, 0); streak != 0 {
		t.Fatalf("expected streak reset outside compression macro context, got %d", streak)
	}
	if streak := nextCompressionZeroEligibleStreak(previous, macroActive, 1); streak != 0 {
		t.Fatalf("expected streak reset once a playbook becomes eligible, got %d", streak)
	}
}
