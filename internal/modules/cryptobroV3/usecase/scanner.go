package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"cpbro-engine/internal/modules/cryptobroV3/entity"
)

type ScannerUsecase struct {
	marketDataUsecase          *MarketDataUsecase
	marketPolicyUsecase        *MarketPolicyUsecase
	universeUsecase            *UniverseUsecase
	strategySelectorUsecase    *StrategySelectorUsecase
	playbookEligibilityUsecase *PlaybookEligibilityUsecase
	playbookQuantEngineUsecase *PlaybookQuantEngineUsecase
	scoringUsecase             *ScoringUsecase
	candidateArbiterUsecase    *CandidateArbiterUsecase
	localGateUsecase           *LocalGateUsecase
	aiCandidateSelectorUsecase *AICandidateSelectorUsecase
	aiAuditorUsecase           *AIAuditorUsecase
	planReconciliationUsecase  *PlanReconciliationUsecase
	stalenessUsecase           *StalenessUsecase
	finalGateUsecase           *FinalGateUsecase
	conflictResolverUsecase    *ConflictResolverUsecase
	signalNotificationUsecase  *SignalNotificationUsecase
	opsNotificationUsecase     *OpsNotificationUsecase
	monitoringUsecase          *MonitoringUsecase
	feedbackUsecase            *FeedbackUsecase
	storageUsecase             *StorageUsecase
	hotSymbolProvider          HotSymbolProvider
}

func (uc *ScannerUsecase) SetHotSymbolProvider(provider HotSymbolProvider) {
	uc.hotSymbolProvider = provider
}

type scanRequestGuardProfile struct {
	Budget                int
	ExpectedWeight        int
	PrefetchLimit         int
	MarketDataConcurrency int
	PipelineConcurrency   int
	Applied               bool
}

func NewScannerUsecase(
	marketData *MarketDataUsecase,
	marketPolicy *MarketPolicyUsecase,
	universe *UniverseUsecase,
	strategySelector *StrategySelectorUsecase,
	playbookEligibility *PlaybookEligibilityUsecase,
	playbookQuantEngine *PlaybookQuantEngineUsecase,
	scoring *ScoringUsecase,
	candidateArbiter *CandidateArbiterUsecase,
	localGate *LocalGateUsecase,
	aiCandidateSelector *AICandidateSelectorUsecase,
	aiAuditor *AIAuditorUsecase,
	planReconciliation *PlanReconciliationUsecase,
	staleness *StalenessUsecase,
	finalGate *FinalGateUsecase,
	conflictResolver *ConflictResolverUsecase,
	signalNotification *SignalNotificationUsecase,
	opsNotification *OpsNotificationUsecase,
	monitoring *MonitoringUsecase,
	feedback *FeedbackUsecase,
	storage *StorageUsecase,
) *ScannerUsecase {
	return &ScannerUsecase{
		marketDataUsecase:          marketData,
		marketPolicyUsecase:        marketPolicy,
		universeUsecase:            universe,
		strategySelectorUsecase:    strategySelector,
		playbookEligibilityUsecase: playbookEligibility,
		playbookQuantEngineUsecase: playbookQuantEngine,
		scoringUsecase:             scoring,
		candidateArbiterUsecase:    candidateArbiter,
		localGateUsecase:           localGate,
		aiCandidateSelectorUsecase: aiCandidateSelector,
		aiAuditorUsecase:           aiAuditor,
		planReconciliationUsecase:  planReconciliation,
		stalenessUsecase:           staleness,
		finalGateUsecase:           finalGate,
		conflictResolverUsecase:    conflictResolver,
		signalNotificationUsecase:  signalNotification,
		opsNotificationUsecase:     opsNotification,
		monitoringUsecase:          monitoring,
		feedbackUsecase:            feedback,
		storageUsecase:             storage,
	}
}

