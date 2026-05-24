package usecase

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"cpbro-engine/internal/modules/cryptobroV3/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Define mock dependencies for compliance tests
type complianceStorageRepo struct {
	latestResult *entity.LatestResult
	history      *entity.SignalHistory
	journal      []SignalJournal
	evalReport   *EvaluationReport
	audits       []DecisionAudit
}

func (m *complianceStorageRepo) LoadLatestResult() (*entity.LatestResult, error) {
	if m.latestResult == nil {
		return &entity.LatestResult{}, nil
	}
	return m.latestResult, nil
}
func (m *complianceStorageRepo) SaveLatestResult(res *entity.LatestResult) error {
	m.latestResult = res
	return nil
}
func (m *complianceStorageRepo) LoadSignalHistory() (*entity.SignalHistory, error) {
	return m.history, nil
}
func (m *complianceStorageRepo) SaveSignalHistory(hist *entity.SignalHistory) error {
	m.history = hist
	return nil
}
func (m *complianceStorageRepo) LoadSignalJournal() ([]SignalJournal, error) {
	return m.journal, nil
}
func (m *complianceStorageRepo) SaveSignalJournal(j []SignalJournal) error {
	m.journal = j
	return nil
}
func (m *complianceStorageRepo) AppendSignalJournal(entry SignalJournal) error {
	m.journal = append(m.journal, entry)
	return nil
}
func (m *complianceStorageRepo) LoadAIAuditCache() (*entity.AIAuditCache, error) {
	return &entity.AIAuditCache{CacheMap: make(map[string]entity.CachedAudit)}, nil
}
func (m *complianceStorageRepo) SaveAIAuditCache(c *entity.AIAuditCache) error {
	return nil
}
func (m *complianceStorageRepo) LoadEvaluationReport() (*EvaluationReport, error) {
	if m.evalReport == nil {
		return &EvaluationReport{GeneratedAt: time.Time{}}, nil
	}
	return m.evalReport, nil
}
func (m *complianceStorageRepo) SaveEvaluationReport(r *EvaluationReport) error {
	m.evalReport = r
	return nil
}
func (m *complianceStorageRepo) LoadDecisionAudits() ([]DecisionAudit, error) {
	return m.audits, nil
}
func (m *complianceStorageRepo) SaveDecisionAudits(a []DecisionAudit) error {
	m.audits = a
	return nil
}
func (m *complianceStorageRepo) AppendDecisionAudit(entry DecisionAudit) error {
	m.audits = append(m.audits, entry)
	if len(m.audits) > 1000 {
		m.audits = m.audits[len(m.audits)-1000:]
	}
	return nil
}

type complianceMarketData struct {
	price   float64
	candles []dto.Candle
	tickers []dto.Ticker24h
	rates   map[string]float64
}

func (m *complianceMarketData) FetchClosedCandles(ctx context.Context, symbol string, interval string, limit int) ([]dto.Candle, error) {
	return m.candles, nil
}
func (m *complianceMarketData) FetchLatestPrice(ctx context.Context, symbol string) (float64, error) {
	return m.price, nil
}
func (m *complianceMarketData) FetchAllFuturesTickers24h(ctx context.Context) ([]dto.Ticker24h, error) {
	return m.tickers, nil
}
func (m *complianceMarketData) FetchPremiumFundingRates(ctx context.Context) (map[string]float64, error) {
	return m.rates, nil
}
func (m *complianceMarketData) FetchOpenInterest(ctx context.Context, symbol string) (float64, error) {
	return 1000.0, nil
}
func (m *complianceMarketData) FetchHistoricalCandles(ctx context.Context, symbol string, interval string, startTime time.Time, endTime time.Time) ([]dto.Candle, error) {
	return m.candles, nil
}

type complianceAIAuditor struct {
	response *dto.AIAuditResponse
}

func (m *complianceAIAuditor) AuditCandidate(ctx context.Context, req dto.AIAuditRequest) (*dto.AIAuditResponse, error) {
	if m.response != nil {
		return m.response, nil
	}
	return &dto.AIAuditResponse{Symbol: req.Symbol, IsApproved: true, Decision: "CONFIRM", Reasoning: "Mock AI"}, nil
}

