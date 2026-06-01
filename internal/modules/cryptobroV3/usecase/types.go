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
	TP1_HIT           Status = "TP1_HIT"
	TP2_HIT           Status = "TP2_HIT"
	SL_HIT            Status = "SL_HIT"
	EXPIRED           Status = "EXPIRED"
	BREAKEVEN         Status = "BREAKEVEN"
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
	MinScoreAI             float64      `json:"min_score_ai"`
	MinScoreExecute        float64      `json:"min_score_execute"`
	MinRRExecute           float64      `json:"min_rr_execute"`
	MinADXExecute          float64      `json:"min_adx_execute"`
	RequireAIConfidence    AIConfidence `json:"require_ai_confidence"`
	RequireFreshEntry      bool         `json:"require_fresh_entry"`
	StalenessATRMultiplier float64      `json:"staleness_atr_multiplier"`
	CooldownMinutes        int          `json:"cooldown_minutes"`
	BtcTrend               string       `json:"btc_trend"`
	BtcScore               float64      `json:"btc_score"`
	BtcChaos               float64      `json:"btc_chaos"`
	Reason                 string       `json:"reason"`
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
	Symbol string `json:"symbol"`
	Tier   Tier   `json:"tier"`
	Status Status `json:"status"`
	Notes  string `json:"notes"`
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
	Passed bool   `json:"passed"`
	Status Status `json:"status"`
	Reason string `json:"reason"`
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
	RequiredRR              float64   `json:"required_rr"`
	AIConfidence            string    `json:"ai_confidence"`
	StalenessStatus         string    `json:"staleness_status"`
	PolicySummary           string    `json:"policy_summary"`
	ThresholdProfileSummary string    `json:"threshold_profile_summary"`
	IsExecutable            bool      `json:"is_executable"`
	Tier                    Tier      `json:"tier"`
	EntryPrice              float64   `json:"entry_price"`
	StopLoss                float64   `json:"stop_loss"`
	TakeProfit              float64   `json:"take_profit"`
	WatchReason             string    `json:"watch_reason,omitempty"`
	RejectReason            string    `json:"reject_reason,omitempty"`
}

type SignalJournal struct {
	SchemaVersion           string    `json:"schema_version,omitempty"`
	ConfigVersion           string    `json:"config_version,omitempty"`
	ID                      string    `json:"signal_id"`
	Symbol                  string    `json:"symbol"`
	Direction               Direction `json:"direction"`
	Playbook                Playbook  `json:"playbook"`
	EntryPrice              float64   `json:"entry"`
	StopLoss                float64   `json:"sl"`
	TP1                     float64   `json:"tp1"`
	TP2                     float64   `json:"tp2"`
	RR                      float64   `json:"rr"`
	QuantScore              float64   `json:"score"`
	AIConfidence            string    `json:"ai_confidence"`
	MarketRegime            string    `json:"market_regime"`
	PolicyMode              string    `json:"policy_mode"`
	ThresholdProfileSummary string    `json:"threshold_profile_summary"`
	CreatedAt               time.Time `json:"created_at"`
	ExpiresAt               time.Time `json:"expires_at"`
	Status                  Status    `json:"status"`
	MFE                     float64   `json:"mfe"`
	MAE                     float64   `json:"mae"`
	TimeToTP1               string    `json:"time_to_tp1"`
	TimeToTP2               string    `json:"time_to_tp2"`
	TimeToSL                string    `json:"time_to_sl"`
	OutcomeReason           string    `json:"outcome_reason"`
	EntryTiming             string    `json:"entry_timing"`
	Tier                    Tier      `json:"tier"`

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
}

