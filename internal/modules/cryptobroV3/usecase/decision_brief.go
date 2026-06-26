package usecase

import "strings"

func buildDecisionBrief(audit DecisionAudit) string {
	parts := make([]string, 0, 6)

	if audit.FinalStatus != "" {
		parts = append(parts, string(audit.FinalStatus))
	}
	if audit.Playbook != "" {
		parts = append(parts, string(audit.Playbook))
	}
	if audit.FinalPrimaryReasonLayer != "" {
		parts = append(parts, "layer="+audit.FinalPrimaryReasonLayer)
	}
	if audit.AIDecision != "" {
		if audit.AIConfidence != "" {
			parts = append(parts, "ai="+audit.AIDecision+"/"+audit.AIConfidence)
		} else {
			parts = append(parts, "ai="+audit.AIDecision)
		}
	}
	if audit.StalenessStatus != "" {
		parts = append(parts, "stale="+audit.StalenessStatus)
	}
	if audit.M5ConfirmationUsed && audit.M5ConfirmationStatus != "" {
		parts = append(parts, "m5="+audit.M5ConfirmationStatus)
	}

	reason := strings.TrimSpace(audit.FinalReason)
	if reason == "" {
		reason = strings.TrimSpace(audit.RejectOrWatchReason)
	}
	if reason != "" {
		parts = append(parts, "reason="+reason)
	}

	return strings.Join(parts, " | ")
}
