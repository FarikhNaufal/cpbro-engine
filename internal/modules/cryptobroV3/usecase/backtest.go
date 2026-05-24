package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

type BacktestRequest struct {
	Symbol          string    `json:"symbol"`
	StartTime       time.Time `json:"start_time"`
	EndTime         time.Time `json:"end_time"`
	Playbook        string    `json:"playbook,omitempty"`          // Optional: filter playbook (e.g. "ALL" or specific name)
	Regime          string    `json:"regime,omitempty"`            // Optional: "DYNAMIC" or forced regime ("BULLISH", "BEARISH", etc.)
	ProfileVersion  string    `json:"profile_version,omitempty"`   // Optional
	AIMode          string    `json:"ai_mode,omitempty"`           // Optional: "MOCK", "RULE_BASED", "SKIP"
	MaxAICandidates int       `json:"max_ai_candidates,omitempty"` // Optional override
}

type BacktestReportSummary struct {
	RunID                      string    `json:"run_id"`
	ConfigVersion              string    `json:"config_version,omitempty"`
	GeneratedAt                time.Time `json:"generated_at"`
	Symbol                     string    `json:"symbol"`
	StartTime                  time.Time `json:"start_time"`
	EndTime                    time.Time `json:"end_time"`
	PlaybookFilter             string    `json:"playbook_filter"`
	RegimeMode                 string    `json:"regime_mode"`
	AIMode                     string    `json:"ai_mode"`
	TotalSetups                int       `json:"total_setups"`
	TotalFinalExecuteSimulated int       `json:"total_final_execute_simulated"`
	TP1Hits                    int       `json:"tp1_hits"`
	TP2Hits                    int       `json:"tp2_hits"`
	SLHits                     int       `json:"sl_hits"`
	ExpiredHits                int       `json:"expired_hits"`
	Winrate                    float64   `json:"winrate"`
	Expectancy                 float64   `json:"expectancy"`
	AvgMFE                     float64   `json:"avg_mfe"`
	AvgMAE                     float64   `json:"avg_mae"`
	StaleRate                  float64   `json:"stale_rate"`
	MissedOpportunityCount     int       `json:"missed_opportunity_count"`
}

type PlaybookPerformanceStats struct {
	Total          int     `json:"total"`
	Executed       int     `json:"executed"`
	Wins           int     `json:"wins"`
	Losses         int     `json:"losses"`
	Expired        int     `json:"expired"`
	Winrate        float64 `json:"winrate"`
	Expectancy     float64 `json:"expectancy"`
	AvgMFE         float64 `json:"avg_mfe"`
	AvgMAE         float64 `json:"avg_mae"`
	FalsePositives int     `json:"false_positives"`
}

type RegimePerformanceStats struct {
	Total      int     `json:"total"`
	Executed   int     `json:"executed"`
	Wins       int     `json:"wins"`
	Losses     int     `json:"losses"`
	Expired    int     `json:"expired"`
	Winrate    float64 `json:"winrate"`
	Expectancy float64 `json:"expectancy"`
}

type SimulatedPositionLog struct {
	ID                      string    `json:"id"`
	Symbol                  string    `json:"symbol"`
	Direction               Direction `json:"direction"`
	Playbook                Playbook  `json:"playbook"`
	EntryPrice              float64   `json:"entry_price"`
	StopLoss                float64   `json:"stop_loss"`
	TP1                     float64   `json:"tp1"`
	TP2                     float64   `json:"tp2"`
	RR                      float64   `json:"rr"`
	QuantScore              float64   `json:"quant_score"`
	AIConfidence            string    `json:"ai_confidence"`
	MarketRegime            string    `json:"market_regime"`
	CreatedAt               time.Time `json:"created_at"`
	ExpiresAt               time.Time `json:"expires_at"`
	ClosedAt                time.Time `json:"closed_at"`
	Status                  Status    `json:"status"` // MONITORING, TP1_HIT, TP2_HIT, SL_HIT, EXPIRED
	MFE                     float64   `json:"mfe"`
	MAE                     float64   `json:"mae"`
	PnlPercentage           float64   `json:"pnl_percentage"`
	TimeToTP1               string    `json:"time_to_tp1"`
	TimeToTP2               string    `json:"time_to_tp2"`
	TimeToSL                string    `json:"time_to_sl"`
	OutcomeReason           string    `json:"outcome_reason"`
	Grade                   string    `json:"grade"`
	ThresholdProfileSummary string    `json:"threshold_profile_summary"`
}

