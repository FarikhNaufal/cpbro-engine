package usecase

import (
	"sort"
	"strings"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"cpbro-engine/internal/modules/cryptobroV3/entity"
)

func NormalizeSignalForFrontend(sig dto.SignalResponse) dto.Signal {
	var tStr string
	if !sig.ReconciledTime.IsZero() {
		tStr = sig.ReconciledTime.Format(time.RFC3339)
	}
	return dto.Signal{
		Symbol:         sig.Symbol,
		Direction:      sig.Direction,
		Timeframe:      sig.Timeframe,
		TriggerPrice:   sig.TriggerPrice,
		StopLoss:       sig.StopLoss,
		TakeProfit:     sig.TakeProfit,
		Score:          sig.Score,
		Strategy:       sig.Strategy,
		AISentiment:    sig.AISentiment,
		IsFinalExecute: sig.IsFinalExecute,
		ReconciledTime: tStr,
		Status:         sig.Status,
	}
}

func NormalizeWatchSignalForFrontend(sig dto.SignalResponse) dto.WatchSignal {
	var tStr string
	if !sig.ReconciledTime.IsZero() {
		tStr = sig.ReconciledTime.Format(time.RFC3339)
	}
	return dto.WatchSignal{
		Symbol:         sig.Symbol,
		Direction:      sig.Direction,
		Timeframe:      sig.Timeframe,
		TriggerPrice:   sig.TriggerPrice,
		StopLoss:       sig.StopLoss,
		TakeProfit:     sig.TakeProfit,
		Score:          sig.Score,
		Strategy:       sig.Strategy,
		AISentiment:    sig.AISentiment,
		IsFinalExecute: sig.IsFinalExecute,
		ReconciledTime: tStr,
		Status:         sig.Status,
	}
}

func NormalizeLatestResultForFrontend(res *entity.LatestResult) dto.LatestResultResponse {
	var out dto.LatestResultResponse
	if res == nil {
		res = &entity.LatestResult{}
	}

	out.ConfigVersion = res.ConfigVersion
	if !res.GeneratedAt.IsZero() {
		out.GeneratedAt = res.GeneratedAt.Format(time.RFC3339)
	} else {
		out.GeneratedAt = ""
	}
	out.ScanID = res.ScanID
	out.MarketPolicy = res.MarketPolicy
	out.MarketRegime = res.MarketRegime
	out.TotalTickers = res.TotalTickers
	out.TotalUniversePass = res.TotalUniversePass
	out.TotalUniverseRejected = res.TotalUniverseRejected
	out.TotalStrategySelected = res.TotalStrategySelected
	out.TotalPlaybookEligible = res.TotalPlaybookEligible
	out.TotalQuantCandidates = res.TotalQuantCandidates
	out.TotalArbiterSelected = res.TotalArbiterSelected
	out.TotalLocalAICandidate = res.TotalLocalAICandidate
	out.TotalAIConfirm = res.TotalAIConfirm
	out.TotalAIWait = res.TotalAIWait
	out.TotalAIReject = res.TotalAIReject
	out.TotalAIError = res.TotalAIError
	out.TotalFinalExecute = res.TotalFinalExecute
	out.TotalFinalWatch = res.TotalFinalWatch
	out.TotalFinalReject = res.TotalFinalReject

	// Normalize ExecuteSignals
	out.ExecuteSignals = make([]dto.Signal, 0)
	for _, sig := range res.ExecuteSignals {
		out.ExecuteSignals = append(out.ExecuteSignals, NormalizeSignalForFrontend(sig))
	}

	// Normalize Watchlist
	out.Watchlist = make([]dto.WatchSignal, 0)
	for _, ws := range res.Watchlist {
		out.Watchlist = append(out.Watchlist, NormalizeWatchSignalForFrontend(ws))
	}

	// Normalize RejectedSummary
	out.RejectedSummary = make([]string, 0)
	for _, item := range res.RejectedSummary {
		if item != "" {
			out.RejectedSummary = append(out.RejectedSummary, item)
		}
	}

	// Normalize PolicyRejectedSummary
	out.PolicyRejectedSummary = make([]string, 0)
	seenPolicyReject := make(map[string]struct{})
	for _, item := range res.PolicyRejectedSummary {
		if item != "" {
			if _, ok := seenPolicyReject[item]; ok {
				continue
			}
			seenPolicyReject[item] = struct{}{}
			out.PolicyRejectedSummary = append(out.PolicyRejectedSummary, item)
		}
	}

	// Normalize SelectedThresholdProfileSummary
	out.ThresholdProfileSummary = make(map[string]string)
	for k, v := range res.SelectedThresholdProfileSummary {
		out.ThresholdProfileSummary[k] = v
	}

	out.EvaluationDataCompletenessHint = res.EvaluationDataCompletenessHint

	// Normalize ArbiterSelectedDetails
	out.ArbiterSelectedDetails = make([]dto.ArbiterSelectedDetail, 0)
	for _, detail := range res.ArbiterSelectedDetails {
		out.ArbiterSelectedDetails = append(out.ArbiterSelectedDetails, dto.ArbiterSelectedDetail{
			Symbol:          detail.Symbol,
			Playbook:        detail.Playbook,
			Direction:       detail.Direction,
			LocalGateStatus: detail.LocalGateStatus,
			AIDecision:      detail.AIDecision,
			AIConfidence:    "",
			StalenessStatus: detail.StalenessStatus,
			FinalStatus:     detail.FinalStatus,
			FinalReason:     detail.FinalReason,
		})
	}

	if !res.LastScanTime.IsZero() {
		out.LastScanTime = res.LastScanTime.Format(time.RFC3339)
	} else {
		out.LastScanTime = ""
	}
	out.Duration = res.Duration

	// Normalize Signals
	out.Signals = make([]dto.Signal, 0)
	for _, sig := range res.Signals {
		out.Signals = append(out.Signals, NormalizeSignalForFrontend(sig))
	}

	out.Warnings = []string{}
	out.PartialErrors = []string{}

	return out
}

