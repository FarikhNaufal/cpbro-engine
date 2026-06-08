package usecase

import (
	"fmt"
	"sort"
	"strings"

	"cpbro-engine/internal/modules/cryptobroV3/entity"
)

const (
	funnelStageUniverseReject    = "universe_reject"
	funnelStagePipelineDrop      = "pipeline_drop"
	funnelStageEligibilityReject = "eligibility_reject"
	funnelStageArbiterReject     = "arbiter_reject"
	funnelStageLocalWatch        = "local_watch"
	funnelStageLocalReject       = "local_reject"
	funnelStageAIWait            = "ai_wait"
	funnelStageAIReject          = "ai_reject"
	funnelStageAIError           = "ai_error"
	funnelStageFinalWatch        = "final_watch"
	funnelStageFinalReject       = "final_reject"
)

var funnelStageOrder = []string{
	funnelStageUniverseReject,
	funnelStagePipelineDrop,
	funnelStageEligibilityReject,
	funnelStageArbiterReject,
	funnelStageLocalWatch,
	funnelStageLocalReject,
	funnelStageAIWait,
	funnelStageAIReject,
	funnelStageAIError,
	funnelStageFinalWatch,
	funnelStageFinalReject,
}

type funnelSummaryAccumulator struct {
	stageReasons map[string]map[string]int
}

type playbookBlockerAccumulator struct {
	byPlaybook map[string]map[string]map[string]int
}

func newFunnelSummaryAccumulator() *funnelSummaryAccumulator {
	return &funnelSummaryAccumulator{
		stageReasons: make(map[string]map[string]int),
	}
}

func newPlaybookBlockerAccumulator() *playbookBlockerAccumulator {
	return &playbookBlockerAccumulator{
		byPlaybook: make(map[string]map[string]map[string]int),
	}
}

func (a *funnelSummaryAccumulator) Add(stage string, rawReason string) {
	reason := normalizeFunnelReason(stage, rawReason)
	if reason == "" {
		reason = "Unspecified"
	}
	if _, ok := a.stageReasons[stage]; !ok {
		a.stageReasons[stage] = make(map[string]int)
	}
	a.stageReasons[stage][reason]++
}

func (a *funnelSummaryAccumulator) Build() []entity.FunnelStageSummary {
	out := make([]entity.FunnelStageSummary, 0)
	for _, stage := range funnelStageOrder {
		reasons, ok := a.stageReasons[stage]
		if !ok || len(reasons) == 0 {
			continue
		}
		items := make([]entity.FunnelReasonCount, 0, len(reasons))
		total := 0
		for reason, count := range reasons {
			total += count
			items = append(items, entity.FunnelReasonCount{
				Reason: reason,
				Count:  count,
			})
		}
		sort.Slice(items, func(i, j int) bool {
			if items[i].Count == items[j].Count {
				return items[i].Reason < items[j].Reason
			}
			return items[i].Count > items[j].Count
		})
		if len(items) > 8 {
			items = items[:8]
		}
		out = append(out, entity.FunnelStageSummary{
			Stage:   stage,
			Total:   total,
			Reasons: items,
		})
	}
	return out
}

func (a *playbookBlockerAccumulator) Add(playbook Playbook, stage string, rawReason string) {
	playbookName := strings.TrimSpace(string(playbook))
	if playbookName == "" {
		return
	}
	reason := normalizeFunnelReason(stage, rawReason)
	if reason == "" {
		reason = "Unspecified"
	}
	if _, ok := a.byPlaybook[playbookName]; !ok {
		a.byPlaybook[playbookName] = make(map[string]map[string]int)
	}
	if _, ok := a.byPlaybook[playbookName][stage]; !ok {
		a.byPlaybook[playbookName][stage] = make(map[string]int)
	}
	a.byPlaybook[playbookName][stage][reason]++
}

