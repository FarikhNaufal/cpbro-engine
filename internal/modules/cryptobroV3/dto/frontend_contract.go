package dto

type HealthResponse struct {
	AppName            string         `json:"app_name"`
	AppVersion         string         `json:"app_version"`
	AppEnv             string         `json:"app_env"`
	Mode               string         `json:"mode"`
	AlertOnly          bool           `json:"alert_only"`
	BinanceReadOnly    bool           `json:"binance_read_only"`
	ScannerRunning     bool           `json:"scanner_running"`
	LastScanTime       string         `json:"last_scan_time"`
	LastEvaluationTime string         `json:"last_evaluation_time"`
	StorageAvailable   bool           `json:"storage_available"`
	SwaggerEnabled     bool           `json:"swagger_enabled,omitempty"`
	UptimeSeconds      float64        `json:"uptime_seconds,omitempty"`
	Status             string         `json:"status"`
	Warnings           []string       `json:"warnings,omitempty"`
	SafeConfig         map[string]any `json:"safe_config,omitempty"`
}

type Signal struct {
	Symbol         string  `json:"symbol"`
	Direction      string  `json:"direction"`
	Timeframe      string  `json:"timeframe"`
	TriggerPrice   float64 `json:"trigger_price"`
	StopLoss       float64 `json:"stop_loss"`
	TakeProfit     float64 `json:"take_profit"`
	Score          float64 `json:"score"`
	Strategy       string  `json:"strategy"`
	AISentiment    string  `json:"ai_sentiment"`
	IsFinalExecute bool    `json:"is_final_execute"`
	ReconciledTime string  `json:"reconciled_time"`
	Status         string  `json:"status"`
}

type WatchSignal struct {
	Symbol         string  `json:"symbol"`
	Direction      string  `json:"direction"`
	Timeframe      string  `json:"timeframe"`
	TriggerPrice   float64 `json:"trigger_price"`
	StopLoss       float64 `json:"stop_loss"`
	TakeProfit     float64 `json:"take_profit"`
	Score          float64 `json:"score"`
	Strategy       string  `json:"strategy"`
	AISentiment    string  `json:"ai_sentiment"`
	IsFinalExecute bool    `json:"is_final_execute"`
	ReconciledTime string  `json:"reconciled_time"`
	Status         string  `json:"status"`
	Reason         string  `json:"reason,omitempty"`
	FinalReason    string  `json:"final_reason,omitempty"`
}

type ArbiterSelectedDetail struct {
	Symbol          string `json:"symbol"`
	Playbook        string `json:"playbook"`
	Direction       string `json:"direction"`
	LocalGateStatus string `json:"local_gate_status"`
	AIDecision      string `json:"ai_decision"`
	AIConfidence    string `json:"ai_confidence,omitempty"`
	StalenessStatus string `json:"staleness_status"`
	FinalStatus     string `json:"final_status"`
	FinalReason     string `json:"final_reason"`
}

type LatestResultResponse struct {
	ConfigVersion                  string                  `json:"config_version"`
	GeneratedAt                    string                  `json:"generated_at"`
	ScanID                         string                  `json:"scan_id"`
	MarketPolicy                   string                  `json:"market_policy"`
	MarketRegime                   string                  `json:"market_regime"`
	TotalTickers                   int                     `json:"total_tickers"`
	TotalUniversePass              int                     `json:"total_universe_pass"`
	TotalUniverseRejected          int                     `json:"total_universe_rejected"`
	TotalStrategySelected          int                     `json:"total_strategy_selected"`
	TotalPlaybookEligible          int                     `json:"total_playbook_eligible"`
	TotalQuantCandidates           int                     `json:"total_quant_candidates"`
	TotalArbiterSelected           int                     `json:"total_arbiter_selected"`
	TotalLocalAICandidate          int                     `json:"total_local_ai_candidate"`
	TotalAIConfirm                 int                     `json:"total_ai_confirm"`
	TotalAIWait                    int                     `json:"total_ai_wait"`
	TotalAIReject                  int                     `json:"total_ai_reject"`
	TotalAIError                   int                     `json:"total_ai_error"`
	TotalFinalExecute              int                     `json:"total_final_execute"`
	TotalFinalWatch                int                     `json:"total_final_watch"`
	TotalFinalReject               int                     `json:"total_final_reject"`
	ExecuteSignals                 []Signal                `json:"execute_signals"`
	Watchlist                      []WatchSignal           `json:"watchlist"`
	RejectedSummary                []string                `json:"rejected_summary"`
	PolicyRejectedSummary          []string                `json:"policy_rejected_summary"`
	ThresholdProfileSummary        map[string]string       `json:"threshold_profile_summary"`
	EvaluationDataCompletenessHint string                  `json:"evaluation_data_completeness_hint"`
	ArbiterSelectedDetails         []ArbiterSelectedDetail `json:"arbiter_selected_details"`
	LastScanTime                   string                  `json:"last_scan_time"`
	Duration                       string                  `json:"duration"`
	Signals                        []Signal                `json:"signals"`
	Warnings                       []string                `json:"warnings"`
	PartialErrors                  []string                `json:"partial_errors"`
}

