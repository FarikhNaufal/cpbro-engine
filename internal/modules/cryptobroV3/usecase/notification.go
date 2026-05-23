package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

type NotificationUsecase struct {
	notifier NotificationService
}

func NewNotificationUsecase(notifier NotificationService) *NotificationUsecase {
	return &NotificationUsecase{
		notifier: notifier,
	}
}

// Send broadcasts the signal to the external notifier (Telegram).
// Must verify that IsFinalExecute is true.
func (uc *NotificationUsecase) Send(ctx context.Context, signal dto.SignalResponse) error {
	if !signal.IsFinalExecute {
		return fmt.Errorf("signal did not pass final gate validation")
	}
	err := uc.notifier.SendFinalExecuteAlert(ctx, signal)
	if err != nil {
		GetGlobalMetrics().IncrementTelegramFail()
	} else {
		GetGlobalMetrics().IncrementTelegramSuccess()
	}
	return err
}

// SendV3 processes final decisions, checks Telegram criteria, and dispatches messages and optional admin warnings.
func (uc *NotificationUsecase) SendV3(
	ctx context.Context,
	reqs []SignalNotificationRequest,
	policy MarketPolicy,
	summary ScannerSummaryV3,
) error {
	for _, req := range reqs {
		dec := req.Decision
		audit := req.AuditResponse

		// Optional admin warning for AI_ERROR_REVIEW status
		if dec.Status == Status("AI_ERROR_REVIEW") {
			adminMsg := fmt.Sprintf(
				"⚠️ *[ADMIN WARNING]*\n"+
					"*AI audit error for symbol:* %s\n"+
					"*Status:* AI_ERROR_REVIEW\n"+
					"*Reasoning:* %s",
				dec.Symbol,
				audit.Reasoning,
			)
			if err := uc.notifier.SendTelegramMessage(ctx, adminMsg); err != nil {
				slog.Error("failed to send Telegram admin warning message", "symbol", dec.Symbol, "error", err)
				GetGlobalMetrics().IncrementTelegramFail()
			} else {
				GetGlobalMetrics().IncrementTelegramSuccess()
			}
			continue
		}

		// Strictly check execution criteria:
		// - status == FINAL_EXECUTE
		// - IsExecutable == true
		// - StalenessStatus == FRESH
		// - AIConfidence == HIGH
		// - Reason is valid (not empty)
		// - TradePlan is valid (EntryPrice > 0, StopLoss > 0, TakeProfit > 0)
		if dec.Status != FINAL_EXECUTE || !dec.IsExecutable {
			continue
		}
		if strings.ToUpper(dec.StalenessStatus) != "FRESH" {
			continue
		}
		if strings.ToUpper(dec.AIConfidence) != "HIGH" {
			continue
		}
		if dec.Reason == "" {
			continue
		}
		if dec.EntryPrice <= 0 || dec.StopLoss <= 0 || dec.TakeProfit <= 0 {
			continue
		}

		// Calculate scaled take profit targets (TP1 & TP2)
		var tp1, tp2 float64
		tp2 = dec.TakeProfit
		if dec.Direction == LONG {
			profit := dec.TakeProfit - dec.EntryPrice
			tp1 = dec.EntryPrice + profit*0.5
		} else {
			profit := dec.EntryPrice - dec.TakeProfit
			tp1 = dec.EntryPrice - profit*0.5
		}

		// Determine if high risk regime warning is needed
		regimeUpper := strings.ToUpper(summary.ActiveRegime)
		if regimeUpper == "" {
			regimeUpper = strings.ToUpper(policy.Reason)
		}
		isHighRiskRegime := strings.Contains(regimeUpper, "CHAOS") ||
			strings.Contains(regimeUpper, "CHOP") ||
			strings.Contains(regimeUpper, "RISK_OFF")

		regimeText := regimeUpper
		if isHighRiskRegime {
			regimeText = fmt.Sprintf("%s ⚠️ *[HIGH RISK]*", regimeUpper)
		}

		grade := getGrade(dec.Score)

		aiReason := audit.Reason
		if aiReason == "" {
			aiReason = dec.Reason
		}

		aiRisk := audit.Risk
		if aiRisk == "" {
			aiRisk = "Standard regime risk level."
		}

		// Build Telegram signal message payload
		message := fmt.Sprintf(
			"🔔 *[CRYPTOBRO V3 SIGNAL]*\n\n"+
				"*Symbol:* %s\n"+
				"*Direction:* %s\n"+
				"*Playbook:* %s\n"+
				"*Grade/Score:* %s / %.2f (Req: %.2f)\n"+
				"*Market Regime:* %s\n"+
				"*Market Policy:* %s\n"+
				"*Threshold Profile:* %s\n"+
				"*Entry:* %.4f\n"+
				"*SL:* %.4f\n"+
				"*TP1:* %.4f\n"+
				"*TP2:* %.4f\n"+
				"*RR:* %.2f (Req: %.2f)\n"+
				"*AI Sentiment:* %s (HIGH)\n"+
				"*AI Reason:* %s\n"+
				"*AI Risk:* %s\n"+
				"*Staleness:* %s\n"+
				"*Invalidation:* Price breaks Stop Loss (%.4f)\n"+
				"*Max Hold:* 8 candle M15 / 120 menit\n\n"+
				"*Mode:* Alert-only, manual execution.",
			dec.Symbol,
			dec.Direction,
			dec.Playbook,
			grade,
			dec.Score,
			dec.RequiredScore,
			regimeText,
			dec.PolicySummary,
			dec.ThresholdProfileSummary,
			dec.EntryPrice,
			dec.StopLoss,
			tp1,
			tp2,
			dec.RR,
			dec.RequiredRR,
			audit.Sentiment,
			aiReason,
			aiRisk,
			dec.StalenessStatus,
			dec.StopLoss,
		)

		// Dispatch signal alert. Any transmission failure is logged and does NOT crash the scanner.
		if err := uc.notifier.SendTelegramMessage(ctx, message); err != nil {
			slog.Error("failed to send Telegram signal alert message", "symbol", dec.Symbol, "error", err)
			GetGlobalMetrics().IncrementTelegramFail()
		} else {
			GetGlobalMetrics().IncrementTelegramSuccess()
		}
	}

	return nil
}

// SendStatus sends an informational status message (startup, scan started, etc).
// It is intentionally best-effort and must never crash the app.
func (uc *NotificationUsecase) SendStatus(ctx context.Context, msg string) {
	if err := uc.notifier.SendTelegramMessage(ctx, msg); err != nil {
		slog.Error("failed to send Telegram status message", "error", err)
		GetGlobalMetrics().IncrementTelegramFail()
		return
	}
	GetGlobalMetrics().IncrementTelegramSuccess()
}