func NormalizeJournalForFrontend(items []SignalJournal, limit int, offset int, filters dto.JournalFilters) dto.JournalResponse {
	outItems := make([]dto.SignalJournalResponse, 0)
	for _, item := range items {
		var createdStr, expiresStr, updatedStr, closedStr string
		if !item.CreatedAt.IsZero() {
			createdStr = item.CreatedAt.Format(time.RFC3339)
		}
		if !item.ExpiresAt.IsZero() {
			expiresStr = item.ExpiresAt.Format(time.RFC3339)
		}
		if !item.UpdatedAt.IsZero() {
			updatedStr = item.UpdatedAt.Format(time.RFC3339)
		}
		if !item.ClosedAt.IsZero() {
			closedStr = item.ClosedAt.Format(time.RFC3339)
		}

		outItems = append(outItems, dto.SignalJournalResponse{
			SchemaVersion:           item.SchemaVersion,
			ConfigVersion:           item.ConfigVersion,
			ID:                      item.ID,
			Symbol:                  item.Symbol,
			Direction:               string(item.Direction),
			Playbook:                string(item.Playbook),
			EntryPrice:              item.EntryPrice,
			StopLoss:                item.StopLoss,
			TP1:                     item.TP1,
			TP2:                     item.TP2,
			RR:                      item.RR,
			QuantScore:              item.QuantScore,
			AIConfidence:            item.AIConfidence,
			MarketRegime:            item.MarketRegime,
			PolicyMode:              item.PolicyMode,
			ThresholdProfileSummary: item.ThresholdProfileSummary,
			CreatedAt:               createdStr,
			ExpiresAt:               expiresStr,
			Status:                  string(item.Status),
			MFE:                     item.MFE,
			MAE:                     item.MAE,
			TimeToTP1:               item.TimeToTP1,
			TimeToTP2:               item.TimeToTP2,
			TimeToSL:                item.TimeToSL,
			OutcomeReason:           item.OutcomeReason,
			EntryTiming:             item.EntryTiming,
			Tier:                    string(item.Tier),
			Timeframe:               item.Timeframe,
			LatestPrice:             item.LatestPrice,
			TakeProfit:              item.TakeProfit,
			AISentiment:             item.AISentiment,
			AIReasoning:             item.AIReasoning,
			PnlPercentage:           item.PnlPercentage,
			UpdatedAt:               updatedStr,
			ClosedAt:                closedStr,
			Reason:                  item.Reason,
			NotificationStatus:      item.NotificationStatus,
			NotificationError:       item.NotificationError,
			BreakoutLevel:           item.BreakoutLevel,
			RetestTouches:           item.RetestTouches,
			RetestHold:              item.RetestHold,
			HasDerivativesEvidence:  item.HasDerivativesEvidence,
		})
	}

	return dto.JournalResponse{
		Items:   outItems,
		Total:   len(items),
		Limit:   limit,
		Offset:  offset,
		Filters: filters,
	}
}