type SignalJournalResponse struct {
	SchemaVersion           string  `json:"schema_version,omitempty"`
	ConfigVersion           string  `json:"config_version,omitempty"`
	ID                      string  `json:"signal_id"`
	Symbol                  string  `json:"symbol"`
	Direction               string  `json:"direction"`
	Playbook                string  `json:"playbook"`
	EntryPrice              float64 `json:"entry"`
	StopLoss                float64 `json:"sl"`
	TP1                     float64 `json:"tp1"`
	TP2                     float64 `json:"tp2"`
	RR                      float64 `json:"rr"`
	QuantScore              float64 `json:"score"`
	AIConfidence            string  `json:"ai_confidence"`
	MarketRegime            string  `json:"market_regime"`
	PolicyMode              string  `json:"policy_mode"`
	ThresholdProfileSummary string  `json:"threshold_profile_summary"`
	CreatedAt               string  `json:"created_at"`
	ExpiresAt               string  `json:"expires_at"`
	Status                  string  `json:"status"`
	MFE                     float64 `json:"mfe"`
	MAE                     float64 `json:"mae"`
	TimeToTP1               string  `json:"time_to_tp1"`
	TimeToTP2               string  `json:"time_to_tp2"`
	TimeToSL                string  `json:"time_to_sl"`
	OutcomeReason           string  `json:"outcome_reason"`
	EntryTiming             string  `json:"entry_timing"`
	Tier                    string  `json:"tier"`
	Timeframe               string  `json:"timeframe,omitempty"`
	LatestPrice             float64 `json:"latest_price,omitempty"`
	TakeProfit              float64 `json:"take_profit,omitempty"`
	AISentiment             string  `json:"ai_sentiment,omitempty"`
	AIReasoning             string  `json:"ai_reasoning,omitempty"`
	PnlPercentage           float64 `json:"pnl_percentage,omitempty"`
	UpdatedAt               string  `json:"updated_at,omitempty"`
	ClosedAt                string  `json:"closed_at,omitempty"`
	Reason                  string  `json:"reason,omitempty"`
	NotificationStatus      string  `json:"notification_status,omitempty"`
	NotificationError       string  `json:"notification_error,omitempty"`
}

type JournalResponse struct {
	Items   []SignalJournalResponse `json:"items"`
	Total   int                     `json:"total"`
	Limit   int                     `json:"limit"`
	Offset  int                     `json:"offset"`
	Filters JournalFilters          `json:"filters"`
}

