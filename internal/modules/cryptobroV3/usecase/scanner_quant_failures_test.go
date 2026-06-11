package usecase

import "testing"

func TestIsArbiterReadyQuantResult(t *testing.T) {
	t.Run("valid long candidate reaches arbiter", func(t *testing.T) {
		quant := QuantResult{
			Direction:    LONG,
			Status:       QUANT_CANDIDATE,
			IndicatorMet: true,
		}

		if !isArbiterReadyQuantResult(quant) {
			t.Fatalf("expected LONG quant candidate to be arbiter-ready")
		}
	})

	t.Run("wait direction is filtered before arbiter", func(t *testing.T) {
		quant := QuantResult{
			Direction:    WAIT,
			Status:       PLAYBOOK_REJECTED,
			IndicatorMet: false,
			Reason:       "Liquidity sweep lacks volume spike confirmation",
		}

		if isArbiterReadyQuantResult(quant) {
			t.Fatalf("expected WAIT quant result to be filtered before arbiter")
		}
	})

	t.Run("playbook rejected short is filtered before arbiter", func(t *testing.T) {
		quant := QuantResult{
			Direction:    SHORT,
			Status:       PLAYBOOK_REJECTED,
			IndicatorMet: false,
			Reason:       "SHORT direction rejected by BTC bullish safety helper rules",
		}

		if isArbiterReadyQuantResult(quant) {
			t.Fatalf("expected playbook-rejected quant result to be filtered before arbiter")
		}
	})
}

func TestQuantFailureReason(t *testing.T) {
	t.Run("preserves explicit quant reason", func(t *testing.T) {
		quant := QuantResult{Reason: "Liquidity sweep lacks volume spike confirmation"}
		if got := quantFailureReason(quant); got != "Liquidity sweep lacks volume spike confirmation" {
			t.Fatalf("unexpected quant failure reason: %s", got)
		}
	})

	t.Run("falls back for invalid direction", func(t *testing.T) {
		quant := QuantResult{
			Direction:    WAIT,
			Status:       QUANT_CANDIDATE,
			IndicatorMet: true,
		}
		if got := quantFailureReason(quant); got != "Invalid direction returned by quant engine" {
			t.Fatalf("unexpected fallback reason: %s", got)
		}
	})
}