func (a *playbookBlockerAccumulator) Build() []entity.PlaybookBlockerSummary {
	out := make([]entity.PlaybookBlockerSummary, 0, len(a.byPlaybook))
	for playbook, stages := range a.byPlaybook {
		stageSummaries := make([]entity.FunnelStageSummary, 0)
		total := 0
		for _, stage := range funnelStageOrder {
			reasons, ok := stages[stage]
			if !ok || len(reasons) == 0 {
				continue
			}
			items := make([]entity.FunnelReasonCount, 0, len(reasons))
			stageTotal := 0
			for reason, count := range reasons {
				stageTotal += count
				items = append(items, entity.FunnelReasonCount{
					Reason: reason,
					Count:  count,
				})
			}
			sort.Slice(items, func(i, j int) bool {
				if items[i].Count == items[j].Count {
					return items[i].Reason < items[j].Reason
				}
				return items[i].Count > items[j].Count
			})
			if len(items) > 5 {
				items = items[:5]
			}
			stageSummaries = append(stageSummaries, entity.FunnelStageSummary{
				Stage:   stage,
				Total:   stageTotal,
				Reasons: items,
			})
			total += stageTotal
		}
		out = append(out, entity.PlaybookBlockerSummary{
			Playbook: playbook,
			Total:    total,
			Stages:   stageSummaries,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Total == out[j].Total {
			return out[i].Playbook < out[j].Playbook
		}
		return out[i].Total > out[j].Total
	})
	return out
}

func normalizeFunnelReason(stage string, raw string) string {
	reason := strings.TrimSpace(raw)
	if reason == "" {
		return ""
	}

	switch stage {
	case funnelStageUniverseReject:
		return reason
	case funnelStagePipelineDrop:
		return normalizePipelineDropReason(reason)
	case funnelStageEligibilityReject:
		return normalizeEligibilityReason(reason)
	case funnelStageArbiterReject:
		return normalizeArbiterReason(reason)
	case funnelStageLocalWatch, funnelStageLocalReject:
		return normalizeLocalGateReason(reason)
	case funnelStageAIWait, funnelStageAIReject, funnelStageAIError:
		return normalizeAIReason(reason)
	case funnelStageFinalWatch, funnelStageFinalReject:
		return normalizeFinalGateReason(reason)
	default:
		return reason
	}
}

func normalizePipelineDropReason(reason string) string {
	switch {
	case strings.Contains(reason, "deferred by market data prefetch limit"):
		return "Deferred by market data prefetch limit"
	case strings.Contains(reason, "failed to fetch market data"):
		return "Failed to fetch market data"
	case strings.Contains(reason, "m15 candles empty"):
		return "M15 candles empty"
	case strings.Contains(reason, "raw candles are stale"):
		return "Raw candles are stale"
	case strings.Contains(reason, "failed to enrich market data"):
		return "Failed to enrich market data"
	case strings.Contains(reason, "insufficient closed m15 candles for quant context"):
		return "Insufficient closed M15 candles for quant context"
	default:
		return reason
	}
}

func normalizeEligibilityReason(reason string) string {
	switch {
	case strings.Contains(reason, "Trend alignment failed:"):
		return "Trend alignment failed"
	case strings.Contains(reason, "outside the value area EMA band"):
		return "Price outside value area EMA band"
	case strings.Contains(reason, "RSI overextended"):
		return "RSI overextended"
	case strings.Contains(reason, "below minimum requirement"):
		return "ADX below minimum requirement"
	case strings.Contains(reason, "Sweep low invalid:"):
		return "Sweep low invalid"
	case strings.Contains(reason, "Sweep high invalid:"):
		return "Sweep high invalid"
	case strings.Contains(reason, "disabled under"):
		return normalizeDisabledByPolicyReason(reason)
	case strings.Contains(reason, "No valid breakout close outside Bollinger Bands"):
		return "No valid breakout close outside Bollinger Bands"
	case strings.Contains(reason, "Range edge reversal invalid: strong trending expansion"):
		return "Range edge reversal invalid due to ADX expansion"
	default:
		return reason
	}
}

func normalizeArbiterReason(reason string) string {
	switch {
	case strings.HasPrefix(reason, "Arbiter reject: Tier "):
		return "Tier not allowed by policy"
	case strings.HasPrefix(reason, "Arbiter reject: Playbook "):
		return "Playbook not allowed by policy"
	case strings.Contains(reason, "LONG trades disallowed by policy"):
		return "LONG trades disallowed by policy"
	case strings.Contains(reason, "SHORT trades disallowed by policy"):
		return "SHORT trades disallowed by policy"
	case strings.Contains(reason, "PULLBACK_ONLY blocks playbook"):
		return "Policy mode PULLBACK_ONLY blocks playbook"
	case strings.Contains(reason, "SWEEP_ONLY blocks playbook"):
		return "Policy mode SWEEP_ONLY blocks playbook"
	case strings.Contains(reason, "BREAKOUT_RETEST_ONLY blocks playbook"):
		return "Policy mode BREAKOUT_RETEST_ONLY blocks playbook"
	case strings.Contains(reason, "REVERSAL_ONLY blocks playbook"):
		return "Policy mode REVERSAL_ONLY blocks playbook"
	case strings.Contains(reason, "Same direction tie-breaker:"):
		return "Same-direction tie-breaker loser"
	case strings.Contains(reason, "Opposing conflict: score difference too small"):
		return "Opposing conflict with score difference too small"
	case strings.Contains(reason, "Opposing conflict: LONG score"):
		return "Opposing conflict lost to stronger LONG setup"
	case strings.Contains(reason, "Opposing conflict: SHORT score"):
		return "Opposing conflict lost to stronger SHORT setup"
	case strings.Contains(reason, "BTCChaos conflict:"):
		return "BTCChaos opposing conflict"
	case strings.Contains(reason, "BTCChaos: only S+ Sweep and Squeeze allowed"):
		return "BTCChaos requires S+ Sweep or Squeeze"
	default:
		return reason
	}
}

func normalizeLocalGateReason(reason string) string {
	switch {
	case strings.Contains(reason, "Risk-to-Reward ratio") && strings.Contains(reason, "below policy requirement"):
		return "Risk-to-Reward ratio below policy requirement"
	case strings.Contains(reason, "Risk-to-Reward ratio") && strings.Contains(reason, "below requirement"):
		return "Risk-to-Reward ratio below requirement"
	case strings.Contains(reason, "below minimum AI score limit"):
		return "Score below minimum AI score limit"
	case strings.Contains(reason, "ADX") && strings.Contains(reason, "below execution threshold"):
		return "ADX below execution threshold"
	case strings.Contains(reason, "high trend expansion detected"):
		return "High trend expansion detected"
	case strings.Contains(reason, "lacks required volume spike confirmation"):
		return "Volume spike confirmation missing"
	case strings.Contains(reason, "lacks required wick rejection confirmation"):
		return "Wick rejection confirmation missing"
	case strings.Contains(reason, "WAIT_RETEST_OR_BREAKOUT_FIRST_CANDLE"):
		return "Wait for retest after breakout first candle"
	case strings.Contains(reason, "retest quality") && strings.Contains(reason, "below minimum threshold"):
		return "Retest quality below minimum threshold"
	case strings.Contains(reason, "range clarity") && strings.Contains(reason, "below minimum threshold"):
		return "Range clarity below minimum threshold"
	case strings.Contains(reason, "lacks required funding/OI crowding evidence"):
		return "Funding or OI crowding evidence missing"
	case strings.Contains(reason, "Technical setup indicators not fully met"):
		return "Technical setup indicators not fully met"
	case strings.Contains(reason, "LongMode is PULLBACK_ONLY; premium reversal playbook"):
		return "Premium reversal downgraded to watch under PULLBACK_ONLY"
	case strings.Contains(reason, "LongMode is PULLBACK_ONLY; playbook"):
		return "PULLBACK_ONLY blocks playbook"
	case strings.Contains(reason, "ShortMode is SWEEP_ONLY;"):
		return "SWEEP_ONLY blocks non-sweep short setup"
	case strings.Contains(reason, "BTCChaos: score"):
		return "BTCChaos score below execution threshold"
	case strings.Contains(reason, "BTCChaos: playbook"):
		return "BTCChaos playbook not permitted"
	case strings.Contains(reason, "BTCChaos: Risk-to-Reward ratio"):
		return "BTCChaos risk-to-reward below requirement"
	case strings.Contains(reason, "wick ratio") && strings.Contains(reason, "below minimum threshold"):
		return "Wick ratio below minimum threshold"
	case strings.Contains(reason, "CROWDING_EVIDENCE_MISSING"):
		return "Crowding evidence missing"
	case strings.Contains(reason, "LONG trades disabled"):
		return "LONG trades disabled by policy"
	case strings.Contains(reason, "SHORT trades disabled"):
		return "SHORT trades disabled by policy"
	default:
		return reason
	}
}

func normalizeAIReason(reason string) string {
	switch {
	case strings.Contains(reason, "AI_SKIPPED"):
		return "AI skipped by MaxAICandidates quota"
	case strings.Contains(reason, "AI_AUDIT_DISABLED"):
		return "AI audit disabled by configuration"
	case strings.Contains(strings.ToUpper(reason), "AI_ERROR"):
		return "AI error"
	case reason == "REJECT":
		return "AI decision REJECT"
	case reason == "WAIT":
		return "AI decision WAIT"
	default:
		return reason
	}
}

func normalizeFinalGateReason(reason string) string {
	switch {
	case strings.Contains(reason, "AI decision is WAIT"):
		return "AI decision is WAIT"
	case strings.Contains(reason, "AI decision is REJECT"):
		return "AI decision is REJECT"
	case strings.Contains(reason, "AI confidence") && strings.Contains(reason, "below required"):
		return "AI confidence below required threshold"
	case strings.Contains(reason, "Staleness status is LATE"):
		return "Staleness status is LATE"
	case strings.Contains(reason, "Staleness status is MISSED"):
		return "Staleness status is MISSED"
	case strings.Contains(reason, "Staleness status") && strings.Contains(reason, "is not FRESH"):
		return "Staleness status is not FRESH"
	case strings.Contains(reason, "Quant score") && strings.Contains(reason, "below minimum execute score"):
		return "Quant score below minimum execute score"
	case strings.Contains(reason, "Actual RR") && strings.Contains(reason, "below minimum required RR"):
		return "Actual RR below minimum required RR"
	case strings.Contains(reason, "ADX") && strings.Contains(reason, "below required threshold"):
		return "ADX below required threshold"
	case strings.Contains(reason, "High trend expansion detected"):
		return "High trend expansion detected"
	case strings.Contains(reason, "Concurrent active signals count"):
		return "Concurrent active signals exceed policy limit"
	case strings.Contains(reason, "Symbol 24h dump"):
		return "24h dump exceeds LONG directional limit"
	case strings.Contains(reason, "Symbol 24h pump"):
		return "24h pump exceeds SHORT directional limit"
	case strings.Contains(reason, "SL distance") && strings.Contains(reason, "minimum required"):
		return "SL distance below minimum ATR requirement"
	case strings.Contains(reason, "Crowded squeeze score"):
		return "Crowded squeeze score below mandatory threshold"
	case strings.Contains(reason, "ACTIVE_MONITORING_EXISTS"):
		return "Active monitoring exists"
	case strings.Contains(reason, "OPPOSITE_SIGNAL_CONFLICT"):
		return "Opposite signal conflict"
	case strings.Contains(reason, "LOWER_PRIORITY_CONFLICT"):
		return "Lower priority conflict"
	case strings.Contains(reason, "BTC_CHAOS_LIMIT"):
		return "BTC chaos final execute limit reached"
	default:
		return reason
	}
}

func normalizeDisabledByPolicyReason(reason string) string {
	switch {
	case strings.Contains(reason, "REVERSAL_ONLY"):
		return "Disabled by REVERSAL_ONLY policy mode"
	case strings.Contains(reason, "SWEEP_ONLY"):
		return "Disabled by SWEEP_ONLY policy mode"
	case strings.Contains(reason, "BREAKOUT_RETEST_ONLY"):
		return "Disabled by BREAKOUT_RETEST_ONLY policy mode"
	case strings.Contains(reason, "PULLBACK_ONLY"):
		return "Disabled by PULLBACK_ONLY policy mode"
	default:
		return reason
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func buildTopFunnelBlockers(stages []entity.FunnelStageSummary, limit int) []string {
	if limit <= 0 {
		limit = 5
	}
	out := make([]string, 0, limit)
	for _, stage := range stages {
		if len(stage.Reasons) == 0 {
			continue
		}
		top := stage.Reasons[0]
		out = append(out, fmt.Sprintf("%s: %s (%d)", stage.Stage, top.Reason, top.Count))
		if len(out) >= limit {
			break
		}
	}
	return out
}

func formatFunnelLogSummary(stages []entity.FunnelStageSummary, limit int) string {
	if limit <= 0 {
		limit = 4
	}
	parts := make([]string, 0, limit)
	for _, stage := range stages {
		if len(stage.Reasons) == 0 {
			continue
		}
		top := stage.Reasons[0]
		parts = append(parts, fmt.Sprintf("%s=%d[%s]", stage.Stage, stage.Total, top.Reason))
		if len(parts) >= limit {
			break
		}
	}
	return strings.Join(parts, "; ")
}
