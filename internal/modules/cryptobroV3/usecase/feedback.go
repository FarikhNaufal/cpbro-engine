package usecase

import (
	"cpbro-engine/internal/modules/cryptobroV3/config"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"
)

type FeedbackUsecase struct {
	storageUsecase *StorageUsecase
}

type feedbackSourceFiles struct {
	latestResult  string
	signalJournal string
	watchJournal  string
	decisionAudit string
}

func NewFeedbackUsecase(storage *StorageUsecase) *FeedbackUsecase {
	return &FeedbackUsecase{
		storageUsecase: storage,
	}
}

func resolveFeedbackSourceFiles() feedbackSourceFiles {
	settings := getRuntimeSettings()
	files := feedbackSourceFiles{
		latestResult:  strings.TrimSpace(settings.LatestResultFile),
		signalJournal: strings.TrimSpace(settings.SignalJournalFile),
		watchJournal:  strings.TrimSpace(settings.WatchJournalFile),
		decisionAudit: strings.TrimSpace(settings.DecisionAuditFile),
	}
	if files.latestResult == "" {
		files.latestResult = config.DefaultLatestResultFile
	}
	if files.signalJournal == "" {
		files.signalJournal = config.DefaultSignalJournalFile
	}
	if files.watchJournal == "" {
		files.watchJournal = config.DefaultWatchJournalFile
	}
	if files.decisionAudit == "" {
		files.decisionAudit = config.DefaultDecisionAuditFile
	}
	return files
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

func maxFloat(left, right float64) float64 {
	if left > right {
		return left
	}
	return right
}

func maxEvalInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func canonicalRegimeLabel(label string) string {
	normalized := strings.ToUpper(strings.TrimSpace(label))
	switch normalized {
	case "LOW_VOLATILITY":
		return string(LOW_VOL)
	case "HIGH_VOLATILITY":
		return string(HIGH_VOL)
	case "CHOP":
		return string(CHOP_RANGE)
	case "BEARISH":
		return string(RISK_OFF)
	case "BULLISH":
		return string(ALT_SUPPORTIVE)
	default:
		switch {
		case strings.Contains(normalized, string(BTC_CHAOS)) || strings.Contains(normalized, "CHAOS"):
			return string(BTC_CHAOS)
		case strings.Contains(normalized, string(HIGH_VOL)) || strings.Contains(normalized, "HIGH_VOLATILITY"):
			return string(HIGH_VOL)
		case strings.Contains(normalized, string(LOW_VOL)) || strings.Contains(normalized, "LOW_VOLATILITY"):
			return string(LOW_VOL)
		case strings.Contains(normalized, string(COMPRESSION)):
			return string(COMPRESSION)
		case strings.Contains(normalized, string(ALT_SUPPORTIVE)) || strings.Contains(normalized, "BULLISH"):
			return string(ALT_SUPPORTIVE)
		case strings.Contains(normalized, string(BTC_DOMINANCE)) || strings.Contains(normalized, "DOMINANCE"):
			return string(BTC_DOMINANCE)
		case strings.Contains(normalized, string(RISK_OFF)) || strings.Contains(normalized, "BEARISH"):
			return string(RISK_OFF)
		case strings.Contains(normalized, string(CHOP_RANGE)) || strings.Contains(normalized, "SIDEWAYS"):
			return string(CHOP_RANGE)
		default:
			return normalized
		}
	}
}

func regimeIsAny(label string, regimes ...MarketRegime) bool {
	canonical := canonicalRegimeLabel(label)
	for _, regime := range regimes {
		if canonical == string(regime) {
			return true
		}
	}
	return false
}

func formatFloatThreshold(name string, value float64) string {
	if value == float64(int(value)) {
		return fmt.Sprintf("%s: %.0f", name, value)
	}
	return fmt.Sprintf("%s: %.2f", name, value)
}

func resolveFeedbackPolicyForRegime(label string) MarketPolicy {
	canonical := canonicalRegimeLabel(label)

	switch canonical {
	case string(BTC_CHAOS):
		policy := mustResolvePolicyBaseline("BTC_CHAOS")
		policy.Regime = BTC_CHAOS
		return policy
	case string(BTC_DOMINANCE):
		policy := mustResolvePolicyBaseline("BTC_DOMINANCE")
		policy.Regime = BTC_DOMINANCE
		return policy
	case string(ALT_SUPPORTIVE):
		policy := mustResolvePolicyBaseline("ALT_SUPPORTIVE")
		policy.Regime = ALT_SUPPORTIVE
		return policy
	case string(RISK_OFF):
		policy := mustResolvePolicyBaseline("RISK_OFF")
		policy.Regime = RISK_OFF
		return policy
	case string(CHOP_RANGE):
		policy := mustResolvePolicyBaseline("CHOP_RANGE")
		policy.Regime = CHOP_RANGE
		return policy
	case string(COMPRESSION):
		return normalizeCompressionPolicy(mustResolvePolicyBaseline("COMPRESSION"), "")
	case string(LOW_VOL):
		return normalizeLowVolPolicy(mustResolvePolicyBaseline("DEFAULT"), "", "LOW_VOL active - reversal/watch mode")
	case string(HIGH_VOL):
		policy := mustResolvePolicyBaseline("DEFAULT")
		policy.Regime = HIGH_VOL
		policy.MinVolume = 10000000.0
		policy.AllowedTiers = []Tier{TierA, TierB}
		policy.MaxFinalExecute = 2
		policy.StalenessATRMultiplier = 0.8
		policy.AllowedPlaybooks = []Playbook{LIQUIDITY_SWEEP_REVERSAL, TREND_PULLBACK}
		policy.RequireAIConfidence = AIConfidenceHigh
		policy.RequireFreshEntry = true
		policy.MaxPriceMove24hLong = 0.08
		policy.MaxPriceMove24hShort = 0.10
		policy.Reason = "HIGH_VOL active - strict risk reduction mode"
		return policy
	default:
		policy := mustResolvePolicyBaseline("DEFAULT")
		policy.Regime = DEFAULT
		return policy
	}
}

func resolveFeedbackExecutionConfig(playbook Playbook, regimeLabel string, tier Tier) (MarketPolicy, PlaybookThresholdProfile) {
	policy := resolveFeedbackPolicyForRegime(regimeLabel)
	profile := GetPlaybookThresholdProfile(playbook, policy, tier)
	return policy, profile
}

func feedbackPolicyModeForDirection(policy MarketPolicy, direction Direction) string {
	if direction == SHORT {
		return string(policy.ShortMode)
	}
	return string(policy.LongMode)
}

func feedbackPolicyAllowsPlaybook(policy MarketPolicy, playbook Playbook) bool {
	for _, allowed := range policy.AllowedPlaybooks {
		if allowed == playbook {
			return true
		}
	}
	return false
}

func resolveFeedbackSliceBlockReason(playbook Playbook, regimeLabel string, direction Direction) (bool, string, string) {
	policy := resolveFeedbackPolicyForRegime(regimeLabel)
	policyMode := feedbackPolicyModeForDirection(policy, direction)

	if !feedbackPolicyAllowsPlaybook(policy, playbook) {
		return true, policyMode, fmt.Sprintf("Current %s policy does not include playbook %s in AllowedPlaybooks", policy.Regime, playbook)
	}
	if !ValidateDirectionalPath(policy, direction, playbook) {
		return true, policyMode, modeRejectReason(policy, direction, playbook)
	}

	canonicalRegime := canonicalRegimeLabel(regimeLabel)
	if (canonicalRegime == "" || canonicalRegime == string(DEFAULT)) && direction == LONG && playbook == COMPRESSION_BREAKOUT_RETEST {
		return true, policyMode, "Current selector suppresses LONG COMPRESSION_BREAKOUT_RETEST in DEFAULT regime"
	}

	return false, policyMode, ""
}

func normalizeM5ConfirmationModeLabel(value string) M5ConfirmationMode {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case string(M5ConfirmationWatchOnlyHint):
		return M5ConfirmationWatchOnlyHint
	case string(M5ConfirmationSoftConfirm):
		return M5ConfirmationSoftConfirm
	case string(M5ConfirmationHardConfirm):
		return M5ConfirmationHardConfirm
	default:
		return M5ConfirmationDisabled
	}
}

