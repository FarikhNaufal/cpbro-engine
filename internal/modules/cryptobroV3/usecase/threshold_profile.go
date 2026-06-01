package usecase

import (
	"fmt"
	"math"
)

type PlaybookThresholdProfile struct {
	Playbook                 Playbook `json:"playbook"`
	MinScoreAI               float64  `json:"min_score_ai"`
	MinScoreExecute          float64  `json:"min_score_execute"`
	MinRR                    float64  `json:"min_rr"`
	MinADX                   float64  `json:"min_adx"`
	MaxADX                   float64  `json:"max_adx"`
	RequireADX               bool     `json:"require_adx"`
	RejectADXExpansion       bool     `json:"reject_adx_expansion"`
	MinVolumeRatio           float64  `json:"min_volume_ratio"`
	MinWickRatio             float64  `json:"min_wick_ratio"`
	MinRetestQuality         float64  `json:"min_retest_quality"`
	MinRangeClarity          float64  `json:"min_range_clarity"`
	MinCrowdingScore         float64  `json:"min_crowding_score"`
	StalenessATR             float64  `json:"staleness_atr"`
	RequireVolumeConfirm     bool     `json:"require_volume_confirm"`
	RequireRejection         bool     `json:"require_rejection"`
	RequireConfirmation      bool     `json:"require_confirmation"`
	RequireRetest            bool     `json:"require_retest"`
	AllowBreakoutCandleEntry bool     `json:"allow_breakout_candle_entry"`
	RequireCrowdingEvidence  bool     `json:"require_crowding_evidence"`
	RequireAIHigh            bool     `json:"require_ai_high"`
	Reason                   string   `json:"reason"`
}

// GetPlaybookThresholdProfile returns the customized threshold profile for a given playbook, tier, and policy.
func GetPlaybookThresholdProfile(playbook Playbook, policy MarketPolicy, tier Tier) PlaybookThresholdProfile {
	var profile PlaybookThresholdProfile
	found := false

	reg := GetGlobalConfigRegistry()
	if reg != nil {
		profile, found = reg.GetPlaybookProfile(playbook)
	}

	if !found {
		// Set playbook specific defaults
		switch playbook {
		case TREND_PULLBACK:
			profile = PlaybookThresholdProfile{
				Playbook:                 TREND_PULLBACK,
				MinScoreAI:               7.0,
				MinScoreExecute:          7.3,
				MinRR:                    1.5,
				MinADX:                   20.0,
				RequireADX:               true,
				RejectADXExpansion:       false,
				RequireVolumeConfirm:     false,
				RequireRejection:         false,
				RequireConfirmation:      false,
				RequireRetest:            false,
				AllowBreakoutCandleEntry: false,
				StalenessATR:             0.45,
				Reason:                   "Trend Pullback profile",
			}

		case LIQUIDITY_SWEEP_REVERSAL:
			profile = PlaybookThresholdProfile{
				Playbook:                 LIQUIDITY_SWEEP_REVERSAL,
				MinScoreAI:               7.0,
				MinScoreExecute:          7.3,
				MinRR:                    1.7,
				MinADX:                   0.0,
				RequireADX:               false,
				RejectADXExpansion:       false,
				RequireVolumeConfirm:     true,
				RequireRejection:         true,
				RequireConfirmation:      true,
				RequireRetest:            false,
				MinVolumeRatio:           1.3,
				MinWickRatio:             0.3,
				AllowBreakoutCandleEntry: false,
				StalenessATR:             0.30,
				Reason:                   "Liquidity Sweep Reversal profile",
			}

		case COMPRESSION_BREAKOUT_RETEST:
			profile = PlaybookThresholdProfile{
				Playbook:                 COMPRESSION_BREAKOUT_RETEST,
				MinScoreAI:               7.0,
				MinScoreExecute:          7.3,
				MinRR:                    1.6,
				MinADX:                   0.0,
				RequireADX:               false,
				RejectADXExpansion:       false,
				RequireVolumeConfirm:     true,
				RequireRejection:         false,
				RequireConfirmation:      true,
				RequireRetest:            true,
				MinVolumeRatio:           1.2,
				MinRetestQuality:         0.5,
				AllowBreakoutCandleEntry: false,
				StalenessATR:             0.30,
				Reason:                   "Compression Breakout Retest profile",
			}

		case RANGE_EDGE_REVERSAL:
			profile = PlaybookThresholdProfile{
				Playbook:                 RANGE_EDGE_REVERSAL,
				MinScoreAI:               7.2,
				MinScoreExecute:          7.5,
				MinRR:                    1.7,
				MinADX:                   0.0,
				MaxADX:                   30.0,
				RejectADXExpansion:       true,
				RequireVolumeConfirm:     false,
				RequireRejection:         true,
				RequireConfirmation:      true,
				RequireRetest:            false,
				MinRangeClarity:          0.5,
				AllowBreakoutCandleEntry: false,
				StalenessATR:             0.30,
				Reason:                   "Range Edge Reversal profile",
			}

		case CROWDED_POSITIONING_SQUEEZE:
			profile = PlaybookThresholdProfile{
				Playbook:                 CROWDED_POSITIONING_SQUEEZE,
				MinScoreAI:               7.5,
				MinScoreExecute:          7.8,
				MinRR:                    1.8,
				MinADX:                   0.0,
				RequireADX:               false,
				RequireVolumeConfirm:     false,
				RequireRejection:         true,
				RequireConfirmation:      true,
				RequireCrowdingEvidence:  true,
				MinCrowdingScore:         0.5,
				RequireAIHigh:            true,
				AllowBreakoutCandleEntry: false,
				StalenessATR:             0.35,
				Reason:                   "Crowded Positioning Squeeze profile",
			}

		default:
			// Default defensive profile if playbook is unknown
			profile = PlaybookThresholdProfile{
				Playbook:                 playbook,
				MinScoreAI:               7.2,
				MinScoreExecute:          7.5,
				MinRR:                    1.7,
				MinADX:                   20.0,
				RequireADX:               true,
				RejectADXExpansion:       true,
				RequireVolumeConfirm:     true,
				RequireRejection:         true,
				RequireConfirmation:      true,
				AllowBreakoutCandleEntry: false,
				StalenessATR:             0.30,
				Reason:                   "Default defensive profile",
			}
		}
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
	if policy.EffectiveRegime() == BTC_CHAOS {
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

	return profile
}
