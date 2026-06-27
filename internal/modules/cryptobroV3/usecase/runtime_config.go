package usecase

import (
	"strconv"
	"strings"
	"time"
)

const (
	defaultMonitoringMaxHoldMinutes = 120
	safetyADXExpansionCeiling       = 30.0
	defaultScanBoundaryMinutes      = 15
	defaultClosedCandleTimeframe    = 15 * time.Minute
	defaultClosedCandleCacheTTL     = 15 * time.Second
	defaultOpenInterestCacheTTL     = 30 * time.Second
	closedCandleAvailabilityFactor  = 2
)

func getMonitoringMaxHoldMinutes() int {
	minutes := getRuntimeSettings().MonitoringMaxHoldMinutes
	if minutes <= 0 {
		return defaultMonitoringMaxHoldMinutes
	}
	return minutes
}

func getMonitoringMaxHoldDuration() time.Duration {
	return time.Duration(getMonitoringMaxHoldMinutes()) * time.Minute
}

func getClosedCandleFreshnessDuration() time.Duration {
	boundaryMinutes := getRuntimeSettings().ScanBoundaryMinutes
	if boundaryMinutes <= 0 {
		boundaryMinutes = defaultScanBoundaryMinutes
	}
	return time.Duration(boundaryMinutes*closedCandleAvailabilityFactor) * time.Minute
}

func GetClosedCandleFreshnessDurationForExternal() time.Duration {
	return getClosedCandleFreshnessDuration()
}

func getMonitoringMaxHoldLabel() string {
	return strconv.Itoa(getMonitoringMaxHoldMinutes()) + "m"
}

func getRequireAIHighForExecute() bool {
	return getRuntimeSettings().RequireAIHighForExecute
}

func getRequireFreshEntryForExecute() bool {
	return getRuntimeSettings().RequireFreshEntryForExecute
}

func effectiveRequiredAIConfidence(policy MarketPolicy, profile PlaybookThresholdProfile) AIConfidence {
	required := policy.RequireAIConfidence
	if required == "" {
		required = AIConfidenceHigh
	}
	if profile.RequireAIHigh {
		return AIConfidenceHigh
	}
	return required
}

func effectiveRequiredAIConfidenceForPolicy(policy MarketPolicy) AIConfidence {
	required := policy.RequireAIConfidence
	if required == "" {
		required = AIConfidenceHigh
	}
	return required
}

func effectiveRequireFreshEntry(policy MarketPolicy) bool {
	return policy.RequireFreshEntry
}

func aiConfidenceRank(confidence string) int {
	switch strings.ToUpper(strings.TrimSpace(confidence)) {
	case string(AIConfidenceHigh):
		return 3
	case string(AIConfidenceMedium):
		return 2
	case string(AIConfidenceLow):
		return 1
	default:
		return 0
	}
}

func meetsRequiredAIConfidence(actual string, required AIConfidence) bool {
	requiredRank := aiConfidenceRank(string(required))
	if requiredRank == 0 {
		requiredRank = aiConfidenceRank(string(AIConfidenceMedium))
	}
	return aiConfidenceRank(actual) >= requiredRank
}