func NormalizeEvaluationForFrontend(report *EvaluationReport) dto.EvaluationResponse {
	if report == nil {
		report = &EvaluationReport{}
	}

	toDTODataCompleteness := func(dc DataCompleteness) dto.DataCompleteness {
		return dto.DataCompleteness{
			HasSignalJournal:                  dc.HasSignalJournal,
			HasLatestResult:                   dc.HasLatestResult,
			HasDecisionAudit:                  dc.HasDecisionAudit,
			CanEvaluateExecutedOutcome:        dc.CanEvaluateExecutedOutcome,
			CanEvaluateWatchMissedOpportunity: dc.CanEvaluateWatchMissedOpportunity,
			CanEvaluateAIWait:                 dc.CanEvaluateAIWait,
			CanEvaluateConflictDowngrade:      dc.CanEvaluateConflictDowngrade,
		}
	}

	toDTOPlaybookStats := func(s PlaybookStats) dto.PlaybookStats {
		return dto.PlaybookStats{
			TotalSignals: s.TotalSignals,
			WinRate:      s.WinRate,
		}
	}
	toDTORegimeStats := func(s RegimeStats) dto.RegimeStats {
		return dto.RegimeStats{
			TotalSignals: s.TotalSignals,
			WinRate:      s.WinRate,
		}
	}
	toDTOTierStats := func(s TierStats) dto.TierStats {
		return dto.TierStats{
			TotalSignals: s.TotalSignals,
			WinRate:      s.WinRate,
		}
	}
	toDTODirectionStats := func(s DirectionStats) dto.DirectionStats {
		return dto.DirectionStats{
			TotalSignals: s.TotalSignals,
			WinRate:      s.WinRate,
		}
	}
	toDTOAIStats := func(s AIStats) dto.AIStats {
		return dto.AIStats{
			TotalSignals: s.TotalSignals,
			WinRate:      s.WinRate,
		}
	}
	toDTOStalenessStats := func(s StalenessStats) dto.StalenessStats {
		return dto.StalenessStats{
			TotalSignals: s.TotalSignals,
			WinRate:      s.WinRate,
		}
	}

	toDTORecommendation := func(r ThresholdRecommendation) dto.ThresholdRecommendation {
		return dto.ThresholdRecommendation{
			IssueType:          r.IssueType,
			Playbook:           r.Playbook,
			MarketRegime:       r.MarketRegime,
			PolicyMode:         r.PolicyMode,
			Direction:          r.Direction,
			Tier:               r.Tier,
			MetricName:         r.MetricName,
			MetricValue:        r.MetricValue,
			SampleSize:         r.SampleSize,
			CurrentThreshold:   r.CurrentThreshold,
			SuggestedThreshold: r.SuggestedThreshold,
			EvidenceSummary:    r.EvidenceSummary,
			ConfidenceLevel:    r.ConfidenceLevel,
			Reason:             r.Reason,
			SuggestedAction:    r.SuggestedAction,
			DoNotAutoApply:     true,
			RequiresMoreData:   r.RequiresMoreData,
			Severity:           r.Severity,
		}
	}

	out := dto.EvaluationResponse{
		GeneratedAt:      "",
		DataCompleteness: toDTODataCompleteness(report.DataCompleteness),
		TotalSignals:     report.TotalSignals,
		Metrics:          map[string]float64{},
		PlaybookStats:    []dto.NamedPlaybookStats{},
		RegimeStats:      []dto.NamedRegimeStats{},
		TierStats:        []dto.NamedTierStats{},
		DirectionStats:   []dto.NamedDirectionStats{},
		AIStats:          []dto.NamedAIStats{},
		StalenessStats:   []dto.NamedStalenessStats{},
		ConflictStats:    []dto.NamedIntStat{},
		CooldownStats:    []dto.NamedIntStat{},
		GateBugFindings:  []dto.GateBugFinding{},
		Recommendations:  []dto.ThresholdRecommendation{},
		Notes:            []string{},
		Status:           "",
	}

	if !report.GeneratedAt.IsZero() {
		out.GeneratedAt = report.GeneratedAt.Format(time.RFC3339)
	}

	for k, v := range report.Metrics {
		out.Metrics[k] = v
	}

	// Convert maps -> sorted arrays for frontend stability.
	playbookKeys := make([]string, 0, len(report.PlaybookStats))
	for k := range report.PlaybookStats {
		playbookKeys = append(playbookKeys, k)
	}
	sort.Strings(playbookKeys)
	for _, k := range playbookKeys {
		out.PlaybookStats = append(out.PlaybookStats, dto.NamedPlaybookStats{Key: k, Value: toDTOPlaybookStats(report.PlaybookStats[k])})
	}

	regimeKeys := make([]string, 0, len(report.RegimeStats))
	for k := range report.RegimeStats {
		regimeKeys = append(regimeKeys, k)
	}
	sort.Strings(regimeKeys)
	for _, k := range regimeKeys {
		out.RegimeStats = append(out.RegimeStats, dto.NamedRegimeStats{Key: k, Value: toDTORegimeStats(report.RegimeStats[k])})
	}

	tierKeys := make([]string, 0, len(report.TierStats))
	for k := range report.TierStats {
		tierKeys = append(tierKeys, k)
	}
	sort.Strings(tierKeys)
	for _, k := range tierKeys {
		out.TierStats = append(out.TierStats, dto.NamedTierStats{Key: k, Value: toDTOTierStats(report.TierStats[k])})
	}

	dirKeys := make([]string, 0, len(report.DirectionStats))
	for k := range report.DirectionStats {
		dirKeys = append(dirKeys, k)
	}
	sort.Strings(dirKeys)
	for _, k := range dirKeys {
		out.DirectionStats = append(out.DirectionStats, dto.NamedDirectionStats{Key: k, Value: toDTODirectionStats(report.DirectionStats[k])})
	}

	aiKeys := make([]string, 0, len(report.AIStats))
	for k := range report.AIStats {
		aiKeys = append(aiKeys, k)
	}
	sort.Strings(aiKeys)
	for _, k := range aiKeys {
		out.AIStats = append(out.AIStats, dto.NamedAIStats{Key: k, Value: toDTOAIStats(report.AIStats[k])})
	}

	stKeys := make([]string, 0, len(report.StalenessStats))
	for k := range report.StalenessStats {
		stKeys = append(stKeys, k)
	}
	sort.Strings(stKeys)
	for _, k := range stKeys {
		out.StalenessStats = append(out.StalenessStats, dto.NamedStalenessStats{Key: k, Value: toDTOStalenessStats(report.StalenessStats[k])})
	}

	confKeys := make([]string, 0, len(report.ConflictStats))
	for k := range report.ConflictStats {
		confKeys = append(confKeys, k)
	}
	sort.Strings(confKeys)
	for _, k := range confKeys {
		out.ConflictStats = append(out.ConflictStats, dto.NamedIntStat{Key: k, Value: report.ConflictStats[k]})
	}

	cdKeys := make([]string, 0, len(report.CooldownStats))
	for k := range report.CooldownStats {
		cdKeys = append(cdKeys, k)
	}
	sort.Strings(cdKeys)
	for _, k := range cdKeys {
		out.CooldownStats = append(out.CooldownStats, dto.NamedIntStat{Key: k, Value: report.CooldownStats[k]})
	}

	for _, g := range report.GateBugFindings {
		out.GateBugFindings = append(out.GateBugFindings, dto.GateBugFinding(g))
	}
	for _, rec := range report.Recommendations {
		out.Recommendations = append(out.Recommendations, toDTORecommendation(rec))
	}

	out.BestPlaybook = report.BestPlaybook
	out.WorstPlaybook = report.WorstPlaybook
	out.SetupYangSeringLangsungSL = report.SetupYangSeringLangsungSL
	out.SetupYangSeringExpired = report.SetupYangSeringExpired
	out.SetupYangSeringStale = report.SetupYangSeringStale
	out.RegimeYangPalingBuruk = report.RegimeYangPalingBuruk
	out.TierYangPalingBuruk = report.TierYangPalingBuruk
	out.DirectionYangPalingBuruk = report.DirectionYangPalingBuruk
	out.PlaybookDenganMAETerbesar = report.PlaybookDenganMAETerbesar
	out.PlaybookDenganExpiredRate = report.PlaybookDenganExpiredRate
	out.PlaybookDenganTP1Terbaik = report.PlaybookDenganTP1Terbaik
	out.PlaybookDenganTP2Follow = report.PlaybookDenganTP2Follow
	if strings.TrimSpace(report.Notes) != "" {
		out.Notes = append(out.Notes, report.Notes)
	}
	out.Status = string(report.Status)

	return out
}