type SimulatedWatchLog struct {
	Symbol                  string    `json:"symbol"`
	Direction               Direction `json:"direction"`
	Playbook                Playbook  `json:"playbook"`
	EntryPrice              float64   `json:"entry_price"`
	StopLoss                float64   `json:"stop_loss"`
	TP1                     float64   `json:"tp1"`
	TP2                     float64   `json:"tp2"`
	Status                  Status    `json:"status"` // FINAL_WATCH, FINAL_REJECT
	Reason                  string    `json:"reason"`
	CreatedAt               time.Time `json:"created_at"`
	ExpiresAt               time.Time `json:"expires_at"`
	MissedOpportunityResult string    `json:"missed_opportunity_result"` // TP1_HIT, TP2_HIT, SL_HIT, EXPIRED, NOT_FILLED
}

type BacktestReport struct {
	BacktestReportSummary
	PlaybookPerformance map[string]PlaybookPerformanceStats `json:"playbook_performance"`
	RegimePerformance   map[string]RegimePerformanceStats   `json:"regime_performance"`
	Positions           []SimulatedPositionLog              `json:"positions"`
	Watches             []SimulatedWatchLog                 `json:"watches"`
}

type BacktestEngineUsecase struct {
	marketDataProvider         MarketDataProvider
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
	storageUsecase             *StorageUsecase
	storageDir                 string
}

func NewBacktestEngineUsecase(
	provider MarketDataProvider,
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
	storage *StorageUsecase,
	storageDir string,
) *BacktestEngineUsecase {
	return &BacktestEngineUsecase{
		marketDataProvider:         provider,
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
		storageUsecase:             storage,
		storageDir:                 storageDir,
	}
}

// Helper function to slice klines closed before or at time T
func FilterClosedCandles(candles []dto.Candle, t time.Time, duration time.Duration) []dto.Candle {
	var result []dto.Candle
	for _, c := range candles {
		closeTime := c.Time.Add(duration)
		if closeTime.Before(t) || closeTime.Equal(t) {
			result = append(result, c)
		}
	}
	return result
}