type complianceNotification struct {
	calledTimes int
	lastMsg     string
}

func (m *complianceNotification) SendSignalMessage(ctx context.Context, msg string) error {
	m.calledTimes++
	m.lastMsg = msg
	return nil
}

func (m *complianceNotification) SendOpsMessage(ctx context.Context, msg string) error {
	return nil
}

// 1. TEST MARKET POLICY
func TestMarketPolicyCompliance(t *testing.T) {
	uc := NewMarketPolicyUsecase()

	t.Run("BTCChaos blocks Tier C and sets MaxAICandidates", func(t *testing.T) {
		policy := uc.EvaluatePolicy(context.Background(), "BULLISH", 90.0, 0.0, 0.9, "HIGH", 0.5)
		assert.Contains(t, policy.Reason, "BTC_CHAOS")
		assert.Equal(t, 1, policy.MaxAICandidates)
		assert.Equal(t, 1, policy.MaxFinalExecute)
		assert.Equal(t, 8.2, policy.MinScoreExecute)

		// Tier C should be blocked
		allowed, reason := uc.IsAllowed("SOLUSDT", policy, 15000000.0, 0.001, 0.05, TierC)
		assert.False(t, allowed)
		assert.Contains(t, reason, "tier C not allowed")
	})

	t.Run("RiskOff increases MinScoreExecute", func(t *testing.T) {
		policy := uc.EvaluatePolicy(context.Background(), "BEARISH", 40.0, 0.0, 0.2, "NORMAL", 0.2)
		assert.Contains(t, policy.Reason, "RISK_OFF")
		assert.Equal(t, 7.4, policy.MinScoreExecute)
	})

	t.Run("AltSupportive makes ShortMode SWEEP_ONLY", func(t *testing.T) {
		policy := uc.EvaluatePolicy(context.Background(), "BULLISH", 70.0, 0.03, 0.1, "NORMAL", 0.6)
		assert.Contains(t, policy.Reason, "ALT_SUPPORTIVE")
		assert.Equal(t, SWEEP_ONLY, policy.ShortMode)
	})

	t.Run("ChopRange makes LongMode/ShortMode REVERSAL_ONLY", func(t *testing.T) {
		policy := uc.EvaluatePolicy(context.Background(), "SIDEWAYS", 50.0, 0.0, 0.1, "NORMAL", 0.5)
		assert.Contains(t, policy.Reason, "CHOP_RANGE")
		assert.Equal(t, REVERSAL_ONLY, policy.LongMode)
		assert.Equal(t, REVERSAL_ONLY, policy.ShortMode)
	})
}

// 2. TEST PLAYBOOK THRESHOLD PROFILE
func TestPlaybookThresholdProfileCompliance(t *testing.T) {
	policy := MarketPolicy{MinScoreAI: 6.0, MinScoreExecute: 7.0, MinRRExecute: 1.5}

	t.Run("TREND_PULLBACK require adx rules", func(t *testing.T) {
		profile := GetPlaybookThresholdProfile(TREND_PULLBACK, policy, TierA)
		assert.True(t, profile.RequireADX)
		assert.Equal(t, 20.0, profile.MinADX)
	})

	t.Run("LIQUIDITY_SWEEP_REVERSAL overrides require adx and needs volume confirm", func(t *testing.T) {
		profile := GetPlaybookThresholdProfile(LIQUIDITY_SWEEP_REVERSAL, policy, TierA)
		assert.False(t, profile.RequireADX)
		assert.True(t, profile.RequireVolumeConfirm)
	})

	t.Run("COMPRESSION_BREAKOUT_RETEST requires retest and blocks direct entry", func(t *testing.T) {
		profile := GetPlaybookThresholdProfile(COMPRESSION_BREAKOUT_RETEST, policy, TierA)
		assert.True(t, profile.RequireRetest)
		assert.False(t, profile.AllowBreakoutCandleEntry)
	})

	t.Run("RANGE_EDGE_REVERSAL rejects ADX expansion", func(t *testing.T) {
		profile := GetPlaybookThresholdProfile(RANGE_EDGE_REVERSAL, policy, TierA)
		assert.True(t, profile.RejectADXExpansion)
	})

	t.Run("CROWDED_POSITIONING_SQUEEZE requires crowding evidence and AI high", func(t *testing.T) {
		profile := GetPlaybookThresholdProfile(CROWDED_POSITIONING_SQUEEZE, policy, TierA)
		assert.True(t, profile.RequireCrowdingEvidence)
		assert.True(t, profile.RequireAIHigh)
	})

	t.Run("Stricter policy overrides profile and vice-versa", func(t *testing.T) {
		// Stricter policy
		strictPolicy := MarketPolicy{MinScoreExecute: 8.5}
		profile := GetPlaybookThresholdProfile(TREND_PULLBACK, strictPolicy, TierA)
		assert.Equal(t, 8.5, profile.MinScoreExecute)

		// Stricter profile
		loosePolicy := MarketPolicy{MinScoreExecute: 5.0}
		profile2 := GetPlaybookThresholdProfile(TREND_PULLBACK, loosePolicy, TierA)
		assert.Equal(t, 7.3, profile2.MinScoreExecute)
	})
}

