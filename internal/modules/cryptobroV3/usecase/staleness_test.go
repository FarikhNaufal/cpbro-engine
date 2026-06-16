package usecase

import (
	"context"
	"testing"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

type mockLatestPriceFeed struct {
	symbols []string
	prices  map[string]struct {
		price float64
		at    time.Time
		ok    bool
	}
}

func (m *mockLatestPriceFeed) SyncSymbols(symbols []string) error {
	m.symbols = symbols
	return nil
}

func (m *mockLatestPriceFeed) GetLatestPrice(symbol string) (float64, time.Time, bool) {
	if m == nil || m.prices == nil {
		return 0, time.Time{}, false
	}
	entry, ok := m.prices[symbol]
	if !ok {
		return 0, time.Time{}, false
	}
	return entry.price, entry.at, entry.ok
}

type mockLatestPriceFallbackProvider struct {
	price float64
}

func (m *mockLatestPriceFallbackProvider) FetchClosedCandles(ctx context.Context, symbol string, interval string, limit int) ([]dto.Candle, error) {
	return nil, nil
}
func (m *mockLatestPriceFallbackProvider) FetchLatestPrice(ctx context.Context, symbol string) (float64, error) {
	return m.price, nil
}
func (m *mockLatestPriceFallbackProvider) FetchAllFuturesTickers24h(ctx context.Context) ([]dto.Ticker24h, error) {
	return nil, nil
}
func (m *mockLatestPriceFallbackProvider) FetchPremiumFundingRates(ctx context.Context) (map[string]float64, error) {
	return nil, nil
}
func (m *mockLatestPriceFallbackProvider) FetchOpenInterest(ctx context.Context, symbol string) (float64, error) {
	return 0, nil
}
func (m *mockLatestPriceFallbackProvider) FetchHistoricalCandles(ctx context.Context, symbol string, interval string, startTime time.Time, endTime time.Time) ([]dto.Candle, error) {
	return nil, nil
}

func TestStalenessCheck_Evaluate(t *testing.T) {
	uc := NewStalenessUsecase(15 * time.Minute)

	t.Run("Valid ATR - Tier A, Normal Volatility", func(t *testing.T) {
		quant := QuantResult{
			Playbook: TREND_PULLBACK,
			Tier:     TierA,
			TradePlan: TradePlan{
				EntryPrice: 100.0,
			},
			TechnicalSnapshot: TechnicalSnapshot{
				IndicatorValues: map[string]float64{
					IndicatorATR: 10.0,
				},
			},
		}
		review := PlanReview{}
		policy := MarketPolicy{Reason: "Normal"}

		// TREND_PULLBACK StalenessATR = 0.45. Tier A adds 0.05 -> 0.50 ATR (5.0 units of price)
		// Case 1: Fresh (distance = 2.0 -> 0.20 ATR)
		res1 := uc.Evaluate(quant, review, policy, 102.0)
		if res1.Status != FRESH || res1.IsStale {
			t.Errorf("Expected status FRESH, got %s (IsStale=%v)", res1.Status, res1.IsStale)
		}

		// Case 2: Late (distance = 6.0 -> 0.60 ATR, within 0.50 * 1.5 = 0.75 ATR)
		res2 := uc.Evaluate(quant, review, policy, 106.0)
		if res2.Status != LATE || !res2.IsStale {
			t.Errorf("Expected status LATE, got %s (IsStale=%v)", res2.Status, res2.IsStale)
		}

		// Case 3: Missed (distance = 8.0 -> 0.80 ATR > 0.75 ATR)
		res3 := uc.Evaluate(quant, review, policy, 108.0)
		if res3.Status != MISSED || !res3.IsStale {
			t.Errorf("Expected status MISSED, got %s (IsStale=%v)", res3.Status, res3.IsStale)
		}
	})

	t.Run("Valid ATR - Tier B, High Volatility Adjustment", func(t *testing.T) {
		quant := QuantResult{
			Playbook: TREND_PULLBACK,
			Tier:     TierB,
			TradePlan: TradePlan{
				EntryPrice: 100.0,
			},
			TechnicalSnapshot: TechnicalSnapshot{
				IndicatorValues: map[string]float64{
					IndicatorATR: 10.0,
				},
			},
		}
		review := PlanReview{}
		// TREND_PULLBACK StalenessATR = 0.45. Tier B adds 0.0 -> 0.45. High Vol reduces by 0.05 -> 0.40 ATR (4.0 price units)
		policy := MarketPolicy{Reason: "HIGH_VOLATILITY"}

		// Case 1: Fresh (distance = 3.0 -> 0.30 ATR <= 0.40 ATR)
		res1 := uc.Evaluate(quant, review, policy, 103.0)
		if res1.Status != FRESH {
			t.Errorf("Expected status FRESH, got %s", res1.Status)
		}

		// Case 2: Late (distance = 5.0 -> 0.50 ATR <= 0.40 * 1.5 = 0.60 ATR)
		res2 := uc.Evaluate(quant, review, policy, 105.0)
		if res2.Status != LATE {
			t.Errorf("Expected status LATE, got %s", res2.Status)
		}
	})

	t.Run("Valid ATR - BTCChaos Adjustment", func(t *testing.T) {
		quant := QuantResult{
			Playbook: TREND_PULLBACK,
			Tier:     TierA,
			TradePlan: TradePlan{
				EntryPrice: 100.0,
			},
			TechnicalSnapshot: TechnicalSnapshot{
				IndicatorValues: map[string]float64{
					IndicatorATR: 10.0,
				},
			},
		}
		review := PlanReview{}
		// TREND_PULLBACK StalenessATR = 0.45. Tier A adds 0.05 -> 0.50. Chaos/High Vol reduces by 0.05 -> 0.45 ATR (4.5 price units)
		policy := MarketPolicy{Reason: "BTC_CHAOS"}

		// Case 1: Fresh (distance = 1.5 -> 0.15 ATR <= 0.45 ATR)
		res1 := uc.Evaluate(quant, review, policy, 101.5)
		if res1.Status != FRESH {
			t.Errorf("Expected status FRESH, got %s", res1.Status)
		}

		// Case 2: Missed (distance = 8.0 -> 0.80 ATR > 0.45 * 1.5 = 0.675 ATR)
		res2 := uc.Evaluate(quant, review, policy, 108.0)
		if res2.Status != MISSED {
			t.Errorf("Expected status MISSED, got %s", res2.Status)
		}
	})

	t.Run("Fallback Percentage - Normal, No ATR", func(t *testing.T) {
		quant := QuantResult{
			Playbook: TREND_PULLBACK,
			Tier:     TierA,
			TradePlan: TradePlan{
				EntryPrice: 1000.0,
			},
		}
		review := PlanReview{}
		// Fallback normal = 0.35% (3.5 price units)
		policy := MarketPolicy{Reason: "Normal"}

		// Case 1: Fresh (distance = 2.0 -> 0.20% <= 0.35%)
		res1 := uc.Evaluate(quant, review, policy, 1002.0)
		if res1.Status != FRESH {
			t.Errorf("Expected status FRESH, got %s", res1.Status)
		}

		// Case 2: Late (distance = 4.5 -> 0.45% <= 0.35% * 1.5 = 0.525%)
		res2 := uc.Evaluate(quant, review, policy, 1004.5)
		if res2.Status != LATE {
			t.Errorf("Expected status LATE, got %s", res2.Status)
		}

		// Case 3: Missed (distance = 6.0 -> 0.60% > 0.525%)
		res3 := uc.Evaluate(quant, review, policy, 1006.0)
		if res3.Status != MISSED {
			t.Errorf("Expected status MISSED, got %s", res3.Status)
		}
	})

	t.Run("Policy staleness multiplier tightens threshold", func(t *testing.T) {
		quant := QuantResult{
			Playbook: TREND_PULLBACK,
			Tier:     TierA,
			TradePlan: TradePlan{
				EntryPrice: 100.0,
			},
			TechnicalSnapshot: TechnicalSnapshot{
				IndicatorValues: map[string]float64{
					IndicatorATR: 10.0,
				},
			},
		}
		review := PlanReview{}
		normalPolicy := MarketPolicy{Regime: DEFAULT, StalenessATRMultiplier: 1.5}
		tightPolicy := MarketPolicy{Regime: HIGH_VOL, StalenessATRMultiplier: 0.8}

		normal := uc.Evaluate(quant, review, normalPolicy, 103.0)
		tight := uc.Evaluate(quant, review, tightPolicy, 103.0)

		if normal.Status != FRESH {
			t.Fatalf("Expected normal policy to be FRESH, got %s", normal.Status)
		}
		if tight.Status != LATE {
			t.Fatalf("Expected tight high-vol policy to be LATE, got %s", tight.Status)
		}
	})

	t.Run("Runtime settings affect fallback percentage and late multiplier", func(t *testing.T) {
		original := SnapshotRuntimeSettings()
		t.Cleanup(func() { SetRuntimeSettings(original) })

		settings := original
		settings.StalenessBasePctDefault = 0.10
		settings.StalenessLateThresholdMultiplier = 2.0
		SetRuntimeSettings(settings)

		quant := QuantResult{
			Playbook: TREND_PULLBACK,
			Tier:     TierA,
			TradePlan: TradePlan{
				EntryPrice: 1000.0,
			},
		}
		review := PlanReview{}
		policy := MarketPolicy{Reason: "Normal"}

		late := uc.Evaluate(quant, review, policy, 1001.5)
		if late.Status != LATE {
			t.Fatalf("expected custom runtime settings to classify 0.15%% move as LATE, got %s", late.Status)
		}

		missed := uc.Evaluate(quant, review, policy, 1002.2)
		if missed.Status != MISSED {
			t.Fatalf("expected custom runtime settings to classify 0.22%% move as MISSED, got %s", missed.Status)
		}
	})
}

func TestStalenessCheck_IsFreshAtSupportsHistoricalBacktestClock(t *testing.T) {
	uc := NewStalenessUsecase(30 * time.Minute)
	now := time.Date(2026, 5, 24, 12, 30, 0, 0, time.UTC)
	candles := []dto.Candle{
		{Time: now.Add(-30 * time.Minute), Close: 100},
		{Time: now.Add(-15 * time.Minute), Close: 101},
	}

	if !uc.IsFreshAt(candles, now, 15*time.Minute) {
		t.Fatalf("expected historical closed candle to be fresh at simulated current tick")
	}

	staleNow := now.Add(45 * time.Minute)
	if uc.IsFreshAt(candles, staleNow, 15*time.Minute) {
		t.Fatalf("expected historical candle gap to be stale")
	}
}

func TestStalenessCheck_ResolveLatestPricePrefersFeedThenFallbackProvider(t *testing.T) {
	uc := NewStalenessUsecase(30 * time.Minute)
	feed := &mockLatestPriceFeed{
		prices: map[string]struct {
			price float64
			at    time.Time
			ok    bool
		}{
			"BTCUSDT": {price: 101.25, at: time.Now(), ok: true},
		},
	}
	uc.SetLatestPriceFeed(feed)
	uc.SetFallbackProvider(&mockLatestPriceFallbackProvider{price: 99.75})

	got, ok := uc.ResolveLatestPrice(context.Background(), "BTCUSDT")
	if !ok || got != 101.25 {
		t.Fatalf("expected realtime feed price, got %v ok=%v", got, ok)
	}

	got, ok = uc.ResolveLatestPrice(context.Background(), "ETHUSDT")
	if !ok || got != 99.75 {
		t.Fatalf("expected fallback provider price, got %v ok=%v", got, ok)
	}
}

func TestStalenessCheck_ResolveLatestPriceRejectsStaleFallbacks(t *testing.T) {
	uc := NewStalenessUsecase(30 * time.Minute)

	got, ok := uc.ResolveLatestPrice(context.Background(), "BTCUSDT")
	if ok || got != 0 {
		t.Fatalf("expected unavailable latest price, got %v ok=%v", got, ok)
	}
}