// RunBacktest executes the replaying loop
func (uc *BacktestEngineUsecase) RunBacktest(ctx context.Context, req BacktestRequest) (*BacktestReport, error) {
	if req.Symbol == "" {
		return nil, errors.New("symbol is required")
	}
	if req.StartTime.IsZero() || req.EndTime.IsZero() {
		return nil, errors.New("start_time and end_time are required")
	}
	if req.StartTime.After(req.EndTime) {
		return nil, errors.New("start_time cannot be after end_time")
	}

	// Default parameters
	if req.Playbook == "" {
		req.Playbook = "ALL"
	}
	if req.Regime == "" {
		req.Regime = "DYNAMIC"
	}
	if req.AIMode == "" {
		req.AIMode = "MOCK"
	}
	if req.MaxAICandidates <= 0 {
		req.MaxAICandidates = 2
	}

	slog.Info("Starting Historical Backtest Run", "symbol", req.Symbol, "start", req.StartTime, "end", req.EndTime)

	// Fetch historical candles for target symbol and BTCUSDT.
	// We warm up with 24 hours of data before StartTime.
	warmupStart := req.StartTime.Add(-24 * time.Hour)
	allSymbolM15, err := uc.marketDataProvider.FetchHistoricalCandles(ctx, req.Symbol, "15m", warmupStart, req.EndTime)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch M15 candles for %s: %w", req.Symbol, err)
	}
	allSymbolH1, _ := uc.marketDataProvider.FetchHistoricalCandles(ctx, req.Symbol, "1h", warmupStart, req.EndTime)
	allSymbolH4, _ := uc.marketDataProvider.FetchHistoricalCandles(ctx, req.Symbol, "4h", warmupStart, req.EndTime)

	allBtcM15, err := uc.marketDataProvider.FetchHistoricalCandles(ctx, "BTCUSDT", "15m", warmupStart, req.EndTime)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch M15 candles for BTCUSDT: %w", err)
	}

	if len(allSymbolM15) == 0 || len(allBtcM15) == 0 {
		return nil, errors.New("no historical klines returned for backtest interval")
	}

	// Sort candles chronologically to be absolutely safe
	sortCandles(allSymbolM15)
	sortCandles(allSymbolH1)
	sortCandles(allSymbolH4)
	sortCandles(allBtcM15)

	// Initialize tracking structures
	var positions []SimulatedPositionLog
	var watches []SimulatedWatchLog
	var mockHistory []dto.SignalResponse
	staleDetections := 0
	totalScansEvaluated := 0

	// Simulated running list of positions
	var activePositions []*SimulatedPositionLog
	var activeWatches []*SimulatedWatchLog

	// Time progression: step by step 15m intervals
	currentTick := req.StartTime.Truncate(15 * time.Minute)
	endTick := req.EndTime.Truncate(15 * time.Minute)

	for currentTick.Before(endTick) || currentTick.Equal(endTick) {
		// 1. Filter closed candles up to currentTick
		closedM15 := FilterClosedCandles(allSymbolM15, currentTick, 15*time.Minute)
		closedH1 := FilterClosedCandles(allSymbolH1, currentTick, time.Hour)
		closedH4 := FilterClosedCandles(allSymbolH4, currentTick, 4*time.Hour)

		closedBtcM15 := FilterClosedCandles(allBtcM15, currentTick, 15*time.Minute)

		// Ensure we have enough klines for indicators (warmup limit)
		if len(closedM15) < 30 || len(closedBtcM15) < 30 {
			currentTick = currentTick.Add(15 * time.Minute)
			continue
		}

		// Keep only last 50 for memory safety and indicator bounds
		if len(closedM15) > 50 {
			closedM15 = closedM15[len(closedM15)-50:]
		}
		if len(closedH1) > 50 {
			closedH1 = closedH1[len(closedH1)-50:]
		}
		if len(closedH4) > 50 {
			closedH4 = closedH4[len(closedH4)-50:]
		}
		if len(closedBtcM15) > 50 {
			closedBtcM15 = closedBtcM15[len(closedBtcM15)-50:]
		}

		latestPrice := closedM15[len(closedM15)-1].Close

		// 2. Dynamic BTC trend/regime calculation
		btcLatestPrice := closedBtcM15[len(closedBtcM15)-1].Close
		// Find kline from 24h ago
		btc24hAgoPrice := closedBtcM15[0].Close
		target24hAgo := currentTick.Add(-24 * time.Hour)
		for _, c := range closedBtcM15 {
			if c.Time.Equal(target24hAgo) || c.Time.After(target24hAgo) {
				btc24hAgoPrice = c.Close
				break
			}
		}

		btcChangePercent24h := ((btcLatestPrice - btc24hAgoPrice) / btc24hAgoPrice) * 100.0
		btcTrend := "SIDEWAYS"
		btcScore := 50.0 + (btcChangePercent24h * 5.0)
		if btcChangePercent24h > 1.5 {
			btcTrend = "BULLISH"
		} else if btcChangePercent24h < -1.5 {
			btcTrend = "BEARISH"
		}

		if btcScore > 100.0 {
			btcScore = 100.0
		} else if btcScore < 0.0 {
			btcScore = 0.0
		}

		absChange := math.Abs(btcChangePercent24h)
		volatility := "NORMAL"
		btcChaos := 0.2
		if absChange > 5.0 {
			volatility = "HIGH"
			btcChaos = 0.85
		} else if absChange < 0.5 {
			volatility = "LOW"
			btcChaos = 0.1
		}

		// Evaluate Policy
		var policy MarketPolicy
		if req.Regime != "DYNAMIC" {
			// Forced regime
			policy = uc.marketPolicyUsecase.EvaluatePolicy(ctx, req.Regime, 50.0, 0.0, 0.2, "NORMAL", 0.5)
		} else {
			policy = uc.marketPolicyUsecase.EvaluatePolicy(ctx, btcTrend, btcScore, 0.0, btcChaos, volatility, 0.5)
		}

		// Calculate 24h quote volume & price change for the target symbol to build ticker
		symbol24hAgoPrice := closedM15[0].Close
		for _, c := range closedM15 {
			if c.Time.Equal(target24hAgo) || c.Time.After(target24hAgo) {
				symbol24hAgoPrice = c.Close
				break
			}
		}
		symbolChangePercent := ((latestPrice - symbol24hAgoPrice) / symbol24hAgoPrice) * 100.0

		quoteVolume24h := 0.0
		for _, c := range closedM15 {
			quoteVolume24h += c.Close * c.Vol
		}

		// Build simulated tickers & funding rates
		tickers := []dto.Ticker24h{
			{Symbol: "BTCUSDT", LastPrice: btcLatestPrice, PriceChangePercent: btcChangePercent24h, QuoteVolume: 1000000000.0},
			{Symbol: req.Symbol, LastPrice: latestPrice, PriceChangePercent: symbolChangePercent, QuoteVolume: quoteVolume24h},
		}
		fundingRates := map[string]float64{
			"BTCUSDT":  0.0001,
			req.Symbol: 0.0001,
		}

		// 3. Replay active simulated positions updates (simulate Monitoring)
		// We evaluate using the single M15 candle that ended at currentTick
		latestCandle := closedM15[len(closedM15)-1]
		var remainingPositions []*SimulatedPositionLog
		for _, pos := range activePositions {
			// Update MFE / MAE
			if pos.Direction == LONG {
				favorable := ((latestCandle.High - pos.EntryPrice) / pos.EntryPrice) * 100.0
				if favorable > pos.MFE {
					pos.MFE = favorable
				}
				adverse := ((pos.EntryPrice - latestCandle.Low) / pos.EntryPrice) * 100.0
				if adverse > pos.MAE {
					pos.MAE = adverse
				}
			} else { // SHORT
				favorable := ((pos.EntryPrice - latestCandle.Low) / pos.EntryPrice) * 100.0
				if favorable > pos.MFE {
					pos.MFE = favorable
				}
				adverse := ((latestCandle.High - pos.EntryPrice) / pos.EntryPrice) * 100.0
				if adverse > pos.MAE {
					pos.MAE = adverse
				}
			}

			// Check SL (takes precedence)
			isSL := false
			if pos.Direction == LONG && latestCandle.Low <= pos.StopLoss {
				isSL = true
			} else if pos.Direction == SHORT && latestCandle.High >= pos.StopLoss {
				isSL = true
			}

			if isSL {
				pos.Status = SL_HIT
				pos.ClosedAt = currentTick
				pos.TimeToSL = currentTick.Sub(pos.CreatedAt).String()
				pos.OutcomeReason = "Stop Loss hit during candle evaluation"
				pos.PnlPercentage = -100.0 * (math.Abs(pos.EntryPrice-pos.StopLoss) / pos.EntryPrice)
				continue
			}

			// Check TP1
			if pos.Status == MONITORING {
				isTP1 := false
				if pos.Direction == LONG && latestCandle.High >= pos.TP1 {
					isTP1 = true
				} else if pos.Direction == SHORT && latestCandle.Low <= pos.TP1 {
					isTP1 = true
				}

				if isTP1 {
					pos.Status = TP1_HIT
					pos.TimeToTP1 = currentTick.Sub(pos.CreatedAt).String()
					pos.OutcomeReason = "TP1 hit during candle evaluation"
					pos.PnlPercentage = 50.0 * (math.Abs(pos.TP1-pos.EntryPrice) / pos.EntryPrice)
				}
			}

			// Check TP2
			if pos.Status == TP1_HIT {
				isTP2 := false
				if pos.Direction == LONG && latestCandle.High >= pos.TP2 {
					isTP2 = true
				} else if pos.Direction == SHORT && latestCandle.Low <= pos.TP2 {
					isTP2 = true
				}

				if isTP2 {
					pos.Status = TP2_HIT
					pos.ClosedAt = currentTick
					pos.TimeToTP2 = currentTick.Sub(pos.CreatedAt).String()
					pos.OutcomeReason = "TP2 hit during candle evaluation"
					pos.PnlPercentage = 100.0 * (math.Abs(pos.TP2-pos.EntryPrice) / pos.EntryPrice)
					continue
				}
			}

			// Check Expiration
			if currentTick.After(pos.ExpiresAt) || currentTick.Equal(pos.ExpiresAt) {
				pos.ClosedAt = currentTick
				if pos.Status == MONITORING {
					pos.Status = EXPIRED
					pos.OutcomeReason = "Simulated trade expired without hit"
					pos.PnlPercentage = 0.0
				} else if pos.Status == TP1_HIT {
					pos.OutcomeReason = "Simulated trade expired after TP1 (partial win)"
					// keep Pnl from TP1
				}
				continue
			}

			remainingPositions = append(remainingPositions, pos)
		}
		activePositions = remainingPositions

		// Replay simulated watches (to find Missed Opportunities)
		var remainingWatches []*SimulatedWatchLog
		for _, w := range activeWatches {
			// Check if filled
			isFilled := w.MissedOpportunityResult != "NOT_FILLED" && w.MissedOpportunityResult != ""
			if !isFilled {
				// Check if latest candle covers EntryPrice
				filled := false
				if w.Direction == LONG && latestCandle.Low <= w.EntryPrice {
					filled = true
				} else if w.Direction == SHORT && latestCandle.High >= w.EntryPrice {
					filled = true
				}

				if filled {
					w.MissedOpportunityResult = "FILLED"
				}
			}

			if w.MissedOpportunityResult == "FILLED" {
				// Trace outcome
				isSL := false
				if w.Direction == LONG && latestCandle.Low <= w.StopLoss {
					isSL = true
				} else if w.Direction == SHORT && latestCandle.High >= w.StopLoss {
					isSL = true
				}

				if isSL {
					w.MissedOpportunityResult = string(SL_HIT)
					continue
				}

				isTP := false
				if w.Direction == LONG && latestCandle.High >= w.TP1 {
					isTP = true
				} else if w.Direction == SHORT && latestCandle.Low <= w.TP1 {
					isTP = true
				}

				if isTP {
					w.MissedOpportunityResult = string(TP1_HIT)
					continue
				}

				if currentTick.After(w.ExpiresAt) || currentTick.Equal(w.ExpiresAt) {
					w.MissedOpportunityResult = string(EXPIRED)
					continue
				}
			} else {
				// Check if expired before getting filled
				if currentTick.After(w.ExpiresAt) || currentTick.Equal(w.ExpiresAt) {
					w.MissedOpportunityResult = "NOT_FILLED"
					continue
				}
			}

			remainingWatches = append(remainingWatches, w)
		}
		activeWatches = remainingWatches

		// 4. Run Scanner selection for this tick
		totalScansEvaluated++

		// Perform Universe filter
		candidates, _ := uc.universeUsecase.FilterUniverse(tickers, fundingRates, policy)
		if len(candidates) == 0 {
			currentTick = currentTick.Add(15 * time.Minute)
			continue
		}

		cand := candidates[0] // since tickers list contains only target symbol
		if cand.Symbol != req.Symbol {
			currentTick = currentTick.Add(15 * time.Minute)
			continue
		}

		// Staleness check on closed candles (prevents executing stale ticks)
		if !uc.stalenessUsecase.IsFresh(closedM15) {
			staleDetections++
			currentTick = currentTick.Add(15 * time.Minute)
			continue
		}

		// Populate snapshots
		fr := fundingRates[req.Symbol]
		tech, structure := PopulateSnapshots(closedM15, closedH1, closedH4, fr, latestPrice)
		prelimData := MarketData{
			Symbol:      req.Symbol,
			FundingRate: fr,
			M15Candles:  closedM15,
			H1Candles:   closedH1,
			H4Candles:   closedH4,
			LatestPrice: latestPrice,
		}

		// Playbook Selection
		selections := uc.strategySelectorUsecase.SelectPlaybooks(policy, cand, prelimData, tech, structure)
		var listCandidates []QuantResult
		for _, sel := range selections {
			// Filter by playbook name if specified
			if req.Playbook != "ALL" && sel.StrategyName != req.Playbook {
				continue
			}

			eligibilityRes := uc.playbookEligibilityUsecase.CheckEligibility(sel, policy, prelimData, tech, structure)
			if !eligibilityRes.Eligible {
				continue
			}

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

			quantResult := uc.playbookQuantEngineUsecase.RunEngine(playbook, sel.Direction, prelimData, policy)
			quantResult.Tier = cand.Tier
			quantResult.RawKlines = closedM15

			reconciliationDir := uc.conflictResolverUsecase.Resolve(quantResult.Direction, "NEUTRAL")
			_ = uc.scoringUsecase.Calculate(&quantResult, reconciliationDir, policy)

			listCandidates = append(listCandidates, quantResult)
		}

		if len(listCandidates) == 0 {
			currentTick = currentTick.Add(15 * time.Minute)
			continue
		}

		// Arbitrate
		selectedCandidates, _ := uc.candidateArbiterUsecase.Arbitrate(listCandidates, policy)
		if len(selectedCandidates) == 0 {
			currentTick = currentTick.Add(15 * time.Minute)
			continue
		}

		// Evaluate Local Gate
		var localCandidates []QuantResult
		localGateMap := make(map[string]LocalGateResult)
		for _, qResult := range selectedCandidates {
			lgRes := uc.localGateUsecase.Evaluate(qResult, policy, closedM15)
			localGateMap[qResult.Symbol] = lgRes
			if lgRes.Passed {
				localCandidates = append(localCandidates, qResult)
			}
		}

		if len(localCandidates) == 0 {
			currentTick = currentTick.Add(15 * time.Minute)
			continue
		}

		// Select AI candidates (concurrency constraints)
		aiCandidates, _ := uc.aiCandidateSelectorUsecase.SelectCandidates(localCandidates, policy)

		// 5. Simulated AI Auditor (Mocked or Rule-based)
		aiAuditsMap := make(map[string]dto.AIAuditResponse)
		for _, qr := range aiCandidates {
			var resp dto.AIAuditResponse
			if req.AIMode == "RULE_BASED" {
				if qr.Score >= policy.MinScoreExecute {
					resp = dto.AIAuditResponse{
						Symbol:     qr.Symbol,
						IsApproved: true,
						Decision:   "CONFIRM",
						Sentiment:  "BULLISH",
						Reasoning:  "Rule-based approval: Score is higher than execution limit",
					}
				} else {
					resp = dto.AIAuditResponse{
						Symbol:     qr.Symbol,
						IsApproved: false,
						Decision:   "REJECT",
						Sentiment:  "NEUTRAL",
						Reasoning:  "Rule-based rejection: Score too low",
					}
				}
			} else { // "MOCK" or fallback
				resp = dto.AIAuditResponse{
					Symbol:     qr.Symbol,
					IsApproved: true,
					Decision:   "CONFIRM",
					Sentiment:  "BULLISH",
					Reasoning:  "Mock auditor approved",
				}
			}
			aiAuditsMap[qr.Symbol] = resp
		}

		// Staleness check, Plan Reconciliation, and Final Gate Evaluation
		var decisions []FinalDecision
		ctxMap := make(map[string]dto.AIAuditResponse)

		// Map active positions to journal format for active monitoring check in final gate
		var simulatedJournal []SignalJournal
		for _, p := range activePositions {
			simulatedJournal = append(simulatedJournal, SignalJournal{
				Symbol: p.Symbol,
				Status: p.Status,
			})
		}

		for _, qResult := range selectedCandidates {
			lgRes := localGateMap[qResult.Symbol]
			var auditResponse dto.AIAuditResponse
			if !lgRes.Passed {
				decision := "REJECT"
				if lgRes.Status == LOCAL_WATCH {
					decision = "WAIT"
				}
				auditResponse = dto.AIAuditResponse{
					Symbol:     qResult.Symbol,
					IsApproved: false,
					Decision:   decision,
					Reasoning:  "Local gate failed: " + lgRes.Reason,
				}
			} else {
				resp, ok := aiAuditsMap[qResult.Symbol]
				if ok {
					auditResponse = resp
				} else {
					auditResponse = dto.AIAuditResponse{
						Symbol:     qResult.Symbol,
						IsApproved: false,
						Decision:   "WAIT",
						Reasoning:  "AI_SKIPPED",
					}
				}
			}

			planReview := uc.planReconciliationUsecase.Reconcile(qResult, auditResponse)
			stalenessRes := uc.stalenessUsecase.Evaluate(qResult, planReview, policy, latestPrice)

			finalDecision := uc.finalGateUsecase.Evaluate(
				qResult,
				lgRes,
				auditResponse,
				planReview,
				stalenessRes,
				policy,
				latestPrice,
				simulatedJournal,
				mockHistory,
				closedM15,
			)

			ctxMap[qResult.Symbol] = auditResponse
			decisions = append(decisions, finalDecision)
		}

		// 6. Conflict Resolver (using mockHistory of simulated executed signals)
		resolvedDecisions, updatedHistory := uc.conflictResolverUsecase.ResolveConflicts(decisions, mockHistory, simulatedJournal, policy)
		mockHistory = updatedHistory

		// Map decisions to record executables and watches
		for _, fd := range resolvedDecisions {
			if fd.Status == FINAL_EXECUTE {
				tp2 := fd.TakeProfit
				tp1 := fd.EntryPrice + (tp2-fd.EntryPrice)*0.5
				if fd.Direction == SHORT {
					tp1 = fd.EntryPrice - (fd.EntryPrice-tp2)*0.5
				}

				posLog := SimulatedPositionLog{
					ID:                      fmt.Sprintf("%s_%s_%s", currentTick.Format("20060102150405"), fd.Symbol, fd.Playbook),
					Symbol:                  fd.Symbol,
					Direction:               fd.Direction,
					Playbook:                fd.Playbook,
					EntryPrice:              fd.EntryPrice,
					StopLoss:                fd.StopLoss,
					TP1:                     tp1,
					TP2:                     tp2,
					RR:                      fd.RR,
					QuantScore:              fd.Score,
					AIConfidence:            fd.AIConfidence,
					MarketRegime:            policy.Reason,
					CreatedAt:               currentTick,
					ExpiresAt:               currentTick.Add(120 * time.Minute),
					Status:                  MONITORING,
					MFE:                     0.0,
					MAE:                     0.0,
					Grade:                   getGrade(fd.Score),
					ThresholdProfileSummary: fd.ThresholdProfileSummary,
				}

				positions = append(positions, posLog)
				// reference the appended item
				activePositions = append(activePositions, &positions[len(positions)-1])

			} else if fd.Status == FINAL_WATCH || fd.Status == FINAL_REJECT {
				tp2 := fd.TakeProfit
				tp1 := fd.EntryPrice + (tp2-fd.EntryPrice)*0.5
				if fd.Direction == SHORT {
					tp1 = fd.EntryPrice - (fd.EntryPrice-tp2)*0.5
				}

				watchLog := SimulatedWatchLog{
					Symbol:                  fd.Symbol,
					Direction:               fd.Direction,
					Playbook:                fd.Playbook,
					EntryPrice:              fd.EntryPrice,
					StopLoss:                fd.StopLoss,
					TP1:                     tp1,
					TP2:                     tp2,
					Status:                  fd.Status,
					Reason:                  fd.Reason,
					CreatedAt:               currentTick,
					ExpiresAt:               currentTick.Add(120 * time.Minute),
					MissedOpportunityResult: "NOT_FILLED",
				}

				watches = append(watches, watchLog)
				activeWatches = append(activeWatches, &watches[len(watches)-1])
			}
		}

		currentTick = currentTick.Add(15 * time.Minute)
	}

	// 7. Process remaining active positions that are still in MONITORING/TP1_HIT status at EndTime.
	// Force close them or evaluate using the remaining candles in the data history.
	for _, pos := range activePositions {
		if pos.Status == MONITORING {
			pos.Status = EXPIRED
			pos.OutcomeReason = "Force expired at end of backtest window"
			pos.PnlPercentage = 0.0
		} else if pos.Status == TP1_HIT {
			pos.OutcomeReason = "Force expired at end of backtest window with TP1 partial win"
		}
		pos.ClosedAt = req.EndTime
	}

	for _, w := range activeWatches {
		if w.MissedOpportunityResult == "FILLED" || w.MissedOpportunityResult == "" {
			w.MissedOpportunityResult = "NOT_FILLED"
		}
	}

	// 8. Compile and Aggregate Metrics
	var summary BacktestReportSummary
	summary.RunID = fmt.Sprintf("backtest_%s_%s", req.Symbol, time.Now().Format("20060102150405"))
	summary.ConfigVersion = GetGlobalConfigRegistry().GetVersion()
	summary.GeneratedAt = time.Now()
	summary.Symbol = req.Symbol
	summary.StartTime = req.StartTime
	summary.EndTime = req.EndTime
	summary.PlaybookFilter = req.Playbook
	summary.RegimeMode = req.Regime
	summary.AIMode = req.AIMode

	summary.TotalSetups = len(positions) + len(watches)
	summary.TotalFinalExecuteSimulated = len(positions)

	winsCount := 0
	lossesCount := 0
	sumPnl := 0.0
	sumMFE := 0.0
	sumMAE := 0.0

	playbookStats := make(map[string]PlaybookPerformanceStats)
	regimeStats := make(map[string]RegimePerformanceStats)

	getOrInitPlaybook := func(name string) PlaybookPerformanceStats {
		if _, ok := playbookStats[name]; !ok {
			playbookStats[name] = PlaybookPerformanceStats{}
		}
		return playbookStats[name]
	}

	getOrInitRegime := func(name string) RegimePerformanceStats {
		if _, ok := regimeStats[name]; !ok {
			regimeStats[name] = RegimePerformanceStats{}
		}
		return regimeStats[name]
	}

	for _, pos := range positions {
		isWin := pos.Status == TP1_HIT || pos.Status == TP2_HIT
		isLoss := pos.Status == SL_HIT
		isExpired := pos.Status == EXPIRED

		if isWin {
			winsCount++
			summary.TP1Hits++
			if pos.Status == TP2_HIT {
				summary.TP2Hits++
			}
		} else if isLoss {
			lossesCount++
			summary.SLHits++
		} else if isExpired {
			summary.ExpiredHits++
		}

		sumPnl += pos.PnlPercentage
		sumMFE += pos.MFE
		sumMAE += pos.MAE

		// Update Playbook Breakdown
		pbKey := string(pos.Playbook)
		pStats := getOrInitPlaybook(pbKey)
		pStats.Total++
		pStats.Executed++
		if isWin {
			pStats.Wins++
		} else if isLoss {
			pStats.Losses++
			pStats.FalsePositives++
		} else if isExpired {
			pStats.Expired++
		}
		pStats.AvgMFE += pos.MFE
		pStats.AvgMAE += pos.MAE
		pStats.Expectancy += pos.PnlPercentage
		playbookStats[pbKey] = pStats

		// Update Regime Breakdown
		rgKey := pos.MarketRegime
		rStats := getOrInitRegime(rgKey)
		rStats.Total++
		rStats.Executed++
		if isWin {
			rStats.Wins++
		} else if isLoss {
			rStats.Losses++
		} else if isExpired {
			rStats.Expired++
		}
		rStats.Expectancy += pos.PnlPercentage
		regimeStats[rgKey] = rStats
	}

	// Add watches to totals
	for _, w := range watches {
		pbKey := string(w.Playbook)
		pStats := getOrInitPlaybook(pbKey)
		pStats.Total++
		playbookStats[pbKey] = pStats

		rgKey := "UNKNOWN"
		for _, pos := range positions {
			if pos.CreatedAt.Equal(w.CreatedAt) {
				rgKey = pos.MarketRegime
				break
			}
		}
		if rgKey != "UNKNOWN" {
			rStats := getOrInitRegime(rgKey)
			rStats.Total++
			regimeStats[rgKey] = rStats
		}
	}

	// Calculate Averages
	if summary.TotalFinalExecuteSimulated > 0 {
		summary.Winrate = (float64(winsCount) / float64(summary.TotalFinalExecuteSimulated)) * 100.0
		summary.Expectancy = sumPnl / float64(summary.TotalFinalExecuteSimulated)
		summary.AvgMFE = sumMFE / float64(summary.TotalFinalExecuteSimulated)
		summary.AvgMAE = sumMAE / float64(summary.TotalFinalExecuteSimulated)
	}

	if totalScansEvaluated > 0 {
		summary.StaleRate = (float64(staleDetections) / float64(totalScansEvaluated)) * 100.0
	}

	// Missed opportunities count
	missedCount := 0
	for _, w := range watches {
		if w.MissedOpportunityResult == string(TP1_HIT) || w.MissedOpportunityResult == string(TP2_HIT) {
			missedCount++
		}
	}
	summary.MissedOpportunityCount = missedCount

	// Normalize playbook & regime averages
	for k, v := range playbookStats {
		if v.Executed > 0 {
			v.Winrate = (float64(v.Wins) / float64(v.Executed)) * 100.0
			v.Expectancy = v.Expectancy / float64(v.Executed)
			v.AvgMFE = v.AvgMFE / float64(v.Executed)
			v.AvgMAE = v.AvgMAE / float64(v.Executed)
		}
		playbookStats[k] = v
	}

	for k, v := range regimeStats {
		if v.Executed > 0 {
			v.Winrate = (float64(v.Wins) / float64(v.Executed)) * 100.0
			v.Expectancy = v.Expectancy / float64(v.Executed)
		}
		regimeStats[k] = v
	}

	report := &BacktestReport{
		BacktestReportSummary: summary,
		PlaybookPerformance:   playbookStats,
		RegimePerformance:     regimeStats,
		Positions:             positions,
		Watches:               watches,
	}

	// 9. Atomic Storage Writes
	if err := uc.saveReportToFilesystem(report); err != nil {
		slog.Error("Failed to save backtest report atomically", "error", err)
	}

	return report, nil
}

