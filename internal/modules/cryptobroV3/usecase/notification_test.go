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
	uc := usecase.NewSignalNotificationUsecase(mockSvc, nil)

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
	if !strings.Contains(msg, "<b>Symbol:</b> BTCUSDT") {
		t.Errorf("missing Symbol, got %s", msg)
	}
	if !strings.Contains(msg, "<b>Market Policy:</b> AllowLong=true, AllowShort=true, MinScore=7.0, MinRR=1.5, MaxExecute=3") {
		t.Errorf("missing Market Policy, got %s", msg)
	}
	if !strings.Contains(msg, "<b>Threshold Profile:</b> Playbook=TREND_PULLBACK, MinScore=7.0, MinRR=1.5, RequireADX=true") {
		t.Errorf("missing Threshold Profile, got %s", msg)
	}
	if !strings.Contains(msg, "<b>TP1:</b> 52000.0000") {
		t.Errorf("incorrect TP1 scaling, got %s", msg)
	}
	if !strings.Contains(msg, "<b>TP2:</b> 54000.0000") {
		t.Errorf("incorrect TP2, got %s", msg)
	}
	if !strings.Contains(msg, "<b>Grade/Score:</b> S / 8.20") {
		t.Errorf("incorrect grade or score representation, got %s", msg)
	}
	if !strings.Contains(msg, "<i>Mode: Alert-only, manual execution.</i>") {
		t.Errorf("missing manual execution warning, got %s", msg)
	}
}

func TestSignalNotification_SendV3Signals_HighRiskRegimeWarning(t *testing.T) {
	mockSvc := &mockNotificationService{}
	uc := usecase.NewSignalNotificationUsecase(mockSvc, nil)

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
	if !strings.Contains(msg, "BTC_CHAOS ⚠️ <b>[HIGH RISK]</b>") {
		t.Errorf("missing high risk regime warning, got %s", msg)
	}
}

