package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"cpbro-engine/internal/modules/cryptobroV3/entity"
)

type mockAIAuditorService struct {
	response    *dto.AIAuditResponse
	err         error
	lastRequest dto.AIAuditRequest
}

func (m *mockAIAuditorService) AuditCandidate(ctx context.Context, req dto.AIAuditRequest) (*dto.AIAuditResponse, error) {
	m.lastRequest = req
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

type mockStorageRepository struct {
	cache *entity.AIAuditCache
}

func (m *mockStorageRepository) LoadLatestResult() (*entity.LatestResult, error) {
	return nil, nil
}

func (m *mockStorageRepository) SaveLatestResult(res *entity.LatestResult) error {
	return nil
}

func (m *mockStorageRepository) LoadSignalHistory() (*entity.SignalHistory, error) {
	return nil, nil
}

func (m *mockStorageRepository) SaveSignalHistory(hist *entity.SignalHistory) error {
	return nil
}

func (m *mockStorageRepository) LoadSignalJournal() ([]SignalJournal, error) {
	return nil, nil
}

func (m *mockStorageRepository) SaveSignalJournal(journal []SignalJournal) error {
	return nil
}

func (m *mockStorageRepository) AppendSignalJournal(entry SignalJournal) error {
	return nil
}

func (m *mockStorageRepository) LoadWatchJournal() ([]WatchJournal, error) {
	return nil, nil
}

func (m *mockStorageRepository) SaveWatchJournal(journal []WatchJournal) error {
	return nil
}

func (m *mockStorageRepository) AppendWatchJournal(entry WatchJournal) error {
	return nil
}

func (m *mockStorageRepository) LoadAIAuditCache() (*entity.AIAuditCache, error) {
	if m.cache == nil {
		return &entity.AIAuditCache{CacheMap: make(map[string]entity.CachedAudit)}, nil
	}
	return m.cache, nil
}

func (m *mockStorageRepository) SaveAIAuditCache(cache *entity.AIAuditCache) error {
	m.cache = cache
	return nil
}

func (m *mockStorageRepository) LoadEvaluationReport() (*EvaluationReport, error) {
	return nil, nil
}

func (m *mockStorageRepository) SaveEvaluationReport(report *EvaluationReport) error {
	return nil
}

func (m *mockStorageRepository) LoadDecisionAudits() ([]DecisionAudit, error) {
	return nil, nil
}

func (m *mockStorageRepository) SaveDecisionAudits(audits []DecisionAudit) error {
	return nil
}

func (m *mockStorageRepository) AppendDecisionAudit(entry DecisionAudit) error {
	return nil
}

func TestAIAuditor_SuccessFlow(t *testing.T) {
	mockResponse := &dto.AIAuditResponse{
		Decision:         "CONFIRM",
		Confidence:       "HIGH",
		CandleNarrative:  "REJECTION",
		Last5CandlesBias: "BULLISH",
		HasRejection:     true,
		HasConfirmation:  true,
		EntryTiming:      "FRESH",
		ConflictWithBot:  false,
		SuggestedAction:  "EXECUTE_IF_NOT_STALE",
		PlanFeedback:     "Setup looks clean",
		Reason:           "Wick rejection on Support",
		Risk:             "High volatility",
	}

	mockService := &mockAIAuditorService{response: mockResponse}
	storage := NewStorageUsecase(&mockStorageRepository{})
	auditor := NewAIAuditorUsecase(mockService, storage)

	quant := QuantResult{
		Symbol:    "NEARUSDT",
		Direction: LONG,
		Playbook:  TREND_PULLBACK,
		Score:     8.5,
		TradePlan: TradePlan{
			EntryPrice: 5.0,
			StopLoss:   4.5,
			TakeProfit: 6.0,
		},
	}

	policy := MarketPolicy{
		Reason: "Normal",
	}

	m15 := []dto.Candle{{Vol: 100}}

	res, err := auditor.Audit(context.Background(), quant, policy, m15, nil, nil)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if !res.IsApproved {
		t.Errorf("Expected IsApproved to be true")
	}
	if res.Sentiment != "BULLISH" {
		t.Errorf("Expected Sentiment BULLISH for confirmed LONG, got %s", res.Sentiment)
	}
	if res.ConfidenceScore != 0.9 {
		t.Errorf("Expected ConfidenceScore 0.9 for HIGH confidence, got %0.1f", res.ConfidenceScore)
	}
	if res.SuggestedStopLoss != 0 || res.SuggestedTakeProfit != 0 {
		t.Errorf("Expected suggested SL/TP to be forced to 0, got SL=%0.1f TP=%0.1f", res.SuggestedStopLoss, res.SuggestedTakeProfit)
	}
}

func TestAIAuditor_ConflictRejection(t *testing.T) {
	mockResponse := &dto.AIAuditResponse{
		Decision:         "CONFIRM",
		Confidence:       "HIGH",
		CandleNarrative:  "REJECTION",
		Last5CandlesBias: "BULLISH",
		HasRejection:     true,
		HasConfirmation:  true,
		EntryTiming:      "FRESH",
		ConflictWithBot:  true,
		SuggestedAction:  "EXECUTE_IF_NOT_STALE",
		PlanFeedback:     "Inconsistent setup",
		Reason:           "Conflict",
		Risk:             "Conflict",
	}

	mockService := &mockAIAuditorService{response: mockResponse}
	storage := NewStorageUsecase(&mockStorageRepository{})
	auditor := NewAIAuditorUsecase(mockService, storage)

	quant := QuantResult{
		Symbol:    "NEARUSDT",
		Direction: LONG,
		Playbook:  TREND_PULLBACK,
		Score:     8.5,
	}

	policy := MarketPolicy{Reason: "Normal"}
	m15 := []dto.Candle{{Vol: 100}}

	res, err := auditor.Audit(context.Background(), quant, policy, m15, nil, nil)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if res.IsApproved {
		t.Errorf("Expected IsApproved to be false due to ConflictWithBot")
	}
	if res.Decision != "REJECT" || res.SuggestedAction != "REJECT" {
		t.Errorf("Expected Decision and SuggestedAction to be REJECT, got Decision=%s Action=%s", res.Decision, res.SuggestedAction)
	}
	if res.Sentiment != "NEUTRAL" {
		t.Errorf("Expected Sentiment NEUTRAL for rejected conflict response, got %s", res.Sentiment)
	}
}

func TestAIAuditor_NormalizesConfirmWatchOnlyToWait(t *testing.T) {
	mockResponse := &dto.AIAuditResponse{
		Decision:         "CONFIRM",
		Confidence:       "HIGH",
		CandleNarrative:  "REJECTION",
		Last5CandlesBias: "BULLISH",
		HasRejection:     false,
		HasConfirmation:  true,
		EntryTiming:      "FRESH",
		ConflictWithBot:  false,
		SuggestedAction:  "WATCH_ONLY",
		PlanFeedback:     "Wait for confirmation candle after sweep.",
		Reason:           "Sweep is valid but confirmation is incomplete.",
		Risk:             "Early reversal risk remains high.",
	}

	mockService := &mockAIAuditorService{response: mockResponse}
	storage := NewStorageUsecase(&mockStorageRepository{})
	auditor := NewAIAuditorUsecase(mockService, storage)

	quant := QuantResult{
		Symbol:    "SOXLUSDT",
		Direction: LONG,
		Playbook:  LIQUIDITY_SWEEP_REVERSAL,
		Score:     8.2,
	}

	res, err := auditor.Audit(context.Background(), quant, MarketPolicy{Reason: "Normal"}, []dto.Candle{{Vol: 100}}, nil, nil)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if res.Decision != "WAIT" {
		t.Fatalf("expected Decision WAIT after normalization, got %s", res.Decision)
	}
	if res.SuggestedAction != "WATCH_ONLY" {
		t.Fatalf("expected SuggestedAction WATCH_ONLY to remain intact, got %s", res.SuggestedAction)
	}
	if res.IsApproved {
		t.Fatalf("expected IsApproved=false after normalization")
	}
	if res.Sentiment != "NEUTRAL" {
		t.Fatalf("expected neutral sentiment for non-executable watch response, got %s", res.Sentiment)
	}
}

func TestAIAuditor_UsesStructuredH1ContextInPayload(t *testing.T) {
	mockResponse := &dto.AIAuditResponse{
		Decision:         "CONFIRM",
		Confidence:       "HIGH",
		CandleNarrative:  "CONTINUATION",
		Last5CandlesBias: "BULLISH",
		HasRejection:     true,
		HasConfirmation:  true,
		EntryTiming:      "FRESH",
		ConflictWithBot:  false,
		SuggestedAction:  "EXECUTE_IF_NOT_STALE",
		PlanFeedback:     "Retest held cleanly",
		Reason:           "Closed candles support continuation",
		Risk:             "Normal continuation risk",
	}

	mockService := &mockAIAuditorService{response: mockResponse}
	storage := NewStorageUsecase(&mockStorageRepository{})
	auditor := NewAIAuditorUsecase(mockService, storage)

	quant := QuantResult{
		Symbol:          "NEARUSDT",
		Direction:       LONG,
		Playbook:        COMPRESSION_BREAKOUT_RETEST,
		SetupType:       "BREAKOUT_RETEST",
		Score:           8.5,
		MarketStructure: "M15_BULLISH_BOS",
		StructureSnapshot: StructureSnapshot{
			H1Structure: "H1_RANGE_BOUND",
			Notes:       "H4Trend: BULLISH | H1Trend: BULLISH",
		},
		TradePlan: TradePlan{
			EntryPrice: 5.0,
			StopLoss:   4.5,
			TakeProfit: 6.0,
		},
	}

	policy := MarketPolicy{Reason: "Normal"}
	m15 := []dto.Candle{
		{Time: time.Now().Add(-45 * time.Minute), Open: 5, High: 5.1, Low: 4.9, Close: 5.0, Vol: 100},
		{Time: time.Now().Add(-30 * time.Minute), Open: 5, High: 5.2, Low: 4.95, Close: 5.1, Vol: 120},
		{Time: time.Now().Add(-15 * time.Minute), Open: 5.1, High: 5.25, Low: 5.05, Close: 5.2, Vol: 130},
	}

	_, err := auditor.Audit(context.Background(), quant, policy, m15, nil, nil)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if mockService.lastRequest.Payload.Structure.H1Structure != "H1_RANGE_BOUND" {
		t.Fatalf("expected AI payload to use StructureSnapshot.H1Structure, got %q", mockService.lastRequest.Payload.Structure.H1Structure)
	}
}

func TestAIAuditor_APIFailure(t *testing.T) {
	mockService := &mockAIAuditorService{err: errors.New("timeout calling Gemini API")}
	storage := NewStorageUsecase(&mockStorageRepository{})
	auditor := NewAIAuditorUsecase(mockService, storage)

	quant := QuantResult{
		Symbol:    "NEARUSDT",
		Direction: LONG,
		Playbook:  TREND_PULLBACK,
		Score:     8.5,
	}

	policy := MarketPolicy{Reason: "Normal"}
	m15 := []dto.Candle{{Vol: 100}}

	res, err := auditor.Audit(context.Background(), quant, policy, m15, nil, nil)
	if err == nil {
		t.Fatalf("Expected error calling Gemini API, got nil")
	}

	if res.IsApproved {
		t.Errorf("Expected IsApproved to be false on API error")
	}
	if res.Sentiment != "NEUTRAL" {
		t.Errorf("Expected Sentiment to be NEUTRAL on API error, got %s", res.Sentiment)
	}
}
