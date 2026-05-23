package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

type SignalNotificationUsecase struct {
	notifier SignalNotificationService
}

func NewSignalNotificationUsecase(notifier SignalNotificationService) *SignalNotificationUsecase {
	return &SignalNotificationUsecase{
		notifier: notifier,
	}
}

// SendSignal broadcasts the signal to the external notifier (Telegram SIGNAL channel).
// Must verify that IsFinalExecute is true.
func (uc *SignalNotificationUsecase) SendSignal(ctx context.Context, signal dto.SignalResponse) error {
	if !signal.IsFinalExecute {
		return fmt.Errorf("refusing to send signal: IsFinalExecute must be true")
	}
	msg := fmt.Sprintf(
		"[CRYPTOBRO V3 SIGNAL]\n\n"+
			"*Symbol:* %s\n"+
			"*Strategy:* %s\n"+
			"*Direction:* %s\n"+
			"*Score:* %.2f\n"+
			"*Entry:* %.4f\n"+
			"*SL:* %.4f\n"+
			"*TP:* %.4f\n\n"+
			"*Mode:* Alert-only, manual execution.",
		signal.Symbol,
		signal.Strategy,
		signal.Direction,
		signal.Score,
		signal.TriggerPrice,
		signal.StopLoss,
		signal.TakeProfit,
	)
	err := uc.notifier.SendSignalMessage(ctx, msg)
	if err != nil {
		GetGlobalMetrics().IncrementTelegramFail()
	} else {
		GetGlobalMetrics().IncrementTelegramSuccess()
	}
	return err
}

// SendV3Signals transmits ONLY actionable FINAL_EXECUTE signals (post FinalGate + ConflictResolver).
// It must never transmit FINAL_WATCH, FINAL_REJECT, or AI_ERROR_REVIEW.
func (uc *SignalNotificationUsecase) SendV3Signals(
	ctx context.Context,
	reqs []SignalNotificationRequest,
	policy MarketPolicy,
	summary ScannerSummaryV3,
) {
	for _, req := range reqs {
		dec := req.Decision
		audit := req.AuditResponse

		// Strict execution-only gate for SIGNAL channel.
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

		// SIGNAL payload. Contains entry/SL/TP and is the only actionable channel.
		message := fmt.Sprintf(
			"[CRYPTOBRO V3 SIGNAL]\n\n"+
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
				"*Staleness:* %s\n\n"+
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
		)

		if err := uc.notifier.SendSignalMessage(ctx, message); err != nil {
			slog.Error("failed to send Telegram SIGNAL message", "symbol", dec.Symbol, "error", err)
			GetGlobalMetrics().IncrementTelegramFail()
		} else {
			GetGlobalMetrics().IncrementTelegramSuccess()
		}
	}
}
