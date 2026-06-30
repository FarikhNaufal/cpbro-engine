package usecase

import (
	"context"
	"strings"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"cpbro-engine/internal/modules/cryptobroV3/entity"
)

// Direction represents market trade direction
type Direction string

const (
	LONG  Direction = "LONG"
	SHORT Direction = "SHORT"
	WAIT  Direction = "WAIT"
)

// MarketRegime represents market macro context
type MarketRegime string

const (
	DEFAULT        MarketRegime = "DEFAULT"
	ALT_SUPPORTIVE MarketRegime = "ALT_SUPPORTIVE"
	BTC_DOMINANCE  MarketRegime = "BTC_DOMINANCE"
	RISK_OFF       MarketRegime = "RISK_OFF"
	CHOP_RANGE     MarketRegime = "CHOP_RANGE"
	COMPRESSION    MarketRegime = "COMPRESSION"
	LOW_VOL        MarketRegime = "LOW_VOL"
	HIGH_VOL       MarketRegime = "HIGH_VOL"
	BTC_CHAOS      MarketRegime = "BTC_CHAOS"
	UNKNOWN        MarketRegime = "UNKNOWN"
)

// PolicyMode defines operational settings for screening assets
type PolicyMode string

const (
	NORMAL               PolicyMode = "NORMAL"
	SWEEP_ONLY           PolicyMode = "SWEEP_ONLY"
	REVERSAL_ONLY        PolicyMode = "REVERSAL_ONLY"
	PULLBACK_ONLY        PolicyMode = "PULLBACK_ONLY"
	BREAKOUT_RETEST_ONLY PolicyMode = "BREAKOUT_RETEST_ONLY"
	DISABLED             PolicyMode = "DISABLED"
)

// Tier represents asset priority quality
type Tier string

const (
	TierA Tier = "A"
	TierB Tier = "B"
	TierC Tier = "C"
)

// Playbook represents active strategy playbooks
type Playbook string

const (
	TREND_PULLBACK              Playbook = "TREND_PULLBACK"
	LIQUIDITY_SWEEP_REVERSAL    Playbook = "LIQUIDITY_SWEEP_REVERSAL"
	COMPRESSION_BREAKOUT_RETEST Playbook = "COMPRESSION_BREAKOUT_RETEST"
	RANGE_EDGE_REVERSAL         Playbook = "RANGE_EDGE_REVERSAL"
	CROWDED_POSITIONING_SQUEEZE Playbook = "CROWDED_POSITIONING_SQUEEZE"
)

// AIConfidence represents Gemini confidence scores
type AIConfidence string

const (
	AIConfidenceHigh   AIConfidence = "HIGH"
	AIConfidenceMedium AIConfidence = "MEDIUM"
	AIConfidenceLow    AIConfidence = "LOW"
)

const (
	AIAuditSourceReal               = "REAL"
	AIAuditSourceRealError          = "REAL_ERROR"
	AIAuditSourceSyntheticLocalGate = "SYNTHETIC_LOCAL_GATE"
	AIAuditSourceSyntheticQuota     = "SYNTHETIC_QUOTA"
	AIAuditSourceSyntheticDisabled  = "SYNTHETIC_DISABLED"
)

// Status represents the state of candidate traversal through the execution pipelines
type Status string

const (
	RAW_SYMBOL        Status = "RAW_SYMBOL"
	UNIVERSE_PASS     Status = "UNIVERSE_PASS"
	UNIVERSE_REJECT   Status = "UNIVERSE_REJECT"
	STRATEGY_SELECTED Status = "STRATEGY_SELECTED"
	PLAYBOOK_ELIGIBLE Status = "PLAYBOOK_ELIGIBLE"
	PLAYBOOK_REJECTED Status = "PLAYBOOK_REJECTED"
	QUANT_CANDIDATE   Status = "QUANT_CANDIDATE"
	ARBITER_SELECTED  Status = "ARBITER_SELECTED"
	ARBITER_REJECTED  Status = "ARBITER_REJECTED"
	LOCAL_REJECT      Status = "LOCAL_REJECT"
	LOCAL_WATCH       Status = "LOCAL_WATCH"
	AI_CANDIDATE      Status = "AI_CANDIDATE"
	AI_CONFIRM        Status = "AI_CONFIRM"
	AI_WAIT           Status = "AI_WAIT"
	AI_REJECT         Status = "AI_REJECT"
	AI_ERROR          Status = "AI_ERROR"
	PLAN_VALID        Status = "PLAN_VALID"
	PLAN_NEED_RETEST  Status = "PLAN_NEED_RETEST"
	PLAN_CONFLICT     Status = "PLAN_CONFLICT"
	FRESH             Status = "FRESH"
	LATE              Status = "LATE"
	MISSED            Status = "MISSED"
	FINAL_EXECUTE     Status = "FINAL_EXECUTE"
	FINAL_WATCH       Status = "FINAL_WATCH"
	FINAL_REJECT      Status = "FINAL_REJECT"
	AI_ERROR_REVIEW   Status = "AI_ERROR_REVIEW"
	MONITORING        Status = "MONITORING"
	WATCH_MONITORING  Status = "WATCH_MONITORING"
	TP1_HIT           Status = "TP1_HIT"
	TP2_HIT           Status = "TP2_HIT"
	SL_HIT            Status = "SL_HIT"
	EXPIRED           Status = "EXPIRED"
	BREAKEVEN         Status = "BREAKEVEN"
	VIRTUAL_TP1_HIT   Status = "VIRTUAL_TP1_HIT"
	VIRTUAL_TP2_HIT   Status = "VIRTUAL_TP2_HIT"
	VIRTUAL_SL_HIT    Status = "VIRTUAL_SL_HIT"
	VIRTUAL_EXPIRED   Status = "VIRTUAL_EXPIRED"
	WATCH_PROMOTED    Status = "WATCH_PROMOTED"
	WATCH_INVALIDATED Status = "WATCH_INVALIDATED"
	WATCH_EXPIRED     Status = "WATCH_EXPIRED"
)