// 3. TEST CANDIDATE ARBITER
func TestCandidateArbiterCompliance(t *testing.T) {
	uc := NewCandidateArbiterUsecase()
	policy := MarketPolicy{
		AllowLong:        true,
		AllowShort:       true,
		LongMode:         NORMAL,
		ShortMode:        NORMAL,
		AllowedTiers:     []Tier{TierA, TierB, TierC},
		AllowedPlaybooks: []Playbook{TREND_PULLBACK, LIQUIDITY_SWEEP_REVERSAL, CROWDED_POSITIONING_SQUEEZE},
	}

	t.Run("Symbol same direction choose highest score", func(t *testing.T) {
		candidates := []QuantResult{
			{Symbol: "SOLUSDT", Direction: LONG, Playbook: TREND_PULLBACK, Score: 7.5, Tier: TierA},
			{Symbol: "SOLUSDT", Direction: LONG, Playbook: LIQUIDITY_SWEEP_REVERSAL, Score: 8.2, Tier: TierA},
		}
		selected, rejected := uc.Arbitrate(candidates, policy)
		require.Len(t, selected, 1)
		assert.Equal(t, LIQUIDITY_SWEEP_REVERSAL, selected[0].Playbook)
		assert.Len(t, rejected, 1)
	})

	t.Run("Symbol same direction conflicts reject if gap < 0.7", func(t *testing.T) {
		candidates := []QuantResult{
			{Symbol: "SOLUSDT", Direction: LONG, Playbook: TREND_PULLBACK, Score: 7.5, Tier: TierA},
			{Symbol: "SOLUSDT", Direction: SHORT, Playbook: LIQUIDITY_SWEEP_REVERSAL, Score: 7.8, Tier: TierA},
		}
		selected, rejected := uc.Arbitrate(candidates, policy)
		assert.Empty(t, selected)
		assert.Len(t, rejected, 2)
	})

	t.Run("Symbol same direction conflicts allow if gap >= 0.7", func(t *testing.T) {
		candidates := []QuantResult{
			{Symbol: "SOLUSDT", Direction: LONG, Playbook: TREND_PULLBACK, Score: 8.5, Tier: TierA},
			{Symbol: "SOLUSDT", Direction: SHORT, Playbook: LIQUIDITY_SWEEP_REVERSAL, Score: 7.2, Tier: TierA},
		}
		selected, rejected := uc.Arbitrate(candidates, policy)
		require.Len(t, selected, 1)
		assert.Equal(t, LONG, selected[0].Direction)
		assert.Len(t, rejected, 1)
	})

	t.Run("BTCChaos mode blocks all opposing except S+ premium", func(t *testing.T) {
		chaosPolicy := policy
		chaosPolicy.Reason = "BTCChaos active"
		candidates := []QuantResult{
			{Symbol: "SOLUSDT", Direction: LONG, Playbook: LIQUIDITY_SWEEP_REVERSAL, Score: 8.6, Tier: TierA},
			{Symbol: "SOLUSDT", Direction: SHORT, Playbook: CROWDED_POSITIONING_SQUEEZE, Score: 8.9, Tier: TierA},
		}
		selected, rejected := uc.Arbitrate(candidates, chaosPolicy)
		assert.Empty(t, selected) // both S+ opposing during chaos get rejected
		assert.Len(t, rejected, 2)
	})
}