func normalizeM5ConfirmationStatusLabel(value string) M5ConfirmationStatus {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case string(M5ConfirmationUnavailable):
		return M5ConfirmationUnavailable
	case string(M5ConfirmationConfirmed):
		return M5ConfirmationConfirmed
	case string(M5ConfirmationFailed):
		return M5ConfirmationFailed
	case string(M5ConfirmationInvalidated):
		return M5ConfirmationInvalidated
	default:
		return M5ConfirmationNotUsed
	}
}

func m5ExecutionViolatesMode(mode M5ConfirmationMode, status M5ConfirmationStatus) bool {
	if status == M5ConfirmationInvalidated {
		return true
	}
	if mode == M5ConfirmationSoftConfirm || mode == M5ConfirmationHardConfirm {
		return status == M5ConfirmationFailed || status == M5ConfirmationUnavailable
	}
	return false
}

func isFinalizedSignalJournalForEvaluation(item SignalJournal, now time.Time) bool {
	switch item.Status {
	case TP2_HIT, SL_HIT, EXPIRED, BREAKEVEN:
		return true
	case TP1_HIT:
		return isTP1FinalizedForJournal(item, now)
	default:
		return false
	}
}

func isFinalizedWatchJournalForEvaluation(item WatchJournal, now time.Time) bool {
	switch item.Status {
	case VIRTUAL_TP2_HIT, VIRTUAL_SL_HIT, VIRTUAL_EXPIRED, WATCH_RECHECK_INVALIDATED, WATCH_RECHECK_EXPIRED:
		return true
	case VIRTUAL_TP1_HIT:
		return isTP1FinalizedForJournal(SignalJournal(item), now)
	default:
		return false
	}
}

func signalHasTP1Achievement(item SignalJournal) bool {
	return item.TimeToTP1 != "" || item.Status == TP1_HIT || item.Status == TP2_HIT
}

func watchHasTP1Achievement(item WatchJournal) bool {
	return item.TimeToTP1 != "" || item.Status == VIRTUAL_TP1_HIT || item.Status == VIRTUAL_TP2_HIT
}

func isWinningSignalOutcome(item SignalJournal, now time.Time) bool {
	switch item.Status {
	case TP2_HIT:
		return true
	case TP1_HIT:
		return isTP1FinalizedForJournal(item, now)
	default:
		return false
	}
}

// pnlFormulaMismatchThreshold is the maximum allowed deviation (in percentage points)
// between a stored PnL value and the value recalculated from price targets.
// Values beyond this threshold indicate the record was stored using a legacy formula.
const pnlFormulaMismatchThreshold = 0.1

func realizedEvaluationPnl(item SignalJournal, now time.Time) float64 {
	// In active monitoring state, return current floating Pnl
	if item.Status == MONITORING || item.Status == WATCH_MONITORING {
		return item.PnlPercentage
	}
	if item.Status == TP1_HIT || item.Status == VIRTUAL_TP1_HIT {
		if !isTP1FinalizedForJournal(item, now) {
			return item.PnlPercentage
		}
	}

	// For finalized legacy data where PnlPercentage is missing (0.0) but status is a win/loss:
	if item.PnlPercentage == 0.0 && item.Status != EXPIRED && item.Status != VIRTUAL_EXPIRED {
		// Fallback to recalculation to prevent data corruption or missing records from skewing report
		switch item.Status {
		case TP2_HIT, VIRTUAL_TP2_HIT:
			return realizedTP2Pnl(item.EntryPrice, item.TP1, item.TP2)
		case TP1_HIT, VIRTUAL_TP1_HIT:
			if item.TimeToSL != "" {
				return realizedStoppedAfterTP1Pnl(item.EntryPrice, item.TP1, item.StopLoss)
			}
			return realizedTP1PartialPnl(item.EntryPrice, item.TP1)
		case SL_HIT, VIRTUAL_SL_HIT:
			if item.TimeToTP1 != "" {
				return realizedStoppedAfterTP1Pnl(item.EntryPrice, item.TP1, item.StopLoss)
			}
			return realizedSLPnl(item.EntryPrice, item.StopLoss)
		}
	}

	// Formula mismatch detection: for deterministic terminal statuses (TP2_HIT, SL_HIT),
	// the expected PnL is fully determined by price targets. If the stored value deviates
	// beyond threshold, the record was stored using a legacy formula — recalculate.
	if item.EntryPrice > 0 {
		switch item.Status {
		case TP2_HIT, VIRTUAL_TP2_HIT:
			expected := realizedTP2Pnl(item.EntryPrice, item.TP1, item.TP2)
			if absFloat(item.PnlPercentage-expected) > pnlFormulaMismatchThreshold {
				return expected
			}
		case SL_HIT, VIRTUAL_SL_HIT:
			var expected float64
			if item.TimeToTP1 != "" {
				expected = realizedStoppedAfterTP1Pnl(item.EntryPrice, item.TP1, item.StopLoss)
			} else {
				expected = realizedSLPnl(item.EntryPrice, item.StopLoss)
			}
			if absFloat(item.PnlPercentage-expected) > pnlFormulaMismatchThreshold {
				return expected
			}
		}
	}

	// Default to returning the stored database/PocketBase PnlPercentage value
	return item.PnlPercentage
}

type journalSanityProfile struct {
	maxHold     time.Duration
	expiryGrace time.Duration
}

func getJournalSanityProfile() journalSanityProfile {
	return journalSanityProfile{
		maxHold:     getMonitoringMaxHoldDuration(),
		expiryGrace: 2 * time.Minute,
	}
}

func parseJournalDuration(raw string) (time.Duration, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return 0, false
	}
	if parsed < 0 {
		parsed = -parsed
	}
	return parsed, true
}

func isJournalTimingAnomalous(item SignalJournal, profile journalSanityProfile) bool {
	limit := profile.maxHold + profile.expiryGrace

	tp1Duration, hasTP1 := parseJournalDuration(item.TimeToTP1)
	tp2Duration, hasTP2 := parseJournalDuration(item.TimeToTP2)
	slDuration, hasSL := parseJournalDuration(item.TimeToSL)

	if hasTP1 && tp1Duration > limit {
		return true
	}
	if hasTP2 && tp2Duration > limit {
		return true
	}
	if hasSL && slDuration > limit {
		return true
	}
	if hasTP1 && hasTP2 && tp2Duration+time.Second < tp1Duration {
		return true
	}

	switch item.Status {
	case TP1_HIT, TP2_HIT, SL_HIT, VIRTUAL_TP1_HIT, VIRTUAL_TP2_HIT, VIRTUAL_SL_HIT, BREAKEVEN:
		if !item.ClosedAt.IsZero() && !item.ExpiresAt.IsZero() && item.ClosedAt.After(item.ExpiresAt.Add(profile.expiryGrace)) {
			return true
		}
	}

	return false
}

