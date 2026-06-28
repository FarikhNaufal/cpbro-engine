package usecase

import (
	"math"
	"strings"
	"time"
)

const (
	defaultWatchCooldownMinutes   = 30
	defaultWatchPriceToleranceBps = 50 // 0.50%
)

type watchJournalPersistDecision int

const (
	watchPersistAppend watchJournalPersistDecision = iota
	watchPersistUpdate
	watchPersistSkip
)

func resolveWatchJournalPersistence(current []WatchJournal, incoming WatchJournal, now time.Time) (watchJournalPersistDecision, WatchJournal) {
	if now.IsZero() {
		now = time.Now()
	}

	cooldownWindow := resolveWatchCooldownDuration()

	for i := range current {
		existing := current[i]
		if !isSameWatchSetup(existing, incoming) {
			continue
		}

		if isActiveWatchStatus(existing, now) {
			merged, changed := mergeWatchJournal(existing, incoming, now)
			if !changed {
				return watchPersistSkip, existing
			}
			return watchPersistUpdate, merged
		}

		if isClosedWatchStatus(existing.Status) {
			lastTouch := existing.ClosedAt
			if lastTouch.IsZero() {
				lastTouch = existing.UpdatedAt
			}
			if lastTouch.IsZero() {
				lastTouch = existing.CreatedAt
			}
			if now.Sub(lastTouch) <= cooldownWindow {
				return watchPersistSkip, existing
			}
		}
	}

	return watchPersistAppend, incoming
}