// 4. TEST LOCAL GATE DENGAN PROFILE
func TestLocalGateCompliance(t *testing.T) {
	uc := NewLocalGateUsecase()
	policy := MarketPolicy{AllowLong: true, AllowShort: false}

	t.Run("Short rejected when AllowShort is false", func(t *testing.T) {
		cand := QuantResult{
			Symbol:    "SOLUSDT",
			Direction: SHORT,
			Playbook:  TREND_PULLBACK,
			Score:     7.5,
			TechnicalSnapshot: TechnicalSnapshot{
				IndicatorValues: map[string]float64{
					"adx": 25.0,
				},
			},
		}
		res := uc.Evaluate(cand, policy, []dto.Candle{})
		assert.False(t, res.Passed)
		assert.Contains(t, res.Reason, "SHORT trades disallowed")
	})

	t.Run("Trend Pullback low ADX rejected", func(t *testing.T) {
		cand := QuantResult{
			Symbol:       "SOLUSDT",
			Direction:    LONG,
			Playbook:     TREND_PULLBACK,
			Score:        7.5,
			TriggerPrice: 100.0,
			Tier:         TierA,
			TradePlan: TradePlan{
				EntryPrice: 100.0,
				TakeProfit: 120.0,
				StopLoss:   90.0,
			},
			TechnicalSnapshot: TechnicalSnapshot{
				IndicatorValues: map[string]float64{
					"adx": 15.0,
				},
			},
		}
		policy := MarketPolicy{
			AllowLong:        true,
			AllowedPlaybooks: []Playbook{TREND_PULLBACK},
			AllowedTiers:     []Tier{TierA},
		}
		res := uc.Evaluate(cand, policy, []dto.Candle{})
		assert.False(t, res.Passed)
		assert.Contains(t, res.Reason, "below execution threshold")
	})

	t.Run("Liquidity Sweep requires volume confirm", func(t *testing.T) {
		candNoVol := QuantResult{
			Symbol:       "SOLUSDT",
			Direction:    LONG,
			Playbook:     LIQUIDITY_SWEEP_REVERSAL,
			Score:        7.5,
			TriggerPrice: 100.0,
			Tier:         TierA,
			TradePlan: TradePlan{
				EntryPrice: 100.0,
				TakeProfit: 120.0,
				StopLoss:   90.0,
			},
			TechnicalSnapshot: TechnicalSnapshot{
				IndicatorValues: map[string]float64{
					"adx":          10.0,
					"volume_ratio": 1.1,
				},
			},
		}
		policy := MarketPolicy{
			AllowLong:        true,
			AllowedPlaybooks: []Playbook{LIQUIDITY_SWEEP_REVERSAL},
			AllowedTiers:     []Tier{TierA},
		}
		res := uc.Evaluate(candNoVol, policy, []dto.Candle{})
		assert.False(t, res.Passed)
		assert.Contains(t, res.Reason, "volume")
	})
}

