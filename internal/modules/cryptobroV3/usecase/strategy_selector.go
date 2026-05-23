package usecase

import (
	"strings"
)

type StrategySelectorUsecase struct{}

func NewStrategySelectorUsecase() *StrategySelectorUsecase {
	return &StrategySelectorUsecase{}
}

// Select is kept for compatibility with current scanner but we add the new SelectPlaybooks.
func (uc *StrategySelectorUsecase) Select(symbol string) string {
	return "TREND_PULLBACK"
}

// SelectPlaybooks evaluates and selects possible playbooks for a candidate.
func (uc *StrategySelectorUsecase) SelectPlaybooks(
	policy MarketPolicy,
	candidate UniverseCandidate,
	prelimData MarketData,
	tech *TechnicalSnapshot,
	structure *StructureSnapshot,
) []StrategySelection {
	var selections []StrategySelection

	// Helper to check if a playbook is allowed by the active policy
	isPlaybookAllowed := func(p Playbook) bool {
		for _, ap := range policy.AllowedPlaybooks {
			if ap == p {
				return true
			}
		}
		return false
	}

	reasonStr := strings.ToUpper(policy.Reason)
	policyCtx := policy.Reason

	// 1. BTC_CHAOS regime
	if strings.Contains(reasonStr, "CHAOS") {
		// Default no trade. Only premium sweep or crowded positioning squeeze allowed.
		if policy.AllowLong {
			if isPlaybookAllowed(LIQUIDITY_SWEEP_REVERSAL) {
				selections = append(selections, StrategySelection{
					Symbol:        candidate.Symbol,
					StrategyName:  string(LIQUIDITY_SWEEP_REVERSAL),
					Direction:     LONG,
					Priority:      1,
					Reason:        "Premium lower sweep reversal allowed during BTC_CHAOS",
					PolicyContext: policyCtx,
					Tier:          candidate.Tier,
					Status:        STRATEGY_SELECTED,
				})
			}
			if isPlaybookAllowed(CROWDED_POSITIONING_SQUEEZE) {
				selections = append(selections, StrategySelection{
					Symbol:        candidate.Symbol,
					StrategyName:  string(CROWDED_POSITIONING_SQUEEZE),
					Direction:     LONG,
					Priority:      2,
					Reason:        "Premium crowded short squeeze allowed during BTC_CHAOS",
					PolicyContext: policyCtx,
					Tier:          candidate.Tier,
					Status:        STRATEGY_SELECTED,
				})
			}
		}
		if policy.AllowShort {
			if isPlaybookAllowed(LIQUIDITY_SWEEP_REVERSAL) {
				selections = append(selections, StrategySelection{
					Symbol:        candidate.Symbol,
					StrategyName:  string(LIQUIDITY_SWEEP_REVERSAL),
					Direction:     SHORT,
					Priority:      1,
					Reason:        "Premium upper sweep reversal allowed during BTC_CHAOS",
					PolicyContext: policyCtx,
					Tier:          candidate.Tier,
					Status:        STRATEGY_SELECTED,
				})
			}
			if isPlaybookAllowed(CROWDED_POSITIONING_SQUEEZE) {
				selections = append(selections, StrategySelection{
					Symbol:        candidate.Symbol,
					StrategyName:  string(CROWDED_POSITIONING_SQUEEZE),
					Direction:     SHORT,
					Priority:      2,
					Reason:        "Premium crowded long squeeze allowed during BTC_CHAOS",
					PolicyContext: policyCtx,
					Tier:          candidate.Tier,
					Status:        STRATEGY_SELECTED,
				})
			}
		}
		return selections
	}

	// 2. COMPRESSION regime
	if strings.Contains(reasonStr, "COMPRESSION") {
		// Priority compression breakout retest. No reversal unless sweep/rejection is extremely strong.
		if isPlaybookAllowed(COMPRESSION_BREAKOUT_RETEST) {
			if policy.AllowLong {
				selections = append(selections, StrategySelection{
					Symbol:        candidate.Symbol,
					StrategyName:  string(COMPRESSION_BREAKOUT_RETEST),
					Direction:     LONG,
					Priority:      1,
					Reason:        "LONG compression breakout retest prioritized during COMPRESSION",
					PolicyContext: policyCtx,
					Tier:          candidate.Tier,
					Status:        STRATEGY_SELECTED,
				})
			}
			if policy.AllowShort {
				selections = append(selections, StrategySelection{
					Symbol:        candidate.Symbol,
					StrategyName:  string(COMPRESSION_BREAKOUT_RETEST),
					Direction:     SHORT,
					Priority:      1,
					Reason:        "SHORT compression breakout retest prioritized during COMPRESSION",
					PolicyContext: policyCtx,
					Tier:          candidate.Tier,
					Status:        STRATEGY_SELECTED,
				})
			}
		}
		return selections
	}

	// 3. ALT_SUPPORTIVE regime
	if strings.Contains(reasonStr, "ALT_SUPPORTIVE") {
		// Priority LONG trend pullback. LONG breakout retest and lower sweep are also allowed.
		// SHORT side is strictly restricted to LIQUIDITY_SWEEP_REVERSAL.
		if policy.AllowLong {
			if isPlaybookAllowed(TREND_PULLBACK) {
				selections = append(selections, StrategySelection{
					Symbol:        candidate.Symbol,
					StrategyName:  string(TREND_PULLBACK),
					Direction:     LONG,
					Priority:      1,
					Reason:        "LONG trend pullback prioritized in ALT_SUPPORTIVE regime",
					PolicyContext: policyCtx,
					Tier:          candidate.Tier,
					Status:        STRATEGY_SELECTED,
				})
			}
			if isPlaybookAllowed(COMPRESSION_BREAKOUT_RETEST) {
				selections = append(selections, StrategySelection{
					Symbol:        candidate.Symbol,
					StrategyName:  string(COMPRESSION_BREAKOUT_RETEST),
					Direction:     LONG,
					Priority:      2,
					Reason:        "LONG breakout retest allowed in ALT_SUPPORTIVE regime",
					PolicyContext: policyCtx,
					Tier:          candidate.Tier,
					Status:        STRATEGY_SELECTED,
				})
			}
			if isPlaybookAllowed(LIQUIDITY_SWEEP_REVERSAL) {
				selections = append(selections, StrategySelection{
					Symbol:        candidate.Symbol,
					StrategyName:  string(LIQUIDITY_SWEEP_REVERSAL),
					Direction:     LONG,
					Priority:      3,
					Reason:        "LONG lower sweep reversal allowed in ALT_SUPPORTIVE regime",
					PolicyContext: policyCtx,
					Tier:          candidate.Tier,
					Status:        STRATEGY_SELECTED,
				})
			}
		}
		if policy.AllowShort {
			if isPlaybookAllowed(LIQUIDITY_SWEEP_REVERSAL) {
				selections = append(selections, StrategySelection{
					Symbol:        candidate.Symbol,
					StrategyName:  string(LIQUIDITY_SWEEP_REVERSAL),
					Direction:     SHORT,
					Priority:      1,
					Reason:        "SHORT liquidity sweep reversal (upper sweep) allowed in ALT_SUPPORTIVE regime",
					PolicyContext: policyCtx,
					Tier:          candidate.Tier,
					Status:        STRATEGY_SELECTED,
				})
			}
		}
		return selections
	}

	// 4. BTC_DOMINANCE regime
	if strings.Contains(reasonStr, "DOMINANCE") {
		// LONG allowed but restricted to PULLBACK_ONLY.
		// SHORT restricted to SWEEP_ONLY (LIQUIDITY_SWEEP_REVERSAL).
		if policy.AllowLong {
			if isPlaybookAllowed(TREND_PULLBACK) {
				selections = append(selections, StrategySelection{
					Symbol:        candidate.Symbol,
					StrategyName:  string(TREND_PULLBACK),
					Direction:     LONG,
					Priority:      1,
					Reason:        "LONG trend pullback allowed during BTC_DOMINANCE",
					PolicyContext: policyCtx,
					Tier:          candidate.Tier,
					Status:        STRATEGY_SELECTED,
				})
			}
		}
		if policy.AllowShort {
			if isPlaybookAllowed(LIQUIDITY_SWEEP_REVERSAL) {
				selections = append(selections, StrategySelection{
					Symbol:        candidate.Symbol,
					StrategyName:  string(LIQUIDITY_SWEEP_REVERSAL),
					Direction:     SHORT,
					Priority:      1,
					Reason:        "SHORT sweep only allowed during BTC_DOMINANCE",
					PolicyContext: policyCtx,
					Tier:          candidate.Tier,
					Status:        STRATEGY_SELECTED,
				})
			}
		}
		return selections
	}

	// 5. RISK_OFF regime
	if strings.Contains(reasonStr, "RISK_OFF") || strings.Contains(reasonStr, "BEARISH") {
		// Priority SHORT trend pullback and SHORT breakout retest.
		// LONG is strictly limited to REVERSAL_ONLY (lower sweep, range edge reversal at support, crowded short squeeze).
		if policy.AllowShort {
			if isPlaybookAllowed(TREND_PULLBACK) {
				selections = append(selections, StrategySelection{
					Symbol:        candidate.Symbol,
					StrategyName:  string(TREND_PULLBACK),
					Direction:     SHORT,
					Priority:      1,
					Reason:        "SHORT trend pullback prioritized in RISK_OFF regime",
					PolicyContext: policyCtx,
					Tier:          candidate.Tier,
					Status:        STRATEGY_SELECTED,
				})
			}
			if isPlaybookAllowed(COMPRESSION_BREAKOUT_RETEST) {
				selections = append(selections, StrategySelection{
					Symbol:        candidate.Symbol,
					StrategyName:  string(COMPRESSION_BREAKOUT_RETEST),
					Direction:     SHORT,
					Priority:      2,
					Reason:        "SHORT breakout retest allowed in RISK_OFF regime",
					PolicyContext: policyCtx,
					Tier:          candidate.Tier,
					Status:        STRATEGY_SELECTED,
				})
			}
		}
		if policy.AllowLong {
			if isPlaybookAllowed(LIQUIDITY_SWEEP_REVERSAL) {
				selections = append(selections, StrategySelection{
					Symbol:        candidate.Symbol,
					StrategyName:  string(LIQUIDITY_SWEEP_REVERSAL),
					Direction:     LONG,
					Priority:      1,
					Reason:        "LONG lower sweep reversal allowed in RISK_OFF regime",
					PolicyContext: policyCtx,
					Tier:          candidate.Tier,
					Status:        STRATEGY_SELECTED,
				})
			}
			if isPlaybookAllowed(RANGE_EDGE_REVERSAL) {
				selections = append(selections, StrategySelection{
					Symbol:        candidate.Symbol,
					StrategyName:  string(RANGE_EDGE_REVERSAL),
					Direction:     LONG,
					Priority:      2,
					Reason:        "LONG range edge reversal at support allowed in RISK_OFF regime",
					PolicyContext: policyCtx,
					Tier:          candidate.Tier,
					Status:        STRATEGY_SELECTED,
				})
			}
			if isPlaybookAllowed(CROWDED_POSITIONING_SQUEEZE) {
				selections = append(selections, StrategySelection{
					Symbol:        candidate.Symbol,
					StrategyName:  string(CROWDED_POSITIONING_SQUEEZE),
					Direction:     LONG,
					Priority:      3,
					Reason:        "LONG crowded short squeeze allowed in RISK_OFF regime",
					PolicyContext: policyCtx,
					Tier:          candidate.Tier,
					Status:        STRATEGY_SELECTED,
				})
			}
		}
		return selections
	}

	// 6. CHOP_RANGE regime
	if strings.Contains(reasonStr, "CHOP_RANGE") {
		// Priority range edge reversal and liquidity sweep. Trend pullback lowered priority.
		if isPlaybookAllowed(RANGE_EDGE_REVERSAL) {
			if policy.AllowLong {
				selections = append(selections, StrategySelection{
					Symbol:        candidate.Symbol,
					StrategyName:  string(RANGE_EDGE_REVERSAL),
					Direction:     LONG,
					Priority:      1,
					Reason:        "LONG range edge reversal at support prioritized in CHOP_RANGE",
					PolicyContext: policyCtx,
					Tier:          candidate.Tier,
					Status:        STRATEGY_SELECTED,
				})
			}
			if policy.AllowShort {
				selections = append(selections, StrategySelection{
					Symbol:        candidate.Symbol,
					StrategyName:  string(RANGE_EDGE_REVERSAL),
					Direction:     SHORT,
					Priority:      1,
					Reason:        "SHORT range edge reversal at resistance prioritized in CHOP_RANGE",
					PolicyContext: policyCtx,
					Tier:          candidate.Tier,
					Status:        STRATEGY_SELECTED,
				})
			}
		}
		if isPlaybookAllowed(LIQUIDITY_SWEEP_REVERSAL) {
			if policy.AllowLong {
				selections = append(selections, StrategySelection{
					Symbol:        candidate.Symbol,
					StrategyName:  string(LIQUIDITY_SWEEP_REVERSAL),
					Direction:     LONG,
					Priority:      2,
					Reason:        "LONG lower sweep reversal allowed in CHOP_RANGE",
					PolicyContext: policyCtx,
					Tier:          candidate.Tier,
					Status:        STRATEGY_SELECTED,
				})
			}
			if policy.AllowShort {
				selections = append(selections, StrategySelection{
					Symbol:        candidate.Symbol,
					StrategyName:  string(LIQUIDITY_SWEEP_REVERSAL),
					Direction:     SHORT,
					Priority:      2,
					Reason:        "SHORT upper sweep reversal allowed in CHOP_RANGE",
					PolicyContext: policyCtx,
					Tier:          candidate.Tier,
					Status:        STRATEGY_SELECTED,
				})
			}
		}
		if isPlaybookAllowed(TREND_PULLBACK) {
			if policy.AllowLong {
				selections = append(selections, StrategySelection{
					Symbol:        candidate.Symbol,
					StrategyName:  string(TREND_PULLBACK),
					Direction:     LONG,
					Priority:      3,
					Reason:        "LONG trend pullback (lower priority) in CHOP_RANGE",
					PolicyContext: policyCtx,
					Tier:          candidate.Tier,
					Status:        STRATEGY_SELECTED,
				})
			}
			if policy.AllowShort {
				selections = append(selections, StrategySelection{
					Symbol:        candidate.Symbol,
					StrategyName:  string(TREND_PULLBACK),
					Direction:     SHORT,
					Priority:      3,
					Reason:        "SHORT trend pullback (lower priority) in CHOP_RANGE",
					PolicyContext: policyCtx,
					Tier:          candidate.Tier,
					Status:        STRATEGY_SELECTED,
				})
			}
		}
		return selections
	}

	// 7. Fallback to evaluating allowed playbooks linearly
	for _, p := range policy.AllowedPlaybooks {
		if policy.AllowLong {
			selections = append(selections, StrategySelection{
				Symbol:        candidate.Symbol,
				StrategyName:  string(p),
				Direction:     LONG,
				Priority:      3,
				Reason:        "LONG allowed playbook by active policy constraints",
				PolicyContext: policyCtx,
				Tier:          candidate.Tier,
				Status:        STRATEGY_SELECTED,
			})
		}
		if policy.AllowShort {
			selections = append(selections, StrategySelection{
				Symbol:        candidate.Symbol,
				StrategyName:  string(p),
				Direction:     SHORT,
				Priority:      3,
				Reason:        "SHORT allowed playbook by active policy constraints",
				PolicyContext: policyCtx,
				Tier:          candidate.Tier,
				Status:        STRATEGY_SELECTED,
			})
		}
	}

	return selections
}
