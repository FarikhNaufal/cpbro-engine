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
		},
		Reason: "COMPRESSION active - awaiting breakout retest confirmation",
	}
	selectionsCompression := selector.SelectPlaybooks(policyCompression, candidate, prelimData, tech, structure)
	// Focus on compression breakout retest, no sweep reversal
	if ok, _ := hasSelection(selectionsCompression, COMPRESSION_BREAKOUT_RETEST, LONG); !ok {
		t.Errorf("COMPRESSION: Expected COMPRESSION_BREAKOUT_RETEST to be allowed")
	}
	if ok, _ := hasSelection(selectionsCompression, LIQUIDITY_SWEEP_REVERSAL, LONG); ok {
		t.Errorf("COMPRESSION: LIQUIDITY_SWEEP_REVERSAL should be disabled")
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
}