type M5ConfirmationMode string

const (
	M5ConfirmationDisabled      M5ConfirmationMode = "DISABLED"
	M5ConfirmationWatchOnlyHint M5ConfirmationMode = "WATCH_ONLY_HINT"
	M5ConfirmationSoftConfirm   M5ConfirmationMode = "SOFT_CONFIRM"
	M5ConfirmationHardConfirm   M5ConfirmationMode = "HARD_CONFIRM"
)

type M5ConfirmationStatus string

const (
	M5ConfirmationNotUsed     M5ConfirmationStatus = "NOT_USED"
	M5ConfirmationUnavailable M5ConfirmationStatus = "UNAVAILABLE"
	M5ConfirmationConfirmed   M5ConfirmationStatus = "CONFIRMED"
	M5ConfirmationFailed      M5ConfirmationStatus = "FAILED"
	M5ConfirmationInvalidated M5ConfirmationStatus = "INVALIDATED"
)

// Struct Definitions

type MarketContextData struct {
	Regime           MarketRegime `json:"regime"`
	BTCDominance     float64      `json:"btc_dominance"`
	MarketRegimeTime time.Time    `json:"market_regime_time"`
	Notes            string       `json:"notes"`
}

type MarketPolicy struct {
	Regime                 MarketRegime `json:"regime"`
	AllowLong              bool         `json:"allow_long"`
	AllowShort             bool         `json:"allow_short"`
	LongMode               PolicyMode   `json:"long_mode"`
	ShortMode              PolicyMode   `json:"short_mode"`
	AllowedTiers           []Tier       `json:"allowed_tiers"`
	AllowedPlaybooks       []Playbook   `json:"allowed_playbooks"`
	MaxSymbols             int          `json:"max_symbols"`
	MaxAICandidates        int          `json:"max_ai_candidates"`
	MaxFinalExecute        int          `json:"max_final_execute"`
	MinVolume              float64      `json:"min_volume"`
	MaxFundingAbs          float64      `json:"max_funding_abs"`
	MaxPriceMove24h        float64      `json:"max_price_move_24h"`
	MaxPriceMove24hLong    float64      `json:"max_price_move_24h_long"`  // per-symbol max abs 24h dump for LONG entry
	MaxPriceMove24hShort   float64      `json:"max_price_move_24h_short"` // per-symbol max abs 24h pump for SHORT entry
	MinScoreAI             float64      `json:"min_score_ai"`
	MinScoreExecute        float64      `json:"min_score_execute"`
	MinRRExecute           float64      `json:"min_rr_execute"`
	MinADXExecute          float64      `json:"min_adx_execute"`
	RequireAIConfidence    AIConfidence `json:"require_ai_confidence"`
	RequireFreshEntry      bool         `json:"require_fresh_entry"`
	AllowLateStaleness     bool         `json:"allow_late_staleness"`
	AllowAIQuotaWatch      bool         `json:"allow_ai_quota_watch"`
	AllowAIDisabledWatch   bool         `json:"allow_ai_disabled_watch"`
	StalenessATRMultiplier float64      `json:"staleness_atr_multiplier"`
	CooldownMinutes        int          `json:"cooldown_minutes"`
	BtcTrend               string       `json:"btc_trend"`
	BtcScore               float64      `json:"btc_score"`
	BtcChaos               float64      `json:"btc_chaos"`
	Reason                 string       `json:"reason"`
	HotMaxBoost            float64      `json:"hot_max_boost"`
	HotPrefetchSlotRatio   float64      `json:"hot_prefetch_slot_ratio"`
}

// EffectiveRegime returns a stable regime value for downstream logic.
// It prefers the explicit Regime field, but falls back to parsing Reason for backward compatibility.
func (p MarketPolicy) EffectiveRegime() MarketRegime {
	if p.Regime != "" && p.Regime != UNKNOWN {
		return p.Regime
	}

	reason := strings.ToUpper(p.Reason)
	switch {
	case strings.Contains(reason, "BTC_CHAOS") || strings.Contains(reason, "CHAOS"):
		return BTC_CHAOS
	case strings.Contains(reason, "HIGH_VOL"):
		return HIGH_VOL
	case strings.Contains(reason, "LOW_VOL"):
		return LOW_VOL
	case strings.Contains(reason, "COMPRESSION"):
		return COMPRESSION
	case strings.Contains(reason, "ALT_SUPPORTIVE"):
		return ALT_SUPPORTIVE
	case strings.Contains(reason, "BTC_DOMINANCE") || strings.Contains(reason, "DOMINANCE"):
		return BTC_DOMINANCE
	case strings.Contains(reason, "RISK_OFF"):
		return RISK_OFF
	case strings.Contains(reason, "CHOP_RANGE") || strings.Contains(reason, "SIDEWAYS"):
		return CHOP_RANGE
	}

	return p.Regime
}

type UniverseCandidate struct {
	Symbol             string  `json:"symbol"`
	Tier               Tier    `json:"tier"`
	Status             Status  `json:"status"`
	Notes              string  `json:"notes"`
	IsHot              bool    `json:"is_hot"`
	HotScore           float64 `json:"hot_score"`
	HotSource          string  `json:"hot_source"`
	HotRankType        int     `json:"hot_rank_type"`
	HotOverlaySelected bool    `json:"hot_overlay_selected"`
	LiquidityScore     float64 `json:"liquidity_score,omitempty"`
	ActivityScore      float64 `json:"activity_score,omitempty"`
	CompositeScore     float64 `json:"composite_score,omitempty"`
}

type UniverseRejected struct {
	Symbol string `json:"symbol"`
	Status Status `json:"status"`
	Reason string `json:"reason"`
}