type JournalFilters struct {
	Symbol    string `json:"symbol"`
	Playbook  string `json:"playbook"`
	Status    string `json:"status"`
	Direction string `json:"direction"`
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

type PlaybookStats struct {
	TotalSignals int     `json:"total_signals"`
	WinRate      float64 `json:"win_rate"`
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

type GateBugFinding string

type NamedPlaybookStats struct {
	Key   string        `json:"key"`
	Value PlaybookStats `json:"value"`
}
type NamedRegimeStats struct {
	Key   string      `json:"key"`
	Value RegimeStats `json:"value"`
}
type NamedTierStats struct {
	Key   string    `json:"key"`
	Value TierStats `json:"value"`
}
type NamedDirectionStats struct {
	Key   string         `json:"key"`
	Value DirectionStats `json:"value"`
}
type NamedAIStats struct {
	Key   string  `json:"key"`
	Value AIStats `json:"value"`
}
type NamedStalenessStats struct {
	Key   string         `json:"key"`
	Value StalenessStats `json:"value"`
}
type NamedIntStat struct {
	Key   string `json:"key"`
	Value int    `json:"value"`
}

type EvaluationResponse struct {
	GeneratedAt               string                    `json:"generated_at"`
	DataCompleteness          DataCompleteness          `json:"data_completeness"`
	TotalSignals              int                       `json:"total_signals"`
	Metrics                   map[string]float64        `json:"metrics"`
	PlaybookStats             []NamedPlaybookStats      `json:"playbook_stats"`
	RegimeStats               []NamedRegimeStats        `json:"regime_stats"`
	TierStats                 []NamedTierStats          `json:"tier_stats"`
	DirectionStats            []NamedDirectionStats     `json:"direction_stats"`
	AIStats                   []NamedAIStats            `json:"ai_stats"`
	StalenessStats            []NamedStalenessStats     `json:"staleness_stats"`
	ConflictStats             []NamedIntStat            `json:"conflict_stats"`
	CooldownStats             []NamedIntStat            `json:"cooldown_stats"`
	GateBugFindings           []GateBugFinding          `json:"gate_bug_findings"`
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
	Notes                     []string                  `json:"notes"`
	Status                    string                    `json:"status"`
}

type DecisionAuditRow struct {
	SchemaVersion             string  `json:"schema_version,omitempty"`
	ConfigVersion             string  `json:"config_version,omitempty"`
	ScanID                    string  `json:"scan_id"`
	GeneratedAt               string  `json:"generated_at"`
	Symbol                    string  `json:"symbol"`
	Direction                 string  `json:"direction"`
	Playbook                  string  `json:"playbook"`
	SetupType                 string  `json:"setup_type"`
	Tier                      string  `json:"tier"`
	Grade                     string  `json:"grade"`
	Score                     float64 `json:"score"`
	RR                        float64 `json:"rr"`
	RequiredScore             float64 `json:"required_score"`
	RequiredRR                float64 `json:"required_rr"`
	LocalGateStatus           string  `json:"local_gate_status"`
	LocalGateReason           string  `json:"local_gate_reason"`
	AIDecision                string  `json:"ai_decision"`
	AIConfidence              string  `json:"ai_confidence"`
	AICandleNarrative         string  `json:"ai_candle_narrative"`
	AIEntryTiming             string  `json:"ai_entry_timing"`
	AIConflictWithBot         bool    `json:"ai_conflict_with_bot"`
	PlanStatus                string  `json:"plan_status"`
	PlanConflict              bool    `json:"plan_conflict"`
	NeedRetest                bool    `json:"need_retest"`
	StalenessStatus           string  `json:"staleness_status"`
	FinalStatusBeforeConflict string  `json:"final_status_before_conflict"`
	FinalReasonBeforeConflict string  `json:"final_reason_before_conflict"`
	FinalStatusAfterConflict  string  `json:"final_status_after_conflict"`
	FinalReasonAfterConflict  string  `json:"final_reason_after_conflict"`
	FinalStatus               string  `json:"final_status"`
	FinalReason               string  `json:"final_reason"`
	ConflictReason            string  `json:"conflict_reason"`
	CooldownReason            string  `json:"cooldown_reason"`
	WasNotified               bool    `json:"was_notified"`
	LatestPriceAtDecision     float64 `json:"latest_price_at_decision"`
	EntryPrice                float64 `json:"entry"`
	StopLoss                  float64 `json:"sl"`
	TakeProfit1               float64 `json:"tp1"`
	TakeProfit2               float64 `json:"tp2"`
	MarketRegime              string  `json:"market_regime"`
	PolicyMode                string  `json:"policy_mode"`
	ThresholdProfileSummary   string  `json:"threshold_profile_summary"`
	RejectOrWatchReason       string  `json:"reject_or_watch_reason"`
	CreatedAt                 string  `json:"created_at"`
	HypotheticalEntry         float64 `json:"hypothetical_entry"`
}

type DecisionAuditResponse struct {
	Items   []DecisionAuditRow   `json:"items"`
	Total   int                  `json:"total"`
	Limit   int                  `json:"limit"`
	Offset  int                  `json:"offset"`
	Filters DecisionAuditFilters `json:"filters"`
}

type DecisionAuditFilters struct {
	ScanID      string `json:"scan_id"`
	Symbol      string `json:"symbol"`
	FinalStatus string `json:"final_status"`
	Playbook    string `json:"playbook"`
	Direction   string `json:"direction"`
}
