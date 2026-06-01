package usecase

import (
	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"math"
	"sort"
	"strings"
	"time"
)

type ConflictResolverUsecase struct{}

func NewConflictResolverUsecase() *ConflictResolverUsecase {
	return &ConflictResolverUsecase{}
}

// Resolve is kept for backward compatibility.
func (uc *ConflictResolverUsecase) Resolve(quantDirection Direction, aiSentiment string) Direction {
	if quantDirection == LONG && aiSentiment == "BEARISH" {
		return WAIT
	}
	if quantDirection == SHORT && aiSentiment == "BULLISH" {
		return WAIT
	}
	return quantDirection
}

// ResolveConflicts resolves conflicts between trade plan decisions, applies cooldowns, and limits execution rate.
func (uc *ConflictResolverUsecase) ResolveConflicts(
	decisions []FinalDecision,
	history []dto.SignalResponse,
	active []SignalJournal,
	policy MarketPolicy,
) ([]FinalDecision, []dto.SignalResponse) {
	if len(decisions) == 0 {
		return decisions, history
	}

	now := time.Now()

	// 1. Group decisions by symbol to resolve symbol conflicts
	bySymbol := make(map[string][]FinalDecision)
	for _, d := range decisions {
		bySymbol[d.Symbol] = append(bySymbol[d.Symbol], d)
	}

	var resolvedDecisions []FinalDecision
	for _, cands := range bySymbol {
		resolvedDecisions = append(resolvedDecisions, uc.resolveSymbolConflict(cands, policy)...)
	}

	// 2. Enforce Active Monitoring constraint (Rule 3) and Cooldown limits (Rule 4, 5)
	for i, d := range resolvedDecisions {
		if d.Status != FINAL_EXECUTE || !d.IsExecutable {
			continue
		}

		// Rule 3: Jangan kirim signal baru jika symbol sama masih MONITORING aktif
		hasActiveMonitoring := false
		for _, act := range active {
			if act.Symbol == d.Symbol && act.Status == MONITORING {
				hasActiveMonitoring = true
				break
			}
		}
		if hasActiveMonitoring {
			resolvedDecisions[i].Status = FINAL_WATCH
			resolvedDecisions[i].IsExecutable = false
			resolvedDecisions[i].WatchReason = "ACTIVE_MONITORING_EXISTS"
			resolvedDecisions[i].Reason = "ACTIVE_MONITORING_EXISTS"
			continue
		}

		// Rule 4 & 5: Cooldown check
		cooldownMins := uc.GetDynamicCooldownMinutes(d.Score, policy)
		cooldownDuration := time.Duration(cooldownMins) * time.Minute
		cooldownActive := false
		cooldownReason := ""

		// Check active journal (active monitoring or recently closed)
		for _, act := range active {
			if act.Symbol == d.Symbol && act.Direction == d.Direction {
				// We match playbook if reason or strategy matches. Since journal has no strategy field,
				// we assume playbook match if symbol and direction match.
				elapsed := now.Sub(act.CreatedAt)
				if elapsed < cooldownDuration {
					priceDiff := math.Abs(d.EntryPrice-act.EntryPrice) / act.EntryPrice
					if priceDiff < 0.01 {
						cooldownActive = true
						cooldownReason = "DUPLICATE_SIGNAL_BUCKET"
						break
					} else {
						cooldownActive = true
						cooldownReason = "SYMBOL_COOLDOWN_ACTIVE"
						break
					}
				}
			}
		}

		// Check history
		if !cooldownActive {
			for _, hist := range history {
				if hist.Symbol == d.Symbol && hist.Direction == string(d.Direction) && strings.EqualFold(hist.Strategy, string(d.Playbook)) {
					elapsed := now.Sub(hist.ReconciledTime)
					if elapsed < cooldownDuration {
						priceDiff := math.Abs(d.EntryPrice-hist.TriggerPrice) / hist.TriggerPrice
						if priceDiff < 0.01 {
							cooldownActive = true
							cooldownReason = "DUPLICATE_SIGNAL_BUCKET"
							break
						} else {
							cooldownActive = true
							cooldownReason = "SYMBOL_COOLDOWN_ACTIVE"
							break
						}
					}
				}
			}
		}

		if cooldownActive {
			resolvedDecisions[i].Status = FINAL_WATCH
			resolvedDecisions[i].IsExecutable = false
			resolvedDecisions[i].WatchReason = cooldownReason
			resolvedDecisions[i].Reason = cooldownReason
		}
	}

	// 3. Enforce max concurrent execution limits (Rule 6, 7)
	var execs []FinalDecision
	var nonExecs []FinalDecision
	for _, d := range resolvedDecisions {
		if d.Status == FINAL_EXECUTE && d.IsExecutable {
			execs = append(execs, d)
		} else {
			nonExecs = append(nonExecs, d)
		}
	}

	limit := policy.MaxFinalExecute
	if limit <= 0 {
		limit = 3
	}

	regime := policy.EffectiveRegime()
	isChaos := regime == BTC_CHAOS
	isChop := regime == CHOP_RANGE
	isRiskOff := regime == RISK_OFF

	if isChaos {
		limit = 1
	} else if isChop {
		if limit > 2 {
			limit = 2
		}
	} else if isRiskOff {
		if limit > 3 {
			limit = 3
		}
	}

	if len(execs) > limit {
		uc.sortDecisions(execs, policy)

		// Keep the first `limit` items as FINAL_EXECUTE. Downgrade the rest.
		for i := limit; i < len(execs); i++ {
			execs[i].Status = FINAL_WATCH
			execs[i].IsExecutable = false
			if isChaos {
				execs[i].WatchReason = "BTC_CHAOS_LIMIT"
				execs[i].Reason = "BTC_CHAOS_LIMIT"
			} else {
				execs[i].WatchReason = "MAX_FINAL_EXECUTE_LIMIT"
				execs[i].Reason = "MAX_FINAL_EXECUTE_LIMIT"
			}
		}
	}

	// Reassemble decisions
	finalDecisions := append(execs, nonExecs...)

	// Construct updated history
	updatedHistory := make([]dto.SignalResponse, len(history))
	copy(updatedHistory, history)

	for _, d := range finalDecisions {
		if d.Status == FINAL_EXECUTE && d.IsExecutable {
			updatedHistory = append(updatedHistory, dto.SignalResponse{
				Symbol:         d.Symbol,
				Direction:      string(d.Direction),
				Timeframe:      "M15",
				TriggerPrice:   d.EntryPrice,
				StopLoss:       d.StopLoss,
				TakeProfit:     d.TakeProfit,
				Score:          d.Score,
				Strategy:       string(d.Playbook),
				AISentiment:    d.AIConfidence,
				IsFinalExecute: true,
				ReconciledTime: now,
				Status:         "PENDING",
			})
		}
	}

	return finalDecisions, updatedHistory
}

