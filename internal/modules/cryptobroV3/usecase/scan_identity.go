package usecase

import (
	"strings"
	"time"
)

const (
	scanTriggerStartup   = "startup"
	scanTriggerScheduled = "scheduled"
	scanTriggerManual    = "manual"
	scanTriggerRecheck   = "recheck"
)

func normalizeScanTriggerSource(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case scanTriggerStartup:
		return scanTriggerStartup
	case scanTriggerScheduled:
		return scanTriggerScheduled
	case scanTriggerRecheck:
		return scanTriggerRecheck
	case scanTriggerManual:
		return scanTriggerManual
	default:
		return scanTriggerManual
	}
}

func buildScanID(triggerSource string, boundary time.Time) string {
	return normalizeScanTriggerSource(triggerSource) + "_" + boundary.Format("20060102150405")
}

func BuildScanIDForExternal(triggerSource string, boundary time.Time) string {
	return buildScanID(triggerSource, boundary)
}