// getSampleGuard returns confidence, requiresMoreData, and severity based on sample size
func getSampleGuard(sampleSize int) (confidence string, requiresMoreData bool, severity string) {
	settings := getRuntimeSettings()
	minWarning := settings.EvaluationMinSampleWarning
	minMedium := settings.EvaluationMinSampleMedium
	minHigh := settings.EvaluationMinSampleHigh
	if minWarning <= 0 {
		minWarning = 10
	}
	if minMedium <= minWarning {
		minMedium = 20
	}
	if minHigh < minMedium {
		minHigh = 50
	}
	if sampleSize < minWarning {
		return "LOW", true, "INFO"
	} else if sampleSize < minMedium {
		return "LOW", false, "LOW"
	} else if sampleSize <= minHigh {
		return "MEDIUM", false, "WARNING"
	} else {
		return "HIGH", false, "CRITICAL"
	}
}

func freshnessMarker(source string, lastEventAt, now time.Time, freshThreshold, agingThreshold time.Duration) EvaluationFreshnessMarker {
	marker := EvaluationFreshnessMarker{
		Source:      source,
		LastEventAt: lastEventAt,
		AgeMinutes:  -1,
		Status:      "MISSING",
	}
	if lastEventAt.IsZero() {
		return marker
	}
	age := now.Sub(lastEventAt)
	marker.AgeMinutes = age.Minutes()
	switch {
	case age <= freshThreshold:
		marker.Status = "FRESH"
	case age <= agingThreshold:
		marker.Status = "AGING"
	default:
		marker.Status = "STALE"
	}
	return marker
}

