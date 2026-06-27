package usecase

import (
	"sync"
	"sync/atomic"
	"time"
)

// MetricsRegistry encapsulates thread-safe atomic counters and SRE telemetry tracking.
type MetricsRegistry struct {
	mu sync.RWMutex

	// Counters and gauges
	ScanSuccessCount              uint64
	ScanFailCount                 uint64
	RecheckSuccessCount           uint64
	RecheckFailCount              uint64
	RecheckPromotionCount         uint64
	LastScanDurationMs            uint64
	LastRecheckDurationMs         uint64
	LastMarketDataMs              uint64
	LastCandidateMs               uint64
	LastAIBatchMs                 uint64
	LastFinalGateMs               uint64
	LastRequestWeight             uint64
	LastRequestBudget             uint64
	LastPrefetchCount             uint64
	LastEnrichedCount             uint64
	TotalTickers                  uint64
	UniversePass                  uint64
	UniverseReject                uint64
	MarketDataError               uint64
	BootstrapTickerCacheFallback  uint64
	BootstrapFundingCacheFallback uint64
	LastBootstrapTickerAgeSec     uint64
	LastBootstrapFundingAgeSec    uint64
	AICandidateCount              uint64
	AITimeoutCount                uint64
	TotalAILatencyMs              uint64
	TotalAICalls                  uint64
	StalenessCount                uint64
	StalenessChecked              uint64
	FinalExecuteCount             uint64
	FinalWatchCount               uint64
	FinalRejectCount              uint64
	ConflictDowngrade             uint64
	CooldownReject                uint64
	ScanOverlapSkipCount          uint64
	RecheckOverlapSkipCount       uint64
	TelegramSuccess               uint64
	TelegramFail                  uint64
	StorageWriteFail              uint64
	EvalRecCount                  uint64
	GateBugCount                  uint64

	// Rule-level metrics (protected by mu)
	ruleRejectCount map[string]uint64
	ruleWatchCount  map[string]uint64

	// Timestamp trackers
	LastScanTime               time.Time
	LastSuccessScan            time.Time
	LastRecheckTime            time.Time
	LastEvaluationTime         time.Time
	LastBootstrapTickerSource  string
	LastBootstrapFundingSource string
}

var (
	globalMetrics   *MetricsRegistry
	globalMetricsMu sync.Mutex
)

// GetGlobalMetrics returns the global metrics tracking singleton.
func GetGlobalMetrics() *MetricsRegistry {
	globalMetricsMu.Lock()
	defer globalMetricsMu.Unlock()
	if globalMetrics == nil {
		globalMetrics = &MetricsRegistry{}
	}
	return globalMetrics
}

// SetGlobalMetrics overwrites the global metrics tracker.
func SetGlobalMetrics(m *MetricsRegistry) {
	globalMetricsMu.Lock()
	defer globalMetricsMu.Unlock()
	globalMetrics = m
}

func (m *MetricsRegistry) IncrementScanSuccess() {
	atomic.AddUint64(&m.ScanSuccessCount, 1)
}

func (m *MetricsRegistry) IncrementScanFail() {
	atomic.AddUint64(&m.ScanFailCount, 1)
}

func (m *MetricsRegistry) IncrementRecheckSuccess() {
	atomic.AddUint64(&m.RecheckSuccessCount, 1)
}

func (m *MetricsRegistry) IncrementRecheckFail() {
	atomic.AddUint64(&m.RecheckFailCount, 1)
}

func (m *MetricsRegistry) AddRecheckPromotionCount(val uint64) {
	atomic.AddUint64(&m.RecheckPromotionCount, val)
}

func (m *MetricsRegistry) SetLastScanDuration(d time.Duration) {
	atomic.StoreUint64(&m.LastScanDurationMs, uint64(d.Milliseconds()))
}

func (m *MetricsRegistry) SetLastRecheckDuration(d time.Duration) {
	atomic.StoreUint64(&m.LastRecheckDurationMs, uint64(d.Milliseconds()))
}

