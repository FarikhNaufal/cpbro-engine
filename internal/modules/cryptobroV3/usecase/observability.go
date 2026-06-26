package usecase

import (
	"context"
	"cpbro-engine/internal/modules/cryptobroV3/config"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

// Pingable defines an interface for checking live AI Auditor connectivity
type Pingable interface {
	Ping(ctx context.Context) error
}

// TelegramPingable defines an interface for checking optional Telegram Bot API availability
type TelegramPingable interface {
	Ping(ctx context.Context) error
}

type RealtimePriceStatus struct {
	Enabled         bool      `json:"enabled"`
	Connected       bool      `json:"connected"`
	ActiveSymbols   int       `json:"active_symbols"`
	LastMessageTime time.Time `json:"last_message_time"`
}

type RecheckCadenceStatus struct {
	Enabled                 bool     `json:"enabled"`
	PreventOverlap          bool     `json:"prevent_overlap"`
	SharedPipelineGuard     bool     `json:"shared_pipeline_guard"`
	SkipPrimaryBoundary     bool     `json:"skip_primary_boundary"`
	PrimaryBoundaryMinutes  int      `json:"primary_boundary_minutes"`
	BoundaryMinutes         int      `json:"boundary_minutes"`
	CloseBufferSeconds      int      `json:"close_buffer_seconds"`
	MaxAgeMinutes           int      `json:"max_age_minutes"`
	EffectiveHorizonMinutes int      `json:"effective_horizon_minutes"`
	BatchLimit              int      `json:"batch_limit"`
	AllowedPlaybooks        []string `json:"allowed_playbooks,omitempty"`
}

type BootstrapProvenanceStatus struct {
	TickerSource       string `json:"ticker_source"`
	TickerFresh        bool   `json:"ticker_fresh"`
	TickerCacheAgeSec  uint64 `json:"ticker_cache_age_seconds"`
	FundingSource      string `json:"funding_source"`
	FundingFresh       bool   `json:"funding_fresh"`
	FundingCacheAgeSec uint64 `json:"funding_cache_age_seconds"`
}

func normalizeBootstrapSourceForHealth(source string) string {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return "unknown"
	}
	return trimmed
}

type WatchJournalStatusCounts struct {
	Monitoring         int `json:"monitoring"`
	VirtualTP1         int `json:"virtual_tp1"`
	Promoted           int `json:"promoted"`
	RecheckExpired     int `json:"recheck_expired"`
	RecheckInvalidated int `json:"recheck_invalidated"`
}

type RolloutReadinessStatus struct {
	Ready            bool     `json:"ready"`
	RecommendedPhase string   `json:"recommended_phase"`
	Blockers         []string `json:"blockers,omitempty"`
	RollbackCriteria []string `json:"rollback_criteria,omitempty"`
}

type RealtimeStatusProvider interface {
	RealtimeStatus() RealtimePriceStatus
}

// Global atomic variables for worker running statuses
var (
	ScanWorkerRunning       atomic.Bool
	RecheckWorkerRunning    atomic.Bool
	MonitoringWorkerRunning atomic.Bool
	EvaluationWorkerRunning atomic.Bool
)

type HealthStatus struct {
	Status                   string                    `json:"status"`
	Mode                     string                    `json:"mode"`
	BinanceConnectivity      string                    `json:"binance_connectivity"`
	GeminiAvailability       string                    `json:"gemini_availability"`
	TelegramAvailability     string                    `json:"telegram_availability,omitempty"`
	StorageWritable          string                    `json:"storage_writable"`
	LastScanTime             time.Time                 `json:"last_scan_time"`
	LastScanAgeSec           float64                   `json:"last_scan_age_seconds"`
	LastSuccessfulScan       time.Time                 `json:"last_successful_scan"`
	LastSuccessfulScanAgeSec float64                   `json:"last_successful_scan_age_seconds"`
	LastRecheckTime          time.Time                 `json:"last_recheck_time"`
	LastRecheckAgeSec        float64                   `json:"last_recheck_age_seconds"`
	LastEvaluationTime       time.Time                 `json:"last_evaluation_time"`
	LastEvaluationAgeSec     float64                   `json:"last_evaluation_age_seconds"`
	ScanWorkerRunning        bool                      `json:"scan_worker_running"`
	RecheckWorkerRunning     bool                      `json:"recheck_worker_running"`
	MonitoringWorkerRunning  bool                      `json:"monitoring_worker_running"`
	EvaluationWorkerRunning  bool                      `json:"evaluation_worker_running"`
	Metrics                  *SREMetrics               `json:"metrics"`
	RealtimePrice            RealtimePriceStatus       `json:"realtime_price"`
	RecheckCadence           RecheckCadenceStatus      `json:"recheck_cadence"`
	BootstrapProvenance      BootstrapProvenanceStatus `json:"bootstrap_provenance"`
	WatchJournal             WatchJournalStatusCounts  `json:"watch_journal"`
	RolloutReadiness         RolloutReadinessStatus    `json:"rollout_readiness"`
}