// 5. TEST STALENESS DENGAN PROFILE
func TestStalenessCompliance(t *testing.T) {
	uc := NewStalenessUsecase(30 * time.Minute)
	policy := MarketPolicy{StalenessATRMultiplier: 1.5}

	t.Run("Fresh, Late, and Missed categorization", func(t *testing.T) {
		cand := QuantResult{
			Symbol:    "SOLUSDT",
			Direction: LONG,
			Playbook:  TREND_PULLBACK,
			Tier:      TierA,
			TechnicalSnapshot: TechnicalSnapshot{
				IndicatorValues: map[string]float64{
					"ATR": 1.0,
				},
			},
			TradePlan: TradePlan{
				EntryPrice: 100.0,
			},
		}

		// Fresh (distance = 0.1 <= threshold 0.5)
		res := uc.Evaluate(cand, PlanReview{}, policy, 100.1)
		assert.Equal(t, FRESH, res.Status)

		// Missed (distance = 2.5 > threshold*1.5)
		res2 := uc.Evaluate(cand, PlanReview{}, policy, 102.5)
		assert.Equal(t, MISSED, res2.Status)
	})
}

// 6. TEST FINAL GATE DENGAN PROFILE
func TestFinalGateCompliance(t *testing.T) {
	uc := NewFinalGateUsecase()
	policy := MarketPolicy{MinScoreExecute: 7.0}

	t.Run("AI level and staleness rejections", func(t *testing.T) {
		cand := QuantResult{
			Symbol:    "SOLUSDT",
			Direction: LONG,
			Playbook:  TREND_PULLBACK,
			Score:     7.5,
			TradePlan: TradePlan{EntryPrice: 100.0, TakeProfit: 120.0, StopLoss: 90.0},
		}

		localGate := LocalGateResult{Passed: true, Status: AI_CANDIDATE}
		aiAuditLow := dto.AIAuditResponse{Decision: "REJECT", Reasoning: "AI Sentiment is LOW"}
		planReview := PlanReview{}
		stalenessFresh := StalenessResult{IsStale: false, Status: FRESH}

		// AI low should reject
		dec := uc.Evaluate(cand, localGate, aiAuditLow, planReview, stalenessFresh, policy, 100.0, nil, nil, nil)
		assert.Equal(t, FINAL_REJECT, dec.Status)

		// Staleness Missed should reject
		aiAuditConfirm := dto.AIAuditResponse{Decision: "CONFIRM"}
		stalenessMissed := StalenessResult{IsStale: true, Status: MISSED}
		dec2 := uc.Evaluate(cand, localGate, aiAuditConfirm, planReview, stalenessMissed, policy, 100.0, nil, nil, nil)
		assert.Equal(t, FINAL_REJECT, dec2.Status)
	})
}

// 7. TEST CONFLICT RESOLVER
func TestConflictResolverCompliance(t *testing.T) {
	uc := NewConflictResolverUsecase()

	t.Run("Mutual exclusion of long and short on same symbol", func(t *testing.T) {
		candidates := []FinalDecision{
			{Symbol: "SOLUSDT", Direction: LONG, Playbook: TREND_PULLBACK, Status: FINAL_EXECUTE, IsExecutable: true},
			{Symbol: "SOLUSDT", Direction: SHORT, Playbook: LIQUIDITY_SWEEP_REVERSAL, Status: FINAL_EXECUTE, IsExecutable: true},
		}

		resolved, _ := uc.ResolveConflicts(candidates, []dto.SignalResponse{}, []SignalJournal{}, MarketPolicy{MaxFinalExecute: 5, CooldownMinutes: 15})
		// Long and Short should conflict, causing downgrade to watch/reject
		for _, d := range resolved {
			assert.NotEqual(t, FINAL_EXECUTE, d.Status)
		}
	})

	t.Run("FINAL_WATCH cannot be upgraded to FINAL_EXECUTE", func(t *testing.T) {
		candidates := []FinalDecision{
			{Symbol: "SOLUSDT", Direction: LONG, Playbook: TREND_PULLBACK, Status: FINAL_WATCH, IsExecutable: false, Score: 9.5},
		}

		resolved, _ := uc.ResolveConflicts(candidates, []dto.SignalResponse{}, []SignalJournal{}, MarketPolicy{MaxFinalExecute: 5, CooldownMinutes: 15})
		require.Len(t, resolved, 1)
		assert.Equal(t, FINAL_WATCH, resolved[0].Status)
		assert.False(t, resolved[0].IsExecutable)
	})
}