func (uc *ScannerUsecase) Run(ctx context.Context, req dto.ScanRequest) (dto.ScanResult, error) {
	scanStart := time.Now()
	scanBoundary := req.TriggerTime
	if scanBoundary.IsZero() {
		scanBoundary = scanStart
	}
	scanID := scanBoundary.Format("20060102150405")

	slog.Info("Starting AnalyzeMarketV3 Scan", "scan_id", scanID)
	maxHoldDuration := getMonitoringMaxHoldDuration()

	finalSignals := []dto.SignalResponse{}

	// Load active signals (signal journal) and history signals for final gate evaluation
	activeSignals, _ := uc.storageUsecase.LoadSignalJournal()
	previousLatest, _ := uc.storageUsecase.LoadLatestResult()
	var historySignals []dto.SignalResponse
	if hist, err := uc.storageUsecase.LoadSignalHistory(); err == nil && hist != nil {
		historySignals = hist.Signals
	}

	// Fetch tickers & funding rates to feed the macro Policy Engine
	slog.Info("Fetching futures tickers and premium funding rates from market data provider...", "scan_id", scanID)
	var (
		tickers      []dto.Ticker24h
		fundingRates map[string]float64
		hotSymbols   []HotSymbol
		tickersErr   error
		fundingErr   error
		hotErr       error
		tickersMeta  bootstrapFetchMeta
		fundingMeta  bootstrapFetchMeta
		macroWG      sync.WaitGroup
	)
	macroWG.Add(2)
	if uc.hotSymbolProvider != nil {
		macroWG.Add(1)
		go func() {
			defer macroWG.Done()
			hotSymbols, hotErr = uc.hotSymbolProvider.FetchHotSymbols(context.Background())
		}()
	}
	go func() {
		defer macroWG.Done()
		// Use context.Background() (detached from scanCtx) so that BINANCE_MAX_RETRY
		// retries can execute even when the scanCtx deadline has just been reached.
		// The bootstrapTimeout inside MarketDataUsecase still provides per-attempt timeout.
		tickers, tickersMeta, tickersErr = uc.marketDataUsecase.FetchAllFuturesTickers24hWithMeta(context.Background())
	}()
	go func() {
		defer macroWG.Done()
		fundingRates, fundingMeta, fundingErr = uc.marketDataUsecase.FetchPremiumFundingRatesWithMeta(context.Background())
	}()
	macroWG.Wait()

	hotSymbolMap := make(map[string]HotSymbol)
	if hotErr == nil && len(hotSymbols) > 0 {
		for _, hs := range hotSymbols {
			symbolUpper := strings.ToUpper(strings.TrimSpace(hs.Symbol))
			if symbolUpper == "" {
				continue
			}
			baseSym := NormalizeBaseSymbol(symbolUpper)
			if baseSym == "" {
				continue
			}
			if existing, ok := hotSymbolMap[baseSym]; ok {
				if !strings.Contains(existing.Source, hs.Source) {
					existing.Source = existing.Source + ", " + hs.Source
				}
				if hs.Score > existing.Score {
					existing.Score = hs.Score
				}
				hotSymbolMap[baseSym] = existing
			} else {
				hotSymbolMap[baseSym] = hs
			}
		}
		slog.Info("Hot symbols bootstrapped for universe re-ranking", "scan_id", scanID, "hot_count", len(hotSymbolMap))
	} else if hotErr != nil {
		slog.Warn("Failed to bootstrap hot symbols, proceeding without overlay", "scan_id", scanID, "error", hotErr)
	}

	if tickersErr == nil {
		attrs := []any{"scan_id", scanID, "source", string(tickersMeta.Source), "count", len(tickers)}
		if tickersMeta.Source == bootstrapSourceCache {
			attrs = append(attrs, "cache_age_seconds", roundDurationSeconds(tickersMeta.CacheAge))
		}
		slog.Info("Futures ticker bootstrap ready", attrs...)
	}
	if fundingErr == nil {
		attrs := []any{"scan_id", scanID, "source", string(fundingMeta.Source), "count", len(fundingRates)}
		if fundingMeta.Source == bootstrapSourceCache {
			attrs = append(attrs, "cache_age_seconds", roundDurationSeconds(fundingMeta.CacheAge))
		}
		slog.Info("Premium funding bootstrap ready", attrs...)
	}

	if tickersErr != nil {
		slog.Error("Failed to fetch futures tickers", "scan_id", scanID, "source", string(bootstrapSourceNone), "error", tickersErr)
		GetGlobalMetrics().IncrementScanFail()
		GetGlobalMetrics().SetLastScanTime(scanStart)
		GetGlobalMetrics().IncrementMarketDataError()
		return dto.ScanResult{}, fmt.Errorf("binance ticker total fail: %w", tickersErr)
	}
	if fundingErr != nil {
		slog.Warn("Failed to fetch funding rates; using fallback values", "scan_id", scanID, "source", string(bootstrapSourceNone), "error", fundingErr)
		GetGlobalMetrics().IncrementMarketDataError()
		fundingRates = make(map[string]float64)
	}

	// Map tickers for quick access
	tickerMap := make(map[string]dto.Ticker24h)
	totalTickers := 0
	var ethTicker *dto.Ticker24h

	for i := range tickers {
		t := tickers[i]
		tickerMap[t.Symbol] = t
		totalTickers++
		if t.Symbol == "ETHUSDT" {
			ethTicker = &tickers[i]
		}
	}

	macroState := deriveMacroMarketState(tickers)
	breadth := macroState.Breadth

	ethBtcPerf := 0.0
	if btcTicker, ok := tickerMap["BTCUSDT"]; ok && ethTicker != nil {
		ethBtcPerf = (ethTicker.PriceChangePercent - btcTicker.PriceChangePercent) / 100.0
	}

	// Evaluate global Policy
	policy := uc.marketPolicyUsecase.EvaluatePolicy(ctx, macroState.BTCTrend, macroState.BTCScore, ethBtcPerf, macroState.BTCChaos, macroState.Volatility, breadth)
	compressionMacroActive := isCompressionMacroActive(macroState)
	compressionFallbackActive := shouldFallbackCompressionToLowVol(previousLatest, compressionMacroActive)
	if compressionFallbackActive {
		policy = normalizeLowVolPolicy(policy, macroState.BTCTrend, "LOW_VOL fallback active - prior compression scans produced zero eligible setups")
	}
	previousCompressionZeroStreak := 0
	if previousLatest != nil {
		previousCompressionZeroStreak = previousLatest.CompressionZeroEligibleStreak
	}
	slog.Info(
		"Market policy evaluated",
		"scan_id", scanID,
		"regime", policy.Regime,
		"long_mode", policy.LongMode,
		"short_mode", policy.ShortMode,
		"macro_volatility", macroState.Volatility,
		"breadth", RoundToDecimalPlaces(macroState.Breadth, 4),
		"median_abs_move_24h", RoundToDecimalPlaces(macroState.MedianAbsMove24h, 4),
		"active_move_share", RoundToDecimalPlaces(macroState.ActiveMoveShare, 4),
		"compression_macro_active", compressionMacroActive,
		"compression_zero_streak_prev", previousCompressionZeroStreak,
		"compression_low_vol_fallback", compressionFallbackActive,
	)

	// Filter dynamic universe candidates
	slog.Info("Filtering dynamic universe candidates...", "scan_id", scanID, "total_tickers", len(tickers))
	candidates, rejectedCandidatesList := uc.universeUsecase.FilterUniverse(tickers, fundingRates, policy, hotSymbolMap)
	universePassCount := len(candidates)
	slog.Info("Dynamic universe candidates filtered", "scan_id", scanID, "passed", universePassCount, "rejected", len(rejectedCandidatesList))

	candidateMap := make(map[string]UniverseCandidate)
	for _, c := range candidates {
		candidateMap[c.Symbol] = c
	}

	rejectedSummary := []string{}
	funnelSummary := newFunnelSummaryAccumulator()
	playbookBlockers := newPlaybookBlockerAccumulator()
	for _, rej := range rejectedCandidatesList {
		rejectedSummary = append(rejectedSummary, fmt.Sprintf("%s: %s", rej.Symbol, rej.Reason))
		funnelSummary.Add(funnelStageUniverseReject, rej.Reason)
	}

	metrics := GetGlobalMetrics()

	// Concurrency limited candle fetching
	concurrencyLimit := getRuntimeSettings().MaxMarketDataConcurrency
	if concurrencyLimit <= 0 {
		concurrencyLimit = 5
	}

	type candlesCache struct {
		data MarketData
		err  error
	}
	prefetchCandidates := candidates
	prefetchDeferredSummary := []string{}
	prefetchDebug := prefetchSelectionDebug{}
	prefetchLimit := resolveMarketDataPrefetchLimit(policy, len(candidates))
	requestGuard := resolveAdaptiveScanRequestGuard(policy, len(candidates), prefetchLimit, concurrencyLimit)
	prefetchLimit = requestGuard.PrefetchLimit
	concurrencyLimit = requestGuard.MarketDataConcurrency
	if prefetchLimit > 0 && prefetchLimit < len(candidates) {
		selected, deferred, selectionDebug := selectPrefetchCandidates(candidates, prefetchLimit, policy)
		prefetchDebug = selectionDebug
		var debugCand, debugHot, debugRotation, debugSel []string
		for _, c := range candidates {
			debugCand = append(debugCand, fmt.Sprintf("%s(IsHot:%v,Act:%0.2f,Liq:%0.2f,Comp:%0.2f)", c.Symbol, c.IsHot, c.ActivityScore, c.LiquidityScore, c.CompositeScore))
		}
		for _, c := range candidates {
			if c.IsHot {
				debugHot = append(debugHot, c.Symbol)
			}
			if !c.IsHot && c.ActivityScore >= resolveRotationActivityThreshold(policy) {
				debugRotation = append(debugRotation, c.Symbol)
			}
		}
		for _, c := range selected {
			debugSel = append(debugSel, c.Symbol)
		}
		slog.Info("PREFETCH DEBUG", "limit", prefetchLimit, "hot_slots", selectionDebug.HotSlots, "rotation_slots", selectionDebug.RotationSlots, "candidates", debugCand, "hot_pool", debugHot, "rotation_pool", debugRotation, "selected", debugSel)

		for _, s := range selected {
			if s.HotOverlaySelected {
				info := candidateMap[s.Symbol]
				info.HotOverlaySelected = true
				candidateMap[s.Symbol] = info
			}
		}

		// Handle deferred candidates
		for _, deferredCandidate := range deferred {
			prefetchDeferredSummary = append(prefetchDeferredSummary, fmt.Sprintf("%s: deferred by market data prefetch limit", deferredCandidate.Symbol))
			funnelSummary.Add(funnelStagePipelineDrop, "deferred by market data prefetch limit")
		}
		prefetchCandidates = selected
	} else {
		for i := range prefetchCandidates {
			if prefetchCandidates[i].IsHot {
				prefetchCandidates[i].HotOverlaySelected = true
				info := candidateMap[prefetchCandidates[i].Symbol]
				info.HotOverlaySelected = true
				candidateMap[prefetchCandidates[i].Symbol] = info
			}
		}
	}

	slog.Info("Prefetching market data for passed candidates...", "scan_id", scanID, "candidates_count", len(prefetchCandidates), "concurrency_limit", concurrencyLimit)
	candlesMap := make(map[string]candlesCache)
	var mapMu sync.Mutex

	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrencyLimit)

	for _, cand := range prefetchCandidates {
		wg.Add(1)
		go func(pair string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			md, fetchErr := uc.marketDataUsecase.FetchInitialMarketData(ctx, pair, fundingRates)
			if fetchErr != nil {
				GetGlobalMetrics().IncrementMarketDataError()
			}
			mapMu.Lock()
			candlesMap[pair] = candlesCache{
				data: md,
				err:  fetchErr,
			}
			mapMu.Unlock()
		}(cand.Symbol)
	}
	wg.Wait()
	marketDataDuration := time.Since(scanStart)
	metrics.SetLastMarketDataDuration(marketDataDuration)
	slog.Info("Prefetching market data completed", "scan_id", scanID, "duration", marketDataDuration.String())

	var allCandidates []QuantResult
	policyRejectedSummary := append([]string{}, prefetchDeferredSummary...)
	totalStrategySelected := 0
	totalPlaybookEligible := 0
	var enrichedSymbolCount uint64

	type eligibilityFailure struct {
		Symbol       string
		StrategyName string
		Direction    string
		Reason       string
	}
	type quantFailure struct {
		Symbol       string
		StrategyName string
		Direction    string
		Reason       string
	}
	type candidatePipelineResult struct {
		policyRejectReason  string
		strategySelected    int
		playbookEligible    int
		eligibilityFailures []eligibilityFailure
		quantFailures       []quantFailure
		quantResults        []QuantResult
	}
	var eligibilityFailures []eligibilityFailure
	var quantFailures []quantFailure

	tickerLastPrice := make(map[string]float64, len(tickers))
	for _, t := range tickers {
		tickerLastPrice[t.Symbol] = t.LastPrice
	}

	pipelineConcurrency := minInt(len(prefetchCandidates), maxInt(2, minInt(runtime.NumCPU(), 8)))
	if limit := getRuntimeSettings().MaxCandidatePipelineConcurrency; limit > 0 {
		pipelineConcurrency = minInt(len(prefetchCandidates), limit)
	}
	if pipelineConcurrency <= 0 {
		pipelineConcurrency = 1
	}
	if requestGuard.PipelineConcurrency > 0 && pipelineConcurrency > requestGuard.PipelineConcurrency {
		pipelineConcurrency = requestGuard.PipelineConcurrency
	}

	slog.Info("Running candidate quant pipeline...", "scan_id", scanID, "pipeline_concurrency", pipelineConcurrency)
	pipelineResults := make([]candidatePipelineResult, len(prefetchCandidates))
	var pipelineWG sync.WaitGroup
	pipelineSem := make(chan struct{}, pipelineConcurrency)
	pipelineStart := time.Now()

	for idx, candidate := range prefetchCandidates {
		pipelineWG.Add(1)
		go func(index int, candidate UniverseCandidate) {
			defer pipelineWG.Done()
			pipelineSem <- struct{}{}
			defer func() { <-pipelineSem }()

			pair := candidate.Symbol
			cache, exists := candlesMap[pair]
			if !exists || cache.err != nil {
				pipelineResults[index].policyRejectReason = fmt.Sprintf("%s: failed to fetch market data", pair)
				return
			}
			if len(cache.data.M15Candles) == 0 {
				pipelineResults[index].policyRejectReason = fmt.Sprintf("%s: m15 candles empty", pair)
				return
			}

			metrics.AddStalenessChecked(1)
			if !uc.stalenessUsecase.IsFresh(cache.data.M15Candles) {
				metrics.AddStalenessCount(1)
				pipelineResults[index].policyRejectReason = fmt.Sprintf("%s: raw candles are stale", pair)
				return
			}

			fullData, enrichErr := uc.marketDataUsecase.EnrichMarketData(ctx, cache.data)
			if enrichErr != nil {
				GetGlobalMetrics().IncrementMarketDataError()
				pipelineResults[index].policyRejectReason = fmt.Sprintf("%s: failed to enrich market data", pair)
				return
			}
			atomic.AddUint64(&enrichedSymbolCount, 1)

			latestPrice := tickerLastPrice[pair]
			if latestPrice == 0 && len(fullData.M15Candles) > 0 {
				latestPrice = fullData.M15Candles[len(fullData.M15Candles)-1].Close
			}

			fr := fullData.FundingRate
			priceChange24h := 0.0
			if t, ok := tickerMap[pair]; ok {
				priceChange24h = t.PriceChangePercent
			}
			prelimData := fullData
			prelimData.LatestPrice = latestPrice
			prelimData.PriceChange24h = priceChange24h
			prelimData.FundingRate = fr
			prepared, ok := uc.playbookQuantEngineUsecase.prepareContext(prelimData)
			if !ok {
				pipelineResults[index].policyRejectReason = fmt.Sprintf("%s: insufficient closed m15 candles for quant context", pair)
				return
			}
			tech := &prepared.technicalSnapshot
			structure := &prepared.structureSnapshot

			selections := uc.strategySelectorUsecase.SelectPlaybooks(policy, candidate, prelimData, tech, structure)
			result := &pipelineResults[index]
			result.strategySelected = len(selections)

			for _, sel := range selections {
				eligibilityRes := uc.playbookEligibilityUsecase.CheckEligibility(sel, policy, prelimData, tech, structure)
				if !eligibilityRes.Eligible {
					result.eligibilityFailures = append(result.eligibilityFailures, eligibilityFailure{
						Symbol:       pair,
						StrategyName: sel.StrategyName,
						Direction:    string(sel.Direction),
						Reason:       eligibilityRes.Reason,
					})
					continue
				}
				result.playbookEligible++

				playbook := TREND_PULLBACK
				switch sel.StrategyName {
				case string(COMPRESSION_BREAKOUT_RETEST):
					playbook = COMPRESSION_BREAKOUT_RETEST
				case string(LIQUIDITY_SWEEP_REVERSAL):
					playbook = LIQUIDITY_SWEEP_REVERSAL
				case string(RANGE_EDGE_REVERSAL):
					playbook = RANGE_EDGE_REVERSAL
				case string(CROWDED_POSITIONING_SQUEEZE):
					playbook = CROWDED_POSITIONING_SQUEEZE
				}

				quantResult := uc.playbookQuantEngineUsecase.RunEngineWithPreparedContext(playbook, sel.Direction, prelimData, policy, prepared)
				quantResult.Tier = candidate.Tier
				quantResult.RawKlines = fullData.M15Candles

				if !isArbiterReadyQuantResult(quantResult) {
					result.quantFailures = append(result.quantFailures, quantFailure{
						Symbol:       pair,
						StrategyName: string(playbook),
						Direction:    string(sel.Direction),
						Reason:       quantFailureReason(quantResult),
					})
					continue
				}

				reconciliationDir := uc.conflictResolverUsecase.Resolve(quantResult.Direction, "NEUTRAL")
				_ = uc.scoringUsecase.Calculate(&quantResult, reconciliationDir, policy)

				result.quantResults = append(result.quantResults, quantResult)
			}
		}(idx, candidate)
	}
	pipelineWG.Wait()
	candidatePipelineDuration := time.Since(pipelineStart)
	metrics.SetLastCandidatePipelineDuration(candidatePipelineDuration)
	slog.Info("Candidate quant pipeline completed", "scan_id", scanID, "total_playbook_candidates", len(allCandidates), "duration", candidatePipelineDuration.String())
	estimatedRequestWeight := estimateScanRequestWeight(len(prefetchCandidates), int(atomic.LoadUint64(&enrichedSymbolCount)))
	metrics.SetLastScanRequestWeight(uint64(estimatedRequestWeight))
	metrics.SetLastScanRequestWeightBudget(uint64(requestGuard.Budget))
	metrics.SetLastPrefetchCandidateCount(uint64(len(prefetchCandidates)))
	metrics.SetLastEnrichedCandidateCount(atomic.LoadUint64(&enrichedSymbolCount))

	for _, result := range pipelineResults {
		if result.policyRejectReason != "" {
			policyRejectedSummary = append(policyRejectedSummary, result.policyRejectReason)
			funnelSummary.Add(funnelStagePipelineDrop, result.policyRejectReason)
			continue
		}
		totalStrategySelected += result.strategySelected
		totalPlaybookEligible += result.playbookEligible
		eligibilityFailures = append(eligibilityFailures, result.eligibilityFailures...)
		quantFailures = append(quantFailures, result.quantFailures...)
		allCandidates = append(allCandidates, result.quantResults...)
	}

	type rejectKey struct {
		Symbol       string
		StrategyName string
		Reason       string
	}
	rejectGroups := make(map[rejectKey][]string)
	var rejectKeys []rejectKey

	for _, f := range eligibilityFailures {
		key := rejectKey{Symbol: f.Symbol, StrategyName: f.StrategyName, Reason: f.Reason}
		if _, ok := rejectGroups[key]; !ok {
			rejectKeys = append(rejectKeys, key)
		}
		rejectGroups[key] = append(rejectGroups[key], f.Direction)
	}

	for _, key := range rejectKeys {
		dirs := rejectGroups[key]
		var dirStr string
		isLong := false
		isShort := false
		for _, d := range dirs {
			if d == "LONG" {
				isLong = true
			} else if d == "SHORT" {
				isShort = true
			}
		}
		if isLong && isShort {
			dirStr = "LONG/SHORT"
		} else if isLong {
			dirStr = "LONG"
		} else if isShort {
			dirStr = "SHORT"
		}

		policyRejectedSummary = append(policyRejectedSummary, fmt.Sprintf("%s (%s %s): %s", key.Symbol, key.StrategyName, dirStr, key.Reason))
		funnelSummary.Add(funnelStageEligibilityReject, key.Reason)
		switch key.StrategyName {
		case string(TREND_PULLBACK), string(LIQUIDITY_SWEEP_REVERSAL), string(COMPRESSION_BREAKOUT_RETEST), string(RANGE_EDGE_REVERSAL), string(CROWDED_POSITIONING_SQUEEZE):
			playbookBlockers.Add(Playbook(key.StrategyName), funnelStageEligibilityReject, key.Reason)
		}
	}

	quantRejectGroups := make(map[rejectKey][]string)
	var quantRejectKeys []rejectKey
	for _, f := range quantFailures {
		key := rejectKey{Symbol: f.Symbol, StrategyName: f.StrategyName, Reason: f.Reason}
		if _, ok := quantRejectGroups[key]; !ok {
			quantRejectKeys = append(quantRejectKeys, key)
		}
		quantRejectGroups[key] = append(quantRejectGroups[key], f.Direction)
	}

	for _, key := range quantRejectKeys {
		dirs := quantRejectGroups[key]
		var dirStr string
		isLong := false
		isShort := false
		for _, d := range dirs {
			if d == "LONG" {
				isLong = true
			} else if d == "SHORT" {
				isShort = true
			}
		}
		if isLong && isShort {
			dirStr = "LONG/SHORT"
		} else if isLong {
			dirStr = "LONG"
		} else if isShort {
			dirStr = "SHORT"
		}

		policyRejectedSummary = append(policyRejectedSummary, fmt.Sprintf("%s (%s %s): %s", key.Symbol, key.StrategyName, dirStr, key.Reason))
		funnelSummary.Add(funnelStageQuantReject, key.Reason)
		switch key.StrategyName {
		case string(TREND_PULLBACK), string(LIQUIDITY_SWEEP_REVERSAL), string(COMPRESSION_BREAKOUT_RETEST), string(RANGE_EDGE_REVERSAL), string(CROWDED_POSITIONING_SQUEEZE):
			playbookBlockers.Add(Playbook(key.StrategyName), funnelStageQuantReject, key.Reason)
		}
	}

	// Run Candidate Arbiter
	slog.Info("Running candidate arbiter selection...", "scan_id", scanID, "input_candidates", len(allCandidates))
	selectedCandidates, arbiterRejected := uc.candidateArbiterUsecase.Arbitrate(allCandidates, policy)
	slog.Info("Candidate arbiter completed", "scan_id", scanID, "selected_candidates", len(selectedCandidates), "rejected_candidates", len(arbiterRejected))
	activeWatchSignals, _ := uc.storageUsecase.LoadWatchJournal()
	arbiterSelectedRankMap := make(map[string]int, len(selectedCandidates))
	selectedSymbols := make([]string, 0, len(selectedCandidates))
	for index, candidate := range selectedCandidates {
		selectedSymbols = append(selectedSymbols, candidate.Symbol)
		arbiterSelectedRankMap[candidate.Symbol] = index + 1
	}
	activeSymbols := collectActiveJournalSymbols(activeSignals, activeWatchSignals)
	_ = uc.stalenessUsecase.SyncSymbols(unionActiveSymbols(activeSymbols, selectedSymbols))

	seenArbiterRejections := make(map[string]bool)
	for _, rej := range arbiterRejected {
		reason := rej.Reason
		if reason == "" {
			reason = "failed arbiter filter"
		}
		funnelSummary.Add(funnelStageArbiterReject, reason)
		playbookBlockers.Add(rej.Playbook, funnelStageArbiterReject, reason)
		entry := fmt.Sprintf("%s (%s %s): arbiter rejected - score=%0.1f reason=%s", rej.Symbol, rej.Playbook, rej.Direction, rej.Score, reason)
		if !seenArbiterRejections[entry] {
			seenArbiterRejections[entry] = true
			rejectedSummary = append(rejectedSummary, entry)
		}
	}

	// Evaluate Local Quality Gate
	slog.Info("Evaluating local quality gates for selected candidates...", "scan_id", scanID, "candidates_count", len(selectedCandidates))
	var localCandidates []QuantResult
	localGateMap := make(map[string]LocalGateResult)
	for _, qResult := range selectedCandidates {
		pair := qResult.Symbol
		cache := candlesMap[pair]
		lgRes := uc.localGateUsecase.EvaluateWithContext(ctx, qResult, policy, cache.data.M15Candles)
		localGateMap[pair] = lgRes
		if lgRes.Passed {
			localCandidates = append(localCandidates, qResult)
		} else if lgRes.Status == LOCAL_WATCH {
			funnelSummary.Add(funnelStageLocalWatch, lgRes.Reason)
			playbookBlockers.Add(qResult.Playbook, funnelStageLocalWatch, lgRes.Reason)
		} else {
			funnelSummary.Add(funnelStageLocalReject, lgRes.Reason)
			playbookBlockers.Add(qResult.Playbook, funnelStageLocalReject, lgRes.Reason)
		}
	}
	slog.Info("Local quality gates evaluation completed", "scan_id", scanID, "passed", len(localCandidates), "failed_or_watch", len(selectedCandidates)-len(localCandidates))

	// Select AI Candidates based on MaxAICandidates quota limit
	slog.Info("Selecting AI candidates based on policy MaxAICandidates quota limit...", "scan_id", scanID, "passed_local", len(localCandidates), "max_ai_candidates", policy.MaxAICandidates)
	aiCandidates, skippedCandidates := uc.aiCandidateSelectorUsecase.SelectCandidates(localCandidates, policy)
	enteredAIBatchMap := make(map[string]struct{}, len(aiCandidates))
	for _, candidate := range aiCandidates {
		enteredAIBatchMap[candidate.Symbol] = struct{}{}
	}
	_ = skippedCandidates

	// Fetch Gemini Audits concurrently
	aiConcurrencyLimit := 3
	if limit := getRuntimeSettings().MaxAIConcurrency; limit > 0 {
		aiConcurrencyLimit = limit
	}

	type aiAuditResult struct {
		symbol string
		resp   dto.AIAuditResponse
		err    error
	}
	aiAuditsMap := make(map[string]dto.AIAuditResponse)
	var aiMu sync.Mutex

	var aiWg sync.WaitGroup
	aiSem := make(chan struct{}, aiConcurrencyLimit)

	totalAIConfirm := 0
	totalAIWait := 0
	totalAIReject := 0
	totalAIError := 0
	totalAICalled := 0
	totalAIDisabled := 0

	GetGlobalMetrics().AddAICandidateCount(uint64(len(aiCandidates)))

	if len(aiCandidates) > 0 {
		slog.Info("Dispatching AI Audit queries to Gemini API...", "scan_id", scanID, "candidates_count", len(aiCandidates), "concurrency_limit", aiConcurrencyLimit)
	}
	aiBatchStart := time.Now()
	for _, qResult := range aiCandidates {
		aiWg.Add(1)
		go func(qr QuantResult) {
			defer aiWg.Done()
			aiSem <- struct{}{}
			defer func() { <-aiSem }()

			pair := qr.Symbol
			cache := candlesMap[pair]
			aiStart := time.Now()

			var auditResponse dto.AIAuditResponse
			var auditErr error

			if !getRuntimeSettings().AIAuditEnabled {
				auditResponse = dto.AIAuditResponse{
					Symbol:          pair,
					Decision:        "WAIT",
					IsApproved:      false,
					Sentiment:       "NEUTRAL",
					Confidence:      "LOW",
					ConfidenceScore: 0.3,
					Reasoning:       "AI_AUDIT_DISABLED: AI audit disabled by configuration; forcing non-executable WATCH verdict",
					Reason:          "AI_AUDIT_DISABLED",
					SuggestedAction: "WATCH_ONLY",
					Source:          AIAuditSourceSyntheticDisabled,
				}
			} else {
				auditResponse, auditErr = uc.aiAuditorUsecase.Audit(ctx, qr, policy, cache.data.M15Candles, cache.data.H1Candles, cache.data.H4Candles, localGateMap[pair].M5Summary)
				auditResponse.Source = NormalizeAIAuditSource(auditResponse.Source)
				aiDuration := time.Since(aiStart)
				GetGlobalMetrics().AddAILatency(aiDuration)
			}

			aiMu.Lock()
			if !getRuntimeSettings().AIAuditEnabled {
				totalAIWait++
				totalAIDisabled++
			} else if auditErr != nil {
				totalAICalled++
				totalAIError++
				if ctx.Err() == context.DeadlineExceeded || strings.Contains(strings.ToLower(auditErr.Error()), "timeout") || strings.Contains(strings.ToLower(auditErr.Error()), "deadline exceeded") {
					GetGlobalMetrics().IncrementAITimeoutCount()
				}
				auditResponse = dto.AIAuditResponse{
					Symbol:     pair,
					IsApproved: false,
					Sentiment:  "NEUTRAL",
					Reasoning:  "AI_ERROR: " + auditErr.Error(),
					Reason:     "AI_ERROR",
					Source:     AIAuditSourceRealError,
				}
			} else {
				totalAICalled++
				if auditResponse.Source == "" {
					auditResponse.Source = AIAuditSourceReal
				}
				if auditResponse.Decision == "CONFIRM" {
					totalAIConfirm++
				} else if auditResponse.Decision == "WAIT" {
					totalAIWait++
				} else {
					totalAIReject++
				}
			}
			aiAuditsMap[pair] = auditResponse
			aiMu.Unlock()
		}(qResult)
	}
	aiWg.Wait()
	aiBatchDuration := time.Since(aiBatchStart)
	metrics.SetLastAIBatchDuration(aiBatchDuration)
	if len(aiCandidates) > 0 {
		slog.Info("AI Audit batch processing completed", "scan_id", scanID, "confirm", totalAIConfirm, "wait", totalAIWait, "reject", totalAIReject, "error", totalAIError, "duration", aiBatchDuration.String())
	}

	// Build context and run Staleness Check and Final Execution Gates for all candidates
	var decisions []FinalDecision
	type candContext struct {
		quantResult     QuantResult
		auditResponse   dto.AIAuditResponse
		stalenessRes    StalenessResult
		localGateResult LocalGateResult
		planReview      PlanReview
		latestPrice     float64
		aiSkipped       bool
	}
	ctxMap := make(map[string]candContext)
	totalAISyntheticLocalGate := 0
	totalAISkippedQuota := 0

	slog.Info("Evaluating final execution gates and resolving conflicts...", "scan_id", scanID, "candidates_count", len(selectedCandidates))
	finalGateStart := time.Now()
	for _, qResult := range selectedCandidates {
		pair := qResult.Symbol
		cache := candlesMap[pair]
		lgRes := localGateMap[pair]

		var auditResponse dto.AIAuditResponse
		aiSkipped := false
		if !lgRes.Passed {
			decision := "REJECT"
			reason := "LOCAL_GATE_FAILED"
			if lgRes.Status == LOCAL_WATCH {
				decision = "WAIT"
				reason = "LOCAL_GATE_WATCH"
			}
			auditResponse = dto.AIAuditResponse{
				Symbol:     pair,
				IsApproved: false,
				Decision:   decision,
				Sentiment:  "NEUTRAL",
				Reasoning:  "Local gate failed: " + lgRes.Reason,
				Reason:     reason,
				Source:     AIAuditSourceSyntheticLocalGate,
			}
			totalAISyntheticLocalGate++
		} else {
			resp, audited := aiAuditsMap[pair]
			if audited {
				auditResponse = resp
				if strings.Contains(strings.ToUpper(auditResponse.Reason), "AI_ERROR") || strings.Contains(strings.ToUpper(auditResponse.Reasoning), "AI_ERROR") {
					reason := firstNonEmpty(auditResponse.Reason, auditResponse.Decision, "AI_ERROR")
					funnelSummary.Add(funnelStageAIError, reason)
					playbookBlockers.Add(qResult.Playbook, funnelStageAIError, reason)
				} else if auditResponse.Decision == "WAIT" {
					reason := firstNonEmpty(auditResponse.Reason, auditResponse.Decision)
					funnelSummary.Add(funnelStageAIWait, reason)
					playbookBlockers.Add(qResult.Playbook, funnelStageAIWait, reason)
				} else if auditResponse.Decision == "REJECT" {
					reason := firstNonEmpty(auditResponse.Reason, auditResponse.Decision)
					funnelSummary.Add(funnelStageAIReject, reason)
					playbookBlockers.Add(qResult.Playbook, funnelStageAIReject, reason)
				}
			} else {
				aiSkipped = true
				auditResponse = dto.AIAuditResponse{
					Symbol:     pair,
					IsApproved: false,
					Decision:   "WAIT",
					Sentiment:  "NEUTRAL",
					Reasoning:  "AI_SKIPPED: Exceeded policy MaxAICandidates quota limit",
					Reason:     "AI_SKIPPED",
					Source:     AIAuditSourceSyntheticQuota,
				}
				funnelSummary.Add(funnelStageAIWait, auditResponse.Reason)
				playbookBlockers.Add(qResult.Playbook, funnelStageAIWait, auditResponse.Reason)
				totalAISkippedQuota++
			}
		}

		planReview := uc.planReconciliationUsecase.Reconcile(qResult, auditResponse)

		latestPrice, latestPriceOK := uc.stalenessUsecase.ResolveLatestPrice(ctx, pair)
		if !latestPriceOK {
			latestPrice = 0
		}

		stalenessRes := uc.stalenessUsecase.Evaluate(qResult, planReview, policy, latestPrice)
		GetGlobalMetrics().AddStalenessChecked(1)
		if stalenessRes.IsStale {
			GetGlobalMetrics().AddStalenessCount(1)
		}

		finalDecision := uc.finalGateUsecase.Evaluate(
			qResult,
			lgRes,
			auditResponse,
			planReview,
			stalenessRes,
			policy,
			latestPrice,
			activeSignals,
			historySignals,
			cache.data.M15Candles,
		)

		ctxMap[pair] = candContext{
			quantResult:     qResult,
			auditResponse:   auditResponse,
			stalenessRes:    stalenessRes,
			localGateResult: lgRes,
			planReview:      planReview,
			latestPrice:     latestPrice,
			aiSkipped:       aiSkipped,
		}

		decisions = append(decisions, finalDecision)
	}
	finalGateDuration := time.Since(finalGateStart)
	metrics.SetLastFinalGateDuration(finalGateDuration)

	// Resolve conflicts and cooldown
	resolvedDecisions, updatedHistory := uc.conflictResolverUsecase.ResolveConflicts(decisions, historySignals, activeSignals, policy)

	// Save signal history
	histState := &entity.SignalHistory{
		Signals: updatedHistory,
	}
	if err := uc.storageUsecase.SaveSignalHistory(histState); err != nil {
		slog.Warn("Failed to save signal history", "error", err)
	}

	// Build beforeDecision lookup by symbol to avoid index mismatch after map iteration in ResolveConflicts
	beforeDecisionBySymbol := make(map[string]FinalDecision, len(decisions))
	for _, d := range decisions {
		beforeDecisionBySymbol[d.Symbol] = d
	}

	// Counters and list builders for final summary
	totalFinalExecute := 0
	totalFinalWatch := 0
	totalFinalReject := 0

	executeSignals := []dto.SignalResponse{}
	watchlistSignals := []dto.SignalResponse{}
	decisionAudits := make([]DecisionAudit, 0, len(resolvedDecisions))

	for _, finalDecision := range resolvedDecisions {
		pair := finalDecision.Symbol
		candCtx := ctxMap[pair]
		beforeDecision := beforeDecisionBySymbol[pair]

		// Save decision audit atomically
		conflictReason := ""
		cooldownReason := ""
		if finalDecision.Status != FINAL_EXECUTE && beforeDecision.Status == FINAL_EXECUTE {
			if finalDecision.WatchReason == "ACTIVE_MONITORING_EXISTS" || finalDecision.WatchReason == "OPPOSITE_SIGNAL_CONFLICT" || finalDecision.WatchReason == "LOWER_PRIORITY_CONFLICT" || finalDecision.WatchReason == "BTC_CHAOS_LIMIT" {
				conflictReason = finalDecision.WatchReason
				GetGlobalMetrics().AddConflictDowngrade(1)
			} else {
				cooldownReason = finalDecision.WatchReason
				GetGlobalMetrics().AddCooldownReject(1)
			}
		}

		localGateStatus := "FAILED"
		if candCtx.localGateResult.Passed {
			localGateStatus = "PASSED"
		}

		activeMode := string(policy.LongMode)
		if finalDecision.Direction == SHORT {
			activeMode = string(policy.ShortMode)
		}

		tp1 := 0.0
		tp2 := finalDecision.TakeProfit
		entryPrice := finalDecision.EntryPrice
		if entryPrice > 0 && tp2 > 0 {
			if finalDecision.Direction == LONG {
				tp1 = entryPrice + (tp2-entryPrice)*0.5
			} else {
				tp1 = entryPrice - (entryPrice-tp2)*0.5
			}
		}
		_, enteredAIBatch := enteredAIBatchMap[pair]

		audit := DecisionAudit{
			ScanID:                    scanID,
			ConfigVersion:             GetGlobalConfigRegistry().GetVersion(),
			GeneratedAt:               time.Now(),
			Symbol:                    pair,
			Direction:                 finalDecision.Direction,
			Playbook:                  finalDecision.Playbook,
			SetupType:                 candCtx.quantResult.SetupType,
			Tier:                      finalDecision.Tier,
			Grade:                     getGrade(finalDecision.Score),
			Score:                     finalDecision.Score,
			RR:                        finalDecision.RR,
			RRPlan:                    finalDecision.PlannedRR,
			RRActual:                  finalDecision.ActualRR,
			RequiredScore:             finalDecision.RequiredScore,
			RequiredRR:                finalDecision.RequiredRR,
			LocalGateStatus:           localGateStatus,
			LocalGateReason:           candCtx.localGateResult.Reason,
			EnteredAIBatch:            enteredAIBatch,
			AICalled:                  finalDecision.AICalled,
			AISource:                  finalDecision.AISource,
			AIDecision:                candCtx.auditResponse.Decision,
			AIConfidence:              finalDecision.AIConfidence,
			AICandleNarrative:         candCtx.auditResponse.CandleNarrative,
			AIEntryTiming:             candCtx.auditResponse.EntryTiming,
			AIConflictWithBot:         candCtx.auditResponse.ConflictWithBot,
			PlanStatus:                string(candCtx.planReview.Status),
			PlanConflict:              candCtx.planReview.Conflicted,
			NeedRetest:                candCtx.planReview.NeedRetest,
			StalenessStatus:           string(candCtx.stalenessRes.Status),
			FinalStatusBeforeConflict: beforeDecision.Status,
			FinalReasonBeforeConflict: beforeDecision.Reason,
			FinalStatusAfterConflict:  finalDecision.Status,
			FinalReasonAfterConflict:  finalDecision.Reason,
			FinalStatus:               finalDecision.Status,
			FinalReason:               finalDecision.Reason,
			FinalPrimaryReasonLayer:   finalDecision.PrimaryReasonLayer,
			FinalReasonBreakdown:      append([]string(nil), finalDecision.ReasonBreakdown...),
			ConflictReason:            conflictReason,
			CooldownReason:            cooldownReason,
			WasNotified:               finalDecision.Status == FINAL_EXECUTE && finalDecision.IsExecutable,
			ArbiterSelectedRank:       arbiterSelectedRankMap[pair],
			LatestPriceAtDecision:     candCtx.latestPrice,
			EntryPrice:                entryPrice,
			StopLoss:                  finalDecision.StopLoss,
			TakeProfit1:               tp1,
			TakeProfit2:               tp2,
			MarketRegime:              string(policy.Regime),
			PolicyMode:                activeMode,
			ThresholdProfileSummary:   finalDecision.ThresholdProfileSummary,
			BreakoutLevel:             candCtx.quantResult.TechnicalSnapshot.IndicatorValues[IndicatorBreakoutLevel],
			RetestTouches:             candCtx.quantResult.TechnicalSnapshot.IndicatorValues[IndicatorRetestTouches],
			RetestHold:                candCtx.quantResult.TechnicalSnapshot.IndicatorValues[IndicatorRetestHold] == 1.0,
			HasDerivativesEvidence:    candCtx.quantResult.TechnicalSnapshot.IndicatorValues[IndicatorHasCrowdingEvidence] == 1.0,
			RejectOrWatchReason:       finalDecision.Reason,
			CreatedAt:                 time.Now(),
			HypotheticalEntry:         entryPrice,
		}
		if candCtx.localGateResult.M5Summary != nil {
			audit.M5ConfirmationUsed = candCtx.localGateResult.M5Summary.Used
			audit.M5ConfirmationMode = string(candCtx.localGateResult.M5Summary.Mode)
			audit.M5ConfirmationStatus = string(candCtx.localGateResult.M5Summary.Status)
			audit.M5ConfirmationReason = candCtx.localGateResult.M5Summary.Reason
			audit.M5ConfirmationType = candCtx.localGateResult.M5Summary.ConfirmationType
			audit.M5Confirmed = candCtx.localGateResult.M5Summary.Confirmed
			audit.M5EarlyInvalidation = candCtx.localGateResult.M5Summary.EarlyInvalidation
		}

		if getRuntimeSettings().DecisionAuditEnabled {
			decisionAudits = append(decisionAudits, audit)
		}

		// Count final statuses
		if finalDecision.Status == FINAL_EXECUTE {
			totalFinalExecute++
			now := time.Now()
			sigRes := dto.SignalResponse{
				Symbol:             pair,
				Direction:          string(finalDecision.Direction),
				Timeframe:          "M15",
				TriggerPrice:       finalDecision.EntryPrice,
				StopLoss:           finalDecision.StopLoss,
				TakeProfit:         finalDecision.TakeProfit,
				Score:              finalDecision.Score,
				Strategy:           string(finalDecision.Playbook),
				AISentiment:        candCtx.auditResponse.Sentiment,
				IsFinalExecute:     true,
				ReconciledTime:     now,
				Status:             string(FINAL_EXECUTE),
				FinalReason:        finalDecision.Reason,
				IsHot:              candidateMap[pair].IsHot,
				HotScore:           candidateMap[pair].HotScore,
				HotSource:          candidateMap[pair].HotSource,
				HotRankType:        candidateMap[pair].HotRankType,
				HotOverlaySelected: candidateMap[pair].HotOverlaySelected,
			}
			executeSignals = append(executeSignals, sigRes)
			finalSignals = append(finalSignals, sigRes)

			// Save to virtual journal
			_ = uc.storageUsecase.SaveSignalToJournal(SignalJournal{
				ID:                      now.Format("20060102150405") + "_" + pair,
				ConfigVersion:           GetGlobalConfigRegistry().GetVersion(),
				Symbol:                  pair,
				Direction:               finalDecision.Direction,
				Playbook:                finalDecision.Playbook,
				EntryPrice:              finalDecision.EntryPrice,
				StopLoss:                finalDecision.StopLoss,
				TP1:                     tp1,
				TP2:                     tp2,
				RR:                      finalDecision.RR,
				QuantScore:              finalDecision.Score,
				AIConfidence:            finalDecision.AIConfidence,
				MarketRegime:            string(policy.Regime),
				PolicyMode:              activeMode,
				ThresholdProfileSummary: finalDecision.ThresholdProfileSummary,
				BreakoutLevel:           candCtx.quantResult.TechnicalSnapshot.IndicatorValues[IndicatorBreakoutLevel],
				RetestTouches:           candCtx.quantResult.TechnicalSnapshot.IndicatorValues[IndicatorRetestTouches],
				RetestHold:              candCtx.quantResult.TechnicalSnapshot.IndicatorValues[IndicatorRetestHold] == 1.0,
				HasDerivativesEvidence:  candCtx.quantResult.TechnicalSnapshot.IndicatorValues[IndicatorHasCrowdingEvidence] == 1.0,
				CreatedAt:               now,
				ExpiresAt:               now.Add(maxHoldDuration),
				Status:                  MONITORING,
				MFE:                     0.0,
				MAE:                     0.0,
				TimeToTP1:               "",
				TimeToTP2:               "",
				TimeToSL:                "",
				OutcomeReason:           "",
				EntryTiming:             candCtx.auditResponse.EntryTiming,
				Tier:                    candCtx.quantResult.Tier,
				Timeframe:               "M15",
				LatestPrice:             finalDecision.EntryPrice,
				TakeProfit:              tp2,
				AISentiment:             candCtx.auditResponse.Sentiment,
				AIReasoning:             candCtx.auditResponse.Reasoning,
				UpdatedAt:               now,
				IsHot:                   candidateMap[pair].IsHot,
				HotScore:                candidateMap[pair].HotScore,
				HotSource:               candidateMap[pair].HotSource,
				HotRankType:             candidateMap[pair].HotRankType,
				HotOverlaySelected:      candidateMap[pair].HotOverlaySelected,
				TechnicalSnapshot:       candCtx.quantResult.TechnicalSnapshot,
				StructureSnapshot:       candCtx.quantResult.StructureSnapshot,
			})
		} else if finalDecision.Status == FINAL_WATCH {
			totalFinalWatch++
			reason := firstNonEmpty(finalDecision.WatchReason, finalDecision.Reason)
			funnelSummary.Add(funnelStageFinalWatch, reason)
			playbookBlockers.Add(finalDecision.Playbook, funnelStageFinalWatch, reason)
			now := time.Now()
			watchlistSignals = append(watchlistSignals, dto.SignalResponse{
				Symbol:             pair,
				Direction:          string(finalDecision.Direction),
				Timeframe:          "M15",
				TriggerPrice:       finalDecision.EntryPrice,
				StopLoss:           finalDecision.StopLoss,
				TakeProfit:         finalDecision.TakeProfit,
				Score:              finalDecision.Score,
				Strategy:           string(finalDecision.Playbook),
				AISentiment:        candCtx.auditResponse.Sentiment,
				IsFinalExecute:     false,
				ReconciledTime:     now,
				Status:             string(FINAL_WATCH),
				Reason:             reason,
				FinalReason:        finalDecision.Reason,
				IsHot:              candidateMap[pair].IsHot,
				HotScore:           candidateMap[pair].HotScore,
				HotSource:          candidateMap[pair].HotSource,
				HotRankType:        candidateMap[pair].HotRankType,
				HotOverlaySelected: candidateMap[pair].HotOverlaySelected,
			})
			_ = uc.storageUsecase.SaveWatchToJournal(WatchJournal{
				ID:                      "watch_" + now.Format("20060102150405") + "_" + pair,
				ConfigVersion:           GetGlobalConfigRegistry().GetVersion(),
				Symbol:                  pair,
				Direction:               finalDecision.Direction,
				Playbook:                finalDecision.Playbook,
				EntryPrice:              finalDecision.EntryPrice,
				StopLoss:                finalDecision.StopLoss,
				TP1:                     tp1,
				TP2:                     tp2,
				RR:                      finalDecision.RR,
				QuantScore:              finalDecision.Score,
				AIConfidence:            finalDecision.AIConfidence,
				MarketRegime:            string(policy.Regime),
				PolicyMode:              activeMode,
				ThresholdProfileSummary: finalDecision.ThresholdProfileSummary,
				BreakoutLevel:           candCtx.quantResult.TechnicalSnapshot.IndicatorValues[IndicatorBreakoutLevel],
				RetestTouches:           candCtx.quantResult.TechnicalSnapshot.IndicatorValues[IndicatorRetestTouches],
				RetestHold:              candCtx.quantResult.TechnicalSnapshot.IndicatorValues[IndicatorRetestHold] == 1.0,
				HasDerivativesEvidence:  candCtx.quantResult.TechnicalSnapshot.IndicatorValues[IndicatorHasCrowdingEvidence] == 1.0,
				CreatedAt:               now,
				ExpiresAt:               now.Add(maxHoldDuration),
				Status:                  WATCH_MONITORING,
				MFE:                     0.0,
				MAE:                     0.0,
				TimeToTP1:               "",
				TimeToTP2:               "",
				TimeToSL:                "",
				OutcomeReason:           "",
				EntryTiming:             candCtx.auditResponse.EntryTiming,
				Tier:                    candCtx.quantResult.Tier,
				Timeframe:               "M15",
				LatestPrice:             finalDecision.EntryPrice,
				TakeProfit:              tp2,
				AISentiment:             candCtx.auditResponse.Sentiment,
				AIReasoning:             candCtx.auditResponse.Reasoning,
				UpdatedAt:               now,
				Reason:                  finalDecision.Reason,
				IsHot:                   candidateMap[pair].IsHot,
				HotScore:                candidateMap[pair].HotScore,
				HotSource:               candidateMap[pair].HotSource,
				HotRankType:             candidateMap[pair].HotRankType,
				HotOverlaySelected:      candidateMap[pair].HotOverlaySelected,
				TechnicalSnapshot:       candCtx.quantResult.TechnicalSnapshot,
				StructureSnapshot:       candCtx.quantResult.StructureSnapshot,
			})
		} else {
			totalFinalReject++
			reason := firstNonEmpty(finalDecision.RejectReason, finalDecision.Reason)
			funnelSummary.Add(funnelStageFinalReject, reason)
			playbookBlockers.Add(finalDecision.Playbook, funnelStageFinalReject, reason)
		}
	}

	if getRuntimeSettings().DecisionAuditEnabled && len(decisionAudits) > 0 {
		if err := uc.storageUsecase.SaveDecisionAuditBatch(decisionAudits); err != nil {
			slog.Warn("Failed to save decision audit batch", "count", len(decisionAudits), "error", err)
		}
	}

	thresholdProfileSummary := make(map[string]string)
	for _, fd := range resolvedDecisions {
		thresholdProfileSummary[string(fd.Playbook)] = fd.ThresholdProfileSummary
	}
	funnelStageSummary := funnelSummary.Build()
	topFunnelBlockers := buildTopFunnelBlockers(funnelStageSummary, 5)
	playbookBlockerSummary := playbookBlockers.Build()

	// Dispatch V3 Notifications
	summary := ScannerSummaryV3{
		TotalScanned:                    len(prefetchCandidates),
		CandidatesFound:                 len(decisions),
		StartTime:                       scanStart,
		Duration:                        time.Since(scanStart).String(),
		ActiveRegime:                    string(policy.Regime),
		BtcTrend:                        macroState.BTCTrend,
		TotalTickers:                    totalTickers,
		TotalUniversePass:               universePassCount,
		TotalUniverseRejected:           totalTickers - universePassCount,
		TotalStrategySelected:           totalStrategySelected,
		TotalPlaybookEligible:           totalPlaybookEligible,
		TotalQuantCandidates:            len(allCandidates),
		TotalArbiterSelected:            len(selectedCandidates),
		TotalLocalAICandidate:           len(localCandidates),
		PrefetchLimit:                   prefetchLimit,
		TotalPrefetchSelected:           len(prefetchCandidates),
		TotalPrefetchDeferred:           len(candidates) - len(prefetchCandidates),
		PrefetchHotSlots:                prefetchDebug.HotSlots,
		PrefetchRotationSlots:           prefetchDebug.RotationSlots,
		TotalAIBatchEntered:             len(aiCandidates),
		TotalAICalled:                   totalAICalled,
		TotalAISyntheticLocalGate:       totalAISyntheticLocalGate,
		TotalAISkippedQuota:             totalAISkippedQuota,
		TotalAIDisabled:                 totalAIDisabled,
		TotalAIConfirm:                  totalAIConfirm,
		TotalAIWait:                     totalAIWait,
		TotalAIReject:                   totalAIReject,
		TotalAIError:                    totalAIError,
		TotalFinalExecute:               totalFinalExecute,
		TotalFinalWatch:                 totalFinalWatch,
		TotalFinalReject:                totalFinalReject,
		ExecuteSignals:                  executeSignals,
		Watchlist:                       watchlistSignals,
		RejectedSummary:                 rejectedSummary,
		PolicyRejectedSummary:           policyRejectedSummary,
		SelectedThresholdProfileSummary: thresholdProfileSummary,
		FunnelStageSummary:              funnelStageSummary,
		TopFunnelBlockers:               topFunnelBlockers,
		PlaybookBlockerSummary:          playbookBlockerSummary,
		EvaluationDataCompletenessHint:  "has_decision_audit: true",
	}

	slog.Info("Dispatching scanner notifications and saving latest results...", "scan_id", scanID, "execute_signals", totalFinalExecute, "watch_signals", totalFinalWatch)
	var notificationReqs []SignalNotificationRequest
	for _, dec := range resolvedDecisions {
		pair := dec.Symbol
		candCtx := ctxMap[pair]
		notificationReqs = append(notificationReqs, SignalNotificationRequest{
			Decision:      dec,
			AuditResponse: candCtx.auditResponse,
		})
	}

	// OPS: optional admin warning for AI_ERROR_REVIEW (must NOT be sent via SIGNAL channel).
	if uc.opsNotificationUsecase != nil {
		for _, dec := range resolvedDecisions {
			if dec.Status == AI_ERROR_REVIEW {
				uc.opsNotificationUsecase.SendAdminWarningAIError(
					ctx,
					scanID,
					dec.Symbol,
					string(dec.Playbook),
					string(dec.Status),
					dec.Reason,
				)
			}
		}
	}

	// SIGNAL: only actionable FINAL_EXECUTE after conflict resolution.
	if uc.signalNotificationUsecase != nil {
		uc.signalNotificationUsecase.SendV3Signals(ctx, notificationReqs, policy, summary)
	}

	arbiterDetails := []entity.ArbiterSelectedDetail{}
	for _, dec := range resolvedDecisions {
		pair := dec.Symbol
		candCtx := ctxMap[pair]

		localGateStatus := "FAILED"
		if candCtx.localGateResult.Passed {
			localGateStatus = "PASSED"
		} else if candCtx.localGateResult.Status == LOCAL_WATCH {
			localGateStatus = "LOCAL_WATCH"
		}

		arbiterDetails = append(arbiterDetails, entity.ArbiterSelectedDetail{
			Symbol:          pair,
			Playbook:        string(dec.Playbook),
			Direction:       string(dec.Direction),
			LocalGateStatus: localGateStatus,
			AIDecision:      candCtx.auditResponse.Decision,
			AIConfidence:    candCtx.auditResponse.Confidence,
			StalenessStatus: string(candCtx.stalenessRes.Status),
			FinalStatus:     string(dec.Status),
			FinalReason:     dec.Reason,
		})
	}

	// Save latest scan results
	latestResult := &entity.LatestResult{
		GeneratedAt:                     scanStart,
		ConfigVersion:                   GetGlobalConfigRegistry().GetVersion(),
		ScanID:                          scanID,
		MarketPolicy:                    policy.Reason,
		MarketRegime:                    string(policy.Regime),
		MacroVolatility:                 macroState.Volatility,
		MarketBreadth:                   macroState.Breadth,
		MedianAbsMove24h:                macroState.MedianAbsMove24h,
		ActiveMoveShare:                 macroState.ActiveMoveShare,
		QuietMoveShare:                  macroState.QuietMoveShare,
		TotalTickers:                    totalTickers,
		TotalUniversePass:               universePassCount,
		TotalUniverseRejected:           totalTickers - universePassCount,
		TotalStrategySelected:           totalStrategySelected,
		TotalPlaybookEligible:           totalPlaybookEligible,
		TotalQuantCandidates:            len(allCandidates),
		TotalArbiterSelected:            len(selectedCandidates),
		TotalLocalAICandidate:           len(localCandidates),
		PrefetchLimit:                   prefetchLimit,
		TotalPrefetchSelected:           len(prefetchCandidates),
		TotalPrefetchDeferred:           len(candidates) - len(prefetchCandidates),
		PrefetchHotSlots:                prefetchDebug.HotSlots,
		PrefetchRotationSlots:           prefetchDebug.RotationSlots,
		TotalAIBatchEntered:             len(aiCandidates),
		TotalAICalled:                   totalAICalled,
		TotalAISyntheticLocalGate:       totalAISyntheticLocalGate,
		TotalAISkippedQuota:             totalAISkippedQuota,
		TotalAIDisabled:                 totalAIDisabled,
		TotalAIConfirm:                  totalAIConfirm,
		TotalAIWait:                     totalAIWait,
		TotalAIReject:                   totalAIReject,
		TotalAIError:                    totalAIError,
		TotalFinalExecute:               totalFinalExecute,
		TotalFinalWatch:                 totalFinalWatch,
		TotalFinalReject:                totalFinalReject,
		ExecuteSignals:                  executeSignals,
		Watchlist:                       watchlistSignals,
		RejectedSummary:                 rejectedSummary,
		PolicyRejectedSummary:           policyRejectedSummary,
		SelectedThresholdProfileSummary: thresholdProfileSummary,
		FunnelStageSummary:              funnelStageSummary,
		TopFunnelBlockers:               topFunnelBlockers,
		PlaybookBlockerSummary:          playbookBlockerSummary,
		EvaluationDataCompletenessHint:  "has_decision_audit: true",
		CompressionZeroEligibleStreak:   nextCompressionZeroEligibleStreak(previousLatest, compressionMacroActive, totalPlaybookEligible),
		CompressionLowVolFallbackActive: compressionFallbackActive,
		ArbiterSelectedDetails:          arbiterDetails,

		LastScanTime: scanStart,
		Duration:     time.Since(scanStart).String(),
		Signals:      finalSignals,
	}

	if err := uc.storageUsecase.SaveLatestResult(latestResult); err != nil {
		slog.Error("Failed to save latest scan result to storage", "error", err)
	}

	duration := time.Since(scanStart)
	metrics.SetLastScanDuration(duration)
	metrics.SetLastScanTime(scanStart)
	metrics.IncrementScanSuccess()
	metrics.SetLastSuccessScan(scanStart)
	metrics.AddTotalTickers(uint64(totalTickers))
	metrics.AddUniversePass(uint64(universePassCount))
	metrics.AddUniverseReject(uint64(totalTickers - universePassCount))

	metrics.AddFinalExecuteCount(uint64(totalFinalExecute))
	metrics.AddFinalWatchCount(uint64(totalFinalWatch))
	metrics.AddFinalRejectCount(uint64(totalFinalReject))

	slog.Info("AnalyzeMarketV3 Scan Completed",
		"scan_id", scanID,
		"found_signals", len(finalSignals),
		"prefetch_candidates", len(prefetchCandidates),
		"enriched_candidates", atomic.LoadUint64(&enrichedSymbolCount),
		"estimated_request_weight", estimatedRequestWeight,
		"request_weight_budget", requestGuard.Budget,
		"adaptive_request_guard", requestGuard.Applied,
		"market_data_ms", marketDataDuration.Milliseconds(),
		"candidate_pipeline_ms", candidatePipelineDuration.Milliseconds(),
		"ai_batch_ms", aiBatchDuration.Milliseconds(),
		"final_gate_ms", finalGateDuration.Milliseconds(),
		"total_ms", duration.Milliseconds(),
		"funnel_summary", formatFunnelLogSummary(funnelStageSummary, 5),
		"top_funnel_blockers", topFunnelBlockers,
	)
	if estimatedRequestWeight >= maxInt(1, int(float64(requestGuard.Budget)*0.9)) {
		slog.Warn("AnalyzeMarketV3 estimated request weight is elevated",
			"scan_id", scanID,
			"estimated_request_weight", estimatedRequestWeight,
			"request_weight_budget", requestGuard.Budget,
			"prefetch_candidates", len(prefetchCandidates),
			"enriched_candidates", atomic.LoadUint64(&enrichedSymbolCount),
		)
	}

	return dto.ScanResult{
		Timestamp: scanStart,
		Duration:  time.Since(scanStart).String(),
		Found:     len(finalSignals),
		Signals:   finalSignals,
	}, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func resolveMarketDataPrefetchLimit(policy MarketPolicy, totalCandidates int) int {
	if totalCandidates <= 0 {
		return 0
	}

	if limit := getRuntimeSettings().MaxMarketDataPrefetchSymbols; limit > 0 {
		return minInt(totalCandidates, limit)
	}

	regime := policy.EffectiveRegime()
	base := maxInt(policy.MaxAICandidates*6, policy.MaxFinalExecute*3)
	floor := 14
	hardCeiling := 0

	switch regime {
	case BTC_CHAOS:
		floor = 8
		hardCeiling = 10
	case LOW_VOL:
		floor = 10
	case HIGH_VOL:
		floor = 10
		hardCeiling = 18
	case CHOP_RANGE:
		floor = 12
	case BTC_DOMINANCE:
		floor = 12
	case RISK_OFF:
		floor = 14
	case COMPRESSION:
		floor = 12
	case ALT_SUPPORTIVE:
		floor = 16
	}

	ceiling := resolveBudgetDrivenPrefetchCap(policy)
	if hardCeiling > 0 && (ceiling <= 0 || hardCeiling < ceiling) {
		ceiling = hardCeiling
	}
	if ceiling < 1 {
		ceiling = 1
	}

	recommended := maxInt(base, floor)
	if shouldBroadenPrefetchSampling(policy, totalCandidates) && ceiling > recommended {
		recommended = ceiling
	}
	if recommended > ceiling {
		recommended = ceiling
	}
	if policy.MaxSymbols > 0 && recommended > policy.MaxSymbols {
		recommended = policy.MaxSymbols
	}
	if recommended < 1 {
		recommended = 1
	}

	return minInt(totalCandidates, recommended)
}

