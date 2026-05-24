package usecase

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"strings"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/entity"
)

type OpsNotificationUsecase struct {
	notifier OpsNotificationService
}

func NewOpsNotificationUsecase(notifier OpsNotificationService) *OpsNotificationUsecase {
	return &OpsNotificationUsecase{
		notifier: notifier,
	}
}

func escapeTelegramHTML(value string) string {
	return html.EscapeString(value)
}

func FormatBootStatus(appName, env, version, httpPort string, alertOnly, binanceReadOnly, scanEnabled, monitoringEnabled bool) string {
	atStr := FormatNotificationTime(time.Now())
	return fmt.Sprintf(
		"ℹ️ <b>CRYPTOBRO V3 OPS — NON-SIGNAL</b>\n\n"+
			"🚀 <b>Boot</b>\n\n"+
			"<pre>\n"+
			"App       : %s\n"+
			"Env       : %s\n"+
			"Version   : %s\n"+
			"Port      : %s\n"+
			"AlertOnly : %v\n"+
			"ReadOnly  : %v\n"+
			"Scan      : %v\n"+
			"Monitor   : %v\n"+
			"At        : %s\n"+
			"</pre>\n\n"+
			"<i>No trade action. Informational status only.</i>",
		escapeTelegramHTML(appName),
		escapeTelegramHTML(env),
		escapeTelegramHTML(version),
		escapeTelegramHTML(httpPort),
		alertOnly,
		binanceReadOnly,
		scanEnabled,
		monitoringEnabled,
		escapeTelegramHTML(atStr),
	)
}

func FormatOpsScanStarted(scanID string, boundary time.Time, mode string) string {
	boundaryStr := FormatNotificationTime(boundary)
	startedStr := FormatNotificationTime(time.Now())
	return fmt.Sprintf(
		"🟡 <b>CRYPTOBRO V3 OPS — NON-SIGNAL</b>\n\n"+
			"📡 <b>Scan Started</b>\n\n"+
			"<pre>\n"+
			"Scan ID   : %s\n"+
			"Mode      : %s\n"+
			"Boundary  : %s\n"+
			"Started   : %s\n"+
			"Regime    : Not evaluated yet\n"+
			"</pre>\n\n"+
			"<i>No trade action. Informational status only.</i>",
		escapeTelegramHTML(scanID),
		escapeTelegramHTML(mode),
		escapeTelegramHTML(boundaryStr),
		escapeTelegramHTML(startedStr),
	)
}

func FormatOpsScanDone(latest *entity.LatestResult) string {
	if latest == nil {
		return ""
	}
	durationMs := int64(-1)
	if d, err := time.ParseDuration(latest.Duration); err == nil {
		durationMs = d.Milliseconds()
	}
	durationStr := "N/A"
	if durationMs >= 0 {
		durationStr = fmt.Sprintf("%d ms", durationMs)
	}

	generatedStr := FormatNotificationTime(latest.GeneratedAt)

	return fmt.Sprintf(
		"🟢 <b>CRYPTOBRO V3 OPS — NON-SIGNAL</b>\n\n"+
			"✅ <b>Scan Done</b>\n\n"+
			"<pre>\n"+
			"Scan ID      : %s\n"+
			"Generated    : %s\n"+
			"Regime       : %s\n\n"+
			"Universe     : %d\n"+
			"Eligible     : %d\n"+
			"AI Candidate : %d\n"+
			"Execute      : %d\n"+
			"Watch        : %d\n"+
			"Reject       : %d\n"+
			"AI Error     : %d\n\n"+
			"Duration     : %s\n"+
			"</pre>\n\n"+
			"<i>No trade action. Informational status only.</i>",
		escapeTelegramHTML(latest.ScanID),
		escapeTelegramHTML(generatedStr),
		escapeTelegramHTML(latest.MarketRegime),
		latest.TotalUniversePass,
		latest.TotalPlaybookEligible,
		latest.TotalLocalAICandidate,
		latest.TotalFinalExecute,
		latest.TotalFinalWatch,
		latest.TotalFinalReject,
		latest.TotalAIError,
		escapeTelegramHTML(durationStr),
	)
}