func TestSignalNotification_SendV3Signals_FailureTolerance(t *testing.T) {
	mockSvc := &mockNotificationService{
		shouldFail: true,
	}
	uc := usecase.NewSignalNotificationUsecase(mockSvc, nil)

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
	if !strings.Contains(msg, "CRYPTOBRO V3 OPS — NON-SIGNAL") {
		t.Fatalf("missing ops prefix: %s", msg)
	}
	if !strings.Contains(msg, "No trade action. Informational status only.") {
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
	if !strings.Contains(msg, "Scan Done") {
		t.Fatalf("missing Scan Done: %s", msg)
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
	if !strings.Contains(msg, "Admin Warning") || !strings.Contains(msg, "Type      : AI_ERROR") {
		t.Fatalf("missing admin warning type: %s", msg)
	}
	if strings.Contains(msg, "[CRYPTOBRO V3 SIGNAL]") {
		t.Fatalf("admin warning must not be sent through signal format: %s", msg)
	}
	if !strings.Contains(msg, "No trade action. Informational status only.") {
		t.Fatalf("missing non-signal footer: %s", msg)
	}
}

func TestTelegramOpsHTMLCompliance(t *testing.T) {
	mockSvc := &mockNotificationService{}
	uc := usecase.NewOpsNotificationUsecase(mockSvc)

	t.Run("Scan Started format compliance", func(t *testing.T) {
		mockSvc.opsMessages = nil
		boundary := time.Date(2026, 5, 24, 5, 15, 0, 0, time.UTC)
		uc.SendScanStarted(context.Background(), "20260524051500", boundary, "Startup M15 close scan")

		if len(mockSvc.opsMessages) != 1 {
			t.Fatalf("expected 1 message, got %d", len(mockSvc.opsMessages))
		}
		msg := mockSvc.opsMessages[0]

		// Must contain headers
		if !strings.Contains(msg, "CRYPTOBRO V3 OPS — NON-SIGNAL") {
			t.Errorf("missing ops header: %s", msg)
		}
		if !strings.Contains(msg, "Scan Started") {
			t.Errorf("missing title: %s", msg)
		}

		// Must not contain raw snake_case keys
		if strings.Contains(msg, "scan_id") || strings.Contains(msg, "market_regime") || strings.Contains(msg, "started_at") || strings.Contains(msg, "ops_prefix") {
			t.Errorf("found raw snake_case key in started message: %s", msg)
		}

		// Must contain friendly labels
		if !strings.Contains(msg, "Scan ID   :") || !strings.Contains(msg, "Mode      :") || !strings.Contains(msg, "Boundary  :") || !strings.Contains(msg, "Started   :") || !strings.Contains(msg, "Regime    :") {
			t.Errorf("missing friendly label in started message: %s", msg)
		}

		// Disclaimer must be in the message body
		if !strings.Contains(msg, "No trade action. Informational status only.") {
			t.Errorf("disclaimer is missing from message: %s", msg)
		}
	})

	t.Run("Scan Done format compliance", func(t *testing.T) {
		mockSvc.opsMessages = nil
		latest := &entity.LatestResult{
			ScanID:                "20260524051500",
			GeneratedAt:           time.Date(2026, 5, 24, 5, 27, 40, 0, time.UTC),
			MarketRegime:          "CHOP_RANGE active - <mean reversion only>",
			TotalUniversePass:     50,
			TotalPlaybookEligible: 4,
			TotalLocalAICandidate: 1,
			TotalFinalExecute:     0,
			TotalFinalWatch:       0,
			TotalFinalReject:      1,
			TotalAIError:          0,
			Duration:              "2878ms",
		}
		uc.SendScanDone(context.Background(), latest)

		if len(mockSvc.opsMessages) != 1 {
			t.Fatalf("expected 1 message, got %d", len(mockSvc.opsMessages))
		}
		msg := mockSvc.opsMessages[0]

		// Must contain headers
		if !strings.Contains(msg, "CRYPTOBRO V3 OPS — NON-SIGNAL") {
			t.Errorf("missing ops header: %s", msg)
		}
		if !strings.Contains(msg, "Scan Done") {
			t.Errorf("missing title: %s", msg)
		}

		// HTML escaping: `<mean reversion only>` must be escaped to `&lt;mean reversion only&gt;`
		if strings.Contains(msg, "<mean reversion only>") {
			t.Errorf("dynamic values not HTML escaped: %s", msg)
		}
		if !strings.Contains(msg, "&lt;mean reversion only&gt;") {
			t.Errorf("HTML escaping check failed: %s", msg)
		}

		// Must not contain raw snake_case keys
		if strings.Contains(msg, "scan_id") || strings.Contains(msg, "generated_at") || strings.Contains(msg, "market_regime") {
			t.Errorf("found raw snake_case key in done message: %s", msg)
		}

		// Must contain friendly labels
		if !strings.Contains(msg, "Scan ID      :") || !strings.Contains(msg, "Generated    :") || !strings.Contains(msg, "Regime       :") {
			t.Errorf("missing friendly label in done message: %s", msg)
		}

		// Disclaimer must be in the message body
		if !strings.Contains(msg, "No trade action. Informational status only.") {
			t.Errorf("disclaimer is missing from message: %s", msg)
		}

		// Must not contain signal details (Entry/SL/TP)
		if strings.Contains(msg, "Entry:") || strings.Contains(msg, "SL:") || strings.Contains(msg, "TP1:") || strings.Contains(msg, "TP2:") {
			t.Errorf("OPS message must not contain entry/SL/TP signal details: %s", msg)
		}
		})
	}

type mockNotificationStorageRepo struct {
	journal []usecase.SignalJournal
}

func (m *mockNotificationStorageRepo) LoadLatestResult() (*entity.LatestResult, error) {
	return nil, nil
}
func (m *mockNotificationStorageRepo) SaveLatestResult(res *entity.LatestResult) error {
	return nil
}
func (m *mockNotificationStorageRepo) LoadSignalHistory() (*entity.SignalHistory, error) {
	return nil, nil
}
func (m *mockNotificationStorageRepo) SaveSignalHistory(hist *entity.SignalHistory) error {
	return nil
}
func (m *mockNotificationStorageRepo) LoadSignalJournal() ([]usecase.SignalJournal, error) {
	return m.journal, nil
}
func (m *mockNotificationStorageRepo) SaveSignalJournal(journal []usecase.SignalJournal) error {
	m.journal = journal
	return nil
}
func (m *mockNotificationStorageRepo) AppendSignalJournal(entry usecase.SignalJournal) error {
	m.journal = append(m.journal, entry)
	return nil
}
func (m *mockNotificationStorageRepo) LoadAIAuditCache() (*entity.AIAuditCache, error) {
	return nil, nil
}
func (m *mockNotificationStorageRepo) SaveAIAuditCache(cache *entity.AIAuditCache) error {
	return nil
}
func (m *mockNotificationStorageRepo) LoadEvaluationReport() (*usecase.EvaluationReport, error) {
	return nil, nil
}
func (m *mockNotificationStorageRepo) SaveEvaluationReport(report *usecase.EvaluationReport) error {
	return nil
}
func (m *mockNotificationStorageRepo) LoadDecisionAudits() ([]usecase.DecisionAudit, error) {
	return nil, nil
}
func (m *mockNotificationStorageRepo) SaveDecisionAudits(audits []usecase.DecisionAudit) error {
	return nil
}
func (m *mockNotificationStorageRepo) AppendDecisionAudit(entry usecase.DecisionAudit) error {
	return nil
}

type mockTelegramAPIError struct {
	statusCode  int
	errorCode   int
	description string
}

func (e *mockTelegramAPIError) Error() string {
	return e.description
}
func (e *mockTelegramAPIError) GetStatusCode() int   { return e.statusCode }
func (e *mockTelegramAPIError) GetErrorCode() int    { return e.errorCode }
func (e *mockTelegramAPIError) GetDescription() string { return e.description }

type mockNotificationServiceWithAPIError struct {
	apiErr error
}

func (m *mockNotificationServiceWithAPIError) SendSignalMessage(ctx context.Context, msg string) error {
	return m.apiErr
}
func (m *mockNotificationServiceWithAPIError) SendOpsMessage(ctx context.Context, msg string) error {
	return m.apiErr
}

func TestSignalNotification_EscapingAndChopRange(t *testing.T) {
	mockSvc := &mockNotificationService{}
	uc := usecase.NewSignalNotificationUsecase(mockSvc, nil)

	policy := usecase.MarketPolicy{Reason: "CHOP_RANGE"}
	summary := usecase.ScannerSummaryV3{ActiveRegime: "CHOP_RANGE"}

	reqs := []usecase.SignalNotificationRequest{
		{
			Decision: usecase.FinalDecision{
				Symbol:          "GENIUSUSDT",
				Direction:       usecase.LONG,
				Playbook:        usecase.TREND_PULLBACK,
				Status:          usecase.FINAL_EXECUTE,
				IsExecutable:    true,
				Score:           9.5,
				RequiredScore:   7.0,
				EntryPrice:      1.25,
				StopLoss:        1.10,
				TakeProfit:      1.55,
				RR:              2.0,
				RequiredRR:      1.5,
				AIConfidence:    "HIGH",
				StalenessStatus: "FRESH",
				Reason:          "Setup with <special> characters & symbols_here.",
			},
			AuditResponse: dto.AIAuditResponse{
				Sentiment: "BULLISH",
				Reason:    "",
				Risk:      "Risk assessment with <low> risk & standard setup.",
			},
		},
	}

	uc.SendV3Signals(context.Background(), reqs, policy, summary)

	if len(mockSvc.signalMessages) != 1 {
		t.Fatalf("expected 1 signal message, got %d", len(mockSvc.signalMessages))
	}

	msg := mockSvc.signalMessages[0]
	// Check escaping
	if strings.Contains(msg, "<special>") || strings.Contains(msg, "<low>") {
		t.Errorf("found unescaped HTML tags in message: %s", msg)
	}
	if !strings.Contains(msg, "&amp;") {
		t.Errorf("ampersand should be escaped to &amp;, got: %s", msg)
	}
	if !strings.Contains(msg, "&lt;special&gt;") || !strings.Contains(msg, "&lt;low&gt;") {
		t.Errorf("brackets '<' and '>' should be escaped, got: %s", msg)
	}
	// Check that regime CHOP_RANGE is formatted correctly
	if !strings.Contains(msg, "CHOP_RANGE") {
		t.Errorf("missing regime CHOP_RANGE, got: %s", msg)
	}
}

func TestSignalNotification_Truncation(t *testing.T) {
	mockSvc := &mockNotificationService{}
	uc := usecase.NewSignalNotificationUsecase(mockSvc, nil)

	policy := usecase.MarketPolicy{Reason: "NORMAL"}
	summary := usecase.ScannerSummaryV3{ActiveRegime: "NORMAL"}

	// Construct exceptionally long reason (>4000 characters)
	longReason := strings.Repeat("AI reasoning detail is extremely long. ", 150)

	reqs := []usecase.SignalNotificationRequest{
		{
			Decision: usecase.FinalDecision{
				Symbol:          "BTCUSDT",
				Direction:       usecase.LONG,
				Playbook:        usecase.TREND_PULLBACK,
				Status:          usecase.FINAL_EXECUTE,
				IsExecutable:    true,
				Score:           8.5,
				EntryPrice:      50000,
				StopLoss:        48000,
				TakeProfit:      54000,
				AIConfidence:    "HIGH",
				StalenessStatus: "FRESH",
				Reason:          "Regular reason",
			},
			AuditResponse: dto.AIAuditResponse{
				Reason: longReason,
				Risk:   "Low risk",
			},
		},
	}

	uc.SendV3Signals(context.Background(), reqs, policy, summary)

	if len(mockSvc.signalMessages) != 1 {
		t.Fatalf("expected 1 signal message, got %d", len(mockSvc.signalMessages))
	}

	msg := mockSvc.signalMessages[0]
	if len(msg) > 3900 {
		t.Errorf("expected message to be truncated below 3900 characters, got length: %d", len(msg))
	}
	if !strings.Contains(msg, "...") {
		t.Errorf("expected message to contain truncation indicators '...', got: %s", msg)
	}
}

func TestSignalNotification_JournalUpdates(t *testing.T) {
	t.Run("Journal updated to SUCCESS on successful transmission", func(t *testing.T) {
		mockSvc := &mockNotificationService{}
		mockRepo := &mockNotificationStorageRepo{
			journal: []usecase.SignalJournal{
				{
					Symbol: "ETHUSDT",
					Status: usecase.MONITORING,
				},
			},
		}
		storageUC := usecase.NewStorageUsecase(mockRepo)
		uc := usecase.NewSignalNotificationUsecase(mockSvc, storageUC)

		policy := usecase.MarketPolicy{Reason: "NORMAL"}
		summary := usecase.ScannerSummaryV3{ActiveRegime: "NORMAL"}

		reqs := []usecase.SignalNotificationRequest{
			{
				Decision: usecase.FinalDecision{
					Symbol:          "ETHUSDT",
					Direction:       usecase.LONG,
					Playbook:        usecase.TREND_PULLBACK,
					Status:          usecase.FINAL_EXECUTE,
					IsExecutable:    true,
					Score:           9.0,
					EntryPrice:      3000,
					StopLoss:        2900,
					TakeProfit:      3200,
					AIConfidence:    "HIGH",
					StalenessStatus: "FRESH",
					Reason:          "Go long",
				},
			},
		}

		uc.SendV3Signals(context.Background(), reqs, policy, summary)

		if len(mockRepo.journal) != 1 {
			t.Fatalf("expected 1 journal entry, got %d", len(mockRepo.journal))
		}
		entry := mockRepo.journal[0]
		if entry.NotificationStatus != "SUCCESS" {
			t.Errorf("expected NotificationStatus to be SUCCESS, got: %s", entry.NotificationStatus)
		}
		if entry.NotificationError != "" {
			t.Errorf("expected empty NotificationError, got: %s", entry.NotificationError)
		}
		if entry.Status != usecase.MONITORING {
			t.Errorf("expected Status to remain MONITORING, got: %v", entry.Status)
		}
	})

	t.Run("Journal updated to FAILED on API error, token is redacted", func(t *testing.T) {
		// Mock Telegram API returning a 400 error containing token
		fakeToken := "123456:fake_token_containing_secrets_here"
		apiErr := &mockTelegramAPIError{
			statusCode:  400,
			errorCode:   400,
			description: "can't parse entities under token " + fakeToken,
		}
		mockSvc := &mockNotificationServiceWithAPIError{apiErr: apiErr}

		mockRepo := &mockNotificationStorageRepo{
			journal: []usecase.SignalJournal{
				{
					Symbol: "GENIUSUSDT",
					Status: usecase.MONITORING,
				},
			},
		}
		storageUC := usecase.NewStorageUsecase(mockRepo)
		uc := usecase.NewSignalNotificationUsecase(mockSvc, storageUC)

		policy := usecase.MarketPolicy{Reason: "NORMAL"}
		summary := usecase.ScannerSummaryV3{ActiveRegime: "NORMAL"}

		reqs := []usecase.SignalNotificationRequest{
			{
				Decision: usecase.FinalDecision{
					Symbol:          "GENIUSUSDT",
					Direction:       usecase.LONG,
					Playbook:        usecase.TREND_PULLBACK,
					Status:          usecase.FINAL_EXECUTE,
					IsExecutable:    true,
					Score:           9.0,
					EntryPrice:      1.0,
					StopLoss:        0.9,
					TakeProfit:      1.2,
					AIConfidence:    "HIGH",
					StalenessStatus: "FRESH",
					Reason:          "Breakout",
				},
			},
		}

		uc.SendV3Signals(context.Background(), reqs, policy, summary)

		if len(mockRepo.journal) != 1 {
			t.Fatalf("expected 1 journal entry, got %d", len(mockRepo.journal))
		}
		entry := mockRepo.journal[0]
		if entry.NotificationStatus != "FAILED" {
			t.Errorf("expected NotificationStatus to be FAILED, got: %s", entry.NotificationStatus)
		}
		if entry.Status != usecase.MONITORING {
			t.Errorf("expected Status to remain MONITORING, got: %v", entry.Status)
		}
		// Verify sanitization
		if strings.Contains(entry.NotificationError, fakeToken) {
			t.Errorf("leaked token in NotificationError: %s", entry.NotificationError)
		}
		if !strings.Contains(entry.NotificationError, "[REDACTED_TOKEN]") {
			t.Errorf("expected redacted token in NotificationError, got: %s", entry.NotificationError)
		}
	})
}