// 8. TEST NOTIFICATION
func TestNotificationCompliance(t *testing.T) {
	t.Run("SendV3Signals only transmits FINAL_EXECUTE with HIGH+FRESH", func(t *testing.T) {
		tgMock2 := &complianceNotification{}
		uc2 := NewSignalNotificationUsecase(tgMock2, nil)

		reqs := []SignalNotificationRequest{
			{
				Decision: FinalDecision{
					Symbol:          "ETHUSDT",
					Status:          FINAL_WATCH,
					IsExecutable:    false,
					AIConfidence:    "HIGH",
					StalenessStatus: "FRESH",
					EntryPrice:      3000,
					StopLoss:        2900,
					TakeProfit:      3200,
					Reason:          "Valid setup",
				},
			},
			{
				Decision: FinalDecision{
					Symbol:          "SOLUSDT",
					Status:          FINAL_EXECUTE,
					IsExecutable:    true,
					AIConfidence:    "MEDIUM", // Should be filtered (requires HIGH)
					StalenessStatus: "FRESH",
					EntryPrice:      100,
					StopLoss:        90,
					TakeProfit:      120,
					Reason:          "Valid setup",
				},
			},
			{
				Decision: FinalDecision{
					Symbol:          "NEARUSDT",
					Status:          FINAL_EXECUTE,
					IsExecutable:    true,
					AIConfidence:    "HIGH",
					StalenessStatus: "LATE", // Should be filtered (requires FRESH)
					EntryPrice:      5,
					StopLoss:        4.5,
					TakeProfit:      6.0,
					Reason:          "Valid setup",
				},
			},
			{
				Decision: FinalDecision{
					Symbol:          "BTCUSDT",
					Status:          FINAL_EXECUTE,
					IsExecutable:    true,
					AIConfidence:    "HIGH",
					StalenessStatus: "FRESH",
					EntryPrice:      50000,
					StopLoss:        48000,
					TakeProfit:      55000,
					Reason:          "Valid setup",
				},
				AuditResponse: dto.AIAuditResponse{
					Symbol:    "BTCUSDT",
					Sentiment: "BULLISH",
					Reason:    "Matched",
				},
			},
		}

		uc2.SendV3Signals(context.Background(), reqs, MarketPolicy{Reason: "NORMAL"}, ScannerSummaryV3{ActiveRegime: "NORMAL"})
		// Only BTCUSDT (1 message) should have been sent to Telegram message channel
		assert.Equal(t, 1, tgMock2.calledTimes)
	})
}

// 9. TEST MONITORING
func TestMonitoringCompliance(t *testing.T) {
	mockRepo := &complianceStorageRepo{
		journal: []SignalJournal{
			{
				Symbol:     "SOLUSDT",
				Playbook:   TREND_PULLBACK,
				Direction:  LONG,
				Status:     MONITORING,
				EntryPrice: 100.0,
				StopLoss:   90.0,
				TP1:        110.0,
				TP2:        120.0,
				MFE:        0.0,
				MAE:        0.0,
				CreatedAt:  time.Now().Add(-5 * time.Minute),
			},
		},
	}
	mockData := &complianceMarketData{price: 115.0} // hit TP1
	storageUC := NewStorageUsecase(mockRepo)
	uc := NewMonitoringUsecase(mockData, storageUC)

	t.Run("Check virtual monitoring updates TP1_HIT", func(t *testing.T) {
		err := uc.MonitorVirtualPositions(context.Background())
		assert.NoError(t, err)

		journal, _ := mockRepo.LoadSignalJournal()
		require.Len(t, journal, 1)
		assert.Equal(t, TP1_HIT, journal[0].Status)
	})
}

