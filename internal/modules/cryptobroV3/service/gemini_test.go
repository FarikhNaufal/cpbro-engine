package service

import (
	"strings"
	"testing"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

func TestFormatCompactCandles(t *testing.T) {
	candles := []dto.Candle{
		{
			Time:  time.Date(2026, 5, 25, 8, 45, 0, 0, time.UTC),
			Open:  1.234,
			High:  1.245,
			Low:   1.221,
			Close: 1.239,
			Vol:   123456.78,
		},
	}

	result := formatCompactCandles(candles, 1)

	// Expected string:
	// "[2026-05-25T08:45:00Z | 1779698700000] O=1.23400 H=1.24500 L=1.22100 C=1.23900 V=123456.78"
	if !strings.Contains(result, "2026-05-25T08:45:00Z") {
		t.Errorf("Expected result to contain UTC RFC3339 timestamp '2026-05-25T08:45:00Z', got: %s", result)
	}

	if !strings.Contains(result, "1779698700000") {
		t.Errorf("Expected result to contain open_time_ms '1779698700000', got: %s", result)
	}

	if strings.Contains(result, "08:45") && !strings.Contains(result, "T") {
		t.Errorf("Expected RFC3339 format, but got raw HH:MM format: %s", result)
	}
}

func TestValidateAuditResponseRejectsInconsistentExecuteAction(t *testing.T) {
	res := validAuditResponseForTest()
	res.Confidence = "MEDIUM"

	err := validateAuditResponse(res)
	if err == nil {
		t.Fatalf("expected inconsistent EXECUTE_IF_NOT_STALE response to be rejected")
	}
	if !strings.Contains(err.Error(), "EXECUTE_IF_NOT_STALE requires HIGH confidence") {
		t.Fatalf("expected HIGH confidence consistency error, got %v", err)
	}
}

func TestValidateAuditResponseRejectsWaitWithExecuteAction(t *testing.T) {
	res := validAuditResponseForTest()
	res.Decision = "WAIT"

	err := validateAuditResponse(res)
	if err == nil {
		t.Fatalf("expected WAIT + EXECUTE_IF_NOT_STALE response to be rejected")
	}
	if !strings.Contains(err.Error(), "decision WAIT requires WAIT_RETEST or WATCH_ONLY") {
		t.Fatalf("expected WAIT consistency error, got %v", err)
	}
}

func TestValidateAuditResponseAcceptsConservativeWait(t *testing.T) {
	res := validAuditResponseForTest()
	res.Decision = "WAIT"
	res.Confidence = "MEDIUM"
	res.EntryTiming = "LATE"
	res.SuggestedAction = "WAIT_RETEST"
	res.HasConfirmation = false
	res.CandleNarrative = "CHOP"

	if err := validateAuditResponse(res); err != nil {
		t.Fatalf("expected conservative WAIT response to be accepted, got %v", err)
	}
}

func validAuditResponseForTest() dto.AIAuditResponse {
	return dto.AIAuditResponse{
		Decision:         "CONFIRM",
		Confidence:       "HIGH",
		CandleNarrative:  "CONTINUATION",
		Last5CandlesBias: "BULLISH",
		HasRejection:     true,
		HasConfirmation:  true,
		EntryTiming:      "FRESH",
		ConflictWithBot:  false,
		SuggestedAction:  "EXECUTE_IF_NOT_STALE",
		PlanFeedback:     "Plan is aligned with closed candles",
		Reason:           "Confirmation candle supports direction",
		Risk:             "Normal execution risk",
	}
}