func NormalizeDecisionAuditForFrontend(audits []DecisionAudit, limit, offset int, filters dto.DecisionAuditFilters) dto.DecisionAuditResponse {
	outItems := make([]dto.DecisionAuditRow, 0)
	for _, item := range audits {
		var genStr, createdStr string
		if !item.GeneratedAt.IsZero() {
			genStr = item.GeneratedAt.Format(time.RFC3339)
		}
		if !item.CreatedAt.IsZero() {
			createdStr = item.CreatedAt.Format(time.RFC3339)
		}

		outItems = append(outItems, dto.DecisionAuditRow{
			SchemaVersion:             item.SchemaVersion,
			ConfigVersion:             item.ConfigVersion,
			ScanID:                    item.ScanID,
			GeneratedAt:               genStr,
			Symbol:                    item.Symbol,
			Direction:                 string(item.Direction),
			Playbook:                  string(item.Playbook),
			SetupType:                 item.SetupType,
			Tier:                      string(item.Tier),
			Grade:                     item.Grade,
			Score:                     item.Score,
			RR:                        item.RR,
			RequiredScore:             item.RequiredScore,
			RequiredRR:                item.RequiredRR,
			LocalGateStatus:           item.LocalGateStatus,
			LocalGateReason:           item.LocalGateReason,
			AIDecision:                item.AIDecision,
			AIConfidence:              item.AIConfidence,
			AICandleNarrative:         item.AICandleNarrative,
			AIEntryTiming:             item.AIEntryTiming,
			AIConflictWithBot:         item.AIConflictWithBot,
			PlanStatus:                item.PlanStatus,
			PlanConflict:              item.PlanConflict,
			NeedRetest:                item.NeedRetest,
			StalenessStatus:           item.StalenessStatus,
			FinalStatusBeforeConflict: string(item.FinalStatusBeforeConflict),
			FinalReasonBeforeConflict: item.FinalReasonBeforeConflict,
			FinalStatusAfterConflict:  string(item.FinalStatusAfterConflict),
			FinalReasonAfterConflict:  item.FinalReasonAfterConflict,
			FinalStatus:               string(item.FinalStatus),
			FinalReason:               item.FinalReason,
			ConflictReason:            item.ConflictReason,
			CooldownReason:            item.CooldownReason,
			WasNotified:               item.WasNotified,
			LatestPriceAtDecision:     item.LatestPriceAtDecision,
			EntryPrice:                item.EntryPrice,
			StopLoss:                  item.StopLoss,
			TakeProfit1:               item.TakeProfit1,
			TakeProfit2:               item.TakeProfit2,
			MarketRegime:              item.MarketRegime,
			PolicyMode:                item.PolicyMode,
			ThresholdProfileSummary:   item.ThresholdProfileSummary,
			BreakoutLevel:             item.BreakoutLevel,
			RetestTouches:             item.RetestTouches,
			RetestHold:                item.RetestHold,
			HasDerivativesEvidence:    item.HasDerivativesEvidence,
			RejectOrWatchReason:       item.RejectOrWatchReason,
			CreatedAt:                 createdStr,
			HypotheticalEntry:         item.HypotheticalEntry,
		})
	}

	return dto.DecisionAuditResponse{
		Items:   outItems,
		Total:   len(audits),
		Limit:   limit,
		Offset:  offset,
		Filters: filters,
	}
}
