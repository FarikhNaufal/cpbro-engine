package usecase_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"cpbro-engine/internal/modules/cryptobroV3/usecase"
)

type mockNotificationService struct {
	sentAlerts   []dto.SignalResponse
	sentMessages []string
	shouldFail   bool
}

func (m *mockNotificationService) SendFinalExecuteAlert(ctx context.Context, signal dto.SignalResponse) error {
	m.sentAlerts = append(m.sentAlerts, signal)
	return nil
}

func (m *mockNotificationService) SendTelegramMessage(ctx context.Context, msg string) error {
	if m.shouldFail {
		return errors.New("telegram api error")
	}
	m.sentMessages = append(m.sentMessages, msg)
	return nil
}

func TestNotification_SendV3_Success(t *testing.T) {
	mockSvc := &mockNotificationService{}
	uc := usecase.NewNotificationUsecase(mockSvc)

	policy := usecase.MarketPolicy{
		Reason: "NORMAL",
	}

	summary := usecase.ScannerSummaryV3{
		TotalScanned:    10,
		CandidatesFound: 1,
		StartTime:       time.Now(),
		Duration:        "1.5s",
		ActiveRegime:    "NORMAL",
		BtcTrend:        "UP",
	}

	reqs := []usecase.SignalNotificationRequest{
		{
			Decision: usecase.FinalDecision{
				Symbol:                  "BTCUSDT",
				Direction:               usecase.LONG,
				Playbook:                usecase.TREND_PULLBACK,
				Status:                  usecase.FINAL_EXECUTE,
				IsExecutable:            true,
				Score:                   8.2,
				RequiredScore:           7.0,
				EntryPrice:              50000.0,
				StopLoss:                48000.0,
				TakeProfit:              54000.0,
				RR:                      2.0,
				RequiredRR:              1.5,
				AIConfidence:            "HIGH",
				StalenessStatus:         "FRESH",
				PolicySummary:           "AllowLong=true, AllowShort=true, MinScore=7.0, MinRR=1.5, MaxExecute=3",
				ThresholdProfileSummary: "Playbook=TREND_PULLBACK, MinScore=7.0, MinRR=1.5, RequireADX=true",
				Reason:                  "Valid technical breakout support.",
			},
			AuditResponse: dto.AIAuditResponse{
				Symbol:    "BTCUSDT",
				Sentiment: "BULLISH",
				Reason:    "AI validation matched setup.",
				Risk:      "Low leverage risk.",
			},
		},
	}

	err := uc.SendV3(context.Background(), reqs, policy, summary)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mockSvc.sentMessages) != 1 {
		t.Fatalf("expected 1 telegram message sent, got %d", len(mockSvc.sentMessages))
	}

	msg := mockSvc.sentMessages[0]
	if !strings.Contains(msg, "[CRYPTOBRO V3 SIGNAL]") {
		t.Errorf("missing header, got %s", msg)
	}
	if !strings.Contains(msg, "*Symbol:* BTCUSDT") {
		t.Errorf("missing Symbol, got %s", msg)
	}
	if !strings.Contains(msg, "*Market Policy:* AllowLong=true, AllowShort=true, MinScore=7.0, MinRR=1.5, MaxExecute=3") {
		t.Errorf("missing Market Policy, got %s", msg)
	}
	if !strings.Contains(msg, "*Threshold Profile:* Playbook=TREND_PULLBACK, MinScore=7.0, MinRR=1.5, RequireADX=true") {
		t.Errorf("missing Threshold Profile, got %s", msg)
	}
	if !strings.Contains(msg, "*TP1:* 52000.0000") {
		t.Errorf("incorrect TP1 scaling, got %s", msg)
	}
	if !strings.Contains(msg, "*TP2:* 54000.0000") {
		t.Errorf("incorrect TP2, got %s", msg)
	}
	if !strings.Contains(msg, "*Grade/Score:* S / 8.20") {
		t.Errorf("incorrect grade or score representation, got %s", msg)
	}
	if !strings.Contains(msg, "*Max Hold:* 8 candle M15 / 120 menit") {
		t.Errorf("missing max hold, got %s", msg)
	}
	if !strings.Contains(msg, "*Mode:* Alert-only, manual execution.") {
		t.Errorf("missing manual execution warning, got %s", msg)
	}
}