type MarketData struct {
	Symbol          string       `json:"symbol"`
	M15Candles      []dto.Candle `json:"m15_candles"`
	H1Candles       []dto.Candle `json:"h1_candles"`
	H4Candles       []dto.Candle `json:"h4_candles"`
	BTCH1Candles    []dto.Candle `json:"btc_h1_candles"`
	ETHH1Candles    []dto.Candle `json:"eth_h1_candles"`
	OpenInterestM15 float64      `json:"open_interest_m15"`
	OIChangePct     float64      `json:"oi_change_pct"`
	FundingRate     float64      `json:"funding_rate"`
	LatestPrice     float64      `json:"latest_price"`
	PriceChange24h  float64      `json:"price_change_24h"`
	LastUpdated     time.Time    `json:"last_updated"`
}

type TechnicalSnapshot struct {
	RSI             float64            `json:"rsi"`
	MACD            float64            `json:"macd"`
	EMA200          float64            `json:"ema_200"`
	Timeframe       string             `json:"timeframe"`
	IndicatorValues map[string]float64 `json:"indicator_values"`
	Notes           string             `json:"notes"`
	RSISlope        float64            `json:"rsi_slope"`
	MFI             float64            `json:"mfi"`
	MFISlope        float64            `json:"mfi_slope"`
	ADXSlope        float64            `json:"adx_slope"`
	ATRPercent      float64            `json:"atr_percent"`
	VolumeRatio     float64            `json:"volume_ratio"`
	OIChange        float64            `json:"oi_change"`
	FundingRate     float64            `json:"funding_rate"`
	PriceChange24h  float64            `json:"price_change_24h"`
}

type StructureSnapshot struct {
	MarketStructure string    `json:"market_structure"`
	H1Structure     string    `json:"h1_structure"`
	BOS             bool      `json:"bos"`
	CHOCH           bool      `json:"choch"`
	Highs           []float64 `json:"highs"`
	Lows            []float64 `json:"lows"`
	Support         float64   `json:"support"`
	Resistance      float64   `json:"resistance"`
	SessionHigh     float64   `json:"session_high"`
	SessionLow      float64   `json:"session_low"`
	LiquidityUpper  float64   `json:"liquidity_upper"`
	LiquidityLower  float64   `json:"liquidity_lower"`
	Timeframe       string    `json:"timeframe"`
	Notes           string    `json:"notes"`
}

type StrategySelection struct {
	Symbol        string    `json:"symbol"`
	StrategyName  string    `json:"strategy_name"`
	Direction     Direction `json:"direction,omitempty"`
	Priority      int       `json:"priority"`
	Reason        string    `json:"reason"`
	PolicyContext string    `json:"policy_context"`
	Tier          Tier      `json:"tier"`
	Status        Status    `json:"status"`
}

type PlaybookEligibilityResult struct {
	Playbook Playbook `json:"playbook"`
	Eligible bool     `json:"eligible"`
	Status   Status   `json:"status"`
	Reason   string   `json:"reason"`
}

type TradePlan struct {
	Symbol     string    `json:"symbol"`
	Direction  Direction `json:"direction"`
	EntryPrice float64   `json:"entry_price"`
	StopLoss   float64   `json:"stop_loss"`
	TakeProfit float64   `json:"take_profit"`
	Status     Status    `json:"status"`
	Reason     string    `json:"reason"`
}

type ScoreBreakdown struct {
	BaseScore   float64 `json:"base_score"`
	TrendScore  float64 `json:"trend_score"`
	RegimeScore float64 `json:"regime_score"`
	TotalScore  float64 `json:"total_score"`
	Notes       string  `json:"notes"`
}

type QuantResult struct {
	Symbol            string            `json:"symbol"`
	Direction         Direction         `json:"direction"`
	Playbook          Playbook          `json:"playbook"`
	SetupType         string            `json:"setup_type"`
	TriggerPrice      float64           `json:"trigger_price"`
	StopLoss          float64           `json:"stop_loss"`
	TakeProfit        float64           `json:"take_profit"`
	MarketStructure   string            `json:"market_structure"`
	H1Trend           string            `json:"h1_trend"`
	H4Trend           string            `json:"h4_trend"`
	IndicatorMet      bool              `json:"indicator_met"`
	Status            Status            `json:"status"`
	Reason            string            `json:"reason"`
	Score             float64           `json:"score"`
	Tier              Tier              `json:"tier"`
	TechnicalSnapshot TechnicalSnapshot `json:"technical_snapshot"`
	StructureSnapshot StructureSnapshot `json:"structure_snapshot"`
	TradePlan         TradePlan         `json:"trade_plan"`
	RawKlines         []dto.Candle      `json:"raw_klines"`
}

type CandidateArbiterResult struct {
	Symbol   string  `json:"symbol"`
	Score    float64 `json:"score"`
	Selected bool    `json:"selected"`
	Status   Status  `json:"status"`
	Reason   string  `json:"reason"`
}

type LocalGateResult struct {
	Passed    bool                   `json:"passed"`
	Status    Status                 `json:"status"`
	Reason    string                 `json:"reason"`
	M5Summary *M5ConfirmationSummary `json:"m5_summary,omitempty"`
}

type M5ConfirmationSummary struct {
	Used              bool                 `json:"used"`
	Mode              M5ConfirmationMode   `json:"mode"`
	Status            M5ConfirmationStatus `json:"status"`
	Reason            string               `json:"reason,omitempty"`
	ConfirmationType  string               `json:"confirmation_type,omitempty"`
	Confirmed         bool                 `json:"confirmed"`
	EarlyInvalidation bool                 `json:"early_invalidation"`
	LastClose         float64              `json:"last_close,omitempty"`
	EMA9              float64              `json:"ema9,omitempty"`
	WickRatio         float64              `json:"wick_ratio,omitempty"`
	Threshold         float64              `json:"threshold,omitempty"`
}

