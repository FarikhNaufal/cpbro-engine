package entity

import (
	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"time"
)

type ArbiterSelectedDetail struct {
	Symbol          string `json:"symbol"`
	Playbook        string `json:"playbook"`
	Direction       string `json:"direction"`
	LocalGateStatus string `json:"local_gate_status"`
	AIDecision      string `json:"ai_decision"`
	StalenessStatus string `json:"staleness_status"`
	FinalStatus     string `json:"final_status"`
	FinalReason     string `json:"final_reason"`
}

type FunnelReasonCount struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

type FunnelStageSummary struct {
	Stage   string              `json:"stage"`
	Total   int                 `json:"total"`
	Reasons []FunnelReasonCount `json:"reasons"`
}

type PlaybookBlockerSummary struct {
	Playbook string               `json:"playbook"`
	Total    int                  `json:"total"`
	Stages   []FunnelStageSummary `json:"stages"`
}

type LatestResult struct {
	SchemaVersion                   string                   `json:"schema_version,omitempty"`
	ConfigVersion                   string                   `json:"config_version,omitempty"`
	GeneratedAt                     time.Time                `json:"generated_at"`
	ScanID                          string                   `json:"scan_id"`
	MarketPolicy                    string                   `json:"market_policy"`
	MarketRegime                    string                   `json:"market_regime"`
	MacroVolatility                 string                   `json:"macro_volatility,omitempty"`
	MarketBreadth                   float64                  `json:"market_breadth,omitempty"`
	MedianAbsMove24h                float64                  `json:"median_abs_move_24h,omitempty"`
	ActiveMoveShare                 float64                  `json:"active_move_share,omitempty"`
	QuietMoveShare                  float64                  `json:"quiet_move_share,omitempty"`
	TotalTickers                    int                      `json:"total_tickers"`
	TotalUniversePass               int                      `json:"total_universe_pass"`
	TotalUniverseRejected           int                      `json:"total_universe_rejected"`
	TotalStrategySelected           int                      `json:"total_strategy_selected"`
	TotalPlaybookEligible           int                      `json:"total_playbook_eligible"`
	TotalQuantCandidates            int                      `json:"total_quant_candidates"`
	TotalArbiterSelected            int                      `json:"total_arbiter_selected"`
	TotalLocalAICandidate           int                      `json:"total_local_ai_candidate"`
	TotalAIConfirm                  int                      `json:"total_ai_confirm"`
	TotalAIWait                     int                      `json:"total_ai_wait"`
	TotalAIReject                   int                      `json:"total_ai_reject"`
	TotalAIError                    int                      `json:"total_ai_error"`
	TotalFinalExecute               int                      `json:"total_final_execute"`
	TotalFinalWatch                 int                      `json:"total_final_watch"`
	TotalFinalReject                int                      `json:"total_final_reject"`
	ExecuteSignals                  []dto.SignalResponse     `json:"execute_signals"`
	Watchlist                       []dto.SignalResponse     `json:"watchlist"`
	RejectedSummary                 []string                 `json:"rejected_summary"`
	PolicyRejectedSummary           []string                 `json:"policy_rejected_summary"`
	SelectedThresholdProfileSummary map[string]string        `json:"selected_threshold_profile_summary"`
	FunnelStageSummary              []FunnelStageSummary     `json:"funnel_stage_summary,omitempty"`
	TopFunnelBlockers               []string                 `json:"top_funnel_blockers,omitempty"`
	PlaybookBlockerSummary          []PlaybookBlockerSummary `json:"playbook_blocker_summary,omitempty"`
	EvaluationDataCompletenessHint  string                   `json:"evaluation_data_completeness_hint"`
	CompressionZeroEligibleStreak   int                      `json:"compression_zero_eligible_streak,omitempty"`
	CompressionLowVolFallbackActive bool                     `json:"compression_low_vol_fallback_active,omitempty"`
	ArbiterSelectedDetails          []ArbiterSelectedDetail  `json:"arbiter_selected_details,omitempty"`

	// Backward compatibility fields
	LastScanTime time.Time            `json:"last_scan_time"`
	Duration     string               `json:"duration"`
	Signals      []dto.SignalResponse `json:"signals"`
}

type SignalHistory struct {
	Signals []dto.SignalResponse `json:"signals"`
}

type AIAuditCache struct {
	CacheMap map[string]CachedAudit `json:"cache_map"`
}

type CachedAudit struct {
	Response dto.AIAuditResponse `json:"response"`
	CachedAt time.Time           `json:"cached_at"`
}

type EvaluationReport struct {
	TotalSignals       int       `json:"total_signals"`
	WinCount           int       `json:"win_count"`
	LossCount          int       `json:"loss_count"`
	WinRate            float64   `json:"win_rate"`
	TotalPnlPercentage float64   `json:"total_pnl_percentage"`
	LastEvaluatedAt    time.Time `json:"last_evaluated_at"`
}