func TestNotification_SendV3_Filters(t *testing.T) {
	mockSvc := &mockNotificationService{}
	uc := usecase.NewNotificationUsecase(mockSvc)

	policy := usecase.MarketPolicy{Reason: "NORMAL"}
	summary := usecase.ScannerSummaryV3{ActiveRegime: "NORMAL"}

	reqs := []usecase.SignalNotificationRequest{
		{
			// Watch status should be filtered
			Decision: usecase.FinalDecision{
				Symbol:          "ETHUSDT",
				Status:          usecase.FINAL_WATCH,
				IsExecutable:    false,
				AIConfidence:    "HIGH",
				StalenessStatus: "FRESH",
				EntryPrice:      3000,
				StopLoss:        2900,
				TakeProfit:      3200,
				Reason:          "Valid setup.",
			},
		},
		{
			// Low AI confidence should be filtered
			Decision: usecase.FinalDecision{
				Symbol:          "SOLUSDT",
				Status:          usecase.FINAL_EXECUTE,
				IsExecutable:    true,
				AIConfidence:    "MEDIUM",
				StalenessStatus: "FRESH",
				EntryPrice:      100,
				StopLoss:        90,
				TakeProfit:      120,
				Reason:          "Valid setup.",
			},
		},
		{
			// LATE status should be filtered
			Decision: usecase.FinalDecision{
				Symbol:          "NEARUSDT",
				Status:          usecase.FINAL_EXECUTE,
				IsExecutable:    true,
				AIConfidence:    "HIGH",
				StalenessStatus: "LATE",
				EntryPrice:      5,
				StopLoss:        4.5,
				TakeProfit:      6.0,
				Reason:          "Valid setup.",
			},
		},
	}

	err := uc.SendV3(context.Background(), reqs, policy, summary)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mockSvc.sentMessages) != 0 {
		t.Errorf("expected 0 telegram alerts sent due to filtering, got %d", len(mockSvc.sentMessages))
	}
}

func TestNotification_SendV3_AdminWarning(t *testing.T) {
	mockSvc := &mockNotificationService{}
	uc := usecase.NewNotificationUsecase(mockSvc)

	policy := usecase.MarketPolicy{Reason: "NORMAL"}
	summary := usecase.ScannerSummaryV3{ActiveRegime: "NORMAL"}

	reqs := []usecase.SignalNotificationRequest{
		{
			Decision: usecase.FinalDecision{
				Symbol: "AVAXUSDT",
				Status: "AI_ERROR_REVIEW",
			},
			AuditResponse: dto.AIAuditResponse{
				Reasoning: "AI_ERROR: context deadline exceeded",
			},
		},
	}

	err := uc.SendV3(context.Background(), reqs, policy, summary)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mockSvc.sentMessages) != 1 {
		t.Fatalf("expected 1 admin warning message, got %d", len(mockSvc.sentMessages))
	}

	msg := mockSvc.sentMessages[0]
	if !strings.Contains(msg, "[ADMIN WARNING]") {
		t.Errorf("missing admin warning tag, got %s", msg)
	}
	if !strings.Contains(msg, "AVAXUSDT") {
		t.Errorf("missing Symbol, got %s", msg)
	}
	if !strings.Contains(msg, "AI_ERROR: context deadline exceeded") {
		t.Errorf("missing error details, got %s", msg)
	}
}

func TestNotification_SendV3_HighRiskRegimeWarning(t *testing.T) {
	mockSvc := &mockNotificationService{}
	uc := usecase.NewNotificationUsecase(mockSvc)

	policy := usecase.MarketPolicy{Reason: "BTC_CHAOS"}
	summary := usecase.ScannerSummaryV3{
		ActiveRegime: "BTC_CHAOS",
	}

	reqs := []usecase.SignalNotificationRequest{
		{
			Decision: usecase.FinalDecision{
				Symbol:          "DOTUSDT",
				Direction:       usecase.LONG,
				Playbook:        usecase.TREND_PULLBACK,
				Status:          usecase.FINAL_EXECUTE,
				IsExecutable:    true,
				Score:           8.0,
				EntryPrice:      6.0,
				StopLoss:        5.5,
				TakeProfit:      7.0,
				AIConfidence:    "HIGH",
				StalenessStatus: "FRESH",
				Reason:          "Valid setup.",
			},
		},
	}

	err := uc.SendV3(context.Background(), reqs, policy, summary)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mockSvc.sentMessages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(mockSvc.sentMessages))
	}

	msg := mockSvc.sentMessages[0]
	if !strings.Contains(msg, "BTC_CHAOS ⚠️ *[HIGH RISK]*") {
		t.Errorf("missing high risk regime warning, got %s", msg)
	}
}

func TestNotification_SendV3_FailureTolerance(t *testing.T) {
	mockSvc := &mockNotificationService{
		shouldFail: true,
	}
	uc := usecase.NewNotificationUsecase(mockSvc)

	policy := usecase.MarketPolicy{Reason: "NORMAL"}
	summary := usecase.ScannerSummaryV3{ActiveRegime: "NORMAL"}

	reqs := []usecase.SignalNotificationRequest{
		{
			Decision: usecase.FinalDecision{
				Symbol:          "ADAUSDT",
				Direction:       usecase.LONG,
				Playbook:        usecase.TREND_PULLBACK,
				Status:          usecase.FINAL_EXECUTE,
				IsExecutable:    true,
				Score:           8.0,
				EntryPrice:      0.5,
				StopLoss:        0.45,
				TakeProfit:      0.6,
				AIConfidence:    "HIGH",
				StalenessStatus: "FRESH",
				Reason:          "Valid setup.",
			},
		},
	}

	// Should not crash or return error even if notification dispatch fails
	err := uc.SendV3(context.Background(), reqs, policy, summary)
	if err != nil {
		t.Errorf("SendV3 should handle notification errors gracefully without returning error, got %v", err)
	}
}