func FormatOpsScanFailed(scanID string, boundary time.Time, err error) string {
	errText := "Unknown error"
	if err != nil {
		errText = sanitizeErr(err.Error())
	}
	boundaryStr := FormatNotificationTime(boundary)
	failedStr := FormatNotificationTime(time.Now())
	return fmt.Sprintf(
		"🔴 <b>CRYPTOBRO V3 OPS — NON-SIGNAL</b>\n\n"+
			"⚠️ <b>Scan Failed</b>\n\n"+
			"<pre>\n"+
			"Scan ID   : %s\n"+
			"Boundary  : %s\n"+
			"Failed    : %s\n"+
			"Error     : %s\n"+
			"</pre>\n\n"+
			"<i>No trade action. Informational status only.</i>",
		escapeTelegramHTML(scanID),
		escapeTelegramHTML(boundaryStr),
		escapeTelegramHTML(failedStr),
		escapeTelegramHTML(errText),
	)
}

func FormatAdminWarning(scanID, symbol, playbook, finalStatus, reason string) string {
	atStr := FormatNotificationTime(time.Now())
	return fmt.Sprintf(
		"⚠️ <b>CRYPTOBRO V3 OPS — NON-SIGNAL</b>\n\n"+
			"❌ <b>Admin Warning</b>\n\n"+
			"<pre>\n"+
			"Type      : AI_ERROR\n"+
			"Scan ID   : %s\n"+
			"Symbol    : %s\n"+
			"Playbook  : %s\n"+
			"Status    : %s\n"+
			"Reason    : %s\n"+
			"At        : %s\n"+
			"</pre>\n\n"+
			"<i>No trade action. Informational status only.</i>",
		escapeTelegramHTML(scanID),
		escapeTelegramHTML(symbol),
		escapeTelegramHTML(playbook),
		escapeTelegramHTML(finalStatus),
		escapeTelegramHTML(sanitizeErr(reason)),
		escapeTelegramHTML(atStr),
	)
}

func (uc *OpsNotificationUsecase) SendBootStatus(ctx context.Context, appName, env, version, httpPort string, alertOnly, binanceReadOnly, scanEnabled, monitoringEnabled bool) {
	msg := FormatBootStatus(appName, env, version, httpPort, alertOnly, binanceReadOnly, scanEnabled, monitoringEnabled)
	uc.send(ctx, msg)
}

func (uc *OpsNotificationUsecase) SendScanStarted(ctx context.Context, scanID string, boundary time.Time, mode string) {
	msg := FormatOpsScanStarted(scanID, boundary, mode)
	uc.send(ctx, msg)
}

func (uc *OpsNotificationUsecase) SendScanDone(ctx context.Context, latest *entity.LatestResult) {
	msg := FormatOpsScanDone(latest)
	if msg != "" {
		uc.send(ctx, msg)
	}
}

func (uc *OpsNotificationUsecase) SendScanFailed(ctx context.Context, scanID string, boundary time.Time, err error) {
	msg := FormatOpsScanFailed(scanID, boundary, err)
	uc.send(ctx, msg)
}

func (uc *OpsNotificationUsecase) SendAdminWarningAIError(ctx context.Context, scanID, symbol, playbook, finalStatus, reason string) {
	msg := FormatAdminWarning(scanID, symbol, playbook, finalStatus, reason)
	uc.send(ctx, msg)
}

func (uc *OpsNotificationUsecase) send(ctx context.Context, msg string) {
	if uc == nil || uc.notifier == nil {
		return
	}
	if err := uc.notifier.SendOpsMessage(ctx, msg); err != nil {
		slog.Error("failed to send Telegram OPS message", "error", err)
		GetGlobalMetrics().IncrementTelegramFail()
		return
	}
	GetGlobalMetrics().IncrementTelegramSuccess()
}

func sanitizeErr(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.TrimSpace(s)
	if len(s) > 300 {
		return s[:300] + "…"
	}
	return s
}