// 10. TEST FEEDBACK EVALUATION PART 20 BARU
func TestFeedbackEvaluationCompliance(t *testing.T) {
	mockRepo := &complianceStorageRepo{
		journal: []SignalJournal{
			{
				Symbol:                  "SOLUSDT",
				Playbook:                LIQUIDITY_SWEEP_REVERSAL,
				Direction:               LONG,
				Status:                  SL_HIT,
				EntryPrice:              10.0,
				StopLoss:                9.0,
				TP1:                     12.0,
				TP2:                     14.0,
				ThresholdProfileSummary: "volume confirmation: false, low volume ratio",
			},
		},
		audits: []DecisionAudit{
			{Symbol: "SOLUSDT", FinalStatus: FINAL_EXECUTE, Playbook: LIQUIDITY_SWEEP_REVERSAL},
		},
	}
	storageUC := NewStorageUsecase(mockRepo)
	uc := NewFeedbackUsecase(storageUC)

	t.Run("Flag gate bug when rules violated under low volume", func(t *testing.T) {
		err := uc.GenerateEvaluationReport()
		assert.NoError(t, err)

		report, _ := mockRepo.LoadEvaluationReport()
		require.NotNil(t, report)
		assert.Contains(t, report.GateBugFindings[0], "LIQUIDITY_SWEEP_REVERSAL executed without volume confirmation")
	})
}

// 11. TEST ORCHESTRATOR PART 21
func TestOrchestratorCompliance(t *testing.T) {
	mockRepo := &complianceStorageRepo{}
	mockData := &complianceMarketData{
		price: 100.0,
		tickers: []dto.Ticker24h{
			{Symbol: "SOLUSDT", LastPrice: 100.0, QuoteVolume: 15000000.0},
		},
		candles: []dto.Candle{
			{Time: time.Now(), Open: 100.0, High: 105.0, Low: 95.0, Close: 100.0},
		},
	}
	mockAI := &complianceAIAuditor{}
	mockNotif := &complianceNotification{}

	storageUC := NewStorageUsecase(mockRepo)
	marketDataUC := NewMarketDataUsecase(mockData)
	marketPolicyUC := NewMarketPolicyUsecase()
	universeUC := NewUniverseUsecase()
	strategySelectorUC := NewStrategySelectorUsecase()
	playbookEligibilityUC := NewPlaybookEligibilityUsecase()
	playbookQuantEngineUC := NewPlaybookQuantEngineUsecase()
	scoringUC := NewScoringUsecase()
	candidateArbiterUC := NewCandidateArbiterUsecase()
	localGateUC := NewLocalGateUsecase()
	aiCandidateSelectorUC := NewAICandidateSelectorUsecase(60.0)
	aiAuditorUC := NewAIAuditorUsecase(mockAI, storageUC)
	planReconciliationUC := NewPlanReconciliationUsecase()
	stalenessUC := NewStalenessUsecase(30 * time.Minute)
	finalGateUC := NewFinalGateUsecase()
	conflictResolverUC := NewConflictResolverUsecase()
	signalNotificationUC := NewSignalNotificationUsecase(mockNotif, storageUC)
	opsNotificationUC := NewOpsNotificationUsecase(mockNotif)
	monitoringUC := NewMonitoringUsecase(mockData, storageUC)
	feedbackUC := NewFeedbackUsecase(storageUC)

	scannerUC := NewScannerUsecase(
		marketDataUC,
		marketPolicyUC,
		universeUC,
		strategySelectorUC,
		playbookEligibilityUC,
		playbookQuantEngineUC,
		scoringUC,
		candidateArbiterUC,
		localGateUC,
		aiCandidateSelectorUC,
		aiAuditorUC,
		planReconciliationUC,
		stalenessUC,
		finalGateUC,
		conflictResolverUC,
		signalNotificationUC,
		opsNotificationUC,
		monitoringUC,
		feedbackUC,
		storageUC,
	)

	t.Run("AnalyzeMarketV3 happy path generates results and audits", func(t *testing.T) {
		res, err := scannerUC.Run(context.Background(), dto.ScanRequest{TriggerTime: time.Now()})
		assert.NoError(t, err)
		assert.NotEmpty(t, res.Timestamp)

		// Assert latest result saved
		latest, _ := mockRepo.LoadLatestResult()
		assert.Equal(t, res.Timestamp, latest.LastScanTime)
	})
}