func resolveBudgetDrivenPrefetchCap(policy MarketPolicy) int {
	baseWeight := estimateScanRequestWeight(0, 0)
	perCandidateWeight := estimateScanRequestWeight(1, 1) - baseWeight
	if perCandidateWeight <= 0 {
		perCandidateWeight = 1
	}

	budgetHeadroom := resolveScanRequestWeightBudget(policy) - baseWeight
	if budgetHeadroom <= 0 {
		return 1
	}

	cap := budgetHeadroom / perCandidateWeight
	if cap < 1 {
		return 1
	}
	return cap
}

func shouldBroadenPrefetchSampling(policy MarketPolicy, totalCandidates int) bool {
	switch policy.EffectiveRegime() {
	case CHOP_RANGE, COMPRESSION, LOW_VOL:
	default:
		return false
	}

	if totalCandidates < 10 {
		return false
	}

	if policy.MaxSymbols <= 0 {
		return totalCandidates >= 30
	}

	saturationThreshold := maxInt(10, ((policy.MaxSymbols*4)+4)/5)
	return totalCandidates >= saturationThreshold
}

func resolveAdaptiveScanRequestGuard(policy MarketPolicy, totalCandidates, requestedPrefetch, marketConcurrency int) scanRequestGuardProfile {
	guard := scanRequestGuardProfile{
		Budget:                resolveScanRequestWeightBudget(policy),
		PrefetchLimit:         requestedPrefetch,
		MarketDataConcurrency: marketConcurrency,
	}

	if guard.PrefetchLimit <= 0 {
		guard.PrefetchLimit = 0
		return guard
	}

	for guard.PrefetchLimit > 1 {
		guard.ExpectedWeight = estimateScanRequestWeight(guard.PrefetchLimit, guard.PrefetchLimit)
		if guard.ExpectedWeight <= guard.Budget {
			break
		}
		guard.PrefetchLimit--
		guard.Applied = true
	}
	guard.ExpectedWeight = estimateScanRequestWeight(guard.PrefetchLimit, guard.PrefetchLimit)

	utilization := float64(guard.ExpectedWeight) / float64(maxInt(guard.Budget, 1))
	if utilization >= 1.0 {
		if guard.MarketDataConcurrency > 3 {
			guard.MarketDataConcurrency = 3
			guard.Applied = true
		}
		guard.PipelineConcurrency = 3
	} else if utilization >= 0.85 {
		if guard.MarketDataConcurrency > 4 {
			guard.MarketDataConcurrency = 4
			guard.Applied = true
		}
		guard.PipelineConcurrency = 4
	}

	if policy.EffectiveRegime() == BTC_CHAOS && guard.MarketDataConcurrency > 2 {
		guard.MarketDataConcurrency = 2
		guard.Applied = true
		if guard.PipelineConcurrency == 0 || guard.PipelineConcurrency > 2 {
			guard.PipelineConcurrency = 2
		}
	}

	if guard.PrefetchLimit > totalCandidates {
		guard.PrefetchLimit = totalCandidates
	}
	if guard.PipelineConcurrency > guard.PrefetchLimit {
		guard.PipelineConcurrency = guard.PrefetchLimit
	}

	return guard
}

func resolveScanRequestWeightBudget(policy MarketPolicy) int {
	if limit := getRuntimeSettings().ScanRequestWeightBudget; limit > 0 {
		return limit
	}

	switch policy.EffectiveRegime() {
	case BTC_CHAOS:
		return 110
	case LOW_VOL:
		return 125
	case HIGH_VOL:
		return 140
	case CHOP_RANGE, COMPRESSION:
		return 150
	case BTC_DOMINANCE:
		return 160
	case ALT_SUPPORTIVE:
		return 180
	default:
		return 170
	}
}

func estimateScanRequestWeight(prefetchCandidates int, enrichedCandidates int) int {
	if prefetchCandidates < 0 {
		prefetchCandidates = 0
	}
	if enrichedCandidates < 0 {
		enrichedCandidates = 0
	}

	const (
		allTicker24hWeight   = 40
		premiumIndexWeight   = 10
		initialM15Weight     = 1
		enrichSnapshotWeight = 4 // 1h kline=1, 4h kline(210)=2, open interest=1
	)

	return allTicker24hWeight + premiumIndexWeight + (prefetchCandidates * initialM15Weight) + (enrichedCandidates * enrichSnapshotWeight)
}
