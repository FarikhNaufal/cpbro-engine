package usecase

import (
	"fmt"
	"sort"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

type AICandidateSelectorUsecase struct {
	minScoreThreshold float64
}

func NewAICandidateSelectorUsecase(minScore float64) *AICandidateSelectorUsecase {
	return &AICandidateSelectorUsecase{
		minScoreThreshold: minScore,
	}
}

// IsEligible filters out weak setups, sending only high-scoring setups to the Gemini API.
func (uc *AICandidateSelectorUsecase) IsEligible(score float64) bool {
	return score >= uc.minScoreThreshold
}

// SelectCandidates filters the local quality gate candidates, picking the best candidates up to policy limits.
// Any skipped candidates are returned with LOCAL_WATCH status and AI_LIMIT reason.
func (uc *AICandidateSelectorUsecase) SelectCandidates(candidates []QuantResult, policy MarketPolicy) ([]QuantResult, []QuantResult) {
	var selected []QuantResult
	var skipped []QuantResult

	if len(candidates) == 0 {
		return selected, skipped
	}

	regime := policy.EffectiveRegime()
	isChaos := regime == BTC_CHAOS

	// Sort candidates:
	// 1. Score DESC
	// 2. RR DESC
	// 3. Playbook priority ASC (index 0 > 1 > 2...)
	// 4. Volume ratio DESC
	sorted := make([]QuantResult, len(candidates))
	copy(sorted, candidates)

	sort.Slice(sorted, func(i, j int) bool {
		// Score DESC
		if sorted[i].Score != sorted[j].Score {
			return sorted[i].Score > sorted[j].Score
		}
		// RR DESC
		rrI := uc.calculateRR(sorted[i])
		rrJ := uc.calculateRR(sorted[j])
		if rrI != rrJ {
			return rrI > rrJ
		}
		// Playbook Priority
		prioI := uc.getPlaybookPriorityIndex(sorted[i].Playbook, regime)
		prioJ := uc.getPlaybookPriorityIndex(sorted[j].Playbook, regime)
		if prioI != prioJ {
			return prioI < prioJ
		}
		// Volume ratio DESC
		volRatioI := uc.calculateVolumeRatio(sorted[i].RawKlines)
		volRatioJ := uc.calculateVolumeRatio(sorted[j].RawKlines)
		return volRatioI > volRatioJ
	})

	// Establish total limit
	limit := policy.MaxAICandidates
	if limit <= 0 {
		if isChaos {
			limit = 1
		} else {
			limit = 3
		}
	}

	selectedSymbols := make(map[string]Direction)
	tierCCount := 0
	maxTierC := 1
	if isChaos {
		maxTierC = 0
	}

	for _, cand := range sorted {
		// Avoid selecting two opposing signals or duplicate symbols
		if prevDir, exists := selectedSymbols[cand.Symbol]; exists {
			cand.Status = LOCAL_WATCH
			cand.Reason = fmt.Sprintf("AI_LIMIT: symbol %s already selected with direction %s", cand.Symbol, prevDir)
			skipped = append(skipped, cand)
			continue
		}

		// Tier C count checks
		if cand.Tier == TierC {
			if tierCCount >= maxTierC {
				cand.Status = LOCAL_WATCH
				cand.Reason = "AI_LIMIT: maximum Tier C candidates reached"
				skipped = append(skipped, cand)
				continue
			}
		}

		// Selection size limits
		if len(selected) >= limit {
			cand.Status = LOCAL_WATCH
			cand.Reason = "AI_LIMIT: maximum AI candidates limit reached"
			skipped = append(skipped, cand)
			continue
		}

		// Candidate accepted
		selectedSymbols[cand.Symbol] = cand.Direction
		if cand.Tier == TierC {
			tierCCount++
		}
		selected = append(selected, cand)
	}

	return selected, skipped
}

// calculateVolumeRatio calculates volume of last candle relative to previous 10 candles
func (uc *AICandidateSelectorUsecase) calculateVolumeRatio(candles []dto.Candle) float64 {
	if len(candles) <= 10 {
		return 1.0
	}
	lastCandle := candles[len(candles)-1]
	sum := 0.0
	for i := len(candles) - 2; i >= len(candles)-11; i-- {
		sum += candles[i].Vol
	}
	avg := sum / 10.0
	if avg <= 0 {
		return 1.0
	}
	return lastCandle.Vol / avg
}

// calculateRR extracts Risk-to-Reward ratio from TradePlan parameters
func (uc *AICandidateSelectorUsecase) calculateRR(cand QuantResult) float64 {
	entry := cand.TradePlan.EntryPrice
	tp := cand.TradePlan.TakeProfit
	sl := cand.TradePlan.StopLoss
	if entry <= 0 || tp <= 0 || sl <= 0 {
		return 0.0
	}
	if cand.Direction == LONG {
		if entry > sl {
			return (tp - entry) / (entry - sl)
		}
	} else if cand.Direction == SHORT {
		if sl > entry {
			return (entry - tp) / (sl - entry)
		}
	}
	return 0.0
}

// getPlaybookPriorityIndex maps playbook priority index based on the regime
func (uc *AICandidateSelectorUsecase) getPlaybookPriorityIndex(playbook Playbook, regime MarketRegime) int {
	if regime == CHOP_RANGE {
		switch playbook {
		case LIQUIDITY_SWEEP_REVERSAL:
			return 0
		case RANGE_EDGE_REVERSAL:
			return 1
		case CROWDED_POSITIONING_SQUEEZE:
			return 2
		case COMPRESSION_BREAKOUT_RETEST:
			return 3
		case TREND_PULLBACK:
			return 4
		}
	} else if regime == BTC_CHAOS {
		switch playbook {
		case LIQUIDITY_SWEEP_REVERSAL:
			return 0
		case CROWDED_POSITIONING_SQUEEZE:
			return 1
		default:
			return 99
		}
	} else if regime == RISK_OFF {
		switch playbook {
		case LIQUIDITY_SWEEP_REVERSAL:
			return 0
		case RANGE_EDGE_REVERSAL:
			return 1
		case TREND_PULLBACK:
			return 2
		case COMPRESSION_BREAKOUT_RETEST:
			return 3
		case CROWDED_POSITIONING_SQUEEZE:
			return 4
		}
	} else if regime == ALT_SUPPORTIVE {
		switch playbook {
		case TREND_PULLBACK:
			return 0
		case COMPRESSION_BREAKOUT_RETEST:
			return 1
		case LIQUIDITY_SWEEP_REVERSAL:
			return 2
		case CROWDED_POSITIONING_SQUEEZE:
			return 3
		case RANGE_EDGE_REVERSAL:
			return 4
		}
	}

	// Default normal priority
	switch playbook {
	case TREND_PULLBACK:
		return 0
	case LIQUIDITY_SWEEP_REVERSAL:
		return 1
	case COMPRESSION_BREAKOUT_RETEST:
		return 2
	case CROWDED_POSITIONING_SQUEEZE:
		return 3
	case RANGE_EDGE_REVERSAL:
		return 4
	}
	return 100
}
