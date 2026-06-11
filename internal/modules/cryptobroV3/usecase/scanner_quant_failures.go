package usecase

import "strings"

func isArbiterReadyQuantResult(quant QuantResult) bool {
	if !quant.IndicatorMet || quant.Status == PLAYBOOK_REJECTED {
		return false
	}

	return quant.Direction == LONG || quant.Direction == SHORT
}

func quantFailureReason(quant QuantResult) string {
	reason := strings.TrimSpace(quant.Reason)
	if reason != "" {
		return reason
	}

	if quant.Status == PLAYBOOK_REJECTED {
		return "Quant engine rejected candidate"
	}

	if quant.Direction != LONG && quant.Direction != SHORT {
		return "Invalid direction returned by quant engine"
	}

	return "Quant engine candidate failed validation"
}
