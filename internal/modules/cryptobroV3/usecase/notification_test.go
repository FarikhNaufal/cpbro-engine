package usecase_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"cpbro-engine/internal/modules/cryptobroV3/entity"
	"cpbro-engine/internal/modules/cryptobroV3/usecase"
)

type mockNotificationService struct {
	signalMessages []string
	opsMessages    []string
	shouldFail     bool
}

func (m *mockNotificationService) SendSignalMessage(ctx context.Context, msg string) error {
	if m.shouldFail {
		return errors.New("telegram api error")
	}
	m.signalMessages = append(m.signalMessages, msg)
	return nil
}

func (m *mockNotificationService) SendOpsMessage(ctx context.Context, msg string) error {
	if m.shouldFail {
		return errors.New("telegram api error")
	}
	m.opsMessages = append(m.opsMessages, msg)
	return nil
}

func TestSignalNotification_SendV3Signals_SuccessAndFilters(t *testing.T) {
	mockSvc := &mockNotificationService{}
	uc := usecase.NewSignalNotificationUsecase(mockSvc)

	policy := usecase.MarketPolicy{
		Reason: "NORMAL",
	}

	summary := usecase.ScannerSummaryV3{
		ActiveRegime: "NORMAL",
	}

	reqs := []usecase.SignalNotificationRequest{
		{
			Decision: usecase.FinalDecision{
				Symbol: "AVAXUSDT",
				Status: usecase.AI_ERROR_REVIEW,
			},
			AuditResponse: dto.AIAuditResponse{
				Reasoning: "AI_ERROR: context deadline exceeded",
			},
		},
		{
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
		{
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
	}

	uc.SendV3Signals(context.Background(), reqs, policy, summary)

	if len(mockSvc.signalMessages) != 1 {
		t.Fatalf("expected 1 telegram message sent, got %d", len(mockSvc.signalMessages))
	}

	msg := mockSvc.signalMessages[0]
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
	if !strings.Contains(msg, "*Mode:* Alert-only, manual execution.") {
		t.Errorf("missing manual execution warning, got %s", msg)
	}
}

func TestSignalNotification_SendV3Signals_HighRiskRegimeWarning(t *testing.T) {
	mockSvc := &mockNotificationService{}
	uc := usecase.NewSignalNotificationUsecase(mockSvc)

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

	uc.SendV3Signals(context.Background(), reqs, policy, summary)

	if len(mockSvc.signalMessages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(mockSvc.signalMessages))
	}

	msg := mockSvc.signalMessages[0]
	if !strings.Contains(msg, "BTC_CHAOS ⚠️ *[HIGH RISK]*") {
		t.Errorf("missing high risk regime warning, got %s", msg)
	}
}

func TestSignalNotification_SendV3Signals_FailureTolerance(t *testing.T) {
	mockSvc := &mockNotificationService{
		shouldFail: true,
	}
	uc := usecase.NewSignalNotificationUsecase(mockSvc)

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

	// Should not crash even if dispatch fails
	uc.SendV3Signals(context.Background(), reqs, policy, summary)
}

func TestOpsNotification_NonSignalPrefixAndNoTradeActionFooter(t *testing.T) {
	mockSvc := &mockNotificationService{}
	uc := usecase.NewOpsNotificationUsecase(mockSvc)

	boundary := time.Date(2026, 5, 24, 0, 15, 0, 0, time.UTC)
	uc.SendScanStarted(context.Background(), "scan-123", boundary, "M15 close scan")
	if len(mockSvc.opsMessages) != 1 {
		t.Fatalf("expected 1 ops message, got %d", len(mockSvc.opsMessages))
	}
	msg := mockSvc.opsMessages[0]
	if !strings.Contains(msg, "[CRYPTOBRO V3 OPS][NON-SIGNAL]") {
		t.Fatalf("missing ops prefix: %s", msg)
	}
	if !strings.Contains(msg, "No trade action") {
		t.Fatalf("missing non-signal footer: %s", msg)
	}
	if strings.Contains(msg, "*Entry:*") || strings.Contains(msg, "*SL:*") || strings.Contains(msg, "*TP") {
		t.Fatalf("ops message must not resemble trade signal payload: %s", msg)
	}
}

func TestOpsNotification_ScanDoneSummary_NoEntrySLTP(t *testing.T) {
	mockSvc := &mockNotificationService{}
	uc := usecase.NewOpsNotificationUsecase(mockSvc)

	latest := &entity.LatestResult{
		ScanID:                "scan-abc",
		GeneratedAt:           time.Now(),
		MarketRegime:          "NORMAL",
		TotalUniversePass:     10,
		TotalPlaybookEligible: 3,
		TotalLocalAICandidate: 2,
		TotalFinalExecute:     1,
		TotalFinalWatch:       1,
		TotalFinalReject:      1,
		TotalAIError:          0,
		Duration:              "1500ms",
	}
	uc.SendScanDone(context.Background(), latest)
	if len(mockSvc.opsMessages) != 1 {
		t.Fatalf("expected 1 ops message, got %d", len(mockSvc.opsMessages))
	}
	msg := mockSvc.opsMessages[0]
	if !strings.Contains(msg, "SCAN_DONE") {
		t.Fatalf("missing SCAN_DONE: %s", msg)
	}
	if strings.Contains(msg, "*Entry:*") || strings.Contains(msg, "*SL:*") || strings.Contains(msg, "*TP") {
		t.Fatalf("scan done ops message must not contain entry/sl/tp fields: %s", msg)
	}
}

func TestOpsNotification_AdminWarningAIError_NonSignal(t *testing.T) {
	mockSvc := &mockNotificationService{}
	uc := usecase.NewOpsNotificationUsecase(mockSvc)
	uc.SendAdminWarningAIError(context.Background(), "scan-1", "SOLUSDT", "TREND_PULLBACK", "AI_ERROR_REVIEW", "AI timeout")
	if len(mockSvc.opsMessages) != 1 {
		t.Fatalf("expected 1 ops message, got %d", len(mockSvc.opsMessages))
	}
	msg := mockSvc.opsMessages[0]
	if !strings.Contains(msg, "ADMIN_WARNING") || !strings.Contains(msg, "AI_ERROR") {
		t.Fatalf("missing admin warning type: %s", msg)
	}
	if strings.Contains(msg, "[CRYPTOBRO V3 SIGNAL]") {
		t.Fatalf("admin warning must not be sent through signal format: %s", msg)
	}
	if !strings.Contains(msg, "No trade action") {
		t.Fatalf("missing non-signal footer: %s", msg)
	}
}
