package usecase

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

type CandidateArbiterUsecase struct{}

func NewCandidateArbiterUsecase() *CandidateArbiterUsecase {
	return &CandidateArbiterUsecase{}
}

// Arbitrate selects the best playbook candidates per symbol and resolves direction conflicts.
// It returns (selectedCandidates, rejectedCandidates).
func (uc *CandidateArbiterUsecase) Arbitrate(candidates []QuantResult, policy MarketPolicy) ([]QuantResult, []QuantResult) {
	var selected []QuantResult
	var rejected []QuantResult

	if len(candidates) == 0 {
		return selected, rejected
	}

	isChaos := strings.Contains(strings.ToUpper(policy.Reason), "CHAOS")
	isChop := strings.Contains(strings.ToUpper(policy.Reason), "CHOP")
	isRiskOff := strings.Contains(strings.ToUpper(policy.Reason), "RISK_OFF")
	isAltSupportive := strings.Contains(strings.ToUpper(policy.Reason), "ALT_SUPPORTIVE")

	// Group by symbol
	symbolGroups := make(map[string][]QuantResult)
	for _, cand := range candidates {
		// NaN / Inf safety guards
		if math.IsNaN(cand.Score) || math.IsInf(cand.Score, 0) {
			cand.Status = ARBITER_REJECTED
			cand.Reason = fmt.Sprintf("Arbiter reject: NaN/Inf score detected: %f", cand.Score)
			rejected = append(rejected, cand)
			continue
		}

		// Clamp score to [0.0, 10.0]
		if cand.Score < 0.0 {
			cand.Score = 0.0
		} else if cand.Score > 10.0 {
			cand.Score = 10.0
		}

		// Check general policy: Allowed Tiers
		tierAllowed := false
		for _, t := range policy.AllowedTiers {
			if t == cand.Tier {
				tierAllowed = true
				break
			}
		}
		if !tierAllowed {
			cand.Status = ARBITER_REJECTED
			cand.Reason = fmt.Sprintf("Arbiter reject: Tier %s is not allowed by policy", cand.Tier)
			rejected = append(rejected, cand)
			continue
		}

		// Check general policy: Allowed Playbooks
		playbookAllowed := false
		for _, p := range policy.AllowedPlaybooks {
			if p == cand.Playbook {
				playbookAllowed = true
				break
			}
		}
		if !playbookAllowed {
			cand.Status = ARBITER_REJECTED
			cand.Reason = fmt.Sprintf("Arbiter reject: Playbook %s is not allowed by policy", cand.Playbook)
			rejected = append(rejected, cand)
			continue
		}

		// Check policy directional mode constraints
		if cand.Direction == LONG {
			if !policy.AllowLong || policy.LongMode == DISABLED {
				cand.Status = ARBITER_REJECTED
				cand.Reason = "Arbiter reject: LONG trades disallowed by policy"
				rejected = append(rejected, cand)
				continue
			}
			if policy.LongMode == PULLBACK_ONLY && cand.Playbook != TREND_PULLBACK {
				cand.Status = ARBITER_REJECTED
				cand.Reason = fmt.Sprintf("Arbiter reject: Policy LongMode PULLBACK_ONLY blocks playbook %s", cand.Playbook)
				rejected = append(rejected, cand)
				continue
			}
			if policy.LongMode == SWEEP_ONLY && cand.Playbook != LIQUIDITY_SWEEP_REVERSAL {
				cand.Status = ARBITER_REJECTED
				cand.Reason = fmt.Sprintf("Arbiter reject: Policy LongMode SWEEP_ONLY blocks playbook %s", cand.Playbook)
				rejected = append(rejected, cand)
				continue
			}
			if policy.LongMode == BREAKOUT_RETEST_ONLY && cand.Playbook != COMPRESSION_BREAKOUT_RETEST {
				cand.Status = ARBITER_REJECTED
				cand.Reason = fmt.Sprintf("Arbiter reject: Policy LongMode BREAKOUT_RETEST_ONLY blocks playbook %s", cand.Playbook)
				rejected = append(rejected, cand)
				continue
			}
			if policy.LongMode == REVERSAL_ONLY {
				isReversal := cand.Playbook == LIQUIDITY_SWEEP_REVERSAL || cand.Playbook == RANGE_EDGE_REVERSAL || cand.Playbook == CROWDED_POSITIONING_SQUEEZE
				if !isReversal {
					cand.Status = ARBITER_REJECTED
					cand.Reason = fmt.Sprintf("Arbiter reject: Policy LongMode REVERSAL_ONLY blocks playbook %s", cand.Playbook)
					rejected = append(rejected, cand)
					continue
				}
			}
		} else if cand.Direction == SHORT {
			if !policy.AllowShort || policy.ShortMode == DISABLED {
				cand.Status = ARBITER_REJECTED
				cand.Reason = "Arbiter reject: SHORT trades disallowed by policy"
				rejected = append(rejected, cand)
				continue
			}
			if policy.ShortMode == PULLBACK_ONLY && cand.Playbook != TREND_PULLBACK {
				cand.Status = ARBITER_REJECTED
				cand.Reason = fmt.Sprintf("Arbiter reject: Policy ShortMode PULLBACK_ONLY blocks playbook %s", cand.Playbook)
				rejected = append(rejected, cand)
				continue
			}
			if policy.ShortMode == SWEEP_ONLY && cand.Playbook != LIQUIDITY_SWEEP_REVERSAL {
				cand.Status = ARBITER_REJECTED
				cand.Reason = fmt.Sprintf("Arbiter reject: Policy ShortMode SWEEP_ONLY blocks playbook %s", cand.Playbook)
				rejected = append(rejected, cand)
				continue
			}
			if policy.ShortMode == BREAKOUT_RETEST_ONLY && cand.Playbook != COMPRESSION_BREAKOUT_RETEST {
				cand.Status = ARBITER_REJECTED
				cand.Reason = fmt.Sprintf("Arbiter reject: Policy ShortMode BREAKOUT_RETEST_ONLY blocks playbook %s", cand.Playbook)
				rejected = append(rejected, cand)
				continue
			}
			if policy.ShortMode == REVERSAL_ONLY {
				isReversal := cand.Playbook == LIQUIDITY_SWEEP_REVERSAL || cand.Playbook == RANGE_EDGE_REVERSAL || cand.Playbook == CROWDED_POSITIONING_SQUEEZE
				if !isReversal {
					cand.Status = ARBITER_REJECTED
					cand.Reason = fmt.Sprintf("Arbiter reject: Policy ShortMode REVERSAL_ONLY blocks playbook %s", cand.Playbook)
					rejected = append(rejected, cand)
					continue
				}
			}
		}

		symbolGroups[cand.Symbol] = append(symbolGroups[cand.Symbol], cand)
	}

	for _, group := range symbolGroups {
		// Filter based on specific market regimes (Chaos, Risk Off, Alt Supportive)
		var activeCandidates []QuantResult
		for _, cand := range group {
			if isChaos {
				// Only LIQUIDITY_SWEEP_REVERSAL (S+) and CROWDED_POSITIONING_SQUEEZE (S+) are allowed.
				isSweep := cand.Playbook == LIQUIDITY_SWEEP_REVERSAL
				isSqueeze := cand.Playbook == CROWDED_POSITIONING_SQUEEZE
				isSPlus := cand.Score >= 8.5
				if (isSweep || isSqueeze) && isSPlus {
					activeCandidates = append(activeCandidates, cand)
				} else {
					cand.Status = ARBITER_REJECTED
					cand.Reason = fmt.Sprintf("BTCChaos: only S+ Sweep and Squeeze allowed, got playbook %s score %0.1f", cand.Playbook, cand.Score)
					rejected = append(rejected, cand)
				}
			} else if isRiskOff {
				// LONG candidates: Only LIQUIDITY_SWEEP_REVERSAL & CROWDED_POSITIONING_SQUEEZE and must be premium (score >= 7.8)
				if cand.Direction == LONG {
					isSweep := cand.Playbook == LIQUIDITY_SWEEP_REVERSAL
					isSqueeze := cand.Playbook == CROWDED_POSITIONING_SQUEEZE
					isPremium := cand.Score >= 7.8
					if (isSweep || isSqueeze) && isPremium {
						activeCandidates = append(activeCandidates, cand)
					} else {
						cand.Status = ARBITER_REJECTED
						cand.Reason = fmt.Sprintf("RiskOff LONG filter: only premium (score >= 7.8) Sweep/Squeeze allowed, got playbook %s score %0.1f", cand.Playbook, cand.Score)
						rejected = append(rejected, cand)
					}
				} else {
					activeCandidates = append(activeCandidates, cand)
				}
			} else if isAltSupportive {
				// SHORT candidates: Only LIQUIDITY_SWEEP_REVERSAL & CROWDED_POSITIONING_SQUEEZE and must be premium (score >= 7.8)
				if cand.Direction == SHORT {
					isSweep := cand.Playbook == LIQUIDITY_SWEEP_REVERSAL
					isSqueeze := cand.Playbook == CROWDED_POSITIONING_SQUEEZE
					isPremium := cand.Score >= 7.8
					if (isSweep || isSqueeze) && isPremium {
						activeCandidates = append(activeCandidates, cand)
					} else {
						cand.Status = ARBITER_REJECTED
						cand.Reason = fmt.Sprintf("AltSupportive SHORT filter: only premium (score >= 7.8) Sweep/Squeeze allowed, got playbook %s score %0.1f", cand.Playbook, cand.Score)
						rejected = append(rejected, cand)
					}
				} else {
					activeCandidates = append(activeCandidates, cand)
				}
			} else {
				activeCandidates = append(activeCandidates, cand)
			}
		}

		if len(activeCandidates) == 0 {
			continue
		}

		// Group active candidates by direction
		longs := []QuantResult{}
		shorts := []QuantResult{}
		for _, cand := range activeCandidates {
			if cand.Direction == LONG {
				longs = append(longs, cand)
			} else if cand.Direction == SHORT {
				shorts = append(shorts, cand)
			} else {
				cand.Status = ARBITER_REJECTED
				cand.Reason = "Invalid direction in quant result"
				rejected = append(rejected, cand)
			}
		}

		// Helper to sort same direction candidates with extensive tie-breakers
		sortSameDirection := func(cands []QuantResult) {
			sort.Slice(cands, func(i, j int) bool {
				scoreI := cands[i].Score
				scoreJ := cands[j].Score
				diff := math.Abs(scoreI - scoreJ)
				if diff >= 0.1 {
					return scoreI > scoreJ
				}

				// Tie-breaker 1: playbook priority index
				pI := uc.getPlaybookPriorityIndex(cands[i].Playbook, cands[i].Direction, isChaos, isChop, isRiskOff, isAltSupportive)
				pJ := uc.getPlaybookPriorityIndex(cands[j].Playbook, cands[j].Direction, isChaos, isChop, isRiskOff, isAltSupportive)
				if pI != pJ {
					return pI < pJ
				}

				// Tie-breaker 2: Risk-to-Reward ratio
				rrI := uc.calculateRR(cands[i])
				rrJ := uc.calculateRR(cands[j])
				if math.Abs(rrI-rrJ) > 0.01 {
					return rrI > rrJ
				}

				// Tie-breaker 3: Grade priority S+ > S > A > B
				gradeI := uc.getGradeWeight(cands[i].Reason)
				gradeJ := uc.getGradeWeight(cands[j].Reason)
				if gradeI != gradeJ {
					return gradeI > gradeJ
				}

				return scoreI > scoreJ
			})
		}

		// Same-direction arbitration: pick the best candidate for LONG and SHORT
		var winningLong *QuantResult
		if len(longs) > 0 {
			sortSameDirection(longs)
			winningLong = &longs[0]
			for i := 1; i < len(longs); i++ {
				longs[i].Status = ARBITER_REJECTED
				longs[i].Reason = fmt.Sprintf("Same direction tie-breaker: chosen playbook %s (score %0.1f) over %s (score %0.1f)", winningLong.Playbook, winningLong.Score, longs[i].Playbook, longs[i].Score)
				rejected = append(rejected, longs[i])
			}
		}

		var winningShort *QuantResult
		if len(shorts) > 0 {
			sortSameDirection(shorts)
			winningShort = &shorts[0]
			for i := 1; i < len(shorts); i++ {
				shorts[i].Status = ARBITER_REJECTED
				shorts[i].Reason = fmt.Sprintf("Same direction tie-breaker: chosen playbook %s (score %0.1f) over %s (score %0.1f)", winningShort.Playbook, winningShort.Score, shorts[i].Playbook, shorts[i].Score)
				rejected = append(rejected, shorts[i])
			}
		}

		// Opposing-direction arbitration: resolve conflicts between LONG and SHORT
		if winningLong != nil && winningShort != nil {
			// Under BTCChaos conflict
			if isChaos {
				// Reject both unless one is S+ and the other is not
				longSPlus := winningLong.Score >= 8.5
				shortSPlus := winningShort.Score >= 8.5

				if longSPlus && !shortSPlus {
					winningShort.Status = ARBITER_REJECTED
					winningShort.Reason = "BTCChaos conflict: opposing SHORT rejected as LONG is S+ setup"
					rejected = append(rejected, *winningShort)

					winningLong.Status = ARBITER_SELECTED
					winningLong.Reason = fmt.Sprintf("Arbiter selected LONG (score %0.1f, S+) over SHORT (score %0.1f) during chaos", winningLong.Score, winningShort.Score)
					selected = append(selected, *winningLong)
				} else if shortSPlus && !longSPlus {
					winningLong.Status = ARBITER_REJECTED
					winningLong.Reason = "BTCChaos conflict: opposing LONG rejected as SHORT is S+ setup"
					rejected = append(rejected, *winningLong)

					winningShort.Status = ARBITER_SELECTED
					winningShort.Reason = fmt.Sprintf("Arbiter selected SHORT (score %0.1f, S+) over LONG (score %0.1f) during chaos", winningShort.Score, winningLong.Score)
					selected = append(selected, *winningShort)
				} else {
					winningLong.Status = ARBITER_REJECTED
					winningLong.Reason = "BTCChaos conflict: opposing directions on same symbol during chaos (both rejected)"
					rejected = append(rejected, *winningLong)

					winningShort.Status = ARBITER_REJECTED
					winningShort.Reason = "BTCChaos conflict: opposing directions on same symbol during chaos (both rejected)"
					rejected = append(rejected, *winningShort)
				}
				continue
			}

			// Score comparison under normal/chop/directional regimes
			scoreDiff := winningLong.Score - winningShort.Score
			if math.Abs(scoreDiff) >= 0.7 {
				if scoreDiff > 0 {
					// LONG wins
					winningShort.Status = ARBITER_REJECTED
					winningShort.Reason = fmt.Sprintf("Opposing conflict: LONG score (%0.1f) is significantly higher than SHORT (%0.1f)", winningLong.Score, winningShort.Score)
					rejected = append(rejected, *winningShort)

					winningLong.Status = ARBITER_SELECTED
					winningLong.Reason = fmt.Sprintf("Arbiter selected LONG (score %0.1f) over SHORT (score %0.1f, diff >= 0.7)", winningLong.Score, winningShort.Score)
					selected = append(selected, *winningLong)
				} else {
					// SHORT wins
					winningLong.Status = ARBITER_REJECTED
					winningLong.Reason = fmt.Sprintf("Opposing conflict: SHORT score (%0.1f) is significantly higher than LONG (%0.1f)", winningShort.Score, winningLong.Score)
					rejected = append(rejected, *winningLong)

					winningShort.Status = ARBITER_SELECTED
					winningShort.Reason = fmt.Sprintf("Arbiter selected SHORT (score %0.1f) over LONG (score %0.1f, diff >= 0.7)", winningShort.Score, winningLong.Score)
					selected = append(selected, *winningShort)
				}
			} else {
				// Score difference too small, reject both
				winningLong.Status = ARBITER_REJECTED
				winningLong.Reason = fmt.Sprintf("Opposing conflict: score difference too small (%0.1f vs %0.1f, diff %0.1f < 0.7)", winningLong.Score, winningShort.Score, math.Abs(scoreDiff))
				rejected = append(rejected, *winningLong)

				winningShort.Status = ARBITER_REJECTED
				winningShort.Reason = fmt.Sprintf("Opposing conflict: score difference too small (%0.1f vs %0.1f, diff %0.1f < 0.7)", winningLong.Score, winningShort.Score, math.Abs(scoreDiff))
				rejected = append(rejected, *winningShort)
			}
		} else {
			// Only one direction is present for this symbol
			if winningLong != nil {
				winningLong.Status = ARBITER_SELECTED
				winningLong.Reason = fmt.Sprintf("Arbiter selected single LONG candidate (score %0.1f, playbook %s)", winningLong.Score, winningLong.Playbook)
				selected = append(selected, *winningLong)
			}
			if winningShort != nil {
				winningShort.Status = ARBITER_SELECTED
				winningShort.Reason = fmt.Sprintf("Arbiter selected single SHORT candidate (score %0.1f, playbook %s)", winningShort.Score, winningShort.Playbook)
				selected = append(selected, *winningShort)
			}
		}
	}

	return selected, rejected
}