type AIAuditVerdict struct {
	Sentiment           string       `json:"sentiment"`
	Confidence          AIConfidence `json:"confidence"`
	Reasoning           string       `json:"reasoning"`
	Approved            bool         `json:"approved"`
	SuggestedStopLoss   float64      `json:"suggested_stop_loss"`
	SuggestedTakeProfit float64      `json:"suggested_take_profit"`
	Status              Status       `json:"status"`
	Notes               string       `json:"notes"`
}

type PlanReview struct {
	Conflicted      bool   `json:"conflicted"`
	EntryStillValid bool   `json:"entry_still_valid"`
	NeedRetest      bool   `json:"need_retest"`
	Resolution      string `json:"resolution"`
	Status          Status `json:"status"`
	Reason          string `json:"reason"`
}

type StalenessResult struct {
	IsStale         bool      `json:"is_stale"`
	LastUpdatedTime time.Time `json:"last_updated_time"`
	CurrentTime     time.Time `json:"current_time"`
	Status          Status    `json:"status"`
	Reason          string    `json:"reason"`
}

type FinalDecision struct {
	Symbol                  string    `json:"symbol"`
	Direction               Direction `json:"direction"`
	Playbook                Playbook  `json:"playbook"`
	Status                  Status    `json:"status"`
	Reason                  string    `json:"reason"`
	Score                   float64   `json:"score"`
	RequiredScore           float64   `json:"required_score"`
	RR                      float64   `json:"rr"`
	PlannedRR               float64   `json:"planned_rr,omitempty"`
	ActualRR                float64   `json:"actual_rr,omitempty"`
	RequiredRR              float64   `json:"required_rr"`
	AIConfidence            string    `json:"ai_confidence"`
	AISource                string    `json:"ai_source,omitempty"`
	AICalled                bool      `json:"ai_called,omitempty"`
	StalenessStatus         string    `json:"staleness_status"`
	PolicySummary           string    `json:"policy_summary"`
	ThresholdProfileSummary string    `json:"threshold_profile_summary"`
	PrimaryReasonLayer      string    `json:"primary_reason_layer,omitempty"`
	ReasonBreakdown         []string  `json:"reason_breakdown,omitempty"`
	IsExecutable            bool      `json:"is_executable"`
	Tier                    Tier      `json:"tier"`
	EntryPrice              float64   `json:"entry_price"`
	StopLoss                float64   `json:"stop_loss"`
	TakeProfit              float64   `json:"take_profit"`
	WatchReason             string    `json:"watch_reason,omitempty"`
	RejectReason            string    `json:"reject_reason,omitempty"`
}

type SignalJournal struct {
	SchemaVersion             string    `json:"schema_version,omitempty"`
	ConfigVersion             string    `json:"config_version,omitempty"`
	ID                        string    `json:"signal_id"`
	Symbol                    string    `json:"symbol"`
	Direction                 Direction `json:"direction"`
	Playbook                  Playbook  `json:"playbook"`
	EntryPrice                float64   `json:"entry"`
	StopLoss                  float64   `json:"sl"`
	OriginalStopLoss          float64   `json:"original_sl,omitempty"`
	TP1                       float64   `json:"tp1"`
	TP2                       float64   `json:"tp2"`
	RR                        float64   `json:"rr"`
	QuantScore                float64   `json:"score"`
	AIConfidence              string    `json:"ai_confidence"`
	MarketRegime              string    `json:"market_regime"`
	PolicyMode                string    `json:"policy_mode"`
	PolicyLongMode            string    `json:"policy_long_mode,omitempty"`
	PolicyShortMode           string    `json:"policy_short_mode,omitempty"`
	PolicyRequireAIConfidence string    `json:"policy_require_ai_confidence,omitempty"`
	PolicyRequireFreshEntry   bool      `json:"policy_require_fresh_entry,omitempty"`
	PolicyAllowedPlaybooks    []string  `json:"policy_allowed_playbooks,omitempty"`
	PolicyReason              string    `json:"policy_reason,omitempty"`
	ThresholdProfileSummary   string    `json:"threshold_profile_summary"`
	BreakoutLevel             float64   `json:"breakout_level,omitempty"`
	RetestTouches             float64   `json:"retest_touches,omitempty"`
	RetestHold                bool      `json:"retest_hold,omitempty"`
	HasDerivativesEvidence    bool      `json:"has_derivatives_evidence,omitempty"`
	CreatedAt                 time.Time `json:"created_at"`
	ExpiresAt                 time.Time `json:"expires_at"`
	Status                    Status    `json:"status"`
	MFE                       float64   `json:"mfe"`
	MAE                       float64   `json:"mae"`
	TimeToTP1                 string    `json:"time_to_tp1"`
	TimeToTP2                 string    `json:"time_to_tp2"`
	TimeToSL                  string    `json:"time_to_sl"`
	OutcomeReason             string    `json:"outcome_reason"`
	EntryTiming               string    `json:"entry_timing"`
	Tier                      Tier      `json:"tier"`
	IsHot                     bool      `json:"is_hot,omitempty"`
	HotScore                  float64   `json:"hot_score,omitempty"`
	HotSource                 string    `json:"hot_source,omitempty"`
	HotRankType               int       `json:"hot_rank_type,omitempty"`
	HotOverlaySelected        bool      `json:"hot_overlay_selected,omitempty"`

	// Keep existing fields for backward compatibility
	Timeframe          string    `json:"timeframe,omitempty"`
	LatestPrice        float64   `json:"latest_price,omitempty"`
	TakeProfit         float64   `json:"take_profit,omitempty"`
	AISentiment        string    `json:"ai_sentiment,omitempty"`
	AIReasoning        string    `json:"ai_reasoning,omitempty"`
	PnlPercentage      float64   `json:"pnl_percentage,omitempty"`
	UpdatedAt          time.Time `json:"updated_at,omitempty"`
	ClosedAt           time.Time `json:"closed_at,omitempty"`
	Reason             string    `json:"reason,omitempty"`
	NotificationStatus string    `json:"notification_status,omitempty"`
	NotificationError  string    `json:"notification_error,omitempty"`

	TechnicalSnapshot TechnicalSnapshot `json:"technical_snapshot,omitempty"`
	StructureSnapshot StructureSnapshot `json:"structure_snapshot,omitempty"`
}