func evaluateRolloutReadiness(status HealthStatus) RolloutReadinessStatus {
	blockers := make([]string, 0)
	if status.BinanceConnectivity != "" && !strings.HasPrefix(status.BinanceConnectivity, "OK") {
		blockers = append(blockers, "binance_connectivity_unhealthy")
	}
	if status.GeminiAvailability != "" && !strings.HasPrefix(status.GeminiAvailability, "OK") {
		blockers = append(blockers, "gemini_unhealthy")
	}
	if status.StorageWritable != "" && !strings.HasPrefix(status.StorageWritable, "OK") {
		blockers = append(blockers, "storage_unwritable")
	}
	if status.RealtimePrice.Enabled && status.RealtimePrice.ActiveSymbols > 0 && !status.RealtimePrice.Connected {
		blockers = append(blockers, "realtime_price_disconnected")
	}
	if status.BootstrapProvenance.TickerSource == "unavailable" {
		blockers = append(blockers, "ticker_bootstrap_unavailable")
	}
	if status.BootstrapProvenance.FundingSource == "unavailable" {
		blockers = append(blockers, "funding_bootstrap_unavailable")
	}
	if status.Metrics != nil {
		if status.Metrics.ScanSuccessCount == 0 {
			blockers = append(blockers, "no_successful_scans_yet")
		}
		if status.RecheckCadence.Enabled && status.Metrics.RecheckSuccessCount == 0 {
			blockers = append(blockers, "no_successful_rechecks_yet")
		}
	}
	if status.LastSuccessfulScanAgeSec >= 0 {
		maxScanAgeSec := float64(maxEvalInt(status.RecheckCadence.PrimaryBoundaryMinutes, 1) * 2 * 60)
		if status.LastSuccessfulScanAgeSec > maxScanAgeSec {
			blockers = append(blockers, "last_successful_scan_stale")
		}
	}

	recommendedPhase := "HOLD"
	if len(blockers) == 0 {
		recommendedPhase = "PHASE_0_OBSERVE"
		if status.Metrics != nil && status.Metrics.FinalExecuteCount > 0 {
			recommendedPhase = "PHASE_1_EXPAND"
		}
	}

	rollbackCriteria := []string{
		"rollback if health status degrades or rollout blockers appear",
		"rollback if bootstrap provenance turns unavailable or websocket disconnect persists during active feed usage",
		"rollback if scan or recheck overlap skips start suppressing intended boundaries",
	}

	return RolloutReadinessStatus{
		Ready:            len(blockers) == 0,
		RecommendedPhase: recommendedPhase,
		Blockers:         blockers,
		RollbackCriteria: rollbackCriteria,
	}
}

