package usecase

import (
	"strings"
	"time"
)

type watchRecheckPolicy struct {
	BoundaryMinutes     int
	MaxAgeMinutes       int
	EffectiveHorizon    time.Duration
	BatchLimit          int
	AllowedPlaybooks    map[Playbook]struct{}
	AllowedReasonTokens []string
	BlockedReasonTokens []string
}

func resolveWatchRecheckPolicy() watchRecheckPolicy {
	settings := getRuntimeSettings()

	boundaryMinutes := settings.WatchRecheckBoundaryMinutes
	if boundaryMinutes <= 0 {
		boundaryMinutes = 5
	}
	maxAgeMinutes := settings.WatchRecheckMaxAgeMinutes
	if maxAgeMinutes <= 0 {
		maxAgeMinutes = 12
	}
	batchLimit := settings.WatchRecheckBatchLimit
	if batchLimit <= 0 {
		batchLimit = 6
	}

	effectiveHorizon := time.Duration(maxAgeMinutes) * time.Minute
	if holdMinutes := settings.MonitoringMaxHoldMinutes; holdMinutes > 0 {
		holdHorizon := time.Duration(holdMinutes) * time.Minute
		if holdHorizon < effectiveHorizon {
			effectiveHorizon = holdHorizon
		}
	}

	allowedPlaybooks := make(map[Playbook]struct{})
	for _, raw := range settings.WatchRecheckAllowedPlaybooks {
		playbook := Playbook(strings.ToUpper(strings.TrimSpace(raw)))
		if playbook == "" {
			continue
		}
		allowedPlaybooks[playbook] = struct{}{}
	}
	if len(allowedPlaybooks) == 0 {
		allowedPlaybooks[TREND_PULLBACK] = struct{}{}
		allowedPlaybooks[LIQUIDITY_SWEEP_REVERSAL] = struct{}{}
		allowedPlaybooks[COMPRESSION_BREAKOUT_RETEST] = struct{}{}
	}

	allowedReasonTokens := normalizeWatchRecheckTokens(settings.WatchRecheckAllowedReasonTokens)
	if len(allowedReasonTokens) == 0 {
		allowedReasonTokens = []string{"AI DECISION IS WAIT", "WATCH_ONLY", "WAIT_RETEST", "AI CONFIDENCE", "LOCAL GATE STATUS IS LOCAL_WATCH", "AI CANDIDATE SKIPPED DUE POLICY MAXAICANDIDATES QUOTA", "M5"}
	}
	blockedReasonTokens := normalizeWatchRecheckTokens(settings.WatchRecheckBlockedReasonTokens)
	if len(blockedReasonTokens) == 0 {
		blockedReasonTokens = []string{"ACTIVE_MONITORING_EXISTS", "OPPOSITE_SIGNAL_CONFLICT", "LOWER_PRIORITY_CONFLICT", "DUPLICATE_SIGNAL_BUCKET", "SYMBOL_COOLDOWN_ACTIVE", "MAX_FINAL_EXECUTE_LIMIT"}
	}

	return watchRecheckPolicy{
		BoundaryMinutes:     boundaryMinutes,
		MaxAgeMinutes:       maxAgeMinutes,
		EffectiveHorizon:    effectiveHorizon,
		BatchLimit:          batchLimit,
		AllowedPlaybooks:    allowedPlaybooks,
		AllowedReasonTokens: allowedReasonTokens,
		BlockedReasonTokens: blockedReasonTokens,
	}
}

func normalizeWatchRecheckTokens(raw []string) []string {
	out := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, token := range raw {
		normalized := strings.ToUpper(strings.TrimSpace(token))
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func isWatchRecheckAllowedPlaybook(playbook Playbook, policy watchRecheckPolicy) bool {
	_, ok := policy.AllowedPlaybooks[playbook]
	return ok
}

func isSafeCompressionRecheck(reasonBlob string, playbook Playbook) bool {
	if playbook != COMPRESSION_BREAKOUT_RETEST {
		return false
	}
	return strings.Contains(reasonBlob, "WAIT_RETEST") || strings.Contains(reasonBlob, "WAIT_RETEST_OR_BREAKOUT_FIRST_CANDLE")
}