// WatchJournal intentionally reuses the same persistence shape as SignalJournal,
// but is stored and evaluated separately so FINAL_WATCH remains non-actionable.
type WatchJournal = SignalJournal

// WatchAgeDistribution tracks distribution of watch journal ages at terminal status.
// Used to diagnose whether max age settings are appropriate.
type WatchAgeDistribution struct {
	Bucket0to5    int `json:"bucket_0_to_5_min"`    // 0-5 minutes (fresh)
	Bucket5to15   int `json:"bucket_5_to_15_min"`   // 5-15 minutes
	Bucket15to30  int `json:"bucket_15_to_30_min"`  // 15-30 minutes
	Bucket30to60  int `json:"bucket_30_to_60_min"`  // 30-60 minutes
	Bucket60to120 int `json:"bucket_60_to_120_min"` // 60-120 minutes
	Bucket120plus int `json:"bucket_120_plus_min"`  // >120 minutes
}

// PromotionBlockerStats tracks why watch promotions were blocked.
// Helps diagnose the root cause of 0% watch-to-execute conversion.
type PromotionBlockerStats struct {
	BlockedByAIConfidence int `json:"blocked_by_ai_confidence"` // AI confidence too low
	BlockedByConflict     int `json:"blocked_by_conflict"`      // Opposing direction conflict
	BlockedByCooldown     int `json:"blocked_by_cooldown"`      // Symbol cooldown active
	BlockedByPlaybook     int `json:"blocked_by_playbook"`      // Playbook not in allowed list
	BlockedByTier         int `json:"blocked_by_tier"`          // Tier restriction
	BlockedByOther        int `json:"blocked_by_other"`         // Other reasons
}

type DecisionAudit struct {
	SchemaVersion                   string    `json:"schema_version,omitempty"`
	ConfigVersion                   string    `json:"config_version,omitempty"`
	ScanID                          string    `json:"scan_id"`
	GeneratedAt                     time.Time `json:"generated_at"`
	Symbol                          string    `json:"symbol"`
	Direction                       Direction `json:"direction"`
	Playbook                        Playbook  `json:"playbook"`
	SetupType                       string    `json:"setup_type"`
	Tier                            Tier      `json:"tier"`
	Grade                           string    `json:"grade"`
	Score                           float64   `json:"score"`
	RR                              float64   `json:"rr"`
	RRPlan                          float64   `json:"rr_plan,omitempty"`
	RRActual                        float64   `json:"rr_actual,omitempty"`
	RequiredScore                   float64   `json:"required_score"`
	RequiredRR                      float64   `json:"required_rr"`
	LocalGateStatus                 string    `json:"local_gate_status"`
	LocalGateReason                 string    `json:"local_gate_reason"`
	EnteredAIBatch                  bool      `json:"entered_ai_batch,omitempty"`
	AICalled                        bool      `json:"ai_called,omitempty"`
	AISource                        string    `json:"ai_source,omitempty"`
	AIDecision                      string    `json:"ai_decision"`
	AIConfidence                    string    `json:"ai_confidence"`
	AICandleNarrative               string    `json:"ai_candle_narrative"`
	AIEntryTiming                   string    `json:"ai_entry_timing"`
	AIConflictWithBot               bool      `json:"ai_conflict_with_bot"`
	PlanStatus                      string    `json:"plan_status"`
	PlanConflict                    bool      `json:"plan_conflict"`
	NeedRetest                      bool      `json:"need_retest"`
	StalenessStatus                 string    `json:"staleness_status"`
	FinalStatusBeforeConflict       Status    `json:"final_status_before_conflict"`
	FinalReasonBeforeConflict       string    `json:"final_reason_before_conflict"`
	FinalStatusAfterConflict        Status    `json:"final_status_after_conflict"`
	FinalReasonAfterConflict        string    `json:"final_reason_after_conflict"`
	FinalStatus                     Status    `json:"final_status"`
	FinalReason                     string    `json:"final_reason"`
	DecisionBrief                   string    `json:"decision_brief,omitempty"`
	FinalPrimaryReasonLayer         string    `json:"final_primary_reason_layer,omitempty"`
	FinalReasonBreakdown            []string  `json:"final_reason_breakdown,omitempty"`
	ConflictReason                  string    `json:"conflict_reason"`
	CooldownReason                  string    `json:"cooldown_reason"`
	WasNotified                     bool      `json:"was_notified"`
	ArbiterSelectedRank             int       `json:"arbiter_selected_rank,omitempty"`
	LatestPriceAtDecision           float64   `json:"latest_price_at_decision"`
	EntryPrice                      float64   `json:"entry"`
	StopLoss                        float64   `json:"sl"`
	TakeProfit1                     float64   `json:"tp1"`
	TakeProfit2                     float64   `json:"tp2"`
	MarketRegime                    string    `json:"market_regime"`
	PolicyMode                      string    `json:"policy_mode"`
	PolicyLongMode                  string    `json:"policy_long_mode,omitempty"`
	PolicyShortMode                 string    `json:"policy_short_mode,omitempty"`
	PolicyRequireAIConfidence       string    `json:"policy_require_ai_confidence,omitempty"`
	PolicyRequireFreshEntry         bool      `json:"policy_require_fresh_entry,omitempty"`
	PolicyAllowedPlaybooks          []string  `json:"policy_allowed_playbooks,omitempty"`
	PolicyReason                    string    `json:"policy_reason,omitempty"`
	CompressionLowVolFallbackActive bool      `json:"compression_low_vol_fallback_active,omitempty"`
	BootstrapTickerSource           string    `json:"bootstrap_ticker_source,omitempty"`
	BootstrapTickerCacheAgeSeconds  uint64    `json:"bootstrap_ticker_cache_age_seconds,omitempty"`
	BootstrapFundingSource          string    `json:"bootstrap_funding_source,omitempty"`
	BootstrapFundingCacheAgeSeconds uint64    `json:"bootstrap_funding_cache_age_seconds,omitempty"`
	ThresholdProfileSummary         string    `json:"threshold_profile_summary"`
	BreakoutLevel                   float64   `json:"breakout_level,omitempty"`
	RetestTouches                   float64   `json:"retest_touches,omitempty"`
	RetestHold                      bool      `json:"retest_hold,omitempty"`
	HasDerivativesEvidence          bool      `json:"has_derivatives_evidence,omitempty"`
	RejectOrWatchReason             string    `json:"reject_or_watch_reason"`
	CreatedAt                       time.Time `json:"created_at"`
	M5ConfirmationUsed              bool      `json:"m5_confirmation_used,omitempty"`
	M5ConfirmationMode              string    `json:"m5_confirmation_mode,omitempty"`
	M5ConfirmationStatus            string    `json:"m5_confirmation_status,omitempty"`
	M5ConfirmationReason            string    `json:"m5_confirmation_reason,omitempty"`
	M5ConfirmationType              string    `json:"m5_confirmation_type,omitempty"`
	M5Confirmed                     bool      `json:"m5_confirmed,omitempty"`
	M5EarlyInvalidation             bool      `json:"m5_early_invalidation,omitempty"`

	// Backward compatibility
	HypotheticalEntry float64 `json:"hypothetical_entry"`
}