type SREMetrics struct {
	ScanDurationMs                     uint64  `json:"scan_duration_ms"`
	RecheckDurationMs                  uint64  `json:"recheck_duration_ms"`
	MarketDataDurationMs               uint64  `json:"market_data_duration_ms"`
	CandidatePipelineMs                uint64  `json:"candidate_pipeline_duration_ms"`
	AIBatchDurationMs                  uint64  `json:"ai_batch_duration_ms"`
	FinalGateDurationMs                uint64  `json:"final_gate_duration_ms"`
	EstimatedRequestWeight             uint64  `json:"estimated_request_weight"`
	RequestWeightBudget                uint64  `json:"request_weight_budget"`
	PrefetchCandidateCount             uint64  `json:"prefetch_candidate_count"`
	EnrichedCandidateCount             uint64  `json:"enriched_candidate_count"`
	ScanSuccessCount                   uint64  `json:"scan_success_count"`
	ScanFailCount                      uint64  `json:"scan_fail_count"`
	RecheckSuccessCount                uint64  `json:"recheck_success_count"`
	RecheckFailCount                   uint64  `json:"recheck_fail_count"`
	RecheckPromotionCount              uint64  `json:"recheck_promotion_count"`
	TotalTickers                       uint64  `json:"total_tickers"`
	UniversePassCount                  uint64  `json:"universe_pass_count"`
	UniverseRejectCount                uint64  `json:"universe_reject_count"`
	MarketDataErrorCount               uint64  `json:"market_data_error_count"`
	BootstrapTickerCacheFallbackCount  uint64  `json:"bootstrap_ticker_cache_fallback_count"`
	BootstrapFundingCacheFallbackCount uint64  `json:"bootstrap_funding_cache_fallback_count"`
	BootstrapTickerCacheAgeSeconds     uint64  `json:"bootstrap_ticker_cache_age_seconds"`
	BootstrapFundingCacheAgeSeconds    uint64  `json:"bootstrap_funding_cache_age_seconds"`
	AICandidateCount                   uint64  `json:"ai_candidate_count"`
	AITimeoutCount                     uint64  `json:"ai_timeout_count"`
	AILatencyAverageMs                 float64 `json:"ai_latency_average_ms"`
	StalenessRate                      float64 `json:"staleness_rate"`
	FinalExecuteCount                  uint64  `json:"final_execute_count"`
	FinalWatchCount                    uint64  `json:"final_watch_count"`
	FinalRejectCount                   uint64  `json:"final_reject_count"`
	ConflictDowngradeCount             uint64  `json:"conflict_downgrade_count"`
	CooldownRejectCount                uint64  `json:"cooldown_reject_count"`
	ScanOverlapSkipCount               uint64  `json:"scan_overlap_skip_count"`
	RecheckOverlapSkipCount            uint64  `json:"recheck_overlap_skip_count"`
	TelegramSuccessCount               uint64  `json:"telegram_success_count"`
	TelegramFailCount                  uint64  `json:"telegram_fail_count"`
	MonitoringActiveCount              uint64  `json:"monitoring_active_count"`
	StorageWriteFailCount              uint64  `json:"storage_write_fail_count"`
	EvaluationRecCount                 uint64  `json:"evaluation_recommendation_count"`
	GateBugCount                       uint64  `json:"gate_bug_count"`
}

type ObservabilityUsecase struct {
	provider           MarketDataProvider
	aiService          AIAuditorService
	notifier           any
	storageDir         string
	signalJournalFile  string
	watchJournalFile   string
	healthSnapshotFile string
	realtimeStatusSrc  RealtimeStatusProvider
}

func resolveObservabilityStorageFiles() (string, string, string) {
	settings := getRuntimeSettings()
	signalJournalFile := strings.TrimSpace(settings.SignalJournalFile)
	if signalJournalFile == "" {
		signalJournalFile = config.DefaultSignalJournalFile
	}
	watchJournalFile := strings.TrimSpace(settings.WatchJournalFile)
	if watchJournalFile == "" {
		watchJournalFile = config.DefaultWatchJournalFile
	}
	healthSnapshotFile := strings.TrimSpace(settings.HealthSnapshotFile)
	if healthSnapshotFile == "" {
		healthSnapshotFile = config.DefaultHealthSnapshotFile
	}
	return signalJournalFile, watchJournalFile, healthSnapshotFile
}

func NewObservabilityUsecase(
	provider MarketDataProvider,
	aiService AIAuditorService,
	notifier any,
	storageDir string,
) *ObservabilityUsecase {
	signalJournalFile, watchJournalFile, healthSnapshotFile := resolveObservabilityStorageFiles()
	return &ObservabilityUsecase{
		provider:           provider,
		aiService:          aiService,
		notifier:           notifier,
		storageDir:         storageDir,
		signalJournalFile:  signalJournalFile,
		watchJournalFile:   watchJournalFile,
		healthSnapshotFile: healthSnapshotFile,
	}
}

