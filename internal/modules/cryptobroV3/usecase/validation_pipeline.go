package usecase

import (
	"fmt"
	"math"
	"strings"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

// GateDecision represents the outcome of a gate validation.
type GateDecision struct {
	Pass   bool
	Reject []string
	Watch  []string
}

// GateLayer identifies which gate produced a reject/watch entry.
type GateLayer string

const (
	LayerPolicy    GateLayer = "POLICY"
	LayerPlaybook  GateLayer = "PLAYBOOK"
	LayerTier      GateLayer = "TIER"
	LayerTradePlan GateLayer = "TRADE_PLAN"
	LayerRR        GateLayer = "RR"
	LayerADX       GateLayer = "ADX"
	LayerVolume    GateLayer = "VOLUME"
	LayerRejection GateLayer = "REJECTION"
	LayerConfirm   GateLayer = "CONFIRMATION"
	LayerRetest    GateLayer = "RETEST"
	LayerCrowding  GateLayer = "CROWDING"
	LayerIndicator GateLayer = "INDICATOR"
)

// ValidateCorePolicy checks policy-level constraints shared between Local and Final gates.
// Returns reject/watch reasons.
func ValidateCorePolicy(quant QuantResult, policy MarketPolicy) GateDecision {
	d := GateDecision{Pass: true}

	// Direction validation
	if quant.Direction != LONG && quant.Direction != SHORT {
		d.Pass = false
		d.Reject = append(d.Reject, fmt.Sprintf("Direction %s is invalid; must be LONG or SHORT", quant.Direction))
		return d
	}

	// Policy direction permissions
	if quant.Direction == LONG && !policy.AllowLong {
		d.Pass = false
		d.Reject = append(d.Reject, "LONG trades disallowed by MarketPolicy AllowLong constraint")
	}
	if quant.Direction == SHORT && !policy.AllowShort {
		d.Pass = false
		d.Reject = append(d.Reject, "SHORT trades disallowed by MarketPolicy AllowShort constraint")
	}

	// Allowed playbook validation
	playbookAllowed := false
	for _, p := range policy.AllowedPlaybooks {
		if p == quant.Playbook {
			playbookAllowed = true
			break
		}
	}
	if !playbookAllowed {
		d.Pass = false
		d.Reject = append(d.Reject, fmt.Sprintf("Playbook %s is not permitted in AllowedPlaybooks", quant.Playbook))
	}

	// Tier validation
	tierAllowed := false
	for _, t := range policy.AllowedTiers {
		if t == quant.Tier {
			tierAllowed = true
			break
		}
	}
	if !tierAllowed {
		d.Pass = false
		d.Reject = append(d.Reject, fmt.Sprintf("Tier %s is not permitted in AllowedTiers", quant.Tier))
	}

	return d
}

// ValidateTradePlan checks TradePlan structural validity.
func ValidateTradePlan(quant QuantResult) GateDecision {
	d := GateDecision{Pass: true}
	entry := quant.TradePlan.EntryPrice
	tp := quant.TradePlan.TakeProfit
	sl := quant.TradePlan.StopLoss

	if entry <= 0 {
		d.Pass = false
		d.Reject = append(d.Reject, fmt.Sprintf("Invalid Entry Price: %0.2f", entry))
		return d
	}

	if quant.Direction == LONG {
		if sl >= entry {
			d.Pass = false
			d.Reject = append(d.Reject, fmt.Sprintf("LONG TradePlan alignment error: SL %0.2f must be below Entry %0.2f", sl, entry))
		}
		if tp <= entry {
			d.Pass = false
			d.Reject = append(d.Reject, fmt.Sprintf("LONG TradePlan alignment error: TP %0.2f must be above Entry %0.2f", tp, entry))
		}
	} else if quant.Direction == SHORT {
		if sl <= entry {
			d.Pass = false
			d.Reject = append(d.Reject, fmt.Sprintf("SHORT TradePlan alignment error: SL %0.2f must be above Entry %0.2f", sl, entry))
		}
		if tp >= entry {
			d.Pass = false
			d.Reject = append(d.Reject, fmt.Sprintf("SHORT TradePlan alignment error: TP %0.2f must be below Entry %0.2f", tp, entry))
		}
	}

	return d
}

// ValidateRR checks Risk-to-Reward ratio against profile minimum.
func ValidateRR(quant QuantResult, profile PlaybookThresholdProfile, policy MarketPolicy) GateDecision {
	d := GateDecision{Pass: true}
	rr := CalculateTradePlanRR(quant)
	minRR := math.Max(policy.MinRRExecute, profile.MinRR)

	if rr <= 0 {
		d.Pass = false
		d.Reject = append(d.Reject, fmt.Sprintf("Invalid Risk-to-Reward ratio: %0.2f", rr))
		return d
	}
	if rr < minRR {
		if rr >= 1.5 {
			d.Pass = false
			d.Watch = append(d.Watch, fmt.Sprintf("Risk-to-Reward ratio %0.2f is below policy requirement %0.2f but above hard minimum 1.50", rr, minRR))
		} else {
			d.Pass = false
			d.Reject = append(d.Reject, fmt.Sprintf("Risk-to-Reward ratio %0.2f is below requirement %0.2f", rr, minRR))
		}
	}
	return d
}

// ValidateVolume checks volume confirmation requirements from profile.
func ValidateVolume(quant QuantResult, profile PlaybookThresholdProfile, m15 []dto.Candle) GateDecision {
	d := GateDecision{Pass: true}
	if profile.RequireVolumeConfirm {
		minVolRatio := profile.MinVolumeRatio
		if minVolRatio <= 0 {
			minVolRatio = 1.3
		}
		if !hasVolumeConfirmation(&quant.TechnicalSnapshot, m15, minVolRatio) {
			d.Pass = false
			d.Reject = append(d.Reject, fmt.Sprintf("%s lacks required volume spike confirmation", quant.Playbook))
		}
	}
	return d
}

// ValidateRejection checks rejection requirement from profile.
func ValidateRejection(quant QuantResult, profile PlaybookThresholdProfile) GateDecision {
	d := GateDecision{Pass: true}
	if profile.RequireRejection {
		wickRejection := quant.TechnicalSnapshot.IndicatorValues[IndicatorWickRejection]
		if wickRejection != 1.0 {
			d.Pass = false
			d.Reject = append(d.Reject, fmt.Sprintf("%s lacks required wick rejection confirmation", quant.Playbook))
		}
	}
	return d
}

// ValidateConfirmation checks confirmation requirement.
func ValidateConfirmation(quant QuantResult, profile PlaybookThresholdProfile) GateDecision {
	d := GateDecision{Pass: true}
	if profile.RequireConfirmation {
		if !quant.IndicatorMet {
			d.Pass = false
			d.Reject = append(d.Reject, fmt.Sprintf("%s lacks required indicator confirmation", quant.Playbook))
		}
	}
	return d
}

// ValidateRetest checks retest requirement.
func ValidateRetest(quant QuantResult, profile PlaybookThresholdProfile) GateDecision {
	d := GateDecision{Pass: true}
	if profile.RequireRetest {
		isFirstCandle := strings.Contains(strings.ToUpper(quant.SetupType), "BREAKOUT") && !strings.Contains(strings.ToUpper(quant.SetupType), "RETEST")
		if isFirstCandle && !profile.AllowBreakoutCandleEntry {
			d.Pass = false
			d.Watch = append(d.Watch, "WAIT_RETEST_OR_BREAKOUT_FIRST_CANDLE")
		}
	}
	if profile.MinRetestQuality > 0 && quant.Playbook == COMPRESSION_BREAKOUT_RETEST {
		retestQuality := calculateRetestQuality(quant.TechnicalSnapshot.IndicatorValues)
		if retestQuality < profile.MinRetestQuality {
			d.Pass = false
			d.Watch = append(d.Watch, fmt.Sprintf("%s retest quality %0.2f is below minimum threshold %0.2f", quant.Playbook, retestQuality, profile.MinRetestQuality))
		}
	}
	return d
}

// ValidateCrowding checks crowding evidence requirement.
func ValidateCrowding(quant QuantResult, profile PlaybookThresholdProfile) GateDecision {
	d := GateDecision{Pass: true}
	if profile.RequireCrowdingEvidence {
		hasEvidence := quant.TechnicalSnapshot.IndicatorValues[IndicatorHasCrowdingEvidence]
		crowdingScore := quant.TechnicalSnapshot.IndicatorValues[IndicatorCrowdingScore]
		fundingAbs := math.Abs(quant.TechnicalSnapshot.IndicatorValues[IndicatorFundingRate])
		oiChange := quant.TechnicalSnapshot.IndicatorValues[IndicatorOIChange]

		if hasEvidence != 1.0 {
			d.Pass = false
			d.Reject = append(d.Reject, "CROWDING_EVIDENCE_MISSING")
		}
		if crowdingScore < profile.MinCrowdingScore && fundingAbs == 0 && oiChange == 0 {
			d.Pass = false
			d.Reject = append(d.Reject, fmt.Sprintf("%s lacks required funding/OI crowding evidence", quant.Playbook))
		}
	}
	return d
}

// ValidateADX checks ADX requirements against profile.
func ValidateADX(quant QuantResult, profile PlaybookThresholdProfile) GateDecision {
	d := GateDecision{Pass: true}
	adx := quant.TechnicalSnapshot.IndicatorValues[IndicatorADX]
	if profile.RequireADX {
		if adx < profile.MinADX {
			d.Pass = false
			d.Watch = append(d.Watch, fmt.Sprintf("%s rejected: ADX %0.1f is below execution threshold %0.1f", quant.Playbook, adx, profile.MinADX))
		}
	}
	if profile.RejectADXExpansion {
		maxADX := profile.MaxADX
		if maxADX <= 0 {
			maxADX = safetyADXExpansionCeiling
		}
		if adx > maxADX {
			d.Pass = false
			d.Watch = append(d.Watch, fmt.Sprintf("%s rejected: high trend expansion detected (ADX = %0.1f > %0.1f)", quant.Playbook, adx, maxADX))
		}
	}
	return d
}

// MergeDecisions combines multiple gate decisions into one.
// Returns combined decision with all reject/watch reasons.
func MergeDecisions(decisions ...GateDecision) GateDecision {
	merged := GateDecision{Pass: true}
	for _, d := range decisions {
		if !d.Pass {
			merged.Pass = false
		}
		merged.Reject = append(merged.Reject, d.Reject...)
		merged.Watch = append(merged.Watch, d.Watch...)
	}
	return merged
}