func (m *MetricsRegistry) SetLastMarketDataDuration(d time.Duration) {
	atomic.StoreUint64(&m.LastMarketDataMs, uint64(d.Milliseconds()))
}

func (m *MetricsRegistry) SetLastCandidatePipelineDuration(d time.Duration) {
	atomic.StoreUint64(&m.LastCandidateMs, uint64(d.Milliseconds()))
}

func (m *MetricsRegistry) SetLastAIBatchDuration(d time.Duration) {
	atomic.StoreUint64(&m.LastAIBatchMs, uint64(d.Milliseconds()))
}

func (m *MetricsRegistry) SetLastFinalGateDuration(d time.Duration) {
	atomic.StoreUint64(&m.LastFinalGateMs, uint64(d.Milliseconds()))
}

func (m *MetricsRegistry) SetLastScanRequestWeight(weight uint64) {
	atomic.StoreUint64(&m.LastRequestWeight, weight)
}

func (m *MetricsRegistry) SetLastScanRequestWeightBudget(weight uint64) {
	atomic.StoreUint64(&m.LastRequestBudget, weight)
}

func (m *MetricsRegistry) SetLastPrefetchCandidateCount(count uint64) {
	atomic.StoreUint64(&m.LastPrefetchCount, count)
}

func (m *MetricsRegistry) SetLastEnrichedCandidateCount(count uint64) {
	atomic.StoreUint64(&m.LastEnrichedCount, count)
}

func (m *MetricsRegistry) AddTotalTickers(val uint64) {
	atomic.AddUint64(&m.TotalTickers, val)
}

func (m *MetricsRegistry) AddUniversePass(val uint64) {
	atomic.AddUint64(&m.UniversePass, val)
}

func (m *MetricsRegistry) AddUniverseReject(val uint64) {
	atomic.AddUint64(&m.UniverseReject, val)
}

func (m *MetricsRegistry) IncrementMarketDataError() {
	atomic.AddUint64(&m.MarketDataError, 1)
}

func (m *MetricsRegistry) RecordBootstrapTickerCacheFallback(age time.Duration) {
	atomic.AddUint64(&m.BootstrapTickerCacheFallback, 1)
	if age < 0 {
		age = 0
	}
	atomic.StoreUint64(&m.LastBootstrapTickerAgeSec, uint64(age.Seconds()))
	m.SetBootstrapTickerSource("cache")
}

func (m *MetricsRegistry) RecordBootstrapFundingCacheFallback(age time.Duration) {
	atomic.AddUint64(&m.BootstrapFundingCacheFallback, 1)
	if age < 0 {
		age = 0
	}
	atomic.StoreUint64(&m.LastBootstrapFundingAgeSec, uint64(age.Seconds()))
	m.SetBootstrapFundingSource("cache")
}

func (m *MetricsRegistry) ClearBootstrapTickerCacheAge() {
	atomic.StoreUint64(&m.LastBootstrapTickerAgeSec, 0)
}

func (m *MetricsRegistry) ClearBootstrapFundingCacheAge() {
	atomic.StoreUint64(&m.LastBootstrapFundingAgeSec, 0)
}

func (m *MetricsRegistry) SetBootstrapTickerSource(source string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LastBootstrapTickerSource = source
}

func (m *MetricsRegistry) SetBootstrapFundingSource(source string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LastBootstrapFundingSource = source
}

func (m *MetricsRegistry) AddAICandidateCount(val uint64) {
	atomic.AddUint64(&m.AICandidateCount, val)
}

func (m *MetricsRegistry) IncrementAITimeoutCount() {
	atomic.AddUint64(&m.AITimeoutCount, 1)
}

func (m *MetricsRegistry) AddAILatency(d time.Duration) {
	atomic.AddUint64(&m.TotalAILatencyMs, uint64(d.Milliseconds()))
	atomic.AddUint64(&m.TotalAICalls, 1)
}

func (m *MetricsRegistry) AddStalenessCount(val uint64) {
	atomic.AddUint64(&m.StalenessCount, val)
}

func (m *MetricsRegistry) AddStalenessChecked(val uint64) {
	atomic.AddUint64(&m.StalenessChecked, val)
}