func (uc *ObservabilityUsecase) SetRealtimeStatusProvider(provider RealtimeStatusProvider) {
	if uc == nil {
		return
	}
	uc.realtimeStatusSrc = provider
}

func (uc *ObservabilityUsecase) SetStorageFiles(signalJournalFile, watchJournalFile, healthSnapshotFile string) {
	if uc == nil {
		return
	}
	if trimmed := strings.TrimSpace(signalJournalFile); trimmed != "" {
		uc.signalJournalFile = trimmed
	}
	if trimmed := strings.TrimSpace(watchJournalFile); trimmed != "" {
		uc.watchJournalFile = trimmed
	}
	if trimmed := strings.TrimSpace(healthSnapshotFile); trimmed != "" {
		uc.healthSnapshotFile = trimmed
	}
}

// PerformHealthAudit executes end-to-end connectivity and SRE checks, returns health snapshot and saves it atomically.
func (uc *ObservabilityUsecase) PerformHealthAudit(ctx context.Context) (HealthStatus, error) {
	// 1. Check Binance Read-Only Connection
	binanceStatus := "OK"
	healthTimeout := time.Duration(getRuntimeSettings().HealthCheckTimeoutSeconds) * time.Second
	if healthTimeout <= 0 {
		healthTimeout = 2 * time.Second
	}
	binanceCtx, binanceCancel := context.WithTimeout(ctx, healthTimeout)
	defer binanceCancel()
	if _, err := uc.provider.FetchLatestPrice(binanceCtx, "BTCUSDT"); err != nil {
		binanceStatus = "ERROR: " + err.Error()
	}

	// 2. Check Gemini Availability
	geminiStatus := "OK"
	if pingable, ok := uc.aiService.(Pingable); ok {
		geminiCtx, geminiCancel := context.WithTimeout(ctx, healthTimeout)
		defer geminiCancel()
		if err := pingable.Ping(geminiCtx); err != nil {
			geminiStatus = "ERROR: " + err.Error()
		}
	} else {
		geminiStatus = "OK (MOCKED)"
	}

	// 3. Check Telegram optional availability
	telegramStatus := "NOT_CONFIGURED"
	if pingable, ok := uc.notifier.(TelegramPingable); ok {
		tgCtx, tgCancel := context.WithTimeout(ctx, healthTimeout)
		defer tgCancel()
		if err := pingable.Ping(tgCtx); err != nil {
			telegramStatus = "ERROR: " + err.Error()
		} else {
			telegramStatus = "OK"
		}
	}

	// 4. Check Storage Writable (Conditional check based on HEALTH_STORAGE_CHECK env)
	storageStatus := "OK (SKIPPED)"
	if getRuntimeSettings().HealthStorageCheck {
		storageStatus = "OK"
		testFile := filepath.Join(uc.storageDir, ".health_write_test")
		if err := os.WriteFile(testFile, []byte("write-test"), 0644); err != nil {
			storageStatus = "ERROR: " + err.Error()
		} else {
			_ = os.Remove(testFile)
		}
	}

	var watchCounts WatchJournalStatusCounts
	var activeCount uint64
	journalPath := filepath.Join(uc.storageDir, uc.signalJournalFile)
	if data, err := os.ReadFile(journalPath); err == nil && len(data) > 0 {
		var list []SignalJournal
		if err := json.Unmarshal(data, &list); err == nil {
			for _, item := range list {
				if item.Status == MONITORING || item.Status == TP1_HIT {
					activeCount++
				}
			}
		}
	}
	watchPath := filepath.Join(uc.storageDir, uc.watchJournalFile)
	if data, err := os.ReadFile(watchPath); err == nil && len(data) > 0 {
		var list []WatchJournal
		if err := json.Unmarshal(data, &list); err == nil {
			for _, item := range list {
				switch item.Status {
				case WATCH_MONITORING:
					watchCounts.Monitoring++
				case VIRTUAL_TP1_HIT:
					watchCounts.VirtualTP1++
				case WATCH_PROMOTED:
					watchCounts.Promoted++
				case WATCH_RECHECK_EXPIRED:
					watchCounts.RecheckExpired++
				case WATCH_RECHECK_INVALIDATED:
					watchCounts.RecheckInvalidated++
				}
			}
		}
	}
	recheckPolicy := resolveWatchRecheckPolicy()
	settings := getRuntimeSettings()
	recheckCadence := RecheckCadenceStatus{
		Enabled:                 RecheckWorkerRunning.Load(),
		PreventOverlap:          settings.ScanPreventOverlap,
		SharedPipelineGuard:     settings.ScanPreventOverlap,
		SkipPrimaryBoundary:     true,
		PrimaryBoundaryMinutes:  settings.ScanBoundaryMinutes,
		BoundaryMinutes:         recheckPolicy.BoundaryMinutes,
		CloseBufferSeconds:      settings.ScanCloseCandleBufferSeconds,
		MaxAgeMinutes:           recheckPolicy.MaxAgeMinutes,
		EffectiveHorizonMinutes: int(recheckPolicy.EffectiveHorizon / time.Minute),
		BatchLimit:              recheckPolicy.BatchLimit,
		AllowedPlaybooks:        append([]string(nil), settings.WatchRecheckAllowedPlaybooks...),
	}

	// 6. Calculate ages and metrics
	reg := GetGlobalMetrics()
	lastScan, lastSuccess, lastRecheck, lastEval := reg.GetTimestamps()
	now := time.Now()

	lastScanAge := -1.0
	if !lastScan.IsZero() {
		lastScanAge = now.Sub(lastScan).Seconds()
	}

	lastSuccessAge := -1.0
	if !lastSuccess.IsZero() {
		lastSuccessAge = now.Sub(lastSuccess).Seconds()
	}

	lastEvalAge := -1.0
	if !lastEval.IsZero() {
		lastEvalAge = now.Sub(lastEval).Seconds()
	}

	lastRecheckAge := -1.0
	if !lastRecheck.IsZero() {
		lastRecheckAge = now.Sub(lastRecheck).Seconds()
	}

	metrics := &SREMetrics{
		ScanDurationMs:                     atomic.LoadUint64(&reg.LastScanDurationMs),
		RecheckDurationMs:                  atomic.LoadUint64(&reg.LastRecheckDurationMs),
		MarketDataDurationMs:               atomic.LoadUint64(&reg.LastMarketDataMs),
		CandidatePipelineMs:                atomic.LoadUint64(&reg.LastCandidateMs),
		AIBatchDurationMs:                  atomic.LoadUint64(&reg.LastAIBatchMs),
		FinalGateDurationMs:                atomic.LoadUint64(&reg.LastFinalGateMs),
		EstimatedRequestWeight:             atomic.LoadUint64(&reg.LastRequestWeight),
		RequestWeightBudget:                atomic.LoadUint64(&reg.LastRequestBudget),
		PrefetchCandidateCount:             atomic.LoadUint64(&reg.LastPrefetchCount),
		EnrichedCandidateCount:             atomic.LoadUint64(&reg.LastEnrichedCount),
		ScanSuccessCount:                   atomic.LoadUint64(&reg.ScanSuccessCount),
		ScanFailCount:                      atomic.LoadUint64(&reg.ScanFailCount),
		RecheckSuccessCount:                atomic.LoadUint64(&reg.RecheckSuccessCount),
		RecheckFailCount:                   atomic.LoadUint64(&reg.RecheckFailCount),
		RecheckPromotionCount:              atomic.LoadUint64(&reg.RecheckPromotionCount),
		TotalTickers:                       atomic.LoadUint64(&reg.TotalTickers),
		UniversePassCount:                  atomic.LoadUint64(&reg.UniversePass),
		UniverseRejectCount:                atomic.LoadUint64(&reg.UniverseReject),
		MarketDataErrorCount:               atomic.LoadUint64(&reg.MarketDataError),
		BootstrapTickerCacheFallbackCount:  atomic.LoadUint64(&reg.BootstrapTickerCacheFallback),
		BootstrapFundingCacheFallbackCount: atomic.LoadUint64(&reg.BootstrapFundingCacheFallback),
		BootstrapTickerCacheAgeSeconds:     atomic.LoadUint64(&reg.LastBootstrapTickerAgeSec),
		BootstrapFundingCacheAgeSeconds:    atomic.LoadUint64(&reg.LastBootstrapFundingAgeSec),
		AICandidateCount:                   atomic.LoadUint64(&reg.AICandidateCount),
		AITimeoutCount:                     atomic.LoadUint64(&reg.AITimeoutCount),
		AILatencyAverageMs:                 reg.GetAverageAILatencyMs(),
		StalenessRate:                      reg.GetStalenessRate(),
		FinalExecuteCount:                  atomic.LoadUint64(&reg.FinalExecuteCount),
		FinalWatchCount:                    atomic.LoadUint64(&reg.FinalWatchCount),
		FinalRejectCount:                   atomic.LoadUint64(&reg.FinalRejectCount),
		ConflictDowngradeCount:             atomic.LoadUint64(&reg.ConflictDowngrade),
		CooldownRejectCount:                atomic.LoadUint64(&reg.CooldownReject),
		ScanOverlapSkipCount:               atomic.LoadUint64(&reg.ScanOverlapSkipCount),
		RecheckOverlapSkipCount:            atomic.LoadUint64(&reg.RecheckOverlapSkipCount),
		TelegramSuccessCount:               atomic.LoadUint64(&reg.TelegramSuccess),
		TelegramFailCount:                  atomic.LoadUint64(&reg.TelegramFail),
		MonitoringActiveCount:              activeCount,
		StorageWriteFailCount:              atomic.LoadUint64(&reg.StorageWriteFail),
		EvaluationRecCount:                 atomic.LoadUint64(&reg.EvalRecCount),
		GateBugCount:                       atomic.LoadUint64(&reg.GateBugCount),
	}
	tickerSource, fundingSource := reg.GetBootstrapSources()
	tickerSource = normalizeBootstrapSourceForHealth(tickerSource)
	fundingSource = normalizeBootstrapSourceForHealth(fundingSource)
	bootstrapProvenance := BootstrapProvenanceStatus{
		TickerSource:       tickerSource,
		TickerFresh:        tickerSource == "live",
		TickerCacheAgeSec:  metrics.BootstrapTickerCacheAgeSeconds,
		FundingSource:      fundingSource,
		FundingFresh:       fundingSource == "live",
		FundingCacheAgeSec: metrics.BootstrapFundingCacheAgeSeconds,
	}

	status := HealthStatus{
		Status:                   "UP",
		Mode:                     "alert-only",
		BinanceConnectivity:      binanceStatus,
		GeminiAvailability:       geminiStatus,
		TelegramAvailability:     telegramStatus,
		StorageWritable:          storageStatus,
		LastScanTime:             lastScan,
		LastScanAgeSec:           lastScanAge,
		LastSuccessfulScan:       lastSuccess,
		LastSuccessfulScanAgeSec: lastSuccessAge,
		LastRecheckTime:          lastRecheck,
		LastRecheckAgeSec:        lastRecheckAge,
		LastEvaluationTime:       lastEval,
		LastEvaluationAgeSec:     lastEvalAge,
		ScanWorkerRunning:        ScanWorkerRunning.Load(),
		RecheckWorkerRunning:     RecheckWorkerRunning.Load(),
		MonitoringWorkerRunning:  MonitoringWorkerRunning.Load(),
		EvaluationWorkerRunning:  EvaluationWorkerRunning.Load(),
		Metrics:                  metrics,
		RecheckCadence:           recheckCadence,
		BootstrapProvenance:      bootstrapProvenance,
		WatchJournal:             watchCounts,
	}
	if uc.realtimeStatusSrc != nil {
		status.RealtimePrice = uc.realtimeStatusSrc.RealtimeStatus()
	}
	status.RolloutReadiness = evaluateRolloutReadiness(status)

	// 7. Save Snapshot to File Atomically
	healthPath := filepath.Join(uc.storageDir, uc.healthSnapshotFile)
	tmpPath := healthPath + ".tmp"
	if bytes, err := json.MarshalIndent(status, "", "  "); err == nil {
		if err := os.WriteFile(tmpPath, bytes, 0644); err == nil {
			if err := os.Rename(tmpPath, healthPath); err != nil {
				_ = os.Remove(tmpPath)
			}
		}
	}

	return status, nil
}
