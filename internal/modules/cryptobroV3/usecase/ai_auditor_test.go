package usecase

import (
	"context"
	"errors"
	"testing"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"cpbro-engine/internal/modules/cryptobroV3/entity"
)

type mockAIAuditorService struct {
	response *dto.AIAuditResponse
	err      error
}

func (m *mockAIAuditorService) AuditCandidate(ctx context.Context, req dto.AIAuditRequest) (*dto.AIAuditResponse, error) {
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
