package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/entity"
)

const opsPrefix = "[CRYPTOBRO V3 OPS][NON-SIGNAL]"

func opsFooter() string {
	return "No trade action. Informational status only."
}

type OpsNotificationUsecase struct {
	notifier OpsNotificationService
}

func NewOpsNotificationUsecase(notifier OpsNotificationService) *OpsNotificationUsecase {
	return &OpsNotificationUsecase{
		notifier: notifier,
	}
}

func (uc *OpsNotificationUsecase) SendBootStatus(ctx context.Context, appName, env, version, httpPort string, alertOnly, binanceReadOnly, scanEnabled, monitoringEnabled bool) {
	msg := fmt.Sprintf(
		"%s\n\n"+
			"BOOT\n"+
			"app=%s\n"+
			"env=%s\n"+
			"version=%s\n"+
			"http_port=%s\n"+
			"alert_only=%v\n"+
			"binance_read_only=%v\n"+
			"scan_enabled=%v\n"+
			"monitoring_enabled=%v\n"+
			"at=%s\n\n"+
			"%s",
		opsPrefix,
		appName,
		env,
		version,
		httpPort,
		alertOnly,
		binanceReadOnly,
		scanEnabled,
		monitoringEnabled,
		time.Now().Format(time.RFC3339),
		opsFooter(),
	)
	uc.send(ctx, msg)
}

func (uc *OpsNotificationUsecase) SendScanStarted(ctx context.Context, scanID string, boundary time.Time, mode string) {
	msg := fmt.Sprintf(
		"%s\n\n"+
			"SCAN_STARTED\n"+
			"scan_id=%s\n"+
			"mode=%s\n"+
			"boundary=%s\n"+
			"started_at=%s\n"+
			"market_regime=not evaluated yet\n\n"+
			"%s",
		opsPrefix,
		scanID,
		mode,
		boundary.Format(time.RFC3339),
		time.Now().Format(time.RFC3339),
		opsFooter(),
	)
	uc.send(ctx, msg)
}

func (uc *OpsNotificationUsecase) SendScanDone(ctx context.Context, latest *entity.LatestResult) {
	if latest == nil {
		return
	}
	durationMs := int64(-1)
	if d, err := time.ParseDuration(latest.Duration); err == nil {
		durationMs = d.Milliseconds()
	}

	msg := fmt.Sprintf(
		"%s\n\n"+
			"SCAN_DONE\n"+
			"scan_id=%s\n"+
			"generated_at=%s\n"+
			"market_regime=%s\n"+
			"total_universe_pass=%d\n"+
			"total_playbook_eligible=%d\n"+
			"total_ai_candidate=%d\n"+
			"total_final_execute=%d\n"+
			"total_final_watch=%d\n"+
			"total_final_reject=%d\n"+
			"total_ai_error=%d\n"+
			"duration_ms=%d\n\n"+
			"%s",
		opsPrefix,
		latest.ScanID,
		latest.GeneratedAt.Format(time.RFC3339),
		latest.MarketRegime,
		latest.TotalUniversePass,
		latest.TotalPlaybookEligible,
		latest.TotalLocalAICandidate,
		latest.TotalFinalExecute,
		latest.TotalFinalWatch,
		latest.TotalFinalReject,
		latest.TotalAIError,
		durationMs,
		opsFooter(),
	)
	uc.send(ctx, msg)
}

func (uc *OpsNotificationUsecase) SendScanFailed(ctx context.Context, scanID string, boundary time.Time, err error) {
	errText := ""
	if err != nil {
		errText = sanitizeErr(err.Error())
	}
	msg := fmt.Sprintf(
		"%s\n\n"+
			"SCAN_FAILED\n"+
			"scan_id=%s\n"+
			"boundary=%s\n"+
			"failed_at=%s\n"+
			"error=%s\n\n"+
			"%s",
		opsPrefix,
		scanID,
		boundary.Format(time.RFC3339),
		time.Now().Format(time.RFC3339),
		errText,
		opsFooter(),
	)
	uc.send(ctx, msg)
}

func (uc *OpsNotificationUsecase) SendAdminWarningAIError(ctx context.Context, scanID, symbol, playbook, finalStatus, reason string) {
	msg := fmt.Sprintf(
		"%s\n\n"+
			"ADMIN_WARNING\n"+
			"type=AI_ERROR\n"+
			"scan_id=%s\n"+
			"symbol=%s\n"+
			"playbook=%s\n"+
			"final_status=%s\n"+
			"reason=%s\n"+
			"at=%s\n\n"+
			"%s",
		opsPrefix,
		scanID,
		symbol,
		playbook,
		finalStatus,
		sanitizeErr(reason),
		time.Now().Format(time.RFC3339),
		opsFooter(),
	)
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
