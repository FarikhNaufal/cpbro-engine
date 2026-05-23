package usecase

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type FeedbackUsecase struct {
	storageUsecase *StorageUsecase
}

func NewFeedbackUsecase(storage *StorageUsecase) *FeedbackUsecase {
	return &FeedbackUsecase{
		storageUsecase: storage,
	}
}

// safeRate handles percentage calculation safely
func safeRate(count, total int) float64 {
	if total == 0 {
		return 0.0
	}
	return (float64(count) / float64(total)) * 100.0
}

// safeDiv handles division safely
func safeDiv(val, div float64) float64 {
	if div == 0 {
		return 0.0
	}
	return val / div
}

// getSampleGuard returns confidence, requiresMoreData, and severity based on sample size
func getSampleGuard(sampleSize int) (confidence string, requiresMoreData bool, severity string) {
	if sampleSize < 10 {
		return "LOW", true, "INFO"
	} else if sampleSize < 20 {
		return "LOW", false, "LOW"
	} else if sampleSize <= 50 {
		return "MEDIUM", false, "WARNING"
	} else {
		return "HIGH", false, "CRITICAL"
	}
}

// GenerateEvaluationReport compiles win rates, excursions, durations, and detailed threshold/policy recommendations.
func (uc *FeedbackUsecase) GenerateEvaluationReport() error {
	var sourceFiles []string
	var completeness DataCompleteness

	// 1. Load data sources
	journal, err := uc.storageUsecase.LoadSignalJournal()
	hasJournal := err == nil && len(journal) > 0
	if hasJournal {
		sourceFiles = append(sourceFiles, "signal_journal.json")
		completeness.HasSignalJournal = true
		completeness.CanEvaluateExecutedOutcome = true
	}

	latestRes, err := uc.storageUsecase.LoadLatestResult()
	hasLatest := err == nil && latestRes != nil && len(latestRes.Signals) > 0
	if hasLatest {
		sourceFiles = append(sourceFiles, "latest_result.json")
		completeness.HasLatestResult = true
		completeness.CanEvaluateWatchMissedOpportunity = true
	}

	audits, err := uc.storageUsecase.LoadDecisionAudits()
	hasAudits := err == nil && len(audits) > 0
	if hasAudits {
		sourceFiles = append(sourceFiles, "decision_audit.json")
		completeness.HasDecisionAudit = true
		completeness.CanEvaluateAIWait = true
		completeness.CanEvaluateConflictDowngrade = true
		completeness.CanEvaluateWatchMissedOpportunity = true
	}

	// Early check: if we have absolutely no data, still save a report indicating zero signals
	if len(journal) == 0 && len(audits) == 0 {
		emptyReport := EvaluationReport{
			GeneratedAt:      time.Now(),
			ConfigVersion:    GetGlobalConfigRegistry().GetVersion(),
			SourceFilesUsed:  sourceFiles,
			DataCompleteness: completeness,
			TotalSignals:     0,
			Metrics:          make(map[string]float64),
			PlaybookStats:    make(map[string]PlaybookStats),
			RegimeStats:      make(map[string]RegimeStats),
			TierStats:        make(map[string]TierStats),
			DirectionStats:   make(map[string]DirectionStats),
			AIStats:          make(map[string]AIStats),
			StalenessStats:   make(map[string]StalenessStats),
			Recommendations: []ThresholdRecommendation{
				{
					IssueType:        "INSUFFICIENT_SAMPLE",
					Playbook:         "ALL",
					SampleSize:       0,
					EvidenceSummary:  "No historical trading signals found in storage files.",
					ConfidenceLevel:  "LOW",
					Reason:           "No signals recorded in signal_journal.json or decision_audit.json.",
					SuggestedAction:  "Wait for scanner to compile data.",
					DoNotAutoApply:   true,
					RequiresMoreData: true,
					Severity:         "INFO",
				},
			},
			Notes:  "Feedback Loop executed on empty storage. Insufficient data to evaluate performance.",
			Status: "COMPLETED",
		}
		GetGlobalMetrics().SetEvalMetrics(uint64(len(emptyReport.Recommendations)), uint64(len(emptyReport.GateBugFindings)))
		GetGlobalMetrics().SetLastEvaluationTime(emptyReport.GeneratedAt)
		return uc.storageUsecase.SaveEvaluationReport(emptyReport)
	}

	// 2. Identify finalised signals (from journal)
	var finalized []SignalJournal
	for _, item := range journal {
		if item.Status != MONITORING {
			finalized = append(finalized, item)
		}
	}

	// 3. Count basic rates
	var wins, losses, tp1Hits, tp2Hits, slHits, expiredHits int
	var totalPnl, sumMFE, sumMAE, sumRR float64
	var sumTimeToTP1, sumTimeToTP2, sumTimeToSL, sumHoldingTime float64
	var countTimeToTP1, countTimeToTP2, countTimeToSL, countHoldingTime int

	type rawStats struct {
		total       int
		wins        int
		tp1Count    int
		tp2Count    int
		slCount     int
		expCount    int
		sumMAE      float64
		sumMFE      float64
		maxMAE      float64
		sumHoldTime float64
		holdCount   int
		timeTP1Sum  float64
		timeTP1Cnt  int
		timeTP2Sum  float64
		timeTP2Cnt  int
		timeSLSum   float64
		timeSLCnt   int
	}

	pbRaw := make(map[string]*rawStats)
	regimeRaw := make(map[string]*rawStats)
	tierRaw := make(map[string]*rawStats)
	directionRaw := make(map[string]*rawStats)
	aiRaw := make(map[string]*rawStats)
	stalenessRaw := make(map[string]*rawStats)

	getOrInitRaw := func(m map[string]*rawStats, key string) *rawStats {
		if key == "" {
			key = "UNKNOWN"
		}
		if _, ok := m[key]; !ok {
			m[key] = &rawStats{}
		}
		return m[key]
	}

	for _, item := range finalized {
		isWin := item.Status == TP1_HIT || item.Status == TP2_HIT
		isLoss := item.Status == SL_HIT
		isExpired := item.Status == EXPIRED

		if isWin {
			wins++
		}
		if isLoss {
			losses++
		}

		hasHitTP1 := item.TimeToTP1 != "" || item.Status == TP1_HIT || item.Status == TP2_HIT
		if hasHitTP1 {
			tp1Hits++
		}
		if item.Status == TP2_HIT {
			tp2Hits++
		}
		if item.Status == SL_HIT {
			slHits++
		}
		if item.Status == EXPIRED {
			expiredHits++
		}

		totalPnl += item.PnlPercentage
		sumMFE += item.MFE
		sumMAE += item.MAE
		sumRR += item.RR

		// Holding & metric times
		holdMins := 120.0 // Default maximum holding period
		hasHold := false

		if item.TimeToTP1 != "" {
			if d, err := time.ParseDuration(item.TimeToTP1); err == nil {
				sumTimeToTP1 += d.Minutes()
				countTimeToTP1++
				if item.Status == TP1_HIT {
					holdMins = d.Minutes()
					hasHold = true
				}
			}
		}
		if item.TimeToTP2 != "" {
			if d, err := time.ParseDuration(item.TimeToTP2); err == nil {
				sumTimeToTP2 += d.Minutes()
				countTimeToTP2++
				if item.Status == TP2_HIT {
					holdMins = d.Minutes()
					hasHold = true
				}
			}
		}
		if item.TimeToSL != "" {
			if d, err := time.ParseDuration(item.TimeToSL); err == nil {
				sumTimeToSL += d.Minutes()
				countTimeToSL++
				if item.Status == SL_HIT {
					holdMins = d.Minutes()
					hasHold = true
				}
			}
		}

		if isExpired {
			holdMins = 120.0
			hasHold = true
		}

		if hasHold {
			sumHoldingTime += holdMins
			countHoldingTime++
		}

		// Keys
		pbKey := string(item.Playbook)
		regimeKey := item.MarketRegime
		tierKey := string(item.Tier)
		dirKey := string(item.Direction)
		aiConfKey := item.AIConfidence
		stalenessKey := "FRESH"
		if item.EntryTiming == "LATE" || item.OutcomeReason == "stale" {
			stalenessKey = "LATE"
		}

		// Update raw group stats
		updateRaw := func(rs *rawStats) {
			rs.total++
			if isWin {
				rs.wins++
			}
			if hasHitTP1 {
				rs.tp1Count++
			}
			if item.Status == TP2_HIT {
				rs.tp2Count++
			}
			if isLoss {
				rs.slCount++
			}
			if isExpired {
				rs.expCount++
			}
			rs.sumMAE += item.MAE
			rs.sumMFE += item.MFE
			if item.MAE > rs.maxMAE {
				rs.maxMAE = item.MAE
			}
			if hasHold {
				rs.sumHoldTime += holdMins
				rs.holdCount++
			}
			// Times
			if item.TimeToTP1 != "" {
				if d, err := time.ParseDuration(item.TimeToTP1); err == nil {
					rs.timeTP1Sum += d.Minutes()
					rs.timeTP1Cnt++
				}
			}
			if item.TimeToTP2 != "" {
				if d, err := time.ParseDuration(item.TimeToTP2); err == nil {
					rs.timeTP2Sum += d.Minutes()
					rs.timeTP2Cnt++
				}
			}
			if item.TimeToSL != "" {
				if d, err := time.ParseDuration(item.TimeToSL); err == nil {
					rs.timeSLSum += d.Minutes()
					rs.timeSLCnt++
				}
			}
		}

		updateRaw(getOrInitRaw(pbRaw, pbKey))
		updateRaw(getOrInitRaw(regimeRaw, regimeKey))
		updateRaw(getOrInitRaw(tierRaw, tierKey))
		updateRaw(getOrInitRaw(directionRaw, dirKey))
		updateRaw(getOrInitRaw(aiRaw, aiConfKey))
		updateRaw(getOrInitRaw(stalenessRaw, stalenessKey))
	}

	// Calculate main rates
	totalCount := len(finalized)
	winRate := safeRate(wins, totalCount)
	tp1Rate := safeRate(tp1Hits, totalCount)
	tp2Rate := safeRate(tp2Hits, totalCount)
	slRate := safeRate(slHits, totalCount)
	expiredRate := safeRate(expiredHits, totalCount)

	avgMFE := safeDiv(sumMFE, float64(totalCount))
	avgMAE := safeDiv(sumMAE, float64(totalCount))
	avgRR := safeDiv(sumRR, float64(totalCount))
	avgTimeToTP1 := safeDiv(sumTimeToTP1, float64(countTimeToTP1))
	avgTimeToTP2 := safeDiv(sumTimeToTP2, float64(countTimeToTP2))
	avgTimeToSL := safeDiv(sumTimeToSL, float64(countTimeToSL))
	avgHoldingTime := safeDiv(sumHoldingTime, float64(countHoldingTime))

	// Map raw stats to report models
	playbookStats := make(map[string]PlaybookStats)
	for k, v := range pbRaw {
		playbookStats[k] = PlaybookStats{
			TotalSignals:         v.total,
			WinRate:              safeRate(v.wins, v.total),
			TP1Rate:              safeRate(v.tp1Count, v.total),
			TP2Rate:              safeRate(v.tp2Count, v.total),
			SLRate:               safeRate(v.slCount, v.total),
			ExpiredRate:          safeRate(v.expCount, v.total),
			AverageMAE:           safeDiv(v.sumMAE, float64(v.total)),
			AverageMFE:           safeDiv(v.sumMFE, float64(v.total)),
			AverageHoldTime:      safeDiv(v.sumHoldTime, float64(v.holdCount)),
			AverageTimeToTP1:     safeDiv(v.timeTP1Sum, float64(v.timeTP1Cnt)),
			AverageTimeToTP2:     safeDiv(v.timeTP2Sum, float64(v.timeTP2Cnt)),
			AverageTimeToSL:      safeDiv(v.timeSLSum, float64(v.timeSLCnt)),
			MaxMAE:               v.maxMAE,
			TP2FollowThroughRate: safeRate(v.tp2Count, v.tp1Count),
		}
	}

	regimeStats := make(map[string]RegimeStats)
	for k, v := range regimeRaw {
		regimeStats[k] = RegimeStats{TotalSignals: v.total, WinRate: safeRate(v.wins, v.total)}
	}
	tierStats := make(map[string]TierStats)
	for k, v := range tierRaw {
		tierStats[k] = TierStats{TotalSignals: v.total, WinRate: safeRate(v.wins, v.total)}
	}
	directionStats := make(map[string]DirectionStats)
	for k, v := range directionRaw {
		directionStats[k] = DirectionStats{TotalSignals: v.total, WinRate: safeRate(v.wins, v.total)}
	}
	aiStats := make(map[string]AIStats)
	for k, v := range aiRaw {
		aiStats[k] = AIStats{TotalSignals: v.total, WinRate: safeRate(v.wins, v.total)}
	}
	stalenessStats := make(map[string]StalenessStats)
	for k, v := range stalenessRaw {
		stalenessStats[k] = StalenessStats{TotalSignals: v.total, WinRate: safeRate(v.wins, v.total)}
	}

	// Find best/worst metrics
	var bestPb, worstPb string
	var bestPbRate, worstPbRate float64
	firstPb := true
	for k, v := range playbookStats {
		if firstPb {
			bestPb = k
			bestPbRate = v.WinRate
			worstPb = k
			worstPbRate = v.WinRate
			firstPb = false
		} else {
			if v.WinRate > bestPbRate {
				bestPb = k
				bestPbRate = v.WinRate
			}
			if v.WinRate < worstPbRate {
				worstPb = k
				worstPbRate = v.WinRate
			}
		}
	}

	var worstRegime, worstTier, worstDirection string
	var worstRegRate, worstTierRate, worstDirRate float64
	firstReg, firstTier, firstDir := true, true, true

	for k, v := range regimeStats {
		if firstReg {
			worstRegime = k
			worstRegRate = v.WinRate
			firstReg = false
		} else if v.WinRate < worstRegRate {
			worstRegime = k
			worstRegRate = v.WinRate
		}
	}
	for k, v := range tierStats {
		if firstTier {
			worstTier = k
			worstTierRate = v.WinRate
			firstTier = false
		} else if v.WinRate < worstTierRate {
			worstTier = k
			worstTierRate = v.WinRate
		}
	}
	for k, v := range directionStats {
		if firstDir {
			worstDirection = k
			worstDirRate = v.WinRate
			firstDir = false
		} else if v.WinRate < worstDirRate {
			worstDirection = k
			worstDirRate = v.WinRate
		}
	}

	var pbMaxMAE, pbMaxExp, pbBestTP1, pbBestTP2Follow string
	var maxMAEVal, maxExpVal, bestTP1Val, bestTP2FollowVal float64
	firstMae, firstExp, firstTp1, firstTp2F := true, true, true, true

	for k, v := range playbookStats {
		if firstMae {
			pbMaxMAE = k
			maxMAEVal = v.AverageMAE
			firstMae = false
		} else if v.AverageMAE > maxMAEVal {
			pbMaxMAE = k
			maxMAEVal = v.AverageMAE
		}

		if firstExp {
			pbMaxExp = k
			maxExpVal = v.ExpiredRate
			firstExp = false
		} else if v.ExpiredRate > maxExpVal {
			pbMaxExp = k
			maxExpVal = v.ExpiredRate
		}

		if firstTp1 {
			pbBestTP1 = k
			bestTP1Val = v.TP1Rate
			firstTp1 = false
		} else if v.TP1Rate > bestTP1Val {
			pbBestTP1 = k
			bestTP1Val = v.TP1Rate
		}

		if firstTp2F {
			pbBestTP2Follow = k
			bestTP2FollowVal = v.TP2FollowThroughRate
			firstTp2F = false
		} else if v.TP2FollowThroughRate > bestTP2FollowVal {
			pbBestTP2Follow = k
			bestTP2FollowVal = v.TP2FollowThroughRate
		}
	}

	// Worst setups counts
	setupSLCounts := make(map[string]int)
	setupExpiredCounts := make(map[string]int)
	setupStaleCounts := make(map[string]int)

	for _, item := range finalized {
		stKey := string(item.Direction) + "_" + string(item.Playbook)
		if item.Status == SL_HIT {
			setupSLCounts[stKey]++
		}
		if item.Status == EXPIRED {
			setupExpiredCounts[stKey]++
		}
		if item.EntryTiming == "LATE" || item.OutcomeReason == "stale" {
			setupStaleCounts[stKey]++
		}
	}

	findMaxKey := func(m map[string]int) string {
		maxVal := -1
		maxKey := ""
		for k, v := range m {
			if v > maxVal {
				maxVal = v
				maxKey = k
			}
		}
		return maxKey
	}

	setupYangSeringLangsungSL := findMaxKey(setupSLCounts)
	setupYangSeringExpired := findMaxKey(setupExpiredCounts)
	setupYangSeringStale := findMaxKey(setupStaleCounts)

	// Conflict & Cooldown stats count from decision audits
	conflictStats := make(map[string]int)
	cooldownStats := make(map[string]int)
	if hasAudits {
		for _, a := range audits {
			if a.ConflictReason != "" {
				conflictStats[a.ConflictReason]++
			}
			if a.CooldownReason != "" {
				cooldownStats[a.CooldownReason]++
			}
		}
	}

	// 4. Gate Bug Detection
	var gateBugFindings []string
	gateBugsFound := make(map[string]bool)

	// Helper to add a gate bug finding
	addGateBug := func(playbook, finding string) {
		gateBugFindings = append(gateBugFindings, finding)
		gateBugsFound[playbook] = true
		gateBugsFound["ALL"] = true
	}

	// Inspect signal journal & audits for gate bugs
	for _, item := range finalized {
		if item.Status == TP1_HIT || item.Status == TP2_HIT || item.Status == SL_HIT {
			// Sinyal tereksekusi (FINAL_EXECUTE)
			pb := string(item.Playbook)

			// 1. AI confidence not HIGH
			if item.AIConfidence != "" && item.AIConfidence != "HIGH" {
				addGateBug(pb, "GATE_BUG: AI confidence was "+item.AIConfidence+" but signal was executed on "+item.Symbol)
			}

			// 2. Staleness not FRESH
			if item.EntryTiming == "LATE" || strings.Contains(strings.ToLower(item.ThresholdProfileSummary), "staleness: late") {
				addGateBug(pb, "GATE_BUG: Staleness was not FRESH but signal was executed on "+item.Symbol)
			}

			// 3. Playbook Specific Gate Violations
			summary := strings.ToLower(item.ThresholdProfileSummary)
			if item.Playbook == "LIQUIDITY_SWEEP_REVERSAL" && (strings.Contains(summary, "volume confirmation: false") || strings.Contains(summary, "low volume ratio")) {
				addGateBug(pb, "GATE_BUG: LIQUIDITY_SWEEP_REVERSAL executed without volume confirmation on "+item.Symbol)
			}
			if item.Playbook == "COMPRESSION_BREAKOUT_RETEST" && (strings.Contains(summary, "first breakout candle") || strings.Contains(summary, "no retest")) {
				addGateBug(pb, "GATE_BUG: COMPRESSION_BREAKOUT_RETEST executed on first breakout candle or without retest on "+item.Symbol)
			}
			if item.Playbook == "RANGE_EDGE_REVERSAL" && (strings.Contains(summary, "adx expansion") || strings.Contains(summary, "strong expansion")) {
				addGateBug(pb, "GATE_BUG: RANGE_EDGE_REVERSAL executed during strong ADX expansion on "+item.Symbol)
			}
			if item.Playbook == "CROWDED_POSITIONING_SQUEEZE" && (strings.Contains(summary, "weak crowding") || strings.Contains(summary, "no crowding evidence")) {
				addGateBug(pb, "GATE_BUG: CROWDED_POSITIONING_SQUEEZE executed without crowding evidence on "+item.Symbol)
			}
		}
	}

	if hasAudits {
		for _, a := range audits {
			if a.FinalStatus == FINAL_EXECUTE {
				pb := string(a.Playbook)

				if a.AIConfidence != "HIGH" {
					addGateBug(pb, "GATE_BUG: AI confidence was "+a.AIConfidence+" but final status is FINAL_EXECUTE on "+a.Symbol)
				}
				if a.StalenessStatus == "LATE" {
					addGateBug(pb, "GATE_BUG: Staleness was LATE but final status is FINAL_EXECUTE on "+a.Symbol)
				}

				summary := strings.ToLower(a.ThresholdProfileSummary)
				if a.Playbook == "LIQUIDITY_SWEEP_REVERSAL" && (strings.Contains(summary, "volume confirmation: false") || strings.Contains(summary, "low volume ratio")) {
					addGateBug(pb, "GATE_BUG: LIQUIDITY_SWEEP_REVERSAL executed without volume confirmation on "+a.Symbol)
				}
				if a.Playbook == "COMPRESSION_BREAKOUT_RETEST" && (strings.Contains(summary, "first breakout candle") || strings.Contains(summary, "no retest")) {
					addGateBug(pb, "GATE_BUG: COMPRESSION_BREAKOUT_RETEST executed on first breakout candle or without retest on "+a.Symbol)
				}
				if a.Playbook == "RANGE_EDGE_REVERSAL" && (strings.Contains(summary, "adx expansion") || strings.Contains(summary, "strong expansion")) {
					addGateBug(pb, "GATE_BUG: RANGE_EDGE_REVERSAL executed during strong ADX expansion on "+a.Symbol)
				}
				if a.Playbook == "CROWDED_POSITIONING_SQUEEZE" && (strings.Contains(summary, "weak crowding") || strings.Contains(summary, "no crowding evidence")) {
					addGateBug(pb, "GATE_BUG: CROWDED_POSITIONING_SQUEEZE executed without crowding evidence on "+a.Symbol)
				}
			}
		}
	}

	// 5. Build Recommendations
	var recommendations []ThresholdRecommendation

	// Helper to add recommendation with sample guard
	addRec := func(rec ThresholdRecommendation, sampleSize int) {
		conf, reqMore, sev := getSampleGuard(sampleSize)
		rec.SampleSize = sampleSize
		rec.ConfidenceLevel = conf
		rec.RequiresMoreData = reqMore
		rec.Severity = sev
		rec.DoNotAutoApply = true

		if sampleSize < 10 {
			rec.IssueType = "INSUFFICIENT_SAMPLE"
			rec.SuggestedAction = "HOLD TUNING: Insufficient sample size (< 10) to make recommendations."
			rec.SuggestedThreshold = "KEEP_CURRENT"
		}

		// If a gate bug is detected on this playbook, override action and severity!
		if gateBugsFound[rec.Playbook] {
			rec.SuggestedAction = "HOLD TUNING: do_not_tune_until_gate_fixed (Gate bug detected)"
			rec.Severity = "WARNING"
			rec.Reason = "Tuning is suspended because a gate bug was detected on this playbook. Fix the Local Gate or Final Gate first."
		}

		recommendations = append(recommendations, rec)
	}

	// Recommendation 1: SHORT during BTC Bullish often SL
	shortBullishCount := 0
	shortBullishSLCount := 0
	for _, item := range finalized {
		if item.Direction == SHORT && (item.MarketRegime == "BULLISH" || item.MarketRegime == "ALT_SUPPORTIVE" || item.MarketRegime == "BTC_DOMINANCE") {
			shortBullishCount++
			if item.Status == SL_HIT {
				shortBullishSLCount++
			}
		}
	}
	if shortBullishCount > 0 && safeRate(shortBullishSLCount, shortBullishCount) > 50 {
		addRec(ThresholdRecommendation{
			IssueType:          "POLICY_TUNING",
			Playbook:           "ALL",
			MarketRegime:       "BULLISH",
			Direction:          "SHORT",
			MetricName:         "SHORT_BULLISH_SL_RATE",
			MetricValue:        safeRate(shortBullishSLCount, shortBullishCount),
			CurrentThreshold:   "LongMode/ShortMode active",
			SuggestedThreshold: "ShortMode: SWEEP_ONLY",
			EvidenceSummary:    "Short signals during bullish market regimes suffer high stop-out rates.",
			Reason:             "Counter-trend shorting during bullish regimes leads to stop-outs.",
			SuggestedAction:    "Restrict ShortMode in MarketPolicy to SWEEP_ONLY during ALT_SUPPORTIVE or BULLISH regimes, and block trend continuation shorts.",
		}, shortBullishCount)
	}

	// Recommendation 2: LONG saat RISK_OFF sering SL
	longRiskOffCount := 0
	longRiskOffSLCount := 0
	for _, item := range finalized {
		if item.Direction == LONG && (item.MarketRegime == "RISK_OFF" || item.MarketRegime == "BEARISH") {
			longRiskOffCount++
			if item.Status == SL_HIT {
				longRiskOffSLCount++
			}
		}
	}
	if longRiskOffCount > 0 && safeRate(longRiskOffSLCount, longRiskOffCount) > 50 {
		addRec(ThresholdRecommendation{
			IssueType:          "POLICY_TUNING",
			Playbook:           "ALL",
			MarketRegime:       "RISK_OFF",
			Direction:          "LONG",
			MetricName:         "LONG_RISK_OFF_SL_RATE",
			MetricValue:        safeRate(longRiskOffSLCount, longRiskOffCount),
			CurrentThreshold:   "LongMode/ShortMode active",
			SuggestedThreshold: "LongMode: REVERSAL_ONLY",
			EvidenceSummary:    "Long signals during RISK_OFF regimes suffer high stop-out rates.",
			Reason:             "Trend continuation longs fail when overall market is in RISK_OFF or BEARISH mode.",
			SuggestedAction:    "Restrict LongMode to REVERSAL_ONLY during RISK_OFF regimes, allowing only high-conviction low sweeps or range edge rejections.",
		}, longRiskOffCount)
	}

	// Recommendation 3: Tier C sering gagal
	tierCCount := 0
	tierCSLCount := 0
	var tierCMAESum float64
	for _, item := range finalized {
		if item.Tier == TierC {
			tierCCount++
			tierCMAESum += item.MAE
			if item.Status == SL_HIT {
				tierCSLCount++
			}
		}
	}
	if tierCCount > 0 && (safeRate(tierCSLCount, tierCCount) > 50 || safeDiv(tierCMAESum, float64(tierCCount)) > 5.0) {
		addRec(ThresholdRecommendation{
			IssueType:          "POLICY_TUNING",
			Playbook:           "ALL",
			Tier:               "TierC",
			MetricName:         "TIER_C_SL_RATE",
			MetricValue:        safeRate(tierCSLCount, tierCCount),
			CurrentThreshold:   "AllowedTiers includes Tier C",
			SuggestedThreshold: "Block Tier C in High Volatility / Chaos",
			EvidenceSummary:    "Tier C assets suffer high stop-out rates and large maximum adverse excursions (MAE).",
			Reason:             "Low-liquidity Tier C assets exhibit erratic behavior and extreme slippage during volatility expansion.",
			SuggestedAction:    "Block Tier C execution in MarketPolicy during HIGH_VOLATILITY or BTC_CHAOS, or tighten its StalenessATR constraints.",
		}, tierCCount)
	}

	// Recommendation 4: TREND_PULLBACK sering gagal
	tpCount := 0
	tpSLCount := 0
	tpLowADXSL := 0
	tpChopSL := 0
	for _, item := range finalized {
		if item.Playbook == "TREND_PULLBACK" {
			tpCount++
			if item.Status == SL_HIT {
				tpSLCount++
				summary := strings.ToLower(item.ThresholdProfileSummary)
				if strings.Contains(summary, "adx") || strings.Contains(summary, "momentum") {
					tpLowADXSL++
				}
				if item.MarketRegime == "CHOP" || item.MarketRegime == "RANGE" || item.MarketRegime == "LOW_VOLATILITY" {
					tpChopSL++
				}
			}
		}
	}
	if tpCount > 0 && safeRate(tpSLCount, tpCount) > 40 {
		addRec(ThresholdRecommendation{
			IssueType:          "THRESHOLD_TUNING",
			Playbook:           "TREND_PULLBACK",
			MetricName:         "TREND_PULLBACK_SL_RATE",
			MetricValue:        safeRate(tpSLCount, tpCount),
			CurrentThreshold:   "MinADX: 20",
			SuggestedThreshold: "MinADX: 25",
			EvidenceSummary:    "TREND_PULLBACK has high stop-outs associated with low ADX values and range chops.",
			Reason:             "Trend pullbacks require a strong active trend to continuation; entering in chop leads to range bounds stop-out.",
			SuggestedAction:    "Increase MinADX to 25 and disable TREND_PULLBACK during CHOP_RANGE regimes.",
		}, tpCount)
	}

	// Recommendation 5: LIQUIDITY_SWEEP_REVERSAL sering gagal
	lsCount := 0
	lsSLCount := 0
	for _, item := range finalized {
		if item.Playbook == "LIQUIDITY_SWEEP_REVERSAL" {
			lsCount++
			if item.Status == SL_HIT {
				lsSLCount++
			}
		}
	}
	if lsCount > 0 && safeRate(lsSLCount, lsCount) > 40 {
		addRec(ThresholdRecommendation{
			IssueType:          "THRESHOLD_TUNING",
			Playbook:           "LIQUIDITY_SWEEP_REVERSAL",
			MetricName:         "LIQUIDITY_SWEEP_REVERSAL_SL_RATE",
			MetricValue:        safeRate(lsSLCount, lsCount),
			CurrentThreshold:   "MinVolumeRatio: 1.5",
			SuggestedThreshold: "MinVolumeRatio: 1.8",
			EvidenceSummary:    "Sweep reversals suffer stop-outs on weak volume confirmation.",
			Reason:             "Liquidity sweep reversals require high volume validation to confirm exhaustion of counterparty orders.",
			SuggestedAction:    "Increase MinVolumeRatio to 1.8 and require wick rejection confirmation.",
		}, lsCount)
	}

	// Recommendation 6: COMPRESSION_BREAKOUT_RETEST sering gagal
	cbCount := 0
	cbSLCount := 0
	cbStaleCount := 0
	for _, item := range finalized {
		if item.Playbook == "COMPRESSION_BREAKOUT_RETEST" {
			cbCount++
			if item.Status == SL_HIT {
				cbSLCount++
			}
			if item.Status == EXPIRED || item.EntryTiming == "LATE" {
				cbStaleCount++
			}
		}
	}
	if cbCount > 0 && (safeRate(cbSLCount, cbCount) > 40 || safeRate(cbStaleCount, cbCount) > 40) {
		addRec(ThresholdRecommendation{
			IssueType:          "THRESHOLD_TUNING",
			Playbook:           "COMPRESSION_BREAKOUT_RETEST",
			MetricName:         "COMPRESSION_BREAKOUT_RETEST_FAILURE_RATE",
			MetricValue:        safeRate(cbSLCount+cbStaleCount, cbCount),
			CurrentThreshold:   "AllowBreakoutCandleEntry: true",
			SuggestedThreshold: "AllowBreakoutCandleEntry: false",
			EvidenceSummary:    "Compression breakouts are hit by fake breakouts and expiration.",
			Reason:             "Entering directly on breakout candle increases stop-out rate; waiting for retest confirmation is safer.",
			SuggestedAction:    "Set AllowBreakoutCandleEntry to false, require retest hold, and tighten StalenessATR.",
		}, cbCount)
	}

	// Recommendation 7: RANGE_EDGE_REVERSAL buruk
	reCount := 0
	reSLCount := 0
	for _, item := range finalized {
		if item.Playbook == "RANGE_EDGE_REVERSAL" {
			reCount++
			if item.Status == SL_HIT {
				reSLCount++
			}
		}
	}
	if reCount > 0 && safeRate(reSLCount, reCount) > 40 {
		addRec(ThresholdRecommendation{
			IssueType:          "THRESHOLD_TUNING",
			Playbook:           "RANGE_EDGE_REVERSAL",
			MetricName:         "RANGE_EDGE_REVERSAL_SL_RATE",
			MetricValue:        safeRate(reSLCount, reCount),
			CurrentThreshold:   "MaxADX: 30",
			SuggestedThreshold: "MaxADX: 22",
			EvidenceSummary:    "Range edge reversals fail during active trend expansion.",
			Reason:             "Range boundaries break during ADX trend expansion; reversal trades must be rejected under strong trend.",
			SuggestedAction:    "Decrease MaxADX to 22 and increase MinRangeClarity requirements.",
		}, reCount)
	}

	// Recommendation 8: CROWDED_POSITIONING_SQUEEZE sering gagal
	csCount := 0
	csSLCount := 0
	for _, item := range finalized {
		if item.Playbook == "CROWDED_POSITIONING_SQUEEZE" {
			csCount++
			if item.Status == SL_HIT {
				csSLCount++
			}
		}
	}
	if csCount > 0 && safeRate(csSLCount, csCount) > 40 {
		addRec(ThresholdRecommendation{
			IssueType:          "THRESHOLD_TUNING",
			Playbook:           "CROWDED_POSITIONING_SQUEEZE",
			MetricName:         "CROWDED_POSITIONING_SQUEEZE_SL_RATE",
			MetricValue:        safeRate(csSLCount, csCount),
			CurrentThreshold:   "MinCrowdingScore: 65",
			SuggestedThreshold: "MinCrowdingScore: 75",
			EvidenceSummary:    "Crowded squeeze signals fail due to lack of price action confirmation.",
			Reason:             "Executing squeezes based solely on extreme funding without reclaim/rejection leads to stop-outs.",
			SuggestedAction:    "Increase MinCrowdingScore to 75 and require reclaim/rejection confirmation.",
		}, csCount)
	}

	// Recommendation 8b: PLAYBOOK_DISABLE for extremely poor performing playbooks
	for pbName, stats := range playbookStats {
		if stats.TotalSignals >= 10 && stats.SLRate > 60.0 {
			addRec(ThresholdRecommendation{
				IssueType:          "PLAYBOOK_DISABLE",
				Playbook:           pbName,
				MetricName:         pbName + "_EXTREME_SL_RATE",
				MetricValue:        stats.SLRate,
				CurrentThreshold:   "Playbook enabled",
				SuggestedThreshold: "Disable Playbook",
				EvidenceSummary:    fmt.Sprintf("%s has extremely high SL rate: %.2f%%", pbName, stats.SLRate),
				Reason:             "Playbook is consistently hitting stop-loss and underperforming across recent regimes.",
				SuggestedAction:    "Disable this playbook in allowed playbooks list or set its allowed tiers to empty until re-modeled.",
			}, stats.TotalSignals)
		}
	}

	// Recommendation 9: AI CONFIRM HIGH tapi hasil sering SL
	aiHighCount := 0
	aiHighSLCount := 0
	for _, item := range finalized {
		if item.AIConfidence == "HIGH" {
			aiHighCount++
			if item.Status == SL_HIT {
				aiHighSLCount++
			}
		}
	}
	if aiHighCount > 0 && safeRate(aiHighSLCount, aiHighCount) > 40 {
		addRec(ThresholdRecommendation{
			IssueType:          "AI_PROMPT_TUNING",
			Playbook:           "ALL",
			MetricName:         "AI_CONFIRM_HIGH_SL_RATE",
			MetricValue:        safeRate(aiHighSLCount, aiHighCount),
			CurrentThreshold:   "AI confidence: HIGH",
			SuggestedThreshold: "Revise prompt narrative alignment",
			EvidenceSummary:    "AI HIGH confidence signals suffer high stop-out rates.",
			Reason:             "The AI model is exhibiting high conviction on poor-quality setups.",
			SuggestedAction:    "Revise the prompt narrative checks to penalize late entries or weak volume structure.",
		}, aiHighCount)
	}

	// Recommendation 10: AI WAIT atau AI MEDIUM sering kemudian harga mencapai TP
	// We check decision audits to see if AI MEDIUM or AI WAIT signals exist.
	// If decision audits are missing, we output an INSUFFICIENT_SAMPLE warning indicating we need decision audit logs.
	if !hasAudits {
		addRec(ThresholdRecommendation{
			IssueType:        "INSUFFICIENT_SAMPLE",
			Playbook:         "ALL",
			MetricName:       "MISSED_OPPORTUNITY_EVALUATION",
			MetricValue:      0.0,
			EvidenceSummary:  "decision_audit.json is not available to track WATCH or AI_MEDIUM signals.",
			ConfidenceLevel:  "LOW",
			Reason:           "Need decision_audit/watchlist monitoring to evaluate AI_WAIT or AI_MEDIUM.",
			SuggestedAction:  "Enable and populate decision_audit.json files.",
			DoNotAutoApply:   true,
			RequiresMoreData: true,
			Severity:         "INFO",
		}, 0)
	} else {
		aiMediumCount := 0
		for _, a := range audits {
			if a.AIConfidence == "MEDIUM" || a.FinalStatus == FINAL_WATCH {
				aiMediumCount++
			}
		}
		if aiMediumCount > 0 {
			addRec(ThresholdRecommendation{
				IssueType:          "AI_PROMPT_TUNING",
				Playbook:           "ALL",
				MetricName:         "AI_MEDIUM_WATCH_COUNT",
				MetricValue:        float64(aiMediumCount),
				CurrentThreshold:   "AI MEDIUM -> watch list",
				SuggestedThreshold: "Evaluate WATCH_RETEST alert",
				EvidenceSummary:    "Significant count of AI Medium/Watch decisions recorded.",
				Reason:             "AI Medium decisions can be evaluated to determine if they frequently hit profit targets.",
				SuggestedAction:    "Evaluate WATCH_RETEST alerts and do not immediately execute AI Medium setups.",
			}, aiMediumCount)
		}
	}

	// Recommendation 11: Staleness sering menyebabkan MISSED
	lateCount := 0
	for _, item := range finalized {
		if item.EntryTiming == "LATE" || item.OutcomeReason == "stale" {
			lateCount++
		}
	}
	if hasAudits {
		for _, a := range audits {
			if a.StalenessStatus == "LATE" {
				lateCount++
			}
		}
	}
	if lateCount > 0 {
		addRec(ThresholdRecommendation{
			IssueType:          "STALENESS_TUNING",
			Playbook:           "ALL",
			MetricName:         "STALE_LOG_COUNT",
			MetricValue:        float64(lateCount),
			CurrentThreshold:   "Staleness check limits active",
			SuggestedThreshold: "Optimise StalenessATR limits",
			EvidenceSummary:    "Late execution limits or staleness status triggered frequently.",
			Reason:             "High latency or loose staleness criteria degrades execution quality.",
			SuggestedAction:    "Reduce AI candidates limit or slightly widen StalenessATR for slow playbooks like TREND_PULLBACK.",
		}, lateCount)
	}

	// Recommendation 12: FINAL_EXECUTE sering turun karena Conflict Resolver
	downgradedCount := 0
	if hasAudits {
		for _, a := range audits {
			if a.FinalStatus != FINAL_EXECUTE && (a.ConflictReason == "OPPOSITE_SIGNAL_CONFLICT" || a.ConflictReason == "LOWER_PRIORITY_CONFLICT") {
				downgradedCount++
			}
		}
	}
	if downgradedCount > 0 {
		addRec(ThresholdRecommendation{
			IssueType:          "CONFLICT_TUNING",
			Playbook:           "ALL",
			MetricName:         "CONFLICT_DOWNGRADED_COUNT",
			MetricValue:        float64(downgradedCount),
			CurrentThreshold:   "Conflict resolver active",
			SuggestedThreshold: "Perketat score gap in Arbiter",
			EvidenceSummary:    "Sinyal FINAL_EXECUTE downgraded to FINAL_WATCH due to direction conflict.",
			Reason:             "Arbiter filters are too loose, letting opposing signals reach the execution gate.",
			SuggestedAction:    "Increase the Candidate Arbiter score gap to resolve conflicts earlier.",
		}, downgradedCount)
	}

	// Recommendation 13: Banyak signal kena cooldown
	cooldownCount := 0
	if hasAudits {
		for _, a := range audits {
			if a.CooldownReason != "" {
				cooldownCount++
			}
		}
	}
	if cooldownCount > 0 {
		addRec(ThresholdRecommendation{
			IssueType:          "COOLDOWN_TUNING",
			Playbook:           "ALL",
			MetricName:         "COOLDOWN_REJECTED_COUNT",
			MetricValue:        float64(cooldownCount),
			CurrentThreshold:   "Dynamic cooldown active",
			SuggestedThreshold: "Evaluate cooldown duration",
			EvidenceSummary:    "Signals rejected due to active cooldown limits.",
			Reason:             "Cooldown blocks help prevent repeating poor setups on the same symbol.",
			SuggestedAction:    "Maintain dynamic cooldown duration to prevent consecutive stop-out duplicate losses.",
		}, cooldownCount)
	}

	// Recommendation 14: Banyak EXPIRED
	if expiredRate > 35 {
		addRec(ThresholdRecommendation{
			IssueType:          "TARGET_TUNING",
			Playbook:           "ALL",
			MetricName:         "EXPIRED_RATE",
			MetricValue:        expiredRate,
			CurrentThreshold:   "MaxHold: 120m",
			SuggestedThreshold: "Reduce TP1 distance or adjust MaxHold",
			EvidenceSummary:    "High rate of signals expire before hitting stop-loss or take-profit.",
			Reason:             "Profit targets are too aggressive or volatility is too low for the current timeframe.",
			SuggestedAction:    "Lower TP1 target size for range strategies, and restrict breakout trades in low volatility.",
		}, totalCount)
	}

	// Recommendation 15: TP1 hit lalu sering balik kuat
	tp1ThenSLCount := 0
	for _, item := range finalized {
		if (item.TimeToTP1 != "" || item.Status == TP1_HIT) && item.Status == SL_HIT {
			tp1ThenSLCount++
		}
	}
	if tp1Hits > 0 && safeRate(tp1ThenSLCount, tp1Hits) > 40 {
		addRec(ThresholdRecommendation{
			IssueType:          "TARGET_TUNING",
			Playbook:           "ALL",
			MetricName:         "TP1_REVERSAL_RATE",
			MetricValue:        safeRate(tp1ThenSLCount, tp1Hits),
			CurrentThreshold:   "Standard TP1/TP2 execution",
			SuggestedThreshold: "Move SL to Breakeven after TP1",
			EvidenceSummary:    "Signals frequently hit TP1 but reverse to stop-loss.",
			Reason:             "Holding full position for TP2 exposes capital to pullback risks after initial targets are hit.",
			SuggestedAction:    "Move stop-loss to entry price immediately after TP1 is hit.",
		}, tp1Hits)
	}

	// Recommendation 16: MAE besar sebelum profit
	largeMAECount := 0
	for _, item := range finalized {
		if (item.Status == TP1_HIT || item.Status == TP2_HIT) && item.MAE > 3.0 {
			largeMAECount++
		}
	}
	if wins > 0 && safeRate(largeMAECount, wins) > 40 {
		addRec(ThresholdRecommendation{
			IssueType:          "ENTRY_TUNING",
			Playbook:           "ALL",
			MetricName:         "LARGE_MAE_WIN_RATE",
			MetricValue:        safeRate(largeMAECount, wins),
			CurrentThreshold:   "Standard entry distance",
			SuggestedThreshold: "Wait for closer entry retest",
			EvidenceSummary:    "Winning signals suffer large maximum adverse excursions (MAE) before profit.",
			Reason:             "Entering immediately at trigger price leaves a wide gap to value area, leading to drawdown.",
			SuggestedAction:    "Widen the entry retest zone or restrict entries to pullbacks closer to value support.",
		}, wins)
	}

	// Recommendation 17: RR rendah sering gagal
	lowRRCount := 0
	lowRRSLCount := 0
	for _, item := range finalized {
		if item.RR < 1.5 {
			lowRRCount++
			if item.Status == SL_HIT {
				lowRRSLCount++
			}
		}
	}
	if lowRRCount > 0 && safeRate(lowRRSLCount, lowRRCount) > 50 {
		addRec(ThresholdRecommendation{
			IssueType:          "THRESHOLD_TUNING",
			Playbook:           "ALL",
			MetricName:         "LOW_RR_SL_RATE",
			MetricValue:        safeRate(lowRRSLCount, lowRRCount),
			CurrentThreshold:   "MinRR: 1.0",
			SuggestedThreshold: "MinRR: 1.5",
			EvidenceSummary:    "Low risk-to-reward ratio signals experience high stop-out rates.",
			Reason:             "Low RR trades lack necessary edge to overcome transaction costs and stop slippage.",
			SuggestedAction:    "Increase MinRR threshold per playbook profile to a minimum of 1.5.",
		}, lowRRCount)
	}

	// Recommendation 18: Funding ekstrem melawan arah sering gagal
	fundingOpposeCount := 0
	fundingOpposeSLCount := 0
	for _, item := range finalized {
		summary := strings.ToLower(item.ThresholdProfileSummary)
		if strings.Contains(summary, "funding") || strings.Contains(summary, "crowding") {
			fundingOpposeCount++
			if item.Status == SL_HIT {
				fundingOpposeSLCount++
			}
		}
	}
	if fundingOpposeCount > 0 && safeRate(fundingOpposeSLCount, fundingOpposeCount) > 50 {
		addRec(ThresholdRecommendation{
			IssueType:          "POLICY_TUNING",
			Playbook:           "ALL",
			MetricName:         "FUNDING_OPPOSE_SL_RATE",
			MetricValue:        safeRate(fundingOpposeSLCount, fundingOpposeCount),
			CurrentThreshold:   "MaxFundingAbs: 2.0%",
			SuggestedThreshold: "MaxFundingAbs: 1.0%",
			EvidenceSummary:    "Signals executing against extreme funding rates frequently hit SL.",
			Reason:             "Trading counter to massive funding flows is high risk without confirmed momentum shift.",
			SuggestedAction:    "Restrict trading against funding direction, or decrease MaxFundingAbs to 1.0%.",
		}, fundingOpposeCount)
	}

	// Recommendation 19: High volatility sering SL
	highVolCount := 0
	highVolSLCount := 0
	for _, item := range finalized {
		if item.MarketRegime == "HIGH_VOLATILITY" || item.MarketRegime == "BTC_CHAOS" {
			highVolCount++
			if item.Status == SL_HIT {
				highVolSLCount++
			}
		}
	}
	if highVolCount > 0 && safeRate(highVolSLCount, highVolCount) > 50 {
		addRec(ThresholdRecommendation{
			IssueType:          "POLICY_TUNING",
			Playbook:           "ALL",
			MarketRegime:       "HIGH_VOLATILITY",
			MetricName:         "HIGH_VOL_SL_RATE",
			MetricValue:        safeRate(highVolSLCount, highVolCount),
			CurrentThreshold:   "MaxSymbols: 5",
			SuggestedThreshold: "Reduce MaxSymbols / Increase MinScore",
			EvidenceSummary:    "Signals during high volatility or chaos regimes suffer high stop-outs.",
			Reason:             "Volatility expansions generate false breakouts and spike liquidation wicks.",
			SuggestedAction:    "Reduce MaxSymbols limits, block Tier C assets, and raise MinScoreExecute during chaos.",
		}, highVolCount)
	}

	// Recommendation 20: Low volatility banyak expired
	lowVolCount := 0
	lowVolExpCount := 0
	for _, item := range finalized {
		if item.MarketRegime == "LOW_VOLATILITY" || item.MarketRegime == "CHOP" {
			lowVolCount++
			if item.Status == EXPIRED {
				lowVolExpCount++
			}
		}
	}
	if lowVolCount > 0 && safeRate(lowVolExpCount, lowVolCount) > 50 {
		addRec(ThresholdRecommendation{
			IssueType:          "TARGET_TUNING",
			Playbook:           "ALL",
			MarketRegime:       "LOW_VOLATILITY",
			MetricName:         "LOW_VOL_EXPIRED_RATE",
			MetricValue:        safeRate(lowVolExpCount, lowVolCount),
			CurrentThreshold:   "Standard TP levels",
			SuggestedThreshold: "Focus on Compression / Scalp TP",
			EvidenceSummary:    "Signals in low volatility regimes expire without reaching targets.",
			Reason:             "Market range boundaries are compressed, preventing price from reaching distant profit targets.",
			SuggestedAction:    "Lower take-profit targets (scalp-level) and prioritize compression breakout retests.",
		}, lowVolCount)
	}

	// 5.1 Append GATE_BUG Findings as priority recommendations
	for _, finding := range gateBugFindings {
		playbook := "ALL"
		if strings.Contains(finding, "TREND_PULLBACK") {
			playbook = "TREND_PULLBACK"
		} else if strings.Contains(finding, "LIQUIDITY_SWEEP_REVERSAL") {
			playbook = "LIQUIDITY_SWEEP_REVERSAL"
		} else if strings.Contains(finding, "COMPRESSION_BREAKOUT_RETEST") {
			playbook = "COMPRESSION_BREAKOUT_RETEST"
		} else if strings.Contains(finding, "RANGE_EDGE_REVERSAL") {
			playbook = "RANGE_EDGE_REVERSAL"
		} else if strings.Contains(finding, "CROWDED_POSITIONING_SQUEEZE") {
			playbook = "CROWDED_POSITIONING_SQUEEZE"
		}

		recommendations = append(recommendations, ThresholdRecommendation{
			IssueType:        "GATE_BUG",
			Playbook:         playbook,
			SampleSize:       totalCount,
			EvidenceSummary:  finding,
			ConfidenceLevel:  "HIGH",
			Reason:           "Mandatory execution policy or threshold was violated by a FINAL_EXECUTE decision.",
			SuggestedAction:  "Fix Local Gate / Final Gate / Orchestrator validation bugs immediately.",
			DoNotAutoApply:   true,
			RequiresMoreData: false,
			Severity:         "HIGH",
		})
	}

	// 5.2 Deterministic Sorting of Recommendations
	sort.Slice(recommendations, func(i, j int) bool {
		// Rank Severity: HIGH/CRITICAL first, then WARNING, then INFO
		sevRank := func(s string) int {
			switch s {
			case "CRITICAL":
				return 4
			case "HIGH":
				return 3
			case "WARNING":
				return 2
			case "INFO":
				return 1
			default:
				return 0
			}
		}
		rankI := sevRank(recommendations[i].Severity)
		rankJ := sevRank(recommendations[j].Severity)
		if rankI != rankJ {
			return rankI > rankJ
		}
		if recommendations[i].Playbook != recommendations[j].Playbook {
			return recommendations[i].Playbook < recommendations[j].Playbook
		}
		return recommendations[i].IssueType < recommendations[j].IssueType
	})

	// 6. Build Final Report Object
	report := EvaluationReport{
		GeneratedAt:      time.Now(),
		ConfigVersion:    GetGlobalConfigRegistry().GetVersion(),
		SourceFilesUsed:  sourceFiles,
		DataCompleteness: completeness,
		TotalSignals:     totalCount,
		Metrics: map[string]float64{
			"win_rate":             winRate,
			"tp1_rate":             tp1Rate,
			"tp2_rate":             tp2Rate,
			"sl_rate":              slRate,
			"expired_rate":         expiredRate,
			"average_mfe":          avgMFE,
			"average_mae":          avgMAE,
			"average_rr":           avgRR,
			"average_time_to_tp1":  avgTimeToTP1,
			"average_time_to_tp2":  avgTimeToTP2,
			"average_time_to_sl":   avgTimeToSL,
			"average_holding_time": avgHoldingTime,
			"total_pnl_percentage": totalPnl,
		},
		PlaybookStats:             playbookStats,
		RegimeStats:               regimeStats,
		TierStats:                 tierStats,
		DirectionStats:            directionStats,
		AIStats:                   aiStats,
		StalenessStats:            stalenessStats,
		ConflictStats:             conflictStats,
		CooldownStats:             cooldownStats,
		GateBugFindings:           gateBugFindings,
		Recommendations:           recommendations,
		BestPlaybook:              bestPb,
		WorstPlaybook:             worstPb,
		SetupYangSeringLangsungSL: setupYangSeringLangsungSL,
		SetupYangSeringExpired:    setupYangSeringExpired,
		SetupYangSeringStale:      setupYangSeringStale,
		RegimeYangPalingBuruk:     worstRegime,
		TierYangPalingBuruk:       worstTier,
		DirectionYangPalingBuruk:  worstDirection,
		PlaybookDenganMAETerbesar: pbMaxMAE,
		PlaybookDenganExpiredRate: pbMaxExp,
		PlaybookDenganTP1Terbaik:  pbBestTP1,
		PlaybookDenganTP2Follow:   pbBestTP2Follow,
		Notes:                     "Feedback Loop Revision generated successfully.",
		Status:                    "COMPLETED",
	}
	GetGlobalMetrics().SetEvalMetrics(uint64(len(recommendations)), uint64(len(gateBugFindings)))
	GetGlobalMetrics().SetLastEvaluationTime(report.GeneratedAt)

	return uc.storageUsecase.SaveEvaluationReport(report)
}