func filterWatchJournalCandidates(current []WatchJournal, probe WatchJournal) []WatchJournal {
	if len(current) == 0 {
		return []WatchJournal{}
	}

	filtered := make([]WatchJournal, 0, len(current))
	for i := range current {
		entry := current[i]
		if entry.Symbol != probe.Symbol {
			continue
		}
		if entry.Direction != probe.Direction {
			continue
		}
		if entry.Playbook != probe.Playbook {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func mergeWatchJournal(existing, incoming WatchJournal, now time.Time) (WatchJournal, bool) {
	merged := existing
	changed := false

	assignFloat := func(dst *float64, next float64, epsilon float64) {
		if math.Abs(*dst-next) > epsilon {
			*dst = next
			changed = true
		}
	}
	assignPlanFloat := func(dst *float64, next float64) {
		if !withinRelativeOrRiskTolerance(*dst, next, existing, incoming, resolveWatchPriceTolerance()/2) {
			*dst = next
			changed = true
		}
	}
	assignString := func(dst *string, next string) {
		trimmed := strings.TrimSpace(next)
		if trimmed == "" {
			return
		}
		if strings.TrimSpace(*dst) != trimmed {
			*dst = next
			changed = true
		}
	}
	assignBool := func(dst *bool, next bool) {
		if *dst != next {
			*dst = next
			changed = true
		}
	}
	assignTier := func(dst *Tier, next Tier) {
		if *dst != next && next != "" {
			*dst = next
			changed = true
		}
	}
	assignHotBool := func(dst *bool, next bool) {
		if *dst != next {
			*dst = next
			changed = true
		}
	}
	assignHotFloat := func(dst *float64, next float64) {
		if math.Abs(*dst-next) > 1e-9 {
			*dst = next
			changed = true
		}
	}
	assignHotInt := func(dst *int, next int) {
		if *dst != next {
			*dst = next
			changed = true
		}
	}

	assignPlanFloat(&merged.EntryPrice, incoming.EntryPrice)
	assignPlanFloat(&merged.StopLoss, incoming.StopLoss)
	assignPlanFloat(&merged.TP1, incoming.TP1)
	assignPlanFloat(&merged.TP2, incoming.TP2)
	assignFloat(&merged.RR, incoming.RR, 0.05)
	assignFloat(&merged.QuantScore, incoming.QuantScore, 0.15)
	assignFloat(&merged.BreakoutLevel, incoming.BreakoutLevel, 1e-9)
	assignFloat(&merged.RetestTouches, incoming.RetestTouches, 1e-9)
	assignPlanFloat(&merged.TakeProfit, incoming.TakeProfit)
	assignString(&merged.AIConfidence, incoming.AIConfidence)
	assignString(&merged.MarketRegime, incoming.MarketRegime)
	assignString(&merged.PolicyMode, incoming.PolicyMode)
	assignString(&merged.ThresholdProfileSummary, incoming.ThresholdProfileSummary)
	assignString(&merged.EntryTiming, incoming.EntryTiming)
	assignString(&merged.Timeframe, incoming.Timeframe)
	assignString(&merged.AISentiment, incoming.AISentiment)
	assignString(&merged.AIReasoning, incoming.AIReasoning)
	assignString(&merged.Reason, incoming.Reason)
	assignString(&merged.ConfigVersion, incoming.ConfigVersion)
	assignString(&merged.SchemaVersion, incoming.SchemaVersion)
	assignBool(&merged.RetestHold, incoming.RetestHold)
	assignBool(&merged.HasDerivativesEvidence, incoming.HasDerivativesEvidence)
	assignTier(&merged.Tier, incoming.Tier)
	if incoming.IsHot || merged.IsHot {
		assignHotBool(&merged.IsHot, incoming.IsHot || merged.IsHot)
		if incoming.IsHot {
			assignHotFloat(&merged.HotScore, incoming.HotScore)
			assignString(&merged.HotSource, incoming.HotSource)
			assignHotInt(&merged.HotRankType, incoming.HotRankType)
		}
		assignHotBool(&merged.HotOverlaySelected, incoming.HotOverlaySelected || merged.HotOverlaySelected)
	}

	if merged.Direction != incoming.Direction && incoming.Direction != "" {
		merged.Direction = incoming.Direction
		changed = true
	}
	if merged.Playbook != incoming.Playbook && incoming.Playbook != "" {
		merged.Playbook = incoming.Playbook
		changed = true
	}

	if changed {
		if incoming.ExpiresAt.After(merged.ExpiresAt) {
			merged.ExpiresAt = incoming.ExpiresAt
		}
		merged.UpdatedAt = now
	}

	return merged, changed
}

func isSameWatchSetup(existing, incoming WatchJournal) bool {
	if existing.Symbol != incoming.Symbol || existing.Direction != incoming.Direction || existing.Playbook != incoming.Playbook {
		return false
	}

	tolerance := resolveWatchPriceTolerance()
	return withinRelativeOrRiskTolerance(existing.EntryPrice, incoming.EntryPrice, existing, incoming, tolerance) &&
		withinRelativeOrRiskTolerance(existing.StopLoss, incoming.StopLoss, existing, incoming, tolerance) &&
		withinRelativeOrRiskTolerance(existing.TP2, incoming.TP2, existing, incoming, tolerance*1.5)
}

func withinRelativeOrRiskTolerance(left, right float64, existing, incoming WatchJournal, relTolerance float64) bool {
	if left == 0 || right == 0 {
		return left == right
	}

	maxRef := math.Max(math.Abs(left), math.Abs(right))
	absTolerance := maxRef * relTolerance
	riskDistance := math.Max(
		math.Abs(existing.EntryPrice-existing.StopLoss),
		math.Abs(incoming.EntryPrice-incoming.StopLoss),
	)
	if riskDistance > 0 {
		riskTolerance := riskDistance * 0.20
		riskCap := maxRef * (relTolerance * 2)
		absTolerance = math.Max(absTolerance, math.Min(riskTolerance, riskCap))
	}

	return math.Abs(left-right) <= absTolerance
}

func isActiveWatchStatus(journal WatchJournal, now time.Time) bool {
	expired := !journal.ExpiresAt.IsZero() && now.After(journal.ExpiresAt)
	switch journal.Status {
	case WATCH_MONITORING:
		return !expired
	case VIRTUAL_TP1_HIT:
		return !expired
	default:
		return false
	}
}

func isClosedWatchStatus(status Status) bool {
	switch status {
	case VIRTUAL_TP2_HIT, VIRTUAL_SL_HIT, VIRTUAL_EXPIRED, WATCH_PROMOTED, WATCH_INVALIDATED, WATCH_EXPIRED:
		return true
	default:
		return false
	}
}

func resolveWatchCooldownDuration() time.Duration {
	minutes := getRuntimeSettings().WatchCooldownMinutes
	if minutes > 0 {
		return time.Duration(minutes) * time.Minute
	}
	return time.Duration(defaultWatchCooldownMinutes) * time.Minute
}

func resolveWatchPriceTolerance() float64 {
	bps := getRuntimeSettings().WatchDedupPriceToleranceBps
	if bps > 0 {
		return float64(bps) / 10000.0
	}
	return float64(defaultWatchPriceToleranceBps) / 10000.0
}