func (uc *BacktestEngineUsecase) saveReportToFilesystem(report *BacktestReport) error {
	runsDir := filepath.Join(uc.storageDir, "backtest_runs")
	if err := os.MkdirAll(runsDir, 0755); err != nil {
		return err
	}

	// Save detailed run report atomically
	runFile := filepath.Join(runsDir, fmt.Sprintf("backtest_%s.json", report.RunID))
	tmpFile := runFile + ".tmp"

	bytes, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(tmpFile, bytes, 0644); err != nil {
		return err
	}

	if err := os.Rename(tmpFile, runFile); err != nil {
		_ = os.Remove(tmpFile)
		return err
	}

	// Save summary to backtest_report.json list
	summaryFile := filepath.Join(uc.storageDir, "backtest_report.json")
	var reports []BacktestReportSummary

	summaryData, err := os.ReadFile(summaryFile)
	if err == nil && len(summaryData) > 0 {
		_ = json.Unmarshal(summaryData, &reports)
	}

	reports = append(reports, report.BacktestReportSummary)
	summaryBytes, err := json.MarshalIndent(reports, "", "  ")
	if err != nil {
		return err
	}

	summaryTmp := summaryFile + ".tmp"
	if err := os.WriteFile(summaryTmp, summaryBytes, 0644); err != nil {
		return err
	}

	if err := os.Rename(summaryTmp, summaryFile); err != nil {
		_ = os.Remove(summaryTmp)
		return err
	}

	return nil
}

func sortCandles(candles []dto.Candle) {
	sort.Slice(candles, func(i, j int) bool {
		return candles[i].Time.Before(candles[j].Time)
	})
}
