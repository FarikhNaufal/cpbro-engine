package usecase

import (
	"context"
	"strings"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

type scanCandidateContext struct {
	quantResult     QuantResult
	auditResponse   dto.AIAuditResponse
	stalenessRes    StalenessResult
	localGateResult LocalGateResult
	planReview      PlanReview
	latestPrice     float64
	aiSkipped       bool
}

type selectedCandidateEvaluation struct {
	decision           FinalDecision
	context            scanCandidateContext
	syntheticLocalGate bool
	aiSkippedQuota     bool
	recordAIFunnel     bool
}

func (uc *ScannerUsecase) evaluateSelectedCandidate(
	ctx context.Context,
	qResult QuantResult,
	policy MarketPolicy,
	activeSignals []SignalJournal,
	historySignals []dto.SignalResponse,
	marketData MarketData,
	localGateResult LocalGateResult,
	auditResponse dto.AIAuditResponse,
	hasAuditedResponse bool,
) selectedCandidateEvaluation {
	pair := qResult.Symbol
	resolvedAudit := auditResponse
	aiSkipped := false
	syntheticLocalGate := false
	recordAIFunnel := false

	if !localGateResult.Passed {
		decision := "REJECT"
		reason := "LOCAL_GATE_FAILED"
		if localGateResult.Status == LOCAL_WATCH {
			decision = "WAIT"
			reason = "LOCAL_GATE_WATCH"
		}
		resolvedAudit = dto.AIAuditResponse{
			Symbol:     pair,
			IsApproved: false,
			Decision:   decision,
			Sentiment:  "NEUTRAL",
			Reasoning:  "Local gate failed: " + localGateResult.Reason,
			Reason:     reason,
			Source:     AIAuditSourceSyntheticLocalGate,
		}
		syntheticLocalGate = true
	} else {
		recordAIFunnel = true
		if !hasAuditedResponse {
			aiSkipped = true
			resolvedAudit = dto.AIAuditResponse{
				Symbol:     pair,
				IsApproved: false,
				Decision:   "WAIT",
				Sentiment:  "NEUTRAL",
				Reasoning:  "AI_SKIPPED: Exceeded policy MaxAICandidates quota limit",
				Reason:     "AI_SKIPPED",
				Source:     AIAuditSourceSyntheticQuota,
			}
		}
	}

	planReview := uc.planReconciliationUsecase.Reconcile(qResult, resolvedAudit)

	latestPrice, latestPriceOK := uc.stalenessUsecase.ResolveLatestPrice(ctx, pair)
	if !latestPriceOK {
		latestPrice = 0
	}

	stalenessRes := uc.stalenessUsecase.Evaluate(qResult, planReview, policy, latestPrice)
	metrics := GetGlobalMetrics()
	metrics.AddStalenessChecked(1)
	if stalenessRes.IsStale {
		metrics.AddStalenessCount(1)
	}

	finalDecision := uc.finalGateUsecase.Evaluate(
		qResult,
		localGateResult,
		resolvedAudit,
		planReview,
		stalenessRes,
		policy,
		latestPrice,
		activeSignals,
		historySignals,
		marketData.M15Candles,
	)

	return selectedCandidateEvaluation{
		decision: finalDecision,
		context: scanCandidateContext{
			quantResult:     qResult,
			auditResponse:   resolvedAudit,
			stalenessRes:    stalenessRes,
			localGateResult: localGateResult,
			planReview:      planReview,
			latestPrice:     latestPrice,
			aiSkipped:       aiSkipped,
		},
		syntheticLocalGate: syntheticLocalGate,
		aiSkippedQuota:     aiSkipped,
		recordAIFunnel:     recordAIFunnel,
	}
}

func recordAIAuditFunnelOutcome(
	playbook Playbook,
	auditResponse dto.AIAuditResponse,
	funnelSummary *funnelSummaryAccumulator,
	playbookBlockers *playbookBlockerAccumulator,
) {
	if strings.Contains(strings.ToUpper(auditResponse.Reason), "AI_ERROR") || strings.Contains(strings.ToUpper(auditResponse.Reasoning), "AI_ERROR") {
		reason := firstNonEmpty(auditResponse.Reason, auditResponse.Decision, "AI_ERROR")
		funnelSummary.Add(funnelStageAIError, reason)
		playbookBlockers.Add(playbook, funnelStageAIError, reason)
		return
	}
	if auditResponse.Decision == "WAIT" {
		reason := firstNonEmpty(auditResponse.Reason, auditResponse.Decision)
		funnelSummary.Add(funnelStageAIWait, reason)
		playbookBlockers.Add(playbook, funnelStageAIWait, reason)
		return
	}
	if auditResponse.Decision == "REJECT" {
		reason := firstNonEmpty(auditResponse.Reason, auditResponse.Decision)
		funnelSummary.Add(funnelStageAIReject, reason)
		playbookBlockers.Add(playbook, funnelStageAIReject, reason)
	}
}
