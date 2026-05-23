package usecase

import (
	"testing"
	"time"
)

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
					"ATR": 10.0,
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
					"ATR": 10.0,
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
					"ATR": 10.0,
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
}
