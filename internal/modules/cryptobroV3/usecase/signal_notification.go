package usecase

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"regexp"
	"strings"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

type SignalNotificationUsecase struct {
	notifier       SignalNotificationService
	storageUsecase *StorageUsecase
}

func NewSignalNotificationUsecase(notifier SignalNotificationService, storageUsecase *StorageUsecase) *SignalNotificationUsecase {
	return &SignalNotificationUsecase{
		notifier:       notifier,
		storageUsecase: storageUsecase,
	}
}

// SendSignal broadcasts the signal to the external notifier (Telegram SIGNAL channel).
// Must verify that IsFinalExecute is true.
func (uc *SignalNotificationUsecase) SendSignal(ctx context.Context, signal dto.SignalResponse) error {
	if !signal.IsFinalExecute {
		return fmt.Errorf("refusing to send signal: IsFinalExecute must be true")
	}
	msg := fmt.Sprintf(
		"<b>[CRYPTOBRO V3 SIGNAL]</b>\n\n"+
			"<b>Symbol:</b> %s\n"+
			"<b>Strategy:</b> %s\n"+
			"<b>Direction:</b> %s\n"+
			"<b>Score:</b> %.2f\n"+
			"<b>Entry:</b> %s\n"+
			"<b>SL:</b> %s\n"+
			"<b>TP:</b> %s\n"+
			"<b>Time:</b> %s\n\n"+
			"<b>Mode:</b> Alert-only, manual execution.",
		html.EscapeString(signal.Symbol),
		html.EscapeString(signal.Strategy),
		html.EscapeString(signal.Direction),
		signal.Score,
		formatPrice(signal.TriggerPrice),
		formatPrice(signal.StopLoss),
		formatPrice(signal.TakeProfit),
		html.EscapeString(FormatNotificationTime(signal.ReconciledTime)),
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

		escapedRegime := html.EscapeString(regimeUpper)
		if isHighRiskRegime {
			escapedRegime = fmt.Sprintf("%s ⚠️ <b>[HIGH RISK]</b>", escapedRegime)
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

		// Escape all dynamic strings
		escapedSymbol := html.EscapeString(string(dec.Symbol))
		escapedDirection := html.EscapeString(string(dec.Direction))
		escapedPlaybook := html.EscapeString(string(dec.Playbook))
		escapedGrade := html.EscapeString(grade)
		escapedPolicySummary := html.EscapeString(dec.PolicySummary)
		escapedThresholdProfileSummary := html.EscapeString(dec.ThresholdProfileSummary)
		escapedSentiment := html.EscapeString(audit.Sentiment)
		escapedStaleness := html.EscapeString(dec.StalenessStatus)
		escapedTime := html.EscapeString(FormatNotificationTime(summary.StartTime))

		entryStr := formatPrice(dec.EntryPrice)
		slStr := formatPrice(dec.StopLoss)
		tp1Str := formatPrice(tp1)
		tp2Str := formatPrice(tp2)
		scoreStr := fmt.Sprintf("%.2f", dec.Score)
		reqScoreStr := fmt.Sprintf("%.2f", dec.RequiredScore)
		rrStr := fmt.Sprintf("%.2f", dec.RR)
		reqRRStr := fmt.Sprintf("%.2f", dec.RequiredRR)

		// Truncate raw fields first if message exceeds 3900 characters
		rawReason := aiReason
		rawRisk := aiRisk

		buildMsgWithRaw := func(reason, risk string) string {
			escapedR := html.EscapeString(reason)
			escapedK := html.EscapeString(risk)
			return fmt.Sprintf(
				"🟣 <b>CRYPTOBRO V3 SIGNAL</b>\n"+
					"<b>FINAL_EXECUTE</b>\n\n"+
					"<pre>\n"+
					"Symbol      : %s\n"+
					"Direction   : %s\n"+
					"Playbook    : %s\n"+
					"Grade/Score : %s / %s (Req: %s)\n"+
					"Regime      : %s\n"+
					"Staleness   : %s\n"+
					"AI          : %s (HIGH)\n\n"+
					"Entry       : %s\n"+
					"SL          : %s\n"+
					"TP1         : %s\n"+
					"TP2         : %s\n"+
					"RR          : %s (Req: %s)\n\n"+
					"Policy      : %s\n"+
					"Profile     : %s\n"+
					"Time        : %s\n"+
					"</pre>\n\n"+
					"<b>AI Reason</b>\n%s\n\n"+
					"<b>AI Risk</b>\n%s\n\n"+
					"<i>Alert-only. Manual execution.</i>",
				escapedSymbol,
				escapedDirection,
				escapedPlaybook,
				escapedGrade,
				scoreStr,
				reqScoreStr,
				escapedRegime,
				escapedStaleness,
				escapedSentiment,
				entryStr,
				slStr,
				tp1Str,
				tp2Str,
				rrStr,
				reqRRStr,
				escapedPolicySummary,
				escapedThresholdProfileSummary,
				escapedTime,
				escapedR,
				escapedK,
			)
		}

		message := buildMsgWithRaw(rawReason, rawRisk)
		if len(message) > 3900 {
			if len(rawReason) > 1000 {
				rawReason = rawReason[:1000] + "..."
			}
			if len(rawRisk) > 500 {
				rawRisk = rawRisk[:500] + "..."
			}
			message = buildMsgWithRaw(rawReason, rawRisk)
			if len(message) > 3900 {
				if len(rawReason) > 200 {
					rawReason = rawReason[:200] + "..."
				}
				if len(rawRisk) > 100 {
					rawRisk = rawRisk[:100] + "..."
				}
				message = buildMsgWithRaw(rawReason, rawRisk)
			}
		}

		err := uc.notifier.SendSignalMessage(ctx, message)
		if err != nil {
			var apiErr interface {
				GetStatusCode() int
				GetErrorCode() int
				GetDescription() string
			}
			if errors.As(err, &apiErr) && apiErr.GetStatusCode() == 400 {
				slog.Error("Telegram API 400 Error",
					"status_code", apiErr.GetStatusCode(),
					"telegram_error_code", apiErr.GetErrorCode(),
					"telegram_description", apiErr.GetDescription(),
					"message_length", len(message),
					"parse_mode", "HTML",
					"symbol", dec.Symbol,
					"final_status", string(dec.Status),
				)
			} else {
				slog.Error("failed to send Telegram SIGNAL message",
					"symbol", dec.Symbol,
					"error", sanitizeError(err.Error()),
				)
			}
			GetGlobalMetrics().IncrementTelegramFail()

			// Update journal as FAILED
			if uc.storageUsecase != nil {
				_ = uc.storageUsecase.UpdateSignalJournal(func(journal []SignalJournal) ([]SignalJournal, error) {
					for i := len(journal) - 1; i >= 0; i-- {
						if journal[i].Symbol == dec.Symbol && journal[i].Status == MONITORING && journal[i].NotificationStatus == "" {
							journal[i].NotificationStatus = "FAILED"
							journal[i].NotificationError = sanitizeError(err.Error())
							break
						}
					}
					return journal, nil
				})
			}
		} else {
			GetGlobalMetrics().IncrementTelegramSuccess()

			// Update journal as SUCCESS
			if uc.storageUsecase != nil {
				_ = uc.storageUsecase.UpdateSignalJournal(func(journal []SignalJournal) ([]SignalJournal, error) {
					for i := len(journal) - 1; i >= 0; i-- {
						if journal[i].Symbol == dec.Symbol && journal[i].Status == MONITORING && journal[i].NotificationStatus == "" {
							journal[i].NotificationStatus = "SUCCESS"
							journal[i].NotificationError = ""
							break
						}
					}
					return journal, nil
				})
			}
		}
	}
}

var botTokenRegex = regexp.MustCompile(`\d+:[a-zA-Z0-9_-]+`)

func sanitizeError(errStr string) string {
	return botTokenRegex.ReplaceAllString(errStr, "[REDACTED_TOKEN]")
}

func formatPrice(v float64) string {
	if v == 0 {
		return "0.00"
	}
	if v < 0.0001 {
		return fmt.Sprintf("%.8f", v)
	}
	if v < 1.0 {
		return fmt.Sprintf("%.6f", v)
	}
	return fmt.Sprintf("%.4f", v)
}