type ThresholdRecommendation struct {
	IssueType          string  `json:"issue_type"`
	Playbook           string  `json:"playbook"`
	MarketRegime       string  `json:"market_regime"`
	PolicyMode         string  `json:"policy_mode"`
	Direction          string  `json:"direction"`
	Tier               string  `json:"tier"`
	MetricName         string  `json:"metric_name"`
	MetricValue        float64 `json:"metric_value"`
	SampleSize         int     `json:"sample_size"`
	CurrentThreshold   string  `json:"current_threshold"`
	SuggestedThreshold string  `json:"suggested_threshold"`
	EvidenceSummary    string  `json:"evidence_summary"`
	ConfidenceLevel    string  `json:"confidence_level"`
	Reason             string  `json:"reason"`
	SuggestedAction    string  `json:"suggested_action"`
	DoNotAutoApply     bool    `json:"do_not_auto_apply"`
	RequiresMoreData   bool    `json:"requires_more_data"`
	Severity           string  `json:"severity"`
}

type DataCompleteness struct {
	HasSignalJournal                  bool `json:"has_signal_journal"`
	HasLatestResult                   bool `json:"has_latest_result"`
	HasDecisionAudit                  bool `json:"has_decision_audit"`
	CanEvaluateExecutedOutcome        bool `json:"can_evaluate_executed_outcome"`
	CanEvaluateWatchMissedOpportunity bool `json:"can_evaluate_watch_missed_opportunity"`
	CanEvaluateAIWait                 bool `json:"can_evaluate_ai_wait"`
	CanEvaluateConflictDowngrade      bool `json:"can_evaluate_conflict_downgrade"`
}

type EvaluationFreshnessMarker struct {
	Source      string    `json:"source"`
	LastEventAt time.Time `json:"last_event_at"`
	AgeMinutes  float64   `json:"age_minutes"`
	Status      string    `json:"status"`
}

type PlaybookStats struct {
	TotalSignals         int     `json:"total_signals"`
	WinRate              float64 `json:"win_rate"`
	TP1Rate              float64 `json:"tp1_rate"`
	TP2Rate              float64 `json:"tp2_rate"`
	SLRate               float64 `json:"sl_rate"`
	ExpiredRate          float64 `json:"expired_rate"`
	AverageMAE           float64 `json:"average_mae"`
	AverageMFE           float64 `json:"average_mfe"`
	AverageHoldTime      float64 `json:"average_hold_time_mins"`
	AverageTimeToTP1     float64 `json:"average_time_to_tp1_mins"`
	AverageTimeToTP2     float64 `json:"average_time_to_tp2_mins"`
	AverageTimeToSL      float64 `json:"average_time_to_sl_mins"`
	MaxMAE               float64 `json:"max_mae"`
	TP2FollowThroughRate float64 `json:"tp2_follow_through_rate"` // % of TP1 that hit TP2
}

type RegimeStats struct {
	TotalSignals int     `json:"total_signals"`
	WinRate      float64 `json:"win_rate"`
}

type TierStats struct {
	TotalSignals int     `json:"total_signals"`
	WinRate      float64 `json:"win_rate"`
}

type DirectionStats struct {
	TotalSignals int     `json:"total_signals"`
	WinRate      float64 `json:"win_rate"`
}

type AIStats struct {
	TotalSignals int     `json:"total_signals"`
	WinRate      float64 `json:"win_rate"`
}

type StalenessStats struct {
	TotalSignals int     `json:"total_signals"`
	WinRate      float64 `json:"win_rate"`
}

type SetupDiagnosticStats struct {
	Direction          string  `json:"direction"`
	MarketRegime       string  `json:"market_regime"`
	Playbook           string  `json:"playbook"`
	TotalSignals       int     `json:"total_signals"`
	WinRate            float64 `json:"win_rate"`
	TP1Rate            float64 `json:"tp1_rate"`
	TP2Rate            float64 `json:"tp2_rate"`
	SLRate             float64 `json:"sl_rate"`
	ExpiredRate        float64 `json:"expired_rate"`
	AverageMAE         float64 `json:"average_mae"`
	AverageMFE         float64 `json:"average_mfe"`
	AverageRR          float64 `json:"average_rr"`
	TotalPnlPercentage float64 `json:"total_pnl_percentage"`
}