// GenerateEvaluationReport compiles win rates, excursions, durations, and detailed threshold/policy recommendations.
func (uc *FeedbackUsecase) GenerateEvaluationReport() error {
	slog.Info("Starting Feedback Loop evaluation report generation...")
	var sourceFiles []string
	var completeness DataCompleteness
	sourceLabels := resolveFeedbackSourceFiles()

	// 1. Load data sources
	journal, err := uc.storageUsecase.LoadSignalJournal()
	if err != nil {
		return fmt.Errorf("failed to load signal journal: %w", err)
	}
	hasJournal := err == nil && len(journal) > 0
	if hasJournal {
		sourceFiles = append(sourceFiles, sourceLabels.signalJournal)
		completeness.HasSignalJournal = true
		completeness.CanEvaluateExecutedOutcome = true
	}

	watchJournal, err := uc.storageUsecase.LoadWatchJournal()
	if err != nil {
		return fmt.Errorf("failed to load watch journal: %w", err)
	}
	hasWatchJournal := err == nil && len(watchJournal) > 0
	if hasWatchJournal {
		sourceFiles = append(sourceFiles, sourceLabels.watchJournal)
		completeness.CanEvaluateWatchMissedOpportunity = true
	}

	latestRes, err := uc.storageUsecase.LoadLatestResult()
	if err != nil {
		return fmt.Errorf("failed to load latest result: %w", err)
	}
	hasLatest := err == nil && latestRes != nil && len(latestRes.Signals) > 0
	if hasLatest {
		sourceFiles = append(sourceFiles, sourceLabels.latestResult)
		completeness.HasLatestResult = true
		completeness.CanEvaluateWatchMissedOpportunity = true
	}

	audits, err := uc.storageUsecase.LoadDecisionAudits()
	if err != nil {
		return fmt.Errorf("failed to load decision audits: %w", err)
	}
	hasAudits := err == nil && len(audits) > 0
	if hasAudits {
		sourceFiles = append(sourceFiles, sourceLabels.decisionAudit)
		completeness.HasDecisionAudit = true
		completeness.CanEvaluateAIWait = true
		completeness.CanEvaluateConflictDowngrade = true
		completeness.CanEvaluateWatchMissedOpportunity = true
	}

	// Early check: if we have absolutely no data, still save a report indicating zero signals
	if len(journal) == 0 && len(audits) == 0 && len(watchJournal) == 0 {
		emptyReport := EvaluationReport{
			GeneratedAt:      time.Now(),
			ConfigVersion:    GetGlobalConfigRegistry().GetVersion(),
			SourceFilesUsed:  sourceFiles,
			DataCompleteness: completeness,
			FreshnessMarkers: map[string]EvaluationFreshnessMarker{},
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
					Reason:           fmt.Sprintf("No signals recorded in %s, %s, or %s.", sourceLabels.signalJournal, sourceLabels.watchJournal, sourceLabels.decisionAudit),
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
	now := time.Now()
	sanityProfile := getJournalSanityProfile()
	var finalized []SignalJournal
	excludedSignalAnomalies := 0
	for _, item := range journal {
		if isJournalTimingAnomalous(item, sanityProfile) {
			excludedSignalAnomalies++
			continue
		}
		if isFinalizedSignalJournalForEvaluation(item, now) {
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
		sumRR       float64
		sumPnl      float64
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

	type memorySliceKey struct {
		Symbol       string
		Direction    string
		MarketRegime string
		Playbook     string
	}

	type memorySliceAccum struct {
		stats             rawStats
		lastCreatedAt     time.Time
		lastStatus        string
		lastOutcomeReason string
		lastDecisionBrief string
	}

	pbRaw := make(map[string]*rawStats)
	regimeRaw := make(map[string]*rawStats)
	tierRaw := make(map[string]*rawStats)
	directionRaw := make(map[string]*rawStats)
	aiRaw := make(map[string]*rawStats)
	stalenessRaw := make(map[string]*rawStats)
	longSetupRaw := make(map[string]*rawStats)
	memorySliceRaw := make(map[memorySliceKey]*memorySliceAccum)

	type longSetupMeta struct {
		MarketRegime string
		Playbook     string
	}
	longSetupIndex := make(map[string]longSetupMeta)

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
		isWin := isWinningSignalOutcome(item, now)
		isLoss := item.Status == SL_HIT
		isExpired := item.Status == EXPIRED

		if isWin {
			wins++
		}
		if isLoss {
			losses++
		}

		hasHitTP1 := signalHasTP1Achievement(item)
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

		totalPnl += realizedEvaluationPnl(item, now)
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
		regimeKey := canonicalRegimeLabel(item.MarketRegime)
		if regimeKey == "" {
			regimeKey = string(UNKNOWN)
		}
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
			rs.sumRR += item.RR
			rs.sumPnl += realizedEvaluationPnl(item, now)
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
		sliceKey := memorySliceKey{
			Symbol:       strings.TrimSpace(item.Symbol),
			Direction:    dirKey,
			MarketRegime: regimeKey,
			Playbook:     pbKey,
		}
		if sliceKey.Symbol == "" {
			sliceKey.Symbol = "UNKNOWN"
		}
		if _, ok := memorySliceRaw[sliceKey]; !ok {
			memorySliceRaw[sliceKey] = &memorySliceAccum{}
		}
		mem := memorySliceRaw[sliceKey]
		updateRaw(&mem.stats)
		if item.CreatedAt.After(mem.lastCreatedAt) {
			mem.lastCreatedAt = item.CreatedAt
			mem.lastStatus = string(item.Status)
			mem.lastOutcomeReason = item.OutcomeReason
		}
		if item.Direction == LONG {
			setupKey := regimeKey + "|" + pbKey
			longSetupIndex[setupKey] = longSetupMeta{
				MarketRegime: regimeKey,
				Playbook:     pbKey,
			}
			updateRaw(getOrInitRaw(longSetupRaw, setupKey))
		}
	}
	for _, audit := range audits {
		sliceKey := memorySliceKey{
			Symbol:       strings.TrimSpace(audit.Symbol),
			Direction:    strings.TrimSpace(string(audit.Direction)),
			MarketRegime: canonicalRegimeLabel(audit.MarketRegime),
			Playbook:     strings.TrimSpace(string(audit.Playbook)),
		}
		if sliceKey.Symbol == "" || sliceKey.Direction == "" || sliceKey.Playbook == "" {
			continue
		}
		if sliceKey.MarketRegime == "" {
			sliceKey.MarketRegime = string(UNKNOWN)
		}
		mem, ok := memorySliceRaw[sliceKey]
		if !ok {
			continue
		}
		if audit.GeneratedAt.After(mem.lastCreatedAt) && strings.TrimSpace(audit.DecisionBrief) != "" {
			mem.lastCreatedAt = audit.GeneratedAt
			mem.lastDecisionBrief = audit.DecisionBrief
			if mem.lastStatus == "" {
				mem.lastStatus = string(audit.FinalStatus)
			}
			if mem.lastOutcomeReason == "" {
				mem.lastOutcomeReason = audit.FinalReason
			}
		}
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

	// 2b. Evaluate virtual FINAL_WATCH outcomes separately from executable signals.
	var watchFinalized []WatchJournal
	var watchTP1Hits, watchTP2Hits, watchSLHits, watchExpiredHits int
	var watchPromotedCount, watchRecheckExpiredCount, watchRecheckInvalidatedCount int
	var watchSumMFE, watchSumMAE, watchTotalPnl float64
	excludedWatchAnomalies := 0
	var latestWatchEventAt time.Time

	for _, item := range watchJournal {
		if item.UpdatedAt.After(latestWatchEventAt) {
			latestWatchEventAt = item.UpdatedAt
		}
		if item.ClosedAt.After(latestWatchEventAt) {
			latestWatchEventAt = item.ClosedAt
		}
		if item.CreatedAt.After(latestWatchEventAt) {
			latestWatchEventAt = item.CreatedAt
		}
		switch item.Status {
		case WATCH_PROMOTED:
			watchPromotedCount++
		case WATCH_RECHECK_EXPIRED:
			watchRecheckExpiredCount++
		case WATCH_RECHECK_INVALIDATED:
			watchRecheckInvalidatedCount++
		}
		if isJournalTimingAnomalous(SignalJournal(item), sanityProfile) {
			excludedWatchAnomalies++
			continue
		}
		if !isFinalizedWatchJournalForEvaluation(item, now) {
			continue
		}
		watchFinalized = append(watchFinalized, item)

		if watchHasTP1Achievement(item) {
			watchTP1Hits++
		}
		if item.Status == VIRTUAL_TP2_HIT {
			watchTP2Hits++
		}
		if item.Status == VIRTUAL_SL_HIT {
			watchSLHits++
		}
		if item.Status == VIRTUAL_EXPIRED {
			watchExpiredHits++
		}
		watchSumMFE += item.MFE
		watchSumMAE += item.MAE
		watchTotalPnl += realizedEvaluationPnl(SignalJournal(item), now)
	}

	watchFinalizedCount := len(watchFinalized)
	watchVirtualWinRate := safeRate(watchTP1Hits, watchFinalizedCount)
	watchVirtualTP1Rate := safeRate(watchTP1Hits, watchFinalizedCount)
	watchVirtualTP2Rate := safeRate(watchTP2Hits, watchFinalizedCount)
	watchVirtualSLRate := safeRate(watchSLHits, watchFinalizedCount)
	watchVirtualExpiredRate := safeRate(watchExpiredHits, watchFinalizedCount)
	watchAverageMFE := safeDiv(watchSumMFE, float64(watchFinalizedCount))
	watchAverageMAE := safeDiv(watchSumMAE, float64(watchFinalizedCount))

	var promotedFinalizedCount, promotedWins, promotedTP2Hits, promotedSLHits, promotedExpiredHits int
	var promotedTotalPnl float64
	var latestSignalEventAt time.Time
	for _, item := range journal {
		if item.UpdatedAt.After(latestSignalEventAt) {
			latestSignalEventAt = item.UpdatedAt
		}
		if item.ClosedAt.After(latestSignalEventAt) {
			latestSignalEventAt = item.ClosedAt
		}
		if item.CreatedAt.After(latestSignalEventAt) {
			latestSignalEventAt = item.CreatedAt
		}
		if !isWatchRecheckPromotionReason(item.Reason) {
			continue
		}
		if !isFinalizedSignalJournalForEvaluation(item, now) {
			continue
		}
		promotedFinalizedCount++
		if isWinningSignalOutcome(item, now) {
			promotedWins++
		}
		if item.Status == TP2_HIT {
			promotedTP2Hits++
		}
		if item.Status == SL_HIT {
			promotedSLHits++
		}
		if item.Status == EXPIRED {
			promotedExpiredHits++
		}
		promotedTotalPnl += realizedEvaluationPnl(item, now)
	}

	var latestAuditEventAt time.Time
	for _, audit := range audits {
		if audit.GeneratedAt.After(latestAuditEventAt) {
			latestAuditEventAt = audit.GeneratedAt
		}
		if audit.CreatedAt.After(latestAuditEventAt) {
			latestAuditEventAt = audit.CreatedAt
		}
	}
	promotedConversionRate := safeRate(watchPromotedCount, len(watchJournal))
	promotedWinRate := safeRate(promotedWins, promotedFinalizedCount)
	promotedTP2Rate := safeRate(promotedTP2Hits, promotedFinalizedCount)
	promotedSLRate := safeRate(promotedSLHits, promotedFinalizedCount)
	promotedExpiredRate := safeRate(promotedExpiredHits, promotedFinalizedCount)
	freshThreshold := time.Duration(maxEvalInt(getRuntimeSettings().WatchRecheckBoundaryMinutes, 1)) * time.Minute
	agingThreshold := time.Duration(maxEvalInt(getRuntimeSettings().MonitoringMaxHoldMinutes, maxEvalInt(getRuntimeSettings().WatchRecheckMaxAgeMinutes, 1))) * time.Minute
	if agingThreshold < freshThreshold {
		agingThreshold = freshThreshold
	}
	freshnessMarkers := map[string]EvaluationFreshnessMarker{
		"signal_journal": freshnessMarker("signal_journal", latestSignalEventAt, now, freshThreshold, agingThreshold),
		"watch_journal":  freshnessMarker("watch_journal", latestWatchEventAt, now, freshThreshold, agingThreshold),
		"decision_audit": freshnessMarker("decision_audit", latestAuditEventAt, now, freshThreshold, agingThreshold),
	}

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

	buildSetupDiagnostic := func(direction, regime, playbook string, stats *rawStats) SetupDiagnosticStats {
		if stats == nil {
			return SetupDiagnosticStats{
				Direction:    direction,
				MarketRegime: regime,
				Playbook:     playbook,
			}
		}
		return SetupDiagnosticStats{
			Direction:          direction,
			MarketRegime:       regime,
			Playbook:           playbook,
			TotalSignals:       stats.total,
			WinRate:            safeRate(stats.wins, stats.total),
			TP1Rate:            safeRate(stats.tp1Count, stats.total),
			TP2Rate:            safeRate(stats.tp2Count, stats.total),
			SLRate:             safeRate(stats.slCount, stats.total),
			ExpiredRate:        safeRate(stats.expCount, stats.total),
			AverageMAE:         safeDiv(stats.sumMAE, float64(stats.total)),
			AverageMFE:         safeDiv(stats.sumMFE, float64(stats.total)),
			AverageRR:          safeDiv(stats.sumRR, float64(stats.total)),
			TotalPnlPercentage: stats.sumPnl,
		}
	}

	longRegimePlaybookStats := make([]SetupDiagnosticStats, 0, len(longSetupRaw))
	for key, stats := range longSetupRaw {
		meta, ok := longSetupIndex[key]
		if !ok {
			continue
		}
		longRegimePlaybookStats = append(longRegimePlaybookStats, buildSetupDiagnostic(string(LONG), meta.MarketRegime, meta.Playbook, stats))
	}
	sort.Slice(longRegimePlaybookStats, func(i, j int) bool {
		if longRegimePlaybookStats[i].MarketRegime != longRegimePlaybookStats[j].MarketRegime {
			return longRegimePlaybookStats[i].MarketRegime < longRegimePlaybookStats[j].MarketRegime
		}
		if longRegimePlaybookStats[i].Playbook != longRegimePlaybookStats[j].Playbook {
			return longRegimePlaybookStats[i].Playbook < longRegimePlaybookStats[j].Playbook
		}
		return longRegimePlaybookStats[i].TotalSignals > longRegimePlaybookStats[j].TotalSignals
	})

	weakLongSetups := make([]SetupDiagnosticStats, 0, len(longRegimePlaybookStats))
	for _, stat := range longRegimePlaybookStats {
		if stat.TotalSignals >= 3 {
			weakLongSetups = append(weakLongSetups, stat)
		}
	}
	sort.Slice(weakLongSetups, func(i, j int) bool {
		if weakLongSetups[i].WinRate != weakLongSetups[j].WinRate {
			return weakLongSetups[i].WinRate < weakLongSetups[j].WinRate
		}
		if weakLongSetups[i].SLRate != weakLongSetups[j].SLRate {
			return weakLongSetups[i].SLRate > weakLongSetups[j].SLRate
		}
		if weakLongSetups[i].ExpiredRate != weakLongSetups[j].ExpiredRate {
			return weakLongSetups[i].ExpiredRate > weakLongSetups[j].ExpiredRate
		}
		if weakLongSetups[i].TotalSignals != weakLongSetups[j].TotalSignals {
			return weakLongSetups[i].TotalSignals > weakLongSetups[j].TotalSignals
		}
		if weakLongSetups[i].MarketRegime != weakLongSetups[j].MarketRegime {
			return weakLongSetups[i].MarketRegime < weakLongSetups[j].MarketRegime
		}
		return weakLongSetups[i].Playbook < weakLongSetups[j].Playbook
	})
	if len(weakLongSetups) > 5 {
		weakLongSetups = weakLongSetups[:5]
	}

	setupMemorySlices := make([]SetupMemorySlice, 0, len(memorySliceRaw))
	for key, acc := range memorySliceRaw {
		stats := acc.stats
		setupMemorySlices = append(setupMemorySlices, SetupMemorySlice{
			Symbol:             key.Symbol,
			Direction:          key.Direction,
			MarketRegime:       key.MarketRegime,
			Playbook:           key.Playbook,
			TotalSignals:       stats.total,
			WinRate:            safeRate(stats.wins, stats.total),
			TP2Rate:            safeRate(stats.tp2Count, stats.total),
			SLRate:             safeRate(stats.slCount, stats.total),
			ExpiredRate:        safeRate(stats.expCount, stats.total),
			AverageRR:          safeDiv(stats.sumRR, float64(stats.total)),
			TotalPnlPercentage: stats.sumPnl,
			LastStatus:         acc.lastStatus,
			LastOutcomeReason:  acc.lastOutcomeReason,
			LastDecisionBrief:  acc.lastDecisionBrief,
		})
	}
	sort.Slice(setupMemorySlices, func(i, j int) bool {
		if setupMemorySlices[i].Symbol != setupMemorySlices[j].Symbol {
			return setupMemorySlices[i].Symbol < setupMemorySlices[j].Symbol
		}
		if setupMemorySlices[i].MarketRegime != setupMemorySlices[j].MarketRegime {
			return setupMemorySlices[i].MarketRegime < setupMemorySlices[j].MarketRegime
		}
		if setupMemorySlices[i].Playbook != setupMemorySlices[j].Playbook {
			return setupMemorySlices[i].Playbook < setupMemorySlices[j].Playbook
		}
		return setupMemorySlices[i].Direction < setupMemorySlices[j].Direction
	})

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
	m5UsedAuditCount := 0
	m5ConfirmedAuditCount := 0
	m5FailedAuditCount := 0
	m5UnavailableAuditCount := 0
	m5InvalidatedAuditCount := 0
	m5SoftHardAuditCount := 0
	m5SoftHardUnavailableCount := 0
	m5SoftHardFailedCount := 0
	m5ExecutionViolationCount := 0
	if hasAudits {
		for _, a := range audits {
			if a.ConflictReason != "" {
				conflictStats[a.ConflictReason]++
			}
			if a.CooldownReason != "" {
				cooldownStats[a.CooldownReason]++
			}
			if !a.M5ConfirmationUsed {
				continue
			}

			mode := normalizeM5ConfirmationModeLabel(a.M5ConfirmationMode)
			status := normalizeM5ConfirmationStatusLabel(a.M5ConfirmationStatus)
			m5UsedAuditCount++

			switch status {
			case M5ConfirmationConfirmed:
				m5ConfirmedAuditCount++
			case M5ConfirmationFailed:
				m5FailedAuditCount++
			case M5ConfirmationUnavailable:
				m5UnavailableAuditCount++
			case M5ConfirmationInvalidated:
				m5InvalidatedAuditCount++
			}

			if mode == M5ConfirmationSoftConfirm || mode == M5ConfirmationHardConfirm {
				m5SoftHardAuditCount++
				if status == M5ConfirmationUnavailable {
					m5SoftHardUnavailableCount++
				}
				if status == M5ConfirmationFailed {
					m5SoftHardFailedCount++
				}
			}

			if a.FinalStatus == FINAL_EXECUTE && m5ExecutionViolatesMode(mode, status) {
				m5ExecutionViolationCount++
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
			policy, profile := resolveFeedbackExecutionConfig(item.Playbook, item.MarketRegime, item.Tier)

			// 1. AI confidence below required policy/profile confidence.
			requiredAIConfidence := effectiveRequiredAIConfidence(policy, profile)
			if item.AIConfidence != "" && !meetsRequiredAIConfidence(item.AIConfidence, requiredAIConfidence) {
				addGateBug(pb, "GATE_BUG: AI confidence was "+item.AIConfidence+" but execution required "+string(requiredAIConfidence)+" on "+item.Symbol)
			}

			// 2. Staleness not FRESH only when fresh entry is actually required.
			if effectiveRequireFreshEntry(policy) && (item.EntryTiming == "LATE" || strings.Contains(strings.ToLower(item.ThresholdProfileSummary), "staleness: late")) {
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
				policy, profile := resolveFeedbackExecutionConfig(a.Playbook, a.MarketRegime, a.Tier)
				requiredAIConfidence := effectiveRequiredAIConfidence(policy, profile)

				if a.AIConfidence != "" && !meetsRequiredAIConfidence(a.AIConfidence, requiredAIConfidence) {
					addGateBug(pb, "GATE_BUG: AI confidence was "+a.AIConfidence+" but final status is FINAL_EXECUTE while required confidence is "+string(requiredAIConfidence)+" on "+a.Symbol)
				}
				if effectiveRequireFreshEntry(policy) && a.StalenessStatus == "LATE" {
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

				mode := normalizeM5ConfirmationModeLabel(a.M5ConfirmationMode)
				status := normalizeM5ConfirmationStatusLabel(a.M5ConfirmationStatus)
				if a.M5ConfirmationUsed && m5ExecutionViolatesMode(mode, status) {
					addGateBug(pb, "GATE_BUG: "+string(a.Playbook)+" reached FINAL_EXECUTE with M5 mode "+string(mode)+" and status "+string(status)+" on "+a.Symbol)
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

		if sampleSize < getRuntimeSettings().EvaluationMinSampleWarning {
			minSample := getRuntimeSettings().EvaluationMinSampleWarning
			if minSample <= 0 {
				minSample = 10
			}
			rec.IssueType = "INSUFFICIENT_SAMPLE"
			rec.SuggestedAction = fmt.Sprintf("HOLD TUNING: Insufficient sample size (< %d) to make recommendations.", minSample)
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
		if item.Direction == SHORT && regimeIsAny(item.MarketRegime, ALT_SUPPORTIVE, BTC_DOMINANCE) {
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
			MarketRegime:       string(ALT_SUPPORTIVE),
			Direction:          "SHORT",
			MetricName:         "SHORT_SUPPORTIVE_SL_RATE",
			MetricValue:        safeRate(shortBullishSLCount, shortBullishCount),
			CurrentThreshold:   "LongMode/ShortMode active",
			SuggestedThreshold: "ShortMode: SWEEP_ONLY",
			EvidenceSummary:    "Short signals during supportive or BTC-dominance regimes suffer high stop-out rates.",
			Reason:             "Counter-trend shorting during ALT_SUPPORTIVE or BTC_DOMINANCE conditions leads to stop-outs.",
			SuggestedAction:    "Restrict ShortMode in MarketPolicy to SWEEP_ONLY during ALT_SUPPORTIVE or BTC_DOMINANCE regimes, and block trend continuation shorts.",
		}, shortBullishCount)
	}

	// Recommendation 2: LONG saat RISK_OFF sering SL
	longRiskOffCount := 0
	longRiskOffSLCount := 0
	for _, item := range finalized {
		if item.Direction == LONG && regimeIsAny(item.MarketRegime, RISK_OFF) {
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
			SuggestedThreshold: "LongMode: SWEEP_ONLY",
			EvidenceSummary:    "Long signals during RISK_OFF regimes suffer high stop-out rates.",
			Reason:             "Trend continuation longs fail when overall market is in RISK_OFF or BEARISH mode.",
			SuggestedAction:    "Restrict LongMode to SWEEP_ONLY during RISK_OFF regimes so only lower-sweep reversal longs remain eligible.",
		}, longRiskOffCount)
	}

	longBaselineWinRate := 0.0
	if stats, ok := directionStats[string(LONG)]; ok {
		longBaselineWinRate = stats.WinRate
	}
	for i, stat := range weakLongSetups {
		if i >= 3 {
			break
		}
		if stat.TotalSignals < 10 {
			continue
		}
		if stat.WinRate >= 40.0 && stat.SLRate <= 55.0 && stat.ExpiredRate <= 50.0 && stat.WinRate+10.0 >= longBaselineWinRate {
			continue
		}
		sliceBlocked, policyMode, blockReason := resolveFeedbackSliceBlockReason(Playbook(stat.Playbook), stat.MarketRegime, LONG)
		if sliceBlocked {
			addRec(ThresholdRecommendation{
				IssueType:          "DIRECTIONAL_DIAGNOSTIC",
				Playbook:           stat.Playbook,
				MarketRegime:       stat.MarketRegime,
				PolicyMode:         policyMode,
				Direction:          string(LONG),
				MetricName:         "LONG_REGIME_PLAYBOOK_WIN_RATE",
				MetricValue:        stat.WinRate,
				CurrentThreshold:   "Current policy/selector already blocks this slice",
				SuggestedThreshold: "KEEP_DISABLED",
				EvidenceSummary:    fmt.Sprintf("Historic LONG %s in %s posted %.2f%% win rate with %.2f%% SL rate across %d finalized signals, but the current engine no longer admits this slice.", stat.Playbook, stat.MarketRegime, stat.WinRate, stat.SLRate, stat.TotalSignals),
				Reason:             fmt.Sprintf("The underperformance is real historically, but further tuning would be stale because the live policy path already blocks the slice: %s.", blockReason),
				SuggestedAction:    "Do not retune this slice now; keep the current block in place and wait for fresh post-change data before revisiting.",
			}, stat.TotalSignals)
			continue
		}
		addRec(ThresholdRecommendation{
			IssueType:          "DIRECTIONAL_DIAGNOSTIC",
			Playbook:           stat.Playbook,
			MarketRegime:       stat.MarketRegime,
			PolicyMode:         policyMode,
			Direction:          string(LONG),
			MetricName:         "LONG_REGIME_PLAYBOOK_WIN_RATE",
			MetricValue:        stat.WinRate,
			CurrentThreshold:   "Selector + policy active",
			SuggestedThreshold: "Review LONG slice before global tuning",
			EvidenceSummary:    fmt.Sprintf("LONG %s in %s posts %.2f%% win rate with %.2f%% SL rate across %d finalized signals.", stat.Playbook, stat.MarketRegime, stat.WinRate, stat.SLRate, stat.TotalSignals),
			Reason:             "This specific long slice underperforms on realized outcomes, so the next action should be targeted diagnosis rather than broad tightening.",
			SuggestedAction:    "Review selector eligibility, local/final gate, and target profile for this exact LONG regime/playbook slice; if unchanged, downgrade it to WATCH_ONLY until the slice recovers.",
		}, stat.TotalSignals)
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
	for _, item := range finalized {
		if item.Playbook == "TREND_PULLBACK" {
			tpCount++
			if item.Status == SL_HIT {
				tpSLCount++
			}
		}
	}
	trendProfile := GetPlaybookThresholdProfile(TREND_PULLBACK, MarketPolicy{}, "")
	if tpCount > 0 && safeRate(tpSLCount, tpCount) > 40 {
		addRec(ThresholdRecommendation{
			IssueType:          "THRESHOLD_TUNING",
			Playbook:           "TREND_PULLBACK",
			MetricName:         "TREND_PULLBACK_SL_RATE",
			MetricValue:        safeRate(tpSLCount, tpCount),
			CurrentThreshold:   formatFloatThreshold("MinADX", trendProfile.MinADX),
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
	sweepProfile := GetPlaybookThresholdProfile(LIQUIDITY_SWEEP_REVERSAL, MarketPolicy{}, "")
	if lsCount > 0 && safeRate(lsSLCount, lsCount) > 40 {
		addRec(ThresholdRecommendation{
			IssueType:          "THRESHOLD_TUNING",
			Playbook:           "LIQUIDITY_SWEEP_REVERSAL",
			MetricName:         "LIQUIDITY_SWEEP_REVERSAL_SL_RATE",
			MetricValue:        safeRate(lsSLCount, lsCount),
			CurrentThreshold:   formatFloatThreshold("MinVolumeRatio", sweepProfile.MinVolumeRatio),
			SuggestedThreshold: formatFloatThreshold("MinVolumeRatio", RoundToDecimalPlaces(maxFloat(sweepProfile.MinVolumeRatio+0.2, 1.5), 2)),
			EvidenceSummary:    "Sweep reversals suffer stop-outs on weak volume confirmation.",
			Reason:             "Liquidity sweep reversals require high volume validation to confirm exhaustion of counterparty orders.",
			SuggestedAction:    "Increase MinVolumeRatio and require wick rejection confirmation.",
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
	compressionProfile := GetPlaybookThresholdProfile(COMPRESSION_BREAKOUT_RETEST, MarketPolicy{}, "")
	if cbCount > 0 && (safeRate(cbSLCount, cbCount) > 40 || safeRate(cbStaleCount, cbCount) > 40) {
		currentThreshold := "RequireRetest: true"
		suggestedThreshold := "MinRetestQuality: 0.65 | StalenessATR: 0.25"
		suggestedAction := "Keep breakout-candle entry disabled, require stronger retest hold, and tighten StalenessATR."
		reason := "Even with retest-only execution, weak holds and stale entries still degrade breakout quality."
		if compressionProfile.AllowBreakoutCandleEntry {
			currentThreshold = "AllowBreakoutCandleEntry: true"
			suggestedThreshold = "AllowBreakoutCandleEntry: false"
			suggestedAction = "Set AllowBreakoutCandleEntry to false, require retest hold, and tighten StalenessATR."
			reason = "Entering directly on breakout candle increases stop-out rate; waiting for retest confirmation is safer."
		}
		addRec(ThresholdRecommendation{
			IssueType:          "THRESHOLD_TUNING",
			Playbook:           "COMPRESSION_BREAKOUT_RETEST",
			MetricName:         "COMPRESSION_BREAKOUT_RETEST_FAILURE_RATE",
			MetricValue:        safeRate(cbSLCount+cbStaleCount, cbCount),
			CurrentThreshold:   currentThreshold,
			SuggestedThreshold: suggestedThreshold,
			EvidenceSummary:    "Compression breakouts are hit by fake breakouts and expiration.",
			Reason:             reason,
			SuggestedAction:    suggestedAction,
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
	rangeProfile := GetPlaybookThresholdProfile(RANGE_EDGE_REVERSAL, MarketPolicy{}, "")
	if reCount > 0 && safeRate(reSLCount, reCount) > 40 {
		addRec(ThresholdRecommendation{
			IssueType:          "THRESHOLD_TUNING",
			Playbook:           "RANGE_EDGE_REVERSAL",
			MetricName:         "RANGE_EDGE_REVERSAL_SL_RATE",
			MetricValue:        safeRate(reSLCount, reCount),
			CurrentThreshold:   formatFloatThreshold("MaxADX", rangeProfile.MaxADX),
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
	squeezeProfile := GetPlaybookThresholdProfile(CROWDED_POSITIONING_SQUEEZE, MarketPolicy{}, "")
	if csCount > 0 && safeRate(csSLCount, csCount) > 40 {
		addRec(ThresholdRecommendation{
			IssueType:          "THRESHOLD_TUNING",
			Playbook:           "CROWDED_POSITIONING_SQUEEZE",
			MetricName:         "CROWDED_POSITIONING_SQUEEZE_SL_RATE",
			MetricValue:        safeRate(csSLCount, csCount),
			CurrentThreshold:   formatFloatThreshold("MinCrowdingScore", squeezeProfile.MinCrowdingScore),
			SuggestedThreshold: formatFloatThreshold("MinCrowdingScore", RoundToDecimalPlaces(maxFloat(squeezeProfile.MinCrowdingScore+0.2, 0.7), 2)),
			EvidenceSummary:    "Crowded squeeze signals fail due to lack of price action confirmation.",
			Reason:             "Executing squeezes based solely on extreme funding without reclaim/rejection leads to stop-outs.",
			SuggestedAction:    "Increase MinCrowdingScore and require reclaim/rejection confirmation.",
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

	// Recommendation 10: watch/AI WAIT semantics need observability before prompt tuning.
	if !hasAudits {
		addRec(ThresholdRecommendation{
			IssueType:        "INSUFFICIENT_SAMPLE",
			Playbook:         "ALL",
			MetricName:       "MISSED_OPPORTUNITY_EVALUATION",
			MetricValue:      0.0,
			EvidenceSummary:  fmt.Sprintf("%s is not available to track WATCH or AI_MEDIUM signals.", sourceLabels.decisionAudit),
			ConfidenceLevel:  "LOW",
			Reason:           "Need decision_audit/watchlist monitoring to evaluate AI_WAIT or AI_MEDIUM.",
			SuggestedAction:  fmt.Sprintf("Enable and populate %s.", sourceLabels.decisionAudit),
			DoNotAutoApply:   true,
			RequiresMoreData: true,
			Severity:         "INFO",
		}, 0)
	} else {
		aiWatchCount := 0
		for _, a := range audits {
			if strings.EqualFold(a.AIDecision, "WAIT") || strings.EqualFold(a.AIConfidence, string(AIConfidenceMedium)) || a.FinalStatus == FINAL_WATCH {
				aiWatchCount++
			}
		}
		if aiWatchCount > 0 {
			evidenceSummary := fmt.Sprintf("%d AI WAIT / medium-confidence / FINAL_WATCH decisions recorded.", aiWatchCount)
			if watchFinalizedCount > 0 {
				evidenceSummary = fmt.Sprintf("%d AI WAIT / medium-confidence / FINAL_WATCH decisions and %d finalized virtual watch outcomes recorded.", aiWatchCount, watchFinalizedCount)
			}
			addRec(ThresholdRecommendation{
				IssueType:          "OBSERVABILITY_TUNING",
				Playbook:           "ALL",
				MetricName:         "AI_WATCH_DECISION_COUNT",
				MetricValue:        float64(aiWatchCount),
				CurrentThreshold:   "AI MEDIUM / WAIT remains non-executable",
				SuggestedThreshold: "Correlate watch outcomes before any prompt tightening",
				EvidenceSummary:    evidenceSummary,
				Reason:             "Current evaluation can count watch decisions, but it does not yet attribute each virtual TP/SL outcome back to the precise AI WAIT rationale with certainty.",
				SuggestedAction:    "Review watch_journal outcomes before changing prompt, selector, or promoting AI MEDIUM setups to execute; keep AI MEDIUM non-executable until per-decision attribution is proven.",
			}, maxInt(aiWatchCount, watchFinalizedCount))
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
			CurrentThreshold:   "MaxHold: " + getMonitoringMaxHoldLabel(),
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
		if regimeIsAny(item.MarketRegime, HIGH_VOL, BTC_CHAOS) {
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
			MarketRegime:       string(HIGH_VOL),
			MetricName:         "HIGH_VOL_SL_RATE",
			MetricValue:        safeRate(highVolSLCount, highVolCount),
			CurrentThreshold:   "MaxSymbols: 5",
			SuggestedThreshold: "Reduce MaxSymbols / Increase MinScore",
			EvidenceSummary:    "Signals during high volatility or chaos regimes suffer high stop-outs.",
			Reason:             "Volatility expansions generate false breakouts and spike liquidation wicks.",
			SuggestedAction:    "Reduce MaxSymbols limits, block Tier C assets, and raise MinScoreExecute during chaos.",
		}, highVolCount)
	}

	if m5SoftHardAuditCount > 0 && safeRate(m5SoftHardUnavailableCount, m5SoftHardAuditCount) > 25 {
		addRec(ThresholdRecommendation{
			IssueType:          "OBSERVABILITY_TUNING",
			Playbook:           "ALL",
			MetricName:         "M5_SOFT_HARD_UNAVAILABLE_RATE",
			MetricValue:        safeRate(m5SoftHardUnavailableCount, m5SoftHardAuditCount),
			CurrentThreshold:   "Soft/Hard M5 confirmation active",
			SuggestedThreshold: "Stabilize M5 data path before tightening thresholds",
			EvidenceSummary:    fmt.Sprintf("%d of %d soft/hard M5 confirmations were unavailable in decision audits.", m5SoftHardUnavailableCount, m5SoftHardAuditCount),
			Reason:             "When soft/hard M5 gating is blocked by missing M5 data, evaluation and tuning become noisy because misses are infra-driven rather than market-driven.",
			SuggestedAction:    "Audit M5 fetch/cache/retry reliability first and keep WATCH_ONLY_HINT rollout until unavailable rate normalizes.",
		}, m5SoftHardAuditCount)
	}

	// Recommendation 20: Low volatility banyak expired
	lowVolCount := 0
	lowVolExpCount := 0
	for _, item := range finalized {
		if regimeIsAny(item.MarketRegime, LOW_VOL, CHOP_RANGE) {
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
			MarketRegime:       string(LOW_VOL),
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

	learningReviews := make([]LearningReview, 0, len(recommendations))
	for _, rec := range recommendations {
		summary := strings.TrimSpace(rec.EvidenceSummary)
		if summary == "" {
			summary = strings.TrimSpace(rec.Reason)
		}
		learningReviews = append(learningReviews, LearningReview{
			IssueType:        rec.IssueType,
			Playbook:         rec.Playbook,
			MarketRegime:     rec.MarketRegime,
			Direction:        rec.Direction,
			PolicyMode:       rec.PolicyMode,
			Summary:          summary,
			SuggestedAction:  rec.SuggestedAction,
			ConfidenceLevel:  rec.ConfidenceLevel,
			Severity:         rec.Severity,
			SampleSize:       rec.SampleSize,
			ReviewOnly:       true,
			DoNotAutoApply:   true,
			RequiresMoreData: rec.RequiresMoreData,
		})
	}

	// 6. Build Final Report Object
	report := EvaluationReport{
		GeneratedAt:      time.Now(),
		ConfigVersion:    GetGlobalConfigRegistry().GetVersion(),
		SourceFilesUsed:  sourceFiles,
		DataCompleteness: completeness,
		FreshnessMarkers: freshnessMarkers,
		TotalSignals:     totalCount,
		Metrics: map[string]float64{
			"win_rate":                             winRate,
			"tp1_rate":                             tp1Rate,
			"tp2_rate":                             tp2Rate,
			"sl_rate":                              slRate,
			"expired_rate":                         expiredRate,
			"average_mfe":                          avgMFE,
			"average_mae":                          avgMAE,
			"average_rr":                           avgRR,
			"average_time_to_tp1":                  avgTimeToTP1,
			"average_time_to_tp2":                  avgTimeToTP2,
			"average_time_to_sl":                   avgTimeToSL,
			"average_holding_time":                 avgHoldingTime,
			"total_pnl_percentage":                 totalPnl,
			"raw_signal_journal_count":             float64(len(journal)),
			"excluded_signal_anomaly_count":        float64(excludedSignalAnomalies),
			"watch_total":                          float64(len(watchJournal)),
			"watch_finalized":                      float64(watchFinalizedCount),
			"watch_virtual_win_rate":               watchVirtualWinRate,
			"watch_virtual_tp1_rate":               watchVirtualTP1Rate,
			"watch_virtual_tp2_rate":               watchVirtualTP2Rate,
			"watch_virtual_sl_rate":                watchVirtualSLRate,
			"watch_virtual_expired_rate":           watchVirtualExpiredRate,
			"watch_average_mfe":                    watchAverageMFE,
			"watch_average_mae":                    watchAverageMAE,
			"watch_total_pnl_percentage":           watchTotalPnl,
			"watch_promoted_count":                 float64(watchPromotedCount),
			"watch_recheck_expired_count":          float64(watchRecheckExpiredCount),
			"watch_recheck_invalidated_count":      float64(watchRecheckInvalidatedCount),
			"watch_to_promote_conversion_rate":     promotedConversionRate,
			"promoted_finalized_count":             float64(promotedFinalizedCount),
			"promoted_win_rate":                    promotedWinRate,
			"promoted_tp2_rate":                    promotedTP2Rate,
			"promoted_sl_rate":                     promotedSLRate,
			"promoted_expired_rate":                promotedExpiredRate,
			"promoted_total_pnl_percentage":        promotedTotalPnl,
			"raw_watch_journal_count":              float64(len(watchJournal)),
			"excluded_watch_anomaly_count":         float64(excludedWatchAnomalies),
			"signal_journal_freshness_age_minutes": freshnessMarkers["signal_journal"].AgeMinutes,
			"watch_journal_freshness_age_minutes":  freshnessMarkers["watch_journal"].AgeMinutes,
			"decision_audit_freshness_age_minutes": freshnessMarkers["decision_audit"].AgeMinutes,
			"long_regime_playbook_slice_count":     float64(len(longRegimePlaybookStats)),
			"weak_long_setup_count":                float64(len(weakLongSetups)),
			"m5_used_audit_count":                  float64(m5UsedAuditCount),
			"m5_confirmed_audit_count":             float64(m5ConfirmedAuditCount),
			"m5_failed_audit_count":                float64(m5FailedAuditCount),
			"m5_unavailable_audit_count":           float64(m5UnavailableAuditCount),
			"m5_invalidated_audit_count":           float64(m5InvalidatedAuditCount),
			"m5_soft_hard_audit_count":             float64(m5SoftHardAuditCount),
			"m5_soft_hard_unavailable_count":       float64(m5SoftHardUnavailableCount),
			"m5_soft_hard_failed_count":            float64(m5SoftHardFailedCount),
			"m5_execution_violation_count":         float64(m5ExecutionViolationCount),
		},
		PlaybookStats:             playbookStats,
		RegimeStats:               regimeStats,
		TierStats:                 tierStats,
		DirectionStats:            directionStats,
		AIStats:                   aiStats,
		StalenessStats:            stalenessStats,
		LongRegimePlaybookStats:   longRegimePlaybookStats,
		WeakLongSetups:            weakLongSetups,
		SetupMemorySlices:         setupMemorySlices,
		LearningReviews:           learningReviews,
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
	if excludedSignalAnomalies > 0 || excludedWatchAnomalies > 0 {
		report.Notes = fmt.Sprintf("%s Journal sanity quarantine excluded %d signal rows and %d watch rows from evaluation metrics.", report.Notes, excludedSignalAnomalies, excludedWatchAnomalies)
	}
	for _, marker := range freshnessMarkers {
		if marker.Status == "STALE" {
			report.Notes = fmt.Sprintf("%s Source %s is stale (%.1f minutes old).", report.Notes, marker.Source, marker.AgeMinutes)
		}
	}
	GetGlobalMetrics().SetEvalMetrics(uint64(len(recommendations)), uint64(len(gateBugFindings)))
	GetGlobalMetrics().SetLastEvaluationTime(report.GeneratedAt)

	slog.Info("Feedback Loop evaluation report generated successfully",
		"finalized_signals", len(finalized),
		"recommendations", len(recommendations),
		"gate_bugs", len(gateBugFindings),
		"best_playbook", bestPb,
		"worst_playbook", worstPb,
	)

	return uc.storageUsecase.SaveEvaluationReport(report)
}
