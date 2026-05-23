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
	ScanSuccessCount   uint64
	ScanFailCount      uint64
	LastScanDurationMs uint64
	TotalTickers       uint64
	UniversePass       uint64
	UniverseReject     uint64
	MarketDataError    uint64
	AICandidateCount   uint64
	AITimeoutCount     uint64
	TotalAILatencyMs   uint64
	TotalAICalls       uint64
	StalenessCount     uint64
	StalenessChecked   uint64
	FinalExecuteCount  uint64
	FinalWatchCount    uint64
	FinalRejectCount   uint64
	ConflictDowngrade  uint64
	CooldownReject     uint64
	TelegramSuccess    uint64
	TelegramFail       uint64
	StorageWriteFail   uint64
	EvalRecCount       uint64
	GateBugCount       uint64

	// Timestamp trackers
	LastScanTime       time.Time
	LastSuccessScan    time.Time
	LastEvaluationTime time.Time
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

func (m *MetricsRegistry) SetLastScanDuration(d time.Duration) {
	atomic.StoreUint64(&m.LastScanDurationMs, uint64(d.Milliseconds()))
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

func (m *MetricsRegistry) GetTimestamps() (lastScan, lastSuccess, lastEval time.Time) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.LastScanTime, m.LastSuccessScan, m.LastEvaluationTime
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
