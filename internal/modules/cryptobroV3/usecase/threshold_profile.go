package usecase

import (
	"fmt"
	"math"
)

type PlaybookThresholdProfile struct {
	Playbook                     Playbook           `json:"playbook"`
	MinScoreAI                   float64            `json:"min_score_ai"`
	MinScoreExecute              float64            `json:"min_score_execute"`
	MinRR                        float64            `json:"min_rr"`
	MinADX                       float64            `json:"min_adx"`
	MaxADX                       float64            `json:"max_adx"`
	RequireADX                   bool               `json:"require_adx"`
	RejectADXExpansion           bool               `json:"reject_adx_expansion"`
	MinVolumeRatio               float64            `json:"min_volume_ratio"`
	MinWickRatio                 float64            `json:"min_wick_ratio"`
	MinRetestQuality             float64            `json:"min_retest_quality"`
	MinRangeClarity              float64            `json:"min_range_clarity"`
	MinCrowdingScore             float64            `json:"min_crowding_score"`
	StalenessATR                 float64            `json:"staleness_atr"`
	RequireVolumeConfirm         bool               `json:"require_volume_confirm"`
	RequireRejection             bool               `json:"require_rejection"`
	RequireConfirmation          bool               `json:"require_confirmation"`
	RequireRetest                bool               `json:"require_retest"`
	AllowBreakoutCandleEntry     bool               `json:"allow_breakout_candle_entry"`
	RequireCrowdingEvidence      bool               `json:"require_crowding_evidence"`
	RequireAIHigh                bool               `json:"require_ai_high"`
	RequireM5RejectionConfirm    bool               `json:"require_m5_rejection_confirm"`
	RequireM5ContinuationConfirm bool               `json:"require_m5_continuation_confirm"`
	M5ConfirmationMode           M5ConfirmationMode `json:"m5_confirmation_mode"`
	Reason                       string             `json:"reason"`
}

func usesM5Confirmation(profile PlaybookThresholdProfile) bool {
	return resolveM5ConfirmationMode(profile) != M5ConfirmationDisabled
}

func resolveM5ConfirmationMode(profile PlaybookThresholdProfile) M5ConfirmationMode {
	if !profile.RequireM5RejectionConfirm && !profile.RequireM5ContinuationConfirm {
		return M5ConfirmationDisabled
	}

	switch profile.M5ConfirmationMode {
	case M5ConfirmationWatchOnlyHint, M5ConfirmationSoftConfirm, M5ConfirmationHardConfirm:
		return profile.M5ConfirmationMode
	default:
		return M5ConfirmationWatchOnlyHint
	}
}

func normalizePlaybookThresholdProfile(profile PlaybookThresholdProfile) PlaybookThresholdProfile {
	profile.M5ConfirmationMode = resolveM5ConfirmationMode(profile)
	return profile
}

func resolvePlaybookProfileBaseline(playbook Playbook) (PlaybookThresholdProfile, bool) {
	reg := GetGlobalConfigRegistry()
	if reg != nil {
		if profile, found := reg.GetPlaybookProfile(playbook); found {
			return profile, true
		}
	}
	return GetDefaultPlaybookProfile(playbook)
}

// GetPlaybookThresholdProfile returns the customized threshold profile for a given playbook, tier, and policy.
func GetPlaybookThresholdProfile(playbook Playbook, policy MarketPolicy, tier Tier) PlaybookThresholdProfile {
	profile, found := resolvePlaybookProfileBaseline(playbook)
	if !found {
		profile = GetDefaultDefensivePlaybookProfile(playbook)
	}

	// Apply policy constraints if policy is stricter, override profile
	if policy.MinScoreAI > profile.MinScoreAI {
		profile.MinScoreAI = policy.MinScoreAI
	}
	if policy.MinScoreExecute > profile.MinScoreExecute {
		profile.MinScoreExecute = policy.MinScoreExecute
	}
	if policy.MinRRExecute > profile.MinRR {
		profile.MinRR = policy.MinRRExecute
	}
	if policy.MinADXExecute > profile.MinADX && profile.RequireADX {
		profile.MinADX = policy.MinADXExecute
	}

	// 1. BTCChaos active - stricter profile limits
	regime := policy.EffectiveRegime()
	if regime == BTC_CHAOS {
		if profile.MinScoreAI < 7.8 {
			profile.MinScoreAI = 7.8
		}
		if profile.MinScoreExecute < 8.2 {
			profile.MinScoreExecute = 8.2
		}
		if profile.MinRR < 2.0 {
			profile.MinRR = 2.0
		}
		profile.StalenessATR = math.Max(0.15, profile.StalenessATR-0.10)
		profile.RequireAIHigh = true
		profile.Reason = fmt.Sprintf("%s (Chaos tightened)", profile.Reason)
	}

	// Relax volume spike confirmation under high volatility since baseline volume is already spiked
	if regime == HIGH_VOL || regime == BTC_CHAOS {
		if profile.Playbook == LIQUIDITY_SWEEP_REVERSAL {
			if profile.MinVolumeRatio > 1.15 || profile.MinVolumeRatio <= 0 {
				profile.MinVolumeRatio = 1.15
			}
		}
	}

	// 2. Tier C candidate - stricter limits
	if tier == TierC {
		if profile.MinScoreAI < 7.5 {
			profile.MinScoreAI = 7.5
		}
		if profile.MinScoreExecute < 7.8 {
			profile.MinScoreExecute = 7.8
		}
		if profile.MinRR < 1.8 {
			profile.MinRR = 1.8
		}
		profile.StalenessATR = math.Max(0.15, profile.StalenessATR-0.05)
		profile.RequireAIHigh = true
		profile.Reason = fmt.Sprintf("%s (Tier C tightened)", profile.Reason)
	}

	return normalizePlaybookThresholdProfile(profile)
}