// resolveSymbolConflict resolves conflicts for a single symbol
func (uc *ConflictResolverUsecase) resolveSymbolConflict(cands []FinalDecision, policy MarketPolicy) []FinalDecision {
	if len(cands) <= 1 {
		return cands
	}

	var execs []FinalDecision
	var others []FinalDecision
	for _, c := range cands {
		if c.Status == FINAL_EXECUTE && c.IsExecutable {
			execs = append(execs, c)
		} else {
			others = append(others, c)
		}
	}

	if len(execs) <= 1 {
		return cands
	}

	// Sort execution candidates by quality to determine the best candidate
	uc.sortDecisions(execs, policy)

	best := execs[0]
	allDowngraded := false
	downgradeReason := ""

	// Check if other signals conflict with the best signal
	for i := 1; i < len(execs); i++ {
		other := execs[i]
		// Score difference check only applies if AI confidence levels are equal
		if best.AIConfidence == other.AIConfidence {
			scoreDiff := math.Abs(best.Score - other.Score)
			if scoreDiff < 0.5 {
				if best.Direction != other.Direction {
					allDowngraded = true
					downgradeReason = "DIRECTION_CONFLICT_SCORE_TOO_CLOSE"
					break
				}
			}
		}
	}

	if allDowngraded {
		for i := range execs {
			execs[i].Status = FINAL_WATCH
			execs[i].IsExecutable = false
			execs[i].WatchReason = downgradeReason
			execs[i].Reason = downgradeReason
		}
	} else {
		// Keep only the best candidate, downgrade the rest
		for i := 1; i < len(execs); i++ {
			execs[i].Status = FINAL_WATCH
			execs[i].IsExecutable = false
			if execs[i].Direction != best.Direction {
				execs[i].WatchReason = "OPPOSITE_SIGNAL_CONFLICT"
				execs[i].Reason = "OPPOSITE_SIGNAL_CONFLICT"
			} else {
				execs[i].WatchReason = "LOWER_PRIORITY_CONFLICT"
				execs[i].Reason = "LOWER_PRIORITY_CONFLICT"
			}
		}
	}

	return append(execs, others...)
}