type SetupMemorySlice struct {
	Symbol             string  `json:"symbol"`
	Direction          string  `json:"direction"`
	MarketRegime       string  `json:"market_regime"`
	Playbook           string  `json:"playbook"`
	TotalSignals       int     `json:"total_signals"`
	WinRate            float64 `json:"win_rate"`
	TP2Rate            float64 `json:"tp2_rate"`
	SLRate             float64 `json:"sl_rate"`
	ExpiredRate        float64 `json:"expired_rate"`
	AverageRR          float64 `json:"average_rr"`
	TotalPnlPercentage float64 `json:"total_pnl_percentage"`
	LastStatus         string  `json:"last_status"`
	LastOutcomeReason  string  `json:"last_outcome_reason,omitempty"`
	LastDecisionBrief  string  `json:"last_decision_brief,omitempty"`
}

type LearningReview struct {
	IssueType        string `json:"issue_type"`
	Playbook         string `json:"playbook"`
	MarketRegime     string `json:"market_regime,omitempty"`
	Direction        string `json:"direction,omitempty"`
	PolicyMode       string `json:"policy_mode,omitempty"`
	Summary          string `json:"summary"`
	SuggestedAction  string `json:"suggested_action,omitempty"`
	ConfidenceLevel  string `json:"confidence_level,omitempty"`
	Severity         string `json:"severity,omitempty"`
	SampleSize       int    `json:"sample_size,omitempty"`
	ReviewOnly       bool   `json:"review_only"`
	DoNotAutoApply   bool   `json:"do_not_auto_apply"`
	RequiresMoreData bool   `json:"requires_more_data"`
}

type EvaluationReport struct {
	SchemaVersion             string                               `json:"schema_version,omitempty"`
	ConfigVersion             string                               `json:"config_version,omitempty"`
	GeneratedAt               time.Time                            `json:"generated_at"`
	SourceFilesUsed           []string                             `json:"source_files_used"`
	DataCompleteness          DataCompleteness                     `json:"data_completeness"`
	FreshnessMarkers          map[string]EvaluationFreshnessMarker `json:"freshness_markers,omitempty"`
	TotalSignals              int                                  `json:"total_signals"`
	Metrics                   map[string]float64                   `json:"metrics"`
	PlaybookStats             map[string]PlaybookStats             `json:"playbook_stats"`
	RegimeStats               map[string]RegimeStats               `json:"regime_stats"`
	TierStats                 map[string]TierStats                 `json:"tier_stats"`
	DirectionStats            map[string]DirectionStats            `json:"direction_stats"`
	AIStats                   map[string]AIStats                   `json:"ai_stats"`
	StalenessStats            map[string]StalenessStats            `json:"staleness_stats"`
	LongRegimePlaybookStats   []SetupDiagnosticStats               `json:"long_regime_playbook_stats,omitempty"`
	WeakLongSetups            []SetupDiagnosticStats               `json:"weak_long_setups,omitempty"`
	SetupMemorySlices         []SetupMemorySlice                   `json:"setup_memory_slices,omitempty"`
	LearningReviews           []LearningReview                     `json:"learning_reviews,omitempty"`
	ConflictStats             map[string]int                       `json:"conflict_stats,omitempty"`
	CooldownStats             map[string]int                       `json:"cooldown_stats,omitempty"`
	GateBugFindings           []string                             `json:"gate_bug_findings"`
	Recommendations           []ThresholdRecommendation            `json:"recommendations"`
	WatchAgeDistribution      WatchAgeDistribution                 `json:"watch_age_distribution"`
	PromotionBlockerStats     PromotionBlockerStats                `json:"promotion_blocker_stats"`
	WatchEligibleNotPromoted  int                                  `json:"watch_eligible_not_promoted"`
	BestPlaybook              string                               `json:"best_playbook"`
	WorstPlaybook             string                               `json:"worst_playbook"`
	SetupYangSeringLangsungSL string                               `json:"setup_yang_sering_langsung_sl"`
	SetupYangSeringExpired    string                               `json:"setup_yang_sering_expired"`
	SetupYangSeringStale      string                               `json:"setup_yang_sering_stale"`
	RegimeYangPalingBuruk     string                               `json:"regime_yang_paling_buruk"`
	TierYangPalingBuruk       string                               `json:"tier_yang_paling_buruk"`
	DirectionYangPalingBuruk  string                               `json:"direction_yang_paling_buruk"`
	PlaybookDenganMAETerbesar string                               `json:"playbook_dengan_mae_terbesar"`
	PlaybookDenganExpiredRate string                               `json:"playbook_dengan_expired_rate_terbesar"`
	PlaybookDenganTP1Terbaik  string                               `json:"playbook_dengan_tp1_rate_terbaik"`
	PlaybookDenganTP2Follow   string                               `json:"playbook_dengan_tp2_follow_through_terbaik"`
	Notes                     string                               `json:"notes"`
	Status                    Status                               `json:"status"`
}

