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