func (m *MetricsRegistry) AddFinalExecuteCount(val uint64) {
	atomic.AddUint64(&m.FinalExecuteCount, val)
}

func (m *MetricsRegistry) AddFinalWatchCount(val uint64) {
	atomic.AddUint64(&m.FinalWatchCount, val)
}

func (m *MetricsRegistry) AddFinalRejectCount(val uint64) {
	atomic.AddUint64(&m.FinalRejectCount, val)
}

func (m *MetricsRegistry) AddConflictDowngrade(val uint64) {
	atomic.AddUint64(&m.ConflictDowngrade, val)
}

func (m *MetricsRegistry) AddCooldownReject(val uint64) {
	atomic.AddUint64(&m.CooldownReject, val)
}

func (m *MetricsRegistry) IncrementScanOverlapSkip() {
	atomic.AddUint64(&m.ScanOverlapSkipCount, 1)
}

func (m *MetricsRegistry) IncrementRecheckOverlapSkip() {
	atomic.AddUint64(&m.RecheckOverlapSkipCount, 1)
}

func (m *MetricsRegistry) IncrementTelegramSuccess() {
	atomic.AddUint64(&m.TelegramSuccess, 1)
}

func (m *MetricsRegistry) IncrementTelegramFail() {
	atomic.AddUint64(&m.TelegramFail, 1)
}

func (m *MetricsRegistry) IncrementStorageWriteFail() {
	atomic.AddUint64(&m.StorageWriteFail, 1)
}

func (m *MetricsRegistry) SetEvalMetrics(recCount, gateBugCount uint64) {
	atomic.StoreUint64(&m.EvalRecCount, recCount)
	atomic.StoreUint64(&m.GateBugCount, gateBugCount)
}

func (m *MetricsRegistry) SetLastScanTime(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LastScanTime = t
}

func (m *MetricsRegistry) SetLastSuccessScan(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LastSuccessScan = t
}

func (m *MetricsRegistry) SetLastEvaluationTime(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LastEvaluationTime = t
}

func (m *MetricsRegistry) SetLastRecheckTime(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LastRecheckTime = t
}

func (m *MetricsRegistry) GetTimestamps() (lastScan, lastSuccess, lastRecheck, lastEval time.Time) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.LastScanTime, m.LastSuccessScan, m.LastRecheckTime, m.LastEvaluationTime
}

func (m *MetricsRegistry) GetBootstrapSources() (tickerSource, fundingSource string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.LastBootstrapTickerSource, m.LastBootstrapFundingSource
}

func (m *MetricsRegistry) GetAverageAILatencyMs() float64 {
	calls := atomic.LoadUint64(&m.TotalAICalls)
	if calls == 0 {
		return 0
	}
	latency := atomic.LoadUint64(&m.TotalAILatencyMs)
	return float64(latency) / float64(calls)
}

func (m *MetricsRegistry) GetStalenessRate() float64 {
	checked := atomic.LoadUint64(&m.StalenessChecked)
	if checked == 0 {
		return 0
	}
	stale := atomic.LoadUint64(&m.StalenessCount)
	return float64(stale) / float64(checked)
}

func (m *MetricsRegistry) initRuleMaps() {
	if m.ruleRejectCount == nil {
		m.ruleRejectCount = make(map[string]uint64)
	}
	if m.ruleWatchCount == nil {
		m.ruleWatchCount = make(map[string]uint64)
	}
}

func (m *MetricsRegistry) IncrementRuleReject(layer string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.initRuleMaps()
	m.ruleRejectCount[layer]++
}

func (m *MetricsRegistry) IncrementRuleWatch(layer string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.initRuleMaps()
	m.ruleWatchCount[layer]++
}

func (m *MetricsRegistry) GetRuleRejectCount(layer string) uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.ruleRejectCount == nil {
		return 0
	}
	return m.ruleRejectCount[layer]
}

func (m *MetricsRegistry) GetRuleWatchCount(layer string) uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.ruleWatchCount == nil {
		return 0
	}
	return m.ruleWatchCount[layer]
}