// 12. TEST BACKTEST COMPLIANCE
func TestBacktestCompliance(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "backtest_compliance")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	startTime := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	var candles []dto.Candle
	for i := 0; i < 40; i++ {
		tTime := startTime.Add(time.Duration(i) * 15 * time.Minute)
		candles = append(candles, dto.Candle{
			Time:  tTime,
			Open:  100.0,
			High:  101.0,
			Low:   99.0,
			Close: 100.0,
			Vol:   100.0,
		})
	}

	mockData := &complianceMarketData{
		candles: candles,
		price:   100.0,
	}

	mockRepo := &complianceStorageRepo{}
	storageUC := NewStorageUsecase(mockRepo)
	marketPolicyUC := NewMarketPolicyUsecase()
	universeUC := NewUniverseUsecase()
	strategySelectorUC := NewStrategySelectorUsecase()
	playbookEligibilityUC := NewPlaybookEligibilityUsecase()
	playbookQuantEngineUC := NewPlaybookQuantEngineUsecase()
	scoringUC := NewScoringUsecase()
	candidateArbiterUC := NewCandidateArbiterUsecase()
	localGateUC := NewLocalGateUsecase()
	aiCandidateSelectorUC := NewAICandidateSelectorUsecase(60.0)
	aiAuditorUC := NewAIAuditorUsecase(&complianceAIAuditor{}, storageUC)
	planReconciliationUC := NewPlanReconciliationUsecase()
	stalenessUC := NewStalenessUsecase(30 * time.Minute)
	finalGateUC := NewFinalGateUsecase()
	conflictResolverUC := NewConflictResolverUsecase()

	backtestUC := NewBacktestEngineUsecase(
		mockData,
		marketPolicyUC,
		universeUC,
		strategySelectorUC,
		playbookEligibilityUC,
		playbookQuantEngineUC,
		scoringUC,
		candidateArbiterUC,
		localGateUC,
		aiCandidateSelectorUC,
		aiAuditorUC,
		planReconciliationUC,
		stalenessUC,
		finalGateUC,
		conflictResolverUC,
		storageUC,
		tmpDir,
	)

	req := BacktestRequest{
		Symbol:    "SOLUSDT",
		StartTime: startTime.Add(5 * time.Hour),
		EndTime:   startTime.Add(8 * time.Hour),
		Playbook:  "ALL",
		Regime:    "DYNAMIC",
		AIMode:    "MOCK",
	}

	report, err := backtestUC.RunBacktest(context.Background(), req)
	assert.NoError(t, err)
	require.NotNil(t, report)
	assert.Equal(t, "SOLUSDT", report.Symbol)

	// Ensure it didn't touch live storage signal journal/history
	journal, _ := mockRepo.LoadSignalJournal()
	assert.Empty(t, journal)
	history, _ := mockRepo.LoadSignalHistory()
	assert.Nil(t, history)
}

// 13. TEST SAFETY GLOBAL (Programmatic search for forbidden calls in source code)
func TestGlobalSafetyCompliance(t *testing.T) {
	forbiddenFuncs := []string{
		"NewCreateOrderService",
		"NewCreateBatchOrdersService",
		"NewCancelOrderService",
		"NewChangeLeverageService",
		"NewChangeMarginTypeService",
		"apply-recommendation",
		"auto-tune",
	}

	err := filepath.Walk("../../../", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".go") {
			return nil
		}
		// Skip test files to avoid matching definitions in compliance tests themselves
		if strings.HasSuffix(info.Name(), "_test.go") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		lines := strings.Split(string(data), "\n")
		inCommentBlock := false
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "/*") {
				inCommentBlock = true
				if strings.Contains(trimmed, "*/") {
					inCommentBlock = false
				}
				continue
			}
			if inCommentBlock {
				if strings.Contains(trimmed, "*/") {
					inCommentBlock = false
				}
				continue
			}
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			for _, fn := range forbiddenFuncs {
				if strings.Contains(trimmed, fn) {
					t.Errorf("COMPLIANCE VIOLATION: Forbidden function/route reference %q found in file %s", fn, path)
				}
			}
		}
		return nil
	})
	assert.NoError(t, err)
}
