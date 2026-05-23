package usecase

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScoring_PlaybookCalculations(t *testing.T) {
	uc := NewScoringUsecase()

	policy := MarketPolicy{
		AllowLong:  true,
		AllowShort: true,
		LongMode:   NORMAL,
		ShortMode:  NORMAL,
	}

	t.Run("TREND_PULLBACK high alignment score", func(t *testing.T) {
		quant := &QuantResult{
			Playbook:     TREND_PULLBACK,
			Direction:    LONG,
			IndicatorMet: true,
			TriggerPrice: 100.0,
			StopLoss:     98.0,
			TakeProfit:   105.0,
			H4Trend:      "BULLISH",
			H1Trend:      "BULLISH",
			TechnicalSnapshot: TechnicalSnapshot{
				RSI: 45.0,
				IndicatorValues: map[string]float64{
					IndicatorADX:   25.0,
					"volume_spike": 1.0,
				},
			},
		}

		score := uc.Calculate(quant, LONG, policy)
		assert.True(t, score > 7.0, "Score should be high for aligned trend pullback setup")
		assert.Contains(t, quant.Reason, "Score:")
		assert.Contains(t, quant.Reason, "Grade:")
		assert.Contains(t, quant.Reason, "Breakdown:")
	})

	t.Run("LIQUIDITY_SWEEP_REVERSAL high sweep score", func(t *testing.T) {
		quant := &QuantResult{
			Playbook:     LIQUIDITY_SWEEP_REVERSAL,
			Direction:    LONG,
			IndicatorMet: true,
			TriggerPrice: 100.0,
			StopLoss:     98.0,
			TakeProfit:   105.0,
			TechnicalSnapshot: TechnicalSnapshot{
				RSI: 50.0,
				IndicatorValues: map[string]float64{
					"sweep_low":      1.0,
					"wick_rejection": 1.0,
					"volume_spike":   1.0,
					"pa_rejection":   1.0,
				},
			},
		}

		score := uc.Calculate(quant, LONG, policy)
		assert.True(t, score > 7.0, "Score should be high for valid liquidity sweep setup")
	})

	t.Run("COMPRESSION_BREAKOUT_RETEST high compression score", func(t *testing.T) {
		quant := &QuantResult{
			Playbook:     COMPRESSION_BREAKOUT_RETEST,
			Direction:    LONG,
			IndicatorMet: true,
			TriggerPrice: 100.0,
			StopLoss:     98.0,
			TakeProfit:   105.0,
			TechnicalSnapshot: TechnicalSnapshot{
				RSI: 50.0,
				IndicatorValues: map[string]float64{
					"contraction":  1.0,
					"volume_spike": 1.0,
				},
			},
		}

		score := uc.Calculate(quant, LONG, policy)
		assert.True(t, score > 5.0, "Score should be moderate-high for compression setup")
	})

	t.Run("RANGE_EDGE_REVERSAL high range score", func(t *testing.T) {
		quant := &QuantResult{
			Playbook:     RANGE_EDGE_REVERSAL,
			Direction:    LONG,
			IndicatorMet: true,
			TriggerPrice: 100.0,
			StopLoss:     98.0,
			TakeProfit:   105.0,
			TechnicalSnapshot: TechnicalSnapshot{
				RSI: 50.0,
				IndicatorValues: map[string]float64{
					IndicatorADX:      20.0,
					"near_range_edge": 1.0,
					"wick_rejection":  1.0,
				},
			},
		}

		score := uc.Calculate(quant, LONG, policy)
		assert.True(t, score > 6.0, "Score should be high for range edge reversal setup")
	})

	t.Run("CROWDED_POSITIONING_SQUEEZE high squeeze score", func(t *testing.T) {
		quant := &QuantResult{
			Playbook:     CROWDED_POSITIONING_SQUEEZE,
			Direction:    LONG,
			IndicatorMet: true,
			TriggerPrice: 100.0,
			StopLoss:     98.0,
			TakeProfit:   105.0,
			TechnicalSnapshot: TechnicalSnapshot{
				RSI: 50.0,
				IndicatorValues: map[string]float64{
					"extreme_funding": 1.0,
					"extreme_oi":      1.0,
					"pa_rejection":    1.0,
					"sweep_low":       1.0,
				},
			},
		}

		score := uc.Calculate(quant, LONG, policy)
		assert.True(t, score > 6.0, "Score should be high for squeeze setup")
	})
}

func TestScoring_Penalties(t *testing.T) {
	uc := NewScoringUsecase()

	t.Run("Policy direction disallowance penalty", func(t *testing.T) {
		policy := MarketPolicy{
			AllowLong: false,
		}

		quant := &QuantResult{
			Playbook:     TREND_PULLBACK,
			Direction:    LONG,
			IndicatorMet: true,
			TriggerPrice: 100.0,
			StopLoss:     98.0,
			TakeProfit:   105.0,
			TechnicalSnapshot: TechnicalSnapshot{
				RSI: 45.0,
				IndicatorValues: map[string]float64{
					IndicatorADX: 25.0,
				},
			},
		}

		score := uc.Calculate(quant, LONG, policy)
		assert.True(t, score < 5.0, "Score should be penalized heavily when violating policy directions")
		assert.Contains(t, quant.Reason, "GLOBAL PENALTY: LONG trades disallowed")
	})

	t.Run("Poor Risk-to-Reward penalty", func(t *testing.T) {
		policy := MarketPolicy{
			AllowLong: true,
		}

		quant := &QuantResult{
			Playbook:     TREND_PULLBACK,
			Direction:    LONG,
			IndicatorMet: true,
			TriggerPrice: 100.0,
			StopLoss:     95.0,
			TakeProfit:   101.0, // RR = 1/5 = 0.2 (< 1.5)
			TechnicalSnapshot: TechnicalSnapshot{
				RSI: 45.0,
				IndicatorValues: map[string]float64{
					IndicatorADX: 25.0,
				},
			},
		}

		score := uc.Calculate(quant, LONG, policy)
		assert.True(t, score < 4.0, "Score should be penalized due to poor RR")
		assert.Contains(t, quant.Reason, "Poor Risk-to-Reward ratio")
	})
}
