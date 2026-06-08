package usecase

import (
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultMonitoringMaxHoldMinutes = 120

func getMonitoringMaxHoldMinutes() int {
	raw := strings.TrimSpace(os.Getenv("MONITORING_MAX_HOLD_MINUTES"))
	if raw == "" {
		return defaultMonitoringMaxHoldMinutes
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return defaultMonitoringMaxHoldMinutes
	}
	return parsed
}

func getMonitoringMaxHoldDuration() time.Duration {
	return time.Duration(getMonitoringMaxHoldMinutes()) * time.Minute
}

func getMonitoringMaxHoldLabel() string {
	return strconv.Itoa(getMonitoringMaxHoldMinutes()) + "m"
}

func getRequireAIHighForExecute() bool {
	raw := strings.TrimSpace(os.Getenv("REQUIRE_AI_HIGH_FOR_EXECUTE"))
	if raw == "" {
		return true
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return true
	}
	return parsed
}

func getRequireFreshEntryForExecute() bool {
	raw := strings.TrimSpace(os.Getenv("REQUIRE_FRESH_ENTRY_FOR_EXECUTE"))
	if raw == "" {
		return true
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return true
	}
	return parsed
}

func effectiveRequiredAIConfidence(policy MarketPolicy, profile PlaybookThresholdProfile) AIConfidence {
	required := policy.RequireAIConfidence
	if required == "" {
		required = AIConfidenceMedium
	}
	if profile.RequireAIHigh || getRequireAIHighForExecute() {
		return AIConfidenceHigh
	}
	return required
}

func effectiveRequireFreshEntry(policy MarketPolicy) bool {
	return policy.RequireFreshEntry || getRequireFreshEntryForExecute()
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