type DecisionAudit struct {
	SchemaVersion             string    `json:"schema_version,omitempty"`
	ConfigVersion             string    `json:"config_version,omitempty"`
	ScanID                    string    `json:"scan_id"`
	GeneratedAt               time.Time `json:"generated_at"`
	Symbol                    string    `json:"symbol"`
	Direction                 Direction `json:"direction"`
	Playbook                  Playbook  `json:"playbook"`
	SetupType                 string    `json:"setup_type"`
	Tier                      Tier      `json:"tier"`
	Grade                     string    `json:"grade"`
	Score                     float64   `json:"score"`
	RR                        float64   `json:"rr"`
	RequiredScore             float64   `json:"required_score"`
	RequiredRR                float64   `json:"required_rr"`
	LocalGateStatus           string    `json:"local_gate_status"`
	LocalGateReason           string    `json:"local_gate_reason"`
	AIDecision                string    `json:"ai_decision"`
	AIConfidence              string    `json:"ai_confidence"`
	AICandleNarrative         string    `json:"ai_candle_narrative"`
	AIEntryTiming             string    `json:"ai_entry_timing"`
	AIConflictWithBot         bool      `json:"ai_conflict_with_bot"`
	PlanStatus                string    `json:"plan_status"`
	PlanConflict              bool      `json:"plan_conflict"`
	NeedRetest                bool      `json:"need_retest"`
	StalenessStatus           string    `json:"staleness_status"`
	FinalStatusBeforeConflict Status    `json:"final_status_before_conflict"`
	FinalReasonBeforeConflict string    `json:"final_reason_before_conflict"`
	FinalStatusAfterConflict  Status    `json:"final_status_after_conflict"`
	FinalReasonAfterConflict  string    `json:"final_reason_after_conflict"`
	FinalStatus               Status    `json:"final_status"`
	FinalReason               string    `json:"final_reason"`
	ConflictReason            string    `json:"conflict_reason"`
	CooldownReason            string    `json:"cooldown_reason"`
	WasNotified               bool      `json:"was_notified"`
	LatestPriceAtDecision     float64   `json:"latest_price_at_decision"`
	EntryPrice                float64   `json:"entry"`
	StopLoss                  float64   `json:"sl"`
	TakeProfit1               float64   `json:"tp1"`
	TakeProfit2               float64   `json:"tp2"`
	MarketRegime              string    `json:"market_regime"`
	PolicyMode                string    `json:"policy_mode"`
	ThresholdProfileSummary   string    `json:"threshold_profile_summary"`
	RejectOrWatchReason       string    `json:"reject_or_watch_reason"`
	CreatedAt                 time.Time `json:"created_at"`

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

type EvaluationReport struct {
	SchemaVersion             string                    `json:"schema_version,omitempty"`
	ConfigVersion             string                    `json:"config_version,omitempty"`
	GeneratedAt               time.Time                 `json:"generated_at"`
	SourceFilesUsed           []string                  `json:"source_files_used"`
	DataCompleteness          DataCompleteness          `json:"data_completeness"`
	TotalSignals              int                       `json:"total_signals"`
	Metrics                   map[string]float64        `json:"metrics"`
	PlaybookStats             map[string]PlaybookStats  `json:"playbook_stats"`
	RegimeStats               map[string]RegimeStats    `json:"regime_stats"`
	TierStats                 map[string]TierStats      `json:"tier_stats"`
	DirectionStats            map[string]DirectionStats `json:"direction_stats"`
	AIStats                   map[string]AIStats        `json:"ai_stats"`
	StalenessStats            map[string]StalenessStats `json:"staleness_stats"`
	ConflictStats             map[string]int            `json:"conflict_stats,omitempty"`
	CooldownStats             map[string]int            `json:"cooldown_stats,omitempty"`
	GateBugFindings           []string                  `json:"gate_bug_findings"`
	Recommendations           []ThresholdRecommendation `json:"recommendations"`
	BestPlaybook              string                    `json:"best_playbook"`
	WorstPlaybook             string                    `json:"worst_playbook"`
	SetupYangSeringLangsungSL string                    `json:"setup_yang_sering_langsung_sl"`
	SetupYangSeringExpired    string                    `json:"setup_yang_sering_expired"`
	SetupYangSeringStale      string                    `json:"setup_yang_sering_stale"`
	RegimeYangPalingBuruk     string                    `json:"regime_yang_paling_buruk"`
	TierYangPalingBuruk       string                    `json:"tier_yang_paling_buruk"`
	DirectionYangPalingBuruk  string                    `json:"direction_yang_paling_buruk"`
	PlaybookDenganMAETerbesar string                    `json:"playbook_dengan_mae_terbesar"`
	PlaybookDenganExpiredRate string                    `json:"playbook_dengan_expired_rate_terbesar"`
	PlaybookDenganTP1Terbaik  string                    `json:"playbook_dengan_tp1_rate_terbaik"`
	PlaybookDenganTP2Follow   string                    `json:"playbook_dengan_tp2_follow_through_terbaik"`
	Notes                     string                    `json:"notes"`
	Status                    Status                    `json:"status"`
}

type ScannerSummaryV3 struct {
	TotalScanned                    int                  `json:"total_scanned"`
	CandidatesFound                 int                  `json:"candidates_found"`
	StartTime                       time.Time            `json:"start_time"`
	Duration                        string               `json:"duration"`
	ActiveRegime                    string               `json:"active_regime"`
	BtcTrend                        string               `json:"btc_trend"`
	TotalTickers                    int                  `json:"total_tickers"`
	TotalUniversePass               int                  `json:"total_universe_pass"`
	TotalUniverseRejected           int                  `json:"total_universe_rejected"`
	TotalStrategySelected           int                  `json:"total_strategy_selected"`
	TotalPlaybookEligible           int                  `json:"total_playbook_eligible"`
	TotalQuantCandidates            int                  `json:"total_quant_candidates"`
	TotalArbiterSelected            int                  `json:"total_arbiter_selected"`
	TotalLocalAICandidate           int                  `json:"total_local_ai_candidate"`
	TotalAIConfirm                  int                  `json:"total_ai_confirm"`
	TotalAIWait                     int                  `json:"total_ai_wait"`
	TotalAIReject                   int                  `json:"total_ai_reject"`
	TotalAIError                    int                  `json:"total_ai_error"`
	TotalFinalExecute               int                  `json:"total_final_execute"`
	TotalFinalWatch                 int                  `json:"total_final_watch"`
	TotalFinalReject                int                  `json:"total_final_reject"`
	ExecuteSignals                  []dto.SignalResponse `json:"execute_signals"`
	Watchlist                       []dto.SignalResponse `json:"watchlist"`
	RejectedSummary                 []string             `json:"rejected_summary"`
	PolicyRejectedSummary           []string             `json:"policy_rejected_summary"`
	SelectedThresholdProfileSummary map[string]string    `json:"selected_threshold_profile_summary"`
	EvaluationDataCompletenessHint  string               `json:"evaluation_data_completeness_hint"`
}

type SignalNotificationRequest struct {
	Decision      FinalDecision
	AuditResponse dto.AIAuditResponse
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