// GetDynamicCooldownMinutes maps score and market policy to minutes of cooldown.
func (uc *ConflictResolverUsecase) GetDynamicCooldownMinutes(score float64, policy MarketPolicy) int {
	regime := policy.EffectiveRegime()
	cooldown := 10 // Default
	if regime == LOW_VOL {
		cooldown = 15
	} else if regime == HIGH_VOL {
		cooldown = 5
	}

	// S/S+ grades (score >= 7.8) can get 2 mins cooldown if policy is not chaos
	grade := getGrade(score)
	if (grade == "S" || grade == "S+") && regime != BTC_CHAOS {
		cooldown = 2
	}

	// Chaos mode enforces minimum 10 minutes
	if regime == BTC_CHAOS && cooldown < 10 {
		cooldown = 10
	}

	return cooldown
}

// sortDecisions sorts decisions using the 5 priority levels
func (uc *ConflictResolverUsecase) sortDecisions(cands []FinalDecision, policy MarketPolicy) {
	regime := string(policy.EffectiveRegime())
	sort.Slice(cands, func(i, j int) bool {
		c1 := cands[i]
		c2 := cands[j]

		// 1. AI confidence
		w1 := getConfidenceWeight(c1.AIConfidence)
		w2 := getConfidenceWeight(c2.AIConfidence)
		if w1 != w2 {
			return w1 > w2
		}

		// 2. Score desc
		if c1.Score != c2.Score {
			return c1.Score > c2.Score
		}

		// 3. RR desc
		if c1.RR != c2.RR {
			return c1.RR > c2.RR
		}

		// 4. Playbook priority
		prio1 := uc.getPlaybookPriorityIndex(c1.Playbook, regime)
		prio2 := uc.getPlaybookPriorityIndex(c2.Playbook, regime)
		if prio1 != prio2 {
			return prio1 < prio2
		}

		// 5. Tier priority (A > B > C)
		t1 := getTierPriorityWeight(c1.Tier)
		t2 := getTierPriorityWeight(c2.Tier)
		return t1 > t2
	})
}

// getPlaybookPriorityIndex maps playbook priority index based on the regime
func (uc *ConflictResolverUsecase) getPlaybookPriorityIndex(playbook Playbook, regime string) int {
	regimeUpper := strings.ToUpper(regime)
	if strings.Contains(regimeUpper, "CHOP") {
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
	} else if strings.Contains(regimeUpper, "CHAOS") {
		switch playbook {
		case LIQUIDITY_SWEEP_REVERSAL:
			return 0
		case CROWDED_POSITIONING_SQUEEZE:
			return 1
		default:
			return 99
		}
	} else if strings.Contains(regimeUpper, "RISK_OFF") {
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
	} else if strings.Contains(regimeUpper, "ALT_SUPPORTIVE") {
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

func getGrade(score float64) string {
	if score >= 8.5 {
		return "S+"
	}
	if score >= 7.8 {
		return "S"
	}
	if score >= 7.0 {
		return "A"
	}
	return "B"
}

func getConfidenceWeight(conf string) int {
	switch strings.ToUpper(conf) {
	case "HIGH":
		return 3
	case "MEDIUM":
		return 2
	case "LOW":
		return 1
	default:
		return 0
	}
}

func getTierPriorityWeight(tier Tier) int {
	switch tier {
	case TierA:
		return 3
	case TierB:
		return 2
	case TierC:
		return 1
	default:
		return 0
	}
}