// getPlaybookPriorityIndex returns priority ordering index (lower = higher priority).
func (uc *CandidateArbiterUsecase) getPlaybookPriorityIndex(playbook Playbook, dir Direction, isChaos, isChop, isRiskOff, isAltSupportive bool) int {
	if isChaos {
		switch playbook {
		case LIQUIDITY_SWEEP_REVERSAL:
			return 0
		case CROWDED_POSITIONING_SQUEEZE:
			return 1
		default:
			return 99
		}
	}

	if isChop {
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
		default:
			return 99
		}
	}

	if isRiskOff {
		if dir == SHORT {
			switch playbook {
			case TREND_PULLBACK:
				return 0
			case COMPRESSION_BREAKOUT_RETEST:
				return 1
			case LIQUIDITY_SWEEP_REVERSAL:
				return 2
			default:
				return 99
			}
		} else { // LONG
			switch playbook {
			case LIQUIDITY_SWEEP_REVERSAL:
				return 0
			case CROWDED_POSITIONING_SQUEEZE:
				return 1
			default:
				return 99
			}
		}
	}

	if isAltSupportive {
		if dir == LONG {
			switch playbook {
			case TREND_PULLBACK:
				return 0
			case COMPRESSION_BREAKOUT_RETEST:
				return 1
			case LIQUIDITY_SWEEP_REVERSAL:
				return 2
			default:
				return 99
			}
		} else { // SHORT
			switch playbook {
			case LIQUIDITY_SWEEP_REVERSAL:
				return 0
			case CROWDED_POSITIONING_SQUEEZE:
				return 1
			default:
				return 99
			}
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
	default:
		return 99
	}
}

// calculateRR extracts Risk-to-Reward ratio from TradePlan parameters
func (uc *CandidateArbiterUsecase) calculateRR(cand QuantResult) float64 {
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

// getGradeWeight converts grade inside Reason to weight for comparison
func (uc *CandidateArbiterUsecase) getGradeWeight(reason string) int {
	if strings.Contains(reason, "Grade: S+") {
		return 4
	}
	if strings.Contains(reason, "Grade: S ") || strings.Contains(reason, "Grade: S |") {
		return 3
	}
	if strings.Contains(reason, "Grade: A") {
		return 2
	}
	return 1
}
