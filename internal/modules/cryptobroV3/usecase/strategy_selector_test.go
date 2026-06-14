package usecase

import (
	"testing"
)

func TestSelectPlaybooksRegimes(t *testing.T) {
	selector := NewStrategySelectorUsecase()

	candidate := UniverseCandidate{
		Symbol: "SOLUSDT",
		Tier:   TierB,
		Status: UNIVERSE_PASS,
	}

	prelimData := MarketData{
		Symbol: "SOLUSDT",
	}

	tech := &TechnicalSnapshot{}
	structure := &StructureSnapshot{}

	// Helper to check if a selection is in the result
	hasSelection := func(selections []StrategySelection, playbook Playbook, dir Direction) (bool, int) {
		for _, s := range selections {
			if s.StrategyName == string(playbook) && s.Direction == dir {
				return true, s.Priority
			}
		}
		return false, 0
	}

	// 1. ALT_SUPPORTIVE
	policyAlt := MarketPolicy{
		AllowLong:  true,
		AllowShort: true,
		AllowedPlaybooks: []Playbook{
			TREND_PULLBACK,
			COMPRESSION_BREAKOUT_RETEST,
			LIQUIDITY_SWEEP_REVERSAL,
		},
		Reason: "ALT_SUPPORTIVE + BTC Bullish active - favorable conditions",
	}
	selectionsAlt := selector.SelectPlaybooks(policyAlt, candidate, prelimData, tech, structure)

	// LONG trend pullback priority 1
	if ok, p := hasSelection(selectionsAlt, TREND_PULLBACK, LONG); !ok || p != 1 {
		t.Errorf("ALT_SUPPORTIVE: Expected LONG trend pullback to have priority 1, got ok=%v, priority=%d", ok, p)
	}
	// LONG breakout retest allowed (priority 2)
	if ok, p := hasSelection(selectionsAlt, COMPRESSION_BREAKOUT_RETEST, LONG); !ok || p != 2 {
		t.Errorf("ALT_SUPPORTIVE: Expected LONG breakout retest to have priority 2, got ok=%v, priority=%d", ok, p)
	}
	// SHORT only liquidity sweep reversal allowed
	if ok, _ := hasSelection(selectionsAlt, TREND_PULLBACK, SHORT); ok {
		t.Errorf("ALT_SUPPORTIVE: SHORT trend pullback should be disabled")
	}
	if ok, _ := hasSelection(selectionsAlt, LIQUIDITY_SWEEP_REVERSAL, SHORT); !ok {
		t.Errorf("ALT_SUPPORTIVE: SHORT sweep reversal should be allowed")
	}

	// 2. RISK_OFF
	policyRiskOff := MarketPolicy{
		AllowLong:  true,
		AllowShort: true,
		AllowedPlaybooks: []Playbook{
			LIQUIDITY_SWEEP_REVERSAL,
			RANGE_EDGE_REVERSAL,
		},
		Reason: "RISK_OFF + BTC Bearish active - short bias",
	}
	selectionsRiskOff := selector.SelectPlaybooks(policyRiskOff, candidate, prelimData, tech, structure)

	// SHORT sweep reversal prioritized (priority 1)
	if ok, p := hasSelection(selectionsRiskOff, LIQUIDITY_SWEEP_REVERSAL, SHORT); !ok || p != 1 {
		t.Errorf("RISK_OFF: Expected SHORT sweep reversal to have priority 1, got ok=%v, priority=%d", ok, p)
	}
	// SHORT range edge reversal allowed (priority 2)
	if ok, p := hasSelection(selectionsRiskOff, RANGE_EDGE_REVERSAL, SHORT); !ok || p != 2 {
		t.Errorf("RISK_OFF: Expected SHORT range edge reversal to have priority 2, got ok=%v, priority=%d", ok, p)
	}
	// LONG only reversal/sweep
	if ok, _ := hasSelection(selectionsRiskOff, TREND_PULLBACK, LONG); ok {
		t.Errorf("RISK_OFF: LONG trend pullback should be disabled")
	}
	if ok, _ := hasSelection(selectionsRiskOff, LIQUIDITY_SWEEP_REVERSAL, LONG); !ok {
		t.Errorf("RISK_OFF: LONG sweep reversal should be allowed")
	}

	// 2b. RISK_OFF with SHORT trend pullback enabled by policy
	policyRiskOffTrend := MarketPolicy{
		AllowLong:  true,
		AllowShort: true,
		AllowedPlaybooks: []Playbook{
			TREND_PULLBACK,
			LIQUIDITY_SWEEP_REVERSAL,
			RANGE_EDGE_REVERSAL,
		},
		Reason: "RISK_OFF + BTC Bearish active - short bias",
	}
	selectionsRiskOffTrend := selector.SelectPlaybooks(policyRiskOffTrend, candidate, prelimData, tech, structure)
	if ok, p := hasSelection(selectionsRiskOffTrend, TREND_PULLBACK, SHORT); !ok || p != 3 {
		t.Errorf("RISK_OFF: Expected SHORT trend pullback to be selectable with priority 3 when allowed by policy, got ok=%v priority=%d", ok, p)
	}
	if ok, _ := hasSelection(selectionsRiskOffTrend, TREND_PULLBACK, LONG); ok {
		t.Errorf("RISK_OFF: LONG trend pullback should remain disabled by selector behavior")
	}

	// 3. CHOP_RANGE
	policyChop := MarketPolicy{
		AllowLong:  true,
		AllowShort: true,
		AllowedPlaybooks: []Playbook{
			TREND_PULLBACK,
			LIQUIDITY_SWEEP_REVERSAL,
			RANGE_EDGE_REVERSAL,
		},
		Reason: "CHOP_RANGE active - mean reversion only",
	}
	selectionsChop := selector.SelectPlaybooks(policyChop, candidate, prelimData, tech, structure)
	// Range edge reversal priority 1
	if ok, p := hasSelection(selectionsChop, RANGE_EDGE_REVERSAL, LONG); !ok || p != 1 {
		t.Errorf("CHOP_RANGE: Expected RANGE_EDGE_REVERSAL priority 1, got ok=%v, priority=%d", ok, p)
	}
	// Trend pullback priority 3
	if ok, p := hasSelection(selectionsChop, TREND_PULLBACK, LONG); !ok || p != 3 {
		t.Errorf("CHOP_RANGE: Expected TREND_PULLBACK priority 3, got ok=%v, priority=%d", ok, p)
	}

	// 4. COMPRESSION
	policyCompression := MarketPolicy{
		AllowLong:  true,
		AllowShort: true,
		AllowedPlaybooks: []Playbook{
			COMPRESSION_BREAKOUT_RETEST,
			LIQUIDITY_SWEEP_REVERSAL,
			RANGE_EDGE_REVERSAL,
		},
		Reason: "COMPRESSION active - breakout preferred, reversal fallback enabled",
	}
	techCompression := &TechnicalSnapshot{
		IndicatorValues: map[string]float64{
			IndicatorContraction: 1.0,
			IndicatorBBWidth:     0.05,
		},
	}
	selectionsCompression := selector.SelectPlaybooks(policyCompression, candidate, prelimData, techCompression, structure)
	// Focus on compression breakout retest when symbol has contraction evidence.
	if ok, _ := hasSelection(selectionsCompression, COMPRESSION_BREAKOUT_RETEST, LONG); !ok {
		t.Errorf("COMPRESSION: Expected COMPRESSION_BREAKOUT_RETEST to be allowed")
	}
	if ok, _ := hasSelection(selectionsCompression, LIQUIDITY_SWEEP_REVERSAL, LONG); ok {
		t.Errorf("COMPRESSION: LIQUIDITY_SWEEP_REVERSAL should be disabled when contraction evidence exists")
	}

	techNoCompression := &TechnicalSnapshot{IndicatorValues: map[string]float64{}}
	selectionsCompressionFallback := selector.SelectPlaybooks(policyCompression, candidate, prelimData, techNoCompression, structure)
	if ok, _ := hasSelection(selectionsCompressionFallback, COMPRESSION_BREAKOUT_RETEST, LONG); ok {
		t.Errorf("COMPRESSION fallback: breakout retest should not be selected without contraction evidence")
	}
	if ok, _ := hasSelection(selectionsCompressionFallback, LIQUIDITY_SWEEP_REVERSAL, LONG); !ok {
		t.Errorf("COMPRESSION fallback: expected LIQUIDITY_SWEEP_REVERSAL to be allowed")
	}
	if ok, _ := hasSelection(selectionsCompressionFallback, RANGE_EDGE_REVERSAL, LONG); !ok {
		t.Errorf("COMPRESSION fallback: expected RANGE_EDGE_REVERSAL to be allowed")
	}

	// 5. BTC_CHAOS
	policyChaos := MarketPolicy{
		AllowLong:  true,
		AllowShort: true,
		AllowedPlaybooks: []Playbook{
			TREND_PULLBACK,
			LIQUIDITY_SWEEP_REVERSAL,
			CROWDED_POSITIONING_SQUEEZE,
		},
		Reason: "BTC_CHAOS active - strict restrictions applied",
	}
	selectionsChaos := selector.SelectPlaybooks(policyChaos, candidate, prelimData, tech, structure)
	// Only premium sweep/squeeze, no pullback
	if ok, _ := hasSelection(selectionsChaos, TREND_PULLBACK, LONG); ok {
		t.Errorf("BTC_CHAOS: TREND_PULLBACK should be disabled")
	}
	if ok, _ := hasSelection(selectionsChaos, LIQUIDITY_SWEEP_REVERSAL, LONG); !ok {
		t.Errorf("BTC_CHAOS: LIQUIDITY_SWEEP_REVERSAL should be allowed")
	}

	// 6. DEFAULT fallback should not expose weak LONG default compression breakouts,
	// but can still keep the SHORT side available.
	policyDefault := MarketPolicy{
		Regime:     DEFAULT,
		AllowLong:  true,
		AllowShort: true,
		AllowedPlaybooks: []Playbook{
			TREND_PULLBACK,
			LIQUIDITY_SWEEP_REVERSAL,
			COMPRESSION_BREAKOUT_RETEST,
			RANGE_EDGE_REVERSAL,
		},
		Reason: "DEFAULT active - neutral conditions",
	}
	selectionsDefault := selector.SelectPlaybooks(policyDefault, candidate, prelimData, tech, structure)
	if ok, _ := hasSelection(selectionsDefault, COMPRESSION_BREAKOUT_RETEST, LONG); ok {
		t.Errorf("DEFAULT: LONG COMPRESSION_BREAKOUT_RETEST should be suppressed in neutral regime")
	}
	if ok, _ := hasSelection(selectionsDefault, COMPRESSION_BREAKOUT_RETEST, SHORT); !ok {
		t.Errorf("DEFAULT: SHORT COMPRESSION_BREAKOUT_RETEST should remain available when policy allows it")
	}
}
