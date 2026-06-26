package usecase

import (
	"fmt"
	"strings"
)

const watchRecheckPromotionPrefix = "WATCH_RECHECK_PROMOTION"

func buildWatchRecheckPromotionReason(originID, signalID, finalReason string) string {
	return fmt.Sprintf("%s origin_watch_id=%s promoted_signal_id=%s | %s", watchRecheckPromotionPrefix, originID, signalID, finalReason)
}

func buildWatchRecheckPromotionOutcome(signalID string) string {
	return fmt.Sprintf("Watch closed by secondary recheck promotion into executable signal %s", signalID)
}

func buildWatchRecheckAuditContext(originID, finalReason string) string {
	return fmt.Sprintf("WATCH_RECHECK_CONTEXT origin_watch_id=%s | %s", originID, finalReason)
}

func isWatchRecheckPromotionReason(reason string) bool {
	return strings.Contains(strings.ToUpper(strings.TrimSpace(reason)), watchRecheckPromotionPrefix)
}