type ScannerSummaryV3 struct {
	TotalScanned                    int                             `json:"total_scanned"`
	CandidatesFound                 int                             `json:"candidates_found"`
	StartTime                       time.Time                       `json:"start_time"`
	Duration                        string                          `json:"duration"`
	ActiveRegime                    string                          `json:"active_regime"`
	BtcTrend                        string                          `json:"btc_trend"`
	TotalTickers                    int                             `json:"total_tickers"`
	TotalUniversePass               int                             `json:"total_universe_pass"`
	TotalUniverseRejected           int                             `json:"total_universe_rejected"`
	TotalStrategySelected           int                             `json:"total_strategy_selected"`
	TotalPlaybookEligible           int                             `json:"total_playbook_eligible"`
	TotalQuantCandidates            int                             `json:"total_quant_candidates"`
	TotalArbiterSelected            int                             `json:"total_arbiter_selected"`
	TotalLocalAICandidate           int                             `json:"total_local_ai_candidate"`
	PrefetchLimit                   int                             `json:"prefetch_limit,omitempty"`
	TotalPrefetchSelected           int                             `json:"total_prefetch_selected,omitempty"`
	TotalPrefetchDeferred           int                             `json:"total_prefetch_deferred,omitempty"`
	PrefetchHotSlots                int                             `json:"prefetch_hot_slots,omitempty"`
	PrefetchRotationSlots           int                             `json:"prefetch_rotation_slots,omitempty"`
	TotalAIBatchEntered             int                             `json:"total_ai_batch_entered,omitempty"`
	TotalAICalled                   int                             `json:"total_ai_called,omitempty"`
	TotalAISyntheticLocalGate       int                             `json:"total_ai_synthetic_local_gate,omitempty"`
	TotalAISkippedQuota             int                             `json:"total_ai_skipped_quota,omitempty"`
	TotalAIDisabled                 int                             `json:"total_ai_disabled,omitempty"`
	TotalAIConfirm                  int                             `json:"total_ai_confirm"`
	TotalAIWait                     int                             `json:"total_ai_wait"`
	TotalAIReject                   int                             `json:"total_ai_reject"`
	TotalAIError                    int                             `json:"total_ai_error"`
	TotalFinalExecute               int                             `json:"total_final_execute"`
	TotalFinalWatch                 int                             `json:"total_final_watch"`
	TotalFinalReject                int                             `json:"total_final_reject"`
	ExecuteSignals                  []dto.SignalResponse            `json:"execute_signals"`
	Watchlist                       []dto.SignalResponse            `json:"watchlist"`
	RejectedSummary                 []string                        `json:"rejected_summary"`
	PolicyRejectedSummary           []string                        `json:"policy_rejected_summary"`
	SelectedThresholdProfileSummary map[string]string               `json:"selected_threshold_profile_summary"`
	FunnelStageSummary              []entity.FunnelStageSummary     `json:"funnel_stage_summary,omitempty"`
	TopFunnelBlockers               []string                        `json:"top_funnel_blockers,omitempty"`
	PlaybookBlockerSummary          []entity.PlaybookBlockerSummary `json:"playbook_blocker_summary,omitempty"`
	EvaluationDataCompletenessHint  string                          `json:"evaluation_data_completeness_hint"`
}

type SignalNotificationRequest struct {
	Decision      FinalDecision
	AuditResponse dto.AIAuditResponse
}

type HotSymbol struct {
	Symbol   string  `json:"symbol"`
	Score    float64 `json:"score"`
	Source   string  `json:"source"`
	RankType int     `json:"rank_type"`
}

type HotInfo struct {
	IsHot              bool    `json:"is_hot"`
	HotScore           float64 `json:"hot_score"`
	HotSource          string  `json:"hot_source"`
	HotRankType        int     `json:"hot_rank_type"`
	HotOverlaySelected bool    `json:"hot_overlay_selected"`
}

type HotSymbolProvider interface {
	FetchHotSymbols(ctx context.Context) ([]HotSymbol, error)
}

// Interfaces

type MarketDataProvider interface {
	FetchClosedCandles(ctx context.Context, symbol string, interval string, limit int) ([]dto.Candle, error)
	FetchLatestPrice(ctx context.Context, symbol string) (float64, error)
	FetchAllFuturesTickers24h(ctx context.Context) ([]dto.Ticker24h, error)
	FetchPremiumFundingRates(ctx context.Context) (map[string]float64, error)
	FetchOpenInterest(ctx context.Context, symbol string) (float64, error)
	FetchHistoricalCandles(ctx context.Context, symbol string, interval string, startTime time.Time, endTime time.Time) ([]dto.Candle, error)
}

type AIAuditorService interface {
	AuditCandidate(ctx context.Context, req dto.AIAuditRequest) (*dto.AIAuditResponse, error)
}

type SignalNotificationService interface {
	SendSignalMessage(ctx context.Context, msg string) error
}

type OpsNotificationService interface {
	SendOpsMessage(ctx context.Context, msg string) error
}

type StorageRepository interface {
	LoadLatestResult() (*entity.LatestResult, error)
	SaveLatestResult(res *entity.LatestResult) error

	LoadSignalHistory() (*entity.SignalHistory, error)
	SaveSignalHistory(hist *entity.SignalHistory) error

	LoadSignalJournal() ([]SignalJournal, error)
	SaveSignalJournal(journal []SignalJournal) error
	AppendSignalJournal(entry SignalJournal) error

	LoadWatchJournal() ([]WatchJournal, error)
	SaveWatchJournal(journal []WatchJournal) error
	AppendWatchJournal(entry WatchJournal) error

	LoadAIAuditCache() (*entity.AIAuditCache, error)
	SaveAIAuditCache(cache *entity.AIAuditCache) error

	LoadEvaluationReport() (*EvaluationReport, error)
	SaveEvaluationReport(report *EvaluationReport) error

	LoadDecisionAudits() ([]DecisionAudit, error)
	SaveDecisionAudits(audits []DecisionAudit) error
	AppendDecisionAudit(entry DecisionAudit) error
}

// FormatNotificationTime formats a time.Time into Asia/Jakarta (WIB) timezone for readable Telegram messages.
func FormatNotificationTime(t time.Time) string {
	if t.IsZero() {
		return "N/A"
	}
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		loc = time.Local
	}
	return t.In(loc).Format("2006-01-02 15:04:05 MST")
}
