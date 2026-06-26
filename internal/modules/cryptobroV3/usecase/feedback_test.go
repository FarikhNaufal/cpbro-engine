package usecase_test

import (
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"cpbro-engine/internal/modules/cryptobroV3/entity"
	"cpbro-engine/internal/modules/cryptobroV3/usecase"
)

type mockFeedbackStorageRepo struct {
	journal   []usecase.SignalJournal
	watch     []usecase.WatchJournal
	latestRes *entity.LatestResult
	audits    []usecase.DecisionAudit
	report    *usecase.EvaluationReport
}

func (m *mockFeedbackStorageRepo) LoadLatestResult() (*entity.LatestResult, error) {
	return m.latestRes, nil
}
func (m *mockFeedbackStorageRepo) SaveLatestResult(res *entity.LatestResult) error {
	m.latestRes = res
	return nil
}

func (m *mockFeedbackStorageRepo) LoadSignalHistory() (*entity.SignalHistory, error) {
	return nil, nil
}
func (m *mockFeedbackStorageRepo) SaveSignalHistory(hist *entity.SignalHistory) error {
	return nil
}

func (m *mockFeedbackStorageRepo) LoadAIAuditCache() (*entity.AIAuditCache, error) {
	return nil, nil
}
func (m *mockFeedbackStorageRepo) SaveAIAuditCache(cache *entity.AIAuditCache) error {
	return nil
}

func (m *mockFeedbackStorageRepo) LoadSignalJournal() ([]usecase.SignalJournal, error) {
	return m.journal, nil
}

func (m *mockFeedbackStorageRepo) SaveSignalJournal(journal []usecase.SignalJournal) error {
	m.journal = journal
	return nil
}

func (m *mockFeedbackStorageRepo) AppendSignalJournal(entry usecase.SignalJournal) error {
	m.journal = append(m.journal, entry)
	return nil
}

func (m *mockFeedbackStorageRepo) LoadWatchJournal() ([]usecase.WatchJournal, error) {
	return m.watch, nil
}

func (m *mockFeedbackStorageRepo) SaveWatchJournal(journal []usecase.WatchJournal) error {
	m.watch = journal
	return nil
}

func (m *mockFeedbackStorageRepo) AppendWatchJournal(entry usecase.WatchJournal) error {
	m.watch = append(m.watch, entry)
	return nil
}

func (m *mockFeedbackStorageRepo) LoadEvaluationReport() (*usecase.EvaluationReport, error) {
	return m.report, nil
}

func (m *mockFeedbackStorageRepo) SaveEvaluationReport(report *usecase.EvaluationReport) error {
	m.report = report
	return nil
}

func (m *mockFeedbackStorageRepo) LoadDecisionAudits() ([]usecase.DecisionAudit, error) {
	return m.audits, nil
}

func (m *mockFeedbackStorageRepo) SaveDecisionAudits(audits []usecase.DecisionAudit) error {
	m.audits = audits
	return nil
}

func (m *mockFeedbackStorageRepo) AppendDecisionAudit(entry usecase.DecisionAudit) error {
	m.audits = append(m.audits, entry)
	if len(m.audits) > 1000 {
		m.audits = m.audits[len(m.audits)-1000:]
	}
	return nil
}

// 1. Test empty signal journal
func TestFeedback_EmptyJournalAndAudits(t *testing.T) {
	original := usecase.SnapshotRuntimeSettings()
	t.Cleanup(func() { usecase.SetRuntimeSettings(original) })
	settings := original
	settings.SignalJournalFile = "sig_custom.json"
	settings.WatchJournalFile = "watch_custom.json"
	settings.DecisionAuditFile = "audit_custom.json"
	usecase.SetRuntimeSettings(settings)

	repo := &mockFeedbackStorageRepo{journal: nil, audits: nil}
	storage := usecase.NewStorageUsecase(repo)
	fb := usecase.NewFeedbackUsecase(storage)

	err := fb.GenerateEvaluationReport()
	if err != nil {
		t.Fatalf("Expected no error on empty data, got: %v", err)
	}

	report := repo.report
	if report == nil {
		t.Fatal("Expected report to be saved, got nil")
	}

	if report.TotalSignals != 0 {
		t.Errorf("Expected 0 signals, got %d", report.TotalSignals)
	}

	if len(report.Recommendations) != 1 || report.Recommendations[0].IssueType != "INSUFFICIENT_SAMPLE" {
		t.Errorf("Expected 1 INSUFFICIENT_SAMPLE recommendation, got %v", report.Recommendations)
	}
	if !strings.Contains(report.Recommendations[0].Reason, "sig_custom.json") ||
		!strings.Contains(report.Recommendations[0].Reason, "watch_custom.json") ||
		!strings.Contains(report.Recommendations[0].Reason, "audit_custom.json") {
		t.Fatalf("expected custom source file names in reason, got %q", report.Recommendations[0].Reason)
	}

	if report.DataCompleteness.HasSignalJournal || report.DataCompleteness.HasDecisionAudit {
		t.Error("Expected completeness flags to be false")
	}
}

func TestFeedback_UsesConfiguredSourceFileLabels(t *testing.T) {
	original := usecase.SnapshotRuntimeSettings()
	t.Cleanup(func() { usecase.SetRuntimeSettings(original) })
	settings := original
	settings.LatestResultFile = "latest_custom.json"
	settings.SignalJournalFile = "signal_custom.json"
	settings.WatchJournalFile = "watch_custom.json"
	settings.DecisionAuditFile = "audit_custom.json"
	usecase.SetRuntimeSettings(settings)

	now := time.Now()
	repo := &mockFeedbackStorageRepo{
		journal: []usecase.SignalJournal{{
			ID:        "sig1",
			Symbol:    "BTCUSDT",
			Playbook:  usecase.TREND_PULLBACK,
			Status:    usecase.TP2_HIT,
			CreatedAt: now.Add(-30 * time.Minute),
			UpdatedAt: now.Add(-15 * time.Minute),
			ClosedAt:  now.Add(-10 * time.Minute),
		}},
		watch: []usecase.WatchJournal{{
			ID:        "w1",
			Symbol:    "SOLUSDT",
			Playbook:  usecase.LIQUIDITY_SWEEP_REVERSAL,
			Status:    usecase.WATCH_PROMOTED,
			CreatedAt: now.Add(-40 * time.Minute),
			UpdatedAt: now.Add(-10 * time.Minute),
		}},
		latestRes: &entity.LatestResult{
			Signals: []dto.SignalResponse{{Symbol: "BTCUSDT"}},
		},
		audits: []usecase.DecisionAudit{{GeneratedAt: now.Add(-5 * time.Minute)}},
	}

	fb := usecase.NewFeedbackUsecase(usecase.NewStorageUsecase(repo))
	if err := fb.GenerateEvaluationReport(); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	want := []string{"signal_custom.json", "watch_custom.json", "latest_custom.json", "audit_custom.json"}
	if !reflect.DeepEqual(repo.report.SourceFilesUsed, want) {
		t.Fatalf("expected source files %v, got %v", want, repo.report.SourceFilesUsed)
	}
}

// 2. Test sample guard < 10
func TestFeedback_SmallSampleGuard(t *testing.T) {
	// Create exactly 5 signals (less than 10)
	journal := make([]usecase.SignalJournal, 5)
	for i := 0; i < 5; i++ {
		journal[i] = usecase.SignalJournal{
			ID:           "small_sig",
			Playbook:     "TREND_PULLBACK",
			Status:       usecase.SL_HIT,
			MarketRegime: "BULLISH",
			AIConfidence: "HIGH",
			RR:           1.2,
		}
	}

	repo := &mockFeedbackStorageRepo{journal: journal}
	storage := usecase.NewStorageUsecase(repo)
	fb := usecase.NewFeedbackUsecase(storage)

	err := fb.GenerateEvaluationReport()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	report := repo.report
	for _, rec := range report.Recommendations {
		if rec.Playbook == "TREND_PULLBACK" {
			if rec.IssueType != "INSUFFICIENT_SAMPLE" {
				t.Errorf("Expected IssueType to be INSUFFICIENT_SAMPLE for small sample size, got %s", rec.IssueType)
			}
			if rec.ConfidenceLevel != "LOW" {
				t.Errorf("Expected ConfidenceLevel to be LOW, got %s", rec.ConfidenceLevel)
			}
			if !rec.RequiresMoreData {
				t.Error("Expected RequiresMoreData to be true")
			}
			if !strings.Contains(rec.SuggestedAction, "HOLD TUNING") {
				t.Errorf("Expected SuggestedAction to start with HOLD TUNING, got %s", rec.SuggestedAction)
			}
		}
	}
}

// 3. Test Liquidity Sweep without volume confirmation -> GATE_BUG
func TestFeedback_GateBugLiquiditySweepNoVolume(t *testing.T) {
	// 12 items to pass sample guard (>= 10)
	journal := make([]usecase.SignalJournal, 12)
	for i := 0; i < 12; i++ {
		journal[i] = usecase.SignalJournal{
			ID:                      "gate_bug_sweep",
			Playbook:                "LIQUIDITY_SWEEP_REVERSAL",
			Status:                  usecase.SL_HIT,
			ThresholdProfileSummary: "low volume ratio, volume confirmation: false",
			AIConfidence:            "HIGH", // High confidence but low volume ratio (gate bug)
		}
	}

	repo := &mockFeedbackStorageRepo{journal: journal}
	storage := usecase.NewStorageUsecase(repo)
	fb := usecase.NewFeedbackUsecase(storage)

	err := fb.GenerateEvaluationReport()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	report := repo.report
	foundGateBug := false
	foundTuningSuspended := false

	for _, rec := range report.Recommendations {
		if rec.Playbook == "LIQUIDITY_SWEEP_REVERSAL" {
			if rec.IssueType == "GATE_BUG" {
				foundGateBug = true
				if rec.Severity != "HIGH" {
					t.Errorf("Expected Severity to be HIGH for gate bug, got %s", rec.Severity)
				}
			}
			if rec.IssueType == "THRESHOLD_TUNING" {
				if !strings.Contains(rec.SuggestedAction, "HOLD TUNING") {
					t.Errorf("Expected threshold tuning to be suspended (HOLD TUNING), got %s", rec.SuggestedAction)
				}
				foundTuningSuspended = true
			}
		}
	}

	if !foundGateBug {
		t.Error("Expected GATE_BUG recommendation, but not found")
	}
	if !foundTuningSuspended {
		t.Error("Expected threshold tuning to be suspended due to gate bug priority")
	}
}

func TestFeedback_GateBugAIConfidenceRespectsCurrentRequirement(t *testing.T) {
	original := usecase.SnapshotRuntimeSettings()
	t.Cleanup(func() { usecase.SetRuntimeSettings(original) })
	settings := original
	settings.RequireAIHighForExecute = false
	usecase.SetRuntimeSettings(settings)

	journal := make([]usecase.SignalJournal, 12)
	for i := 0; i < 12; i++ {
		journal[i] = usecase.SignalJournal{
			ID:           "med_conf_exec",
			Playbook:     usecase.TREND_PULLBACK,
			Status:       usecase.TP2_HIT,
			MarketRegime: string(usecase.ALT_SUPPORTIVE),
			AIConfidence: "MEDIUM",
			EntryPrice:   100,
			TP1:          103,
			TP2:          106,
		}
	}

	repo := &mockFeedbackStorageRepo{journal: journal}
	storage := usecase.NewStorageUsecase(repo)
	fb := usecase.NewFeedbackUsecase(storage)

	if err := fb.GenerateEvaluationReport(); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	for _, rec := range repo.report.Recommendations {
		if rec.IssueType == "GATE_BUG" && strings.Contains(rec.EvidenceSummary, "AI confidence was MEDIUM") {
			t.Fatalf("did not expect MEDIUM confidence gate bug when current regime only requires MEDIUM: %+v", rec)
		}
	}
}

func TestFeedback_GateBugStalenessRespectsFreshRequirement(t *testing.T) {
	original := usecase.SnapshotRuntimeSettings()
	t.Cleanup(func() { usecase.SetRuntimeSettings(original) })
	settings := original
	settings.RequireFreshEntryForExecute = false
	usecase.SetRuntimeSettings(settings)

	journal := make([]usecase.SignalJournal, 12)
	for i := 0; i < 12; i++ {
		journal[i] = usecase.SignalJournal{
			ID:           "late_but_allowed",
			Playbook:     usecase.TREND_PULLBACK,
			Status:       usecase.TP2_HIT,
			MarketRegime: string(usecase.ALT_SUPPORTIVE),
			AIConfidence: "HIGH",
			EntryTiming:  "LATE",
			EntryPrice:   100,
			TP1:          103,
			TP2:          106,
		}
	}

	repo := &mockFeedbackStorageRepo{journal: journal}
	storage := usecase.NewStorageUsecase(repo)
	fb := usecase.NewFeedbackUsecase(storage)

	if err := fb.GenerateEvaluationReport(); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	for _, rec := range repo.report.Recommendations {
		if rec.IssueType == "GATE_BUG" && strings.Contains(rec.EvidenceSummary, "Staleness was not FRESH") {
			t.Fatalf("did not expect stale-entry gate bug when fresh entry is not required: %+v", rec)
		}
	}
}

func TestFeedback_GateBugAIConfidenceUsesStoredSignalSnapshot(t *testing.T) {
	journal := make([]usecase.SignalJournal, 12)
	for i := 0; i < 12; i++ {
		journal[i] = usecase.SignalJournal{
			ID:                        "high_vol_exec",
			Symbol:                    "ETHUSDT",
			Playbook:                  usecase.TREND_PULLBACK,
			Status:                    usecase.TP2_HIT,
			MarketRegime:              string(usecase.HIGH_VOL),
			Tier:                      usecase.TierA,
			AIConfidence:              "MEDIUM",
			PolicyLongMode:            string(usecase.NORMAL),
			PolicyShortMode:           string(usecase.NORMAL),
			PolicyRequireAIConfidence: string(usecase.AIConfidenceMedium),
			PolicyAllowedPlaybooks:    []string{string(usecase.TREND_PULLBACK), string(usecase.LIQUIDITY_SWEEP_REVERSAL)},
			EntryPrice:                100,
			TP1:                       103,
			TP2:                       106,
		}
	}

	repo := &mockFeedbackStorageRepo{journal: journal}
	storage := usecase.NewStorageUsecase(repo)
	fb := usecase.NewFeedbackUsecase(storage)

	if err := fb.GenerateEvaluationReport(); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	for _, rec := range repo.report.Recommendations {
		if rec.IssueType == "GATE_BUG" && strings.Contains(rec.EvidenceSummary, "AI confidence was MEDIUM") {
			t.Fatalf("did not expect AI confidence gate bug when stored signal snapshot required only MEDIUM: %+v", rec)
		}
	}
}

func TestFeedback_GateBugStalenessUsesStoredAuditSnapshot(t *testing.T) {
	audits := make([]usecase.DecisionAudit, 12)
	for i := 0; i < 12; i++ {
		audits[i] = usecase.DecisionAudit{
			Symbol:                    "BTCUSDT",
			Playbook:                  usecase.LIQUIDITY_SWEEP_REVERSAL,
			MarketRegime:              string(usecase.HIGH_VOL),
			Tier:                      usecase.TierA,
			AIConfidence:              "HIGH",
			FinalStatus:               usecase.FINAL_EXECUTE,
			StalenessStatus:           "LATE",
			PolicyLongMode:            string(usecase.NORMAL),
			PolicyShortMode:           string(usecase.NORMAL),
			PolicyRequireFreshEntry:   false,
			PolicyAllowedPlaybooks:    []string{string(usecase.LIQUIDITY_SWEEP_REVERSAL), string(usecase.TREND_PULLBACK)},
			PolicyRequireAIConfidence: string(usecase.AIConfidenceHigh),
		}
	}

	repo := &mockFeedbackStorageRepo{audits: audits}
	storage := usecase.NewStorageUsecase(repo)
	fb := usecase.NewFeedbackUsecase(storage)

	if err := fb.GenerateEvaluationReport(); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	for _, rec := range repo.report.Recommendations {
		if rec.IssueType == "GATE_BUG" && strings.Contains(rec.EvidenceSummary, "Staleness was LATE") {
			t.Fatalf("did not expect stale-entry gate bug when stored audit snapshot did not require fresh entry: %+v", rec)
		}
	}
}

// 4. Test AI MEDIUM evaluation without decision_audit.json
func TestFeedback_AIMediumNoDecisionAudit(t *testing.T) {
	// Create sufficient sample journal, but audits is nil
	journal := make([]usecase.SignalJournal, 15)
	for i := 0; i < 15; i++ {
		journal[i] = usecase.SignalJournal{
			ID:       "sig",
			Playbook: "TREND_PULLBACK",
			Status:   usecase.TP1_HIT,
		}
	}

	repo := &mockFeedbackStorageRepo{journal: journal, audits: nil}
	storage := usecase.NewStorageUsecase(repo)
	fb := usecase.NewFeedbackUsecase(storage)

	err := fb.GenerateEvaluationReport()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	report := repo.report
	if report.DataCompleteness.HasDecisionAudit {
		t.Error("Expected HasDecisionAudit to be false")
	}

	foundAIMediumWarning := false
	for _, rec := range report.Recommendations {
		if rec.MetricName == "MISSED_OPPORTUNITY_EVALUATION" {
			foundAIMediumWarning = true
			if rec.IssueType != "INSUFFICIENT_SAMPLE" {
				t.Errorf("Expected IssueType to be INSUFFICIENT_SAMPLE, got %s", rec.IssueType)
			}
			if !strings.Contains(rec.Reason, "Need decision_audit/watchlist monitoring") {
				t.Errorf("Expected reason to mention decision audit requirement, got %s", rec.Reason)
			}
		}
	}
	if !foundAIMediumWarning {
		t.Error("Expected warning about missing decision audit file")
	}
}

func TestFeedback_AIMediumWatchRecommendationBecomesObservabilityReview(t *testing.T) {
	audits := make([]usecase.DecisionAudit, 12)
	for i := 0; i < 12; i++ {
		audits[i] = usecase.DecisionAudit{
			Symbol:       "BTCUSDT",
			Playbook:     usecase.TREND_PULLBACK,
			MarketRegime: string(usecase.ALT_SUPPORTIVE),
			AIConfidence: "MEDIUM",
			AIDecision:   "WAIT",
			FinalStatus:  usecase.FINAL_WATCH,
		}
	}

	repo := &mockFeedbackStorageRepo{audits: audits}
	storage := usecase.NewStorageUsecase(repo)
	fb := usecase.NewFeedbackUsecase(storage)

	if err := fb.GenerateEvaluationReport(); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	found := false
	for _, rec := range repo.report.Recommendations {
		if rec.MetricName == "AI_WATCH_DECISION_COUNT" {
			found = true
			if rec.IssueType != "OBSERVABILITY_TUNING" {
				t.Fatalf("expected observability recommendation, got %s", rec.IssueType)
			}
			if !strings.Contains(rec.SuggestedAction, "keep AI MEDIUM non-executable") {
				t.Fatalf("expected conservative watch review guidance, got %s", rec.SuggestedAction)
			}
		}
	}
	if !found {
		t.Fatal("expected AI_WATCH_DECISION_COUNT observability recommendation")
	}
}

func TestFeedback_WatchJournalOnlyProducesVirtualMetrics(t *testing.T) {
	repo := &mockFeedbackStorageRepo{
		watch: []usecase.WatchJournal{
			{
				ID:            "watch_1",
				Playbook:      usecase.LIQUIDITY_SWEEP_REVERSAL,
				Direction:     usecase.LONG,
				Status:        usecase.VIRTUAL_TP2_HIT,
				MarketRegime:  "CHOP_RANGE",
				MFE:           2.1,
				MAE:           0.4,
				PnlPercentage: 1.8,
			},
			{
				ID:            "watch_2",
				Playbook:      usecase.TREND_PULLBACK,
				Direction:     usecase.SHORT,
				Status:        usecase.VIRTUAL_SL_HIT,
				MarketRegime:  "RISK_OFF",
				MFE:           0.6,
				MAE:           1.2,
				PnlPercentage: -0.9,
			},
		},
	}
	storage := usecase.NewStorageUsecase(repo)
	fb := usecase.NewFeedbackUsecase(storage)

	if err := fb.GenerateEvaluationReport(); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if repo.report == nil {
		t.Fatal("expected report to be saved")
	}
	if repo.report.Metrics["watch_total"] != 2 {
		t.Fatalf("expected watch_total=2, got %v", repo.report.Metrics["watch_total"])
	}
	if repo.report.Metrics["watch_finalized"] != 2 {
		t.Fatalf("expected watch_finalized=2, got %v", repo.report.Metrics["watch_finalized"])
	}
	if repo.report.Metrics["watch_virtual_tp2_rate"] != 50 {
		t.Fatalf("expected watch_virtual_tp2_rate=50, got %v", repo.report.Metrics["watch_virtual_tp2_rate"])
	}
	if !repo.report.DataCompleteness.CanEvaluateWatchMissedOpportunity {
		t.Fatal("expected CanEvaluateWatchMissedOpportunity=true")
	}
}

func TestFeedback_SignalMetrics_ExcludeActiveTP1AndUseRealizedPnL(t *testing.T) {
	now := time.Now().UTC()
	repo := &mockFeedbackStorageRepo{
		journal: []usecase.SignalJournal{
			{
				ID:            "sig_active_tp1",
				Playbook:      usecase.TREND_PULLBACK,
				Direction:     usecase.LONG,
				Status:        usecase.TP1_HIT,
				EntryPrice:    100,
				TP1:           105,
				TP2:           110,
				StopLoss:      95,
				TimeToTP1:     "10m",
				ExpiresAt:     now.Add(30 * time.Minute),
				PnlPercentage: 9.9,
			},
			{
				ID:            "sig_partial_final",
				Playbook:      usecase.LIQUIDITY_SWEEP_REVERSAL,
				Direction:     usecase.LONG,
				Status:        usecase.TP1_HIT,
				EntryPrice:    100,
				TP1:           105,
				TP2:           110,
				StopLoss:      95,
				TimeToTP1:     "12m",
				ExpiresAt:     now.Add(-1 * time.Minute),
				PnlPercentage: 2.5,
			},
			{
				ID:            "sig_tp2",
				Playbook:      usecase.LIQUIDITY_SWEEP_REVERSAL,
				Direction:     usecase.LONG,
				Status:        usecase.TP2_HIT,
				EntryPrice:    100,
				TP1:           105,
				TP2:           110,
				StopLoss:      95,
				TimeToTP1:     "5m",
				TimeToTP2:     "20m",
				PnlPercentage: 7.5,
			},
			{
				ID:            "sig_sl_after_tp1",
				Playbook:      usecase.LIQUIDITY_SWEEP_REVERSAL,
				Direction:     usecase.LONG,
				Status:        usecase.SL_HIT,
				EntryPrice:    100,
				TP1:           105,
				TP2:           110,
				StopLoss:      95,
				TimeToTP1:     "8m",
				TimeToSL:      "15m",
				PnlPercentage: 0.0,
			},
		},
	}
	storage := usecase.NewStorageUsecase(repo)
	fb := usecase.NewFeedbackUsecase(storage)

	if err := fb.GenerateEvaluationReport(); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if repo.report == nil {
		t.Fatal("expected report to be saved")
	}
	if repo.report.TotalSignals != 3 {
		t.Fatalf("expected 3 finalized signals, got %d", repo.report.TotalSignals)
	}
	if math.Abs(repo.report.Metrics["win_rate"]-((2.0/3.0)*100.0)) > 1e-9 {
		t.Fatalf("expected win_rate 66.666..., got %v", repo.report.Metrics["win_rate"])
	}
	expectedPnl := 2.5 + 7.5 + 0.0
	if repo.report.Metrics["total_pnl_percentage"] != expectedPnl {
		t.Fatalf("expected realized total_pnl_percentage %v, got %v", expectedPnl, repo.report.Metrics["total_pnl_percentage"])
	}
}

func TestFeedback_WatchMetrics_ExcludeActiveVirtualTP1AndUseRealizedPnL(t *testing.T) {
	now := time.Now().UTC()
	repo := &mockFeedbackStorageRepo{
		watch: []usecase.WatchJournal{
			{
				ID:            "watch_active_tp1",
				Playbook:      usecase.TREND_PULLBACK,
				Direction:     usecase.LONG,
				Status:        usecase.VIRTUAL_TP1_HIT,
				EntryPrice:    100,
				TP1:           105,
				TP2:           110,
				StopLoss:      95,
				TimeToTP1:     "7m",
				ExpiresAt:     now.Add(25 * time.Minute),
				PnlPercentage: 9.0,
			},
			{
				ID:            "watch_partial_final",
				Playbook:      usecase.LIQUIDITY_SWEEP_REVERSAL,
				Direction:     usecase.LONG,
				Status:        usecase.VIRTUAL_TP1_HIT,
				EntryPrice:    100,
				TP1:           105,
				TP2:           110,
				StopLoss:      95,
				TimeToTP1:     "11m",
				ExpiresAt:     now.Add(-1 * time.Minute),
				PnlPercentage: 2.5,
			},
			{
				ID:            "watch_sl",
				Playbook:      usecase.LIQUIDITY_SWEEP_REVERSAL,
				Direction:     usecase.LONG,
				Status:        usecase.VIRTUAL_SL_HIT,
				EntryPrice:    100,
				TP1:           105,
				TP2:           110,
				StopLoss:      95,
				PnlPercentage: -5.0,
			},
		},
	}
	storage := usecase.NewStorageUsecase(repo)
	fb := usecase.NewFeedbackUsecase(storage)

	if err := fb.GenerateEvaluationReport(); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if repo.report == nil {
		t.Fatal("expected report to be saved")
	}
	if repo.report.Metrics["watch_finalized"] != 2 {
		t.Fatalf("expected 2 finalized virtual watches, got %v", repo.report.Metrics["watch_finalized"])
	}
	if repo.report.Metrics["watch_virtual_win_rate"] != 50.0 {
		t.Fatalf("expected watch_virtual_win_rate 50, got %v", repo.report.Metrics["watch_virtual_win_rate"])
	}
	expectedPnl := 2.5 - 5.0
	if repo.report.Metrics["watch_total_pnl_percentage"] != expectedPnl {
		t.Fatalf("expected watch_total_pnl_percentage %v, got %v", expectedPnl, repo.report.Metrics["watch_total_pnl_percentage"])
	}
}

func TestFeedback_WatchPromotedExcludedFromVirtualWatchMetrics(t *testing.T) {
	now := time.Now().UTC()
	repo := &mockFeedbackStorageRepo{
		watch: []usecase.WatchJournal{
			{
				ID:            "watch_promoted",
				Playbook:      usecase.LIQUIDITY_SWEEP_REVERSAL,
				Direction:     usecase.LONG,
				Status:        usecase.WATCH_PROMOTED,
				EntryPrice:    100,
				TP1:           105,
				TP2:           110,
				StopLoss:      95,
				CreatedAt:     now.Add(-20 * time.Minute),
				ClosedAt:      now.Add(-10 * time.Minute),
				PnlPercentage: 9.0,
			},
			{
				ID:            "watch_virtual_tp2",
				Playbook:      usecase.LIQUIDITY_SWEEP_REVERSAL,
				Direction:     usecase.LONG,
				Status:        usecase.VIRTUAL_TP2_HIT,
				EntryPrice:    100,
				TP1:           105,
				TP2:           110,
				StopLoss:      95,
				CreatedAt:     now.Add(-40 * time.Minute),
				ClosedAt:      now.Add(-5 * time.Minute),
				PnlPercentage: 7.5,
			},
		},
	}

	fb := usecase.NewFeedbackUsecase(usecase.NewStorageUsecase(repo))
	if err := fb.GenerateEvaluationReport(); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if repo.report == nil {
		t.Fatal("expected report to be saved")
	}
	if repo.report.Metrics["watch_finalized"] != 1 {
		t.Fatalf("expected only finalized virtual watch outcomes to be counted, got %v", repo.report.Metrics["watch_finalized"])
	}
	if repo.report.Metrics["watch_promoted_count"] != 1 {
		t.Fatalf("expected watch_promoted_count=1, got %v", repo.report.Metrics["watch_promoted_count"])
	}
}

func TestFeedback_PromotedConversionAndFreshnessMetrics(t *testing.T) {
	now := time.Now().UTC()
	repo := &mockFeedbackStorageRepo{
		journal: []usecase.SignalJournal{
			{
				ID:            "sig_promoted_tp2",
				Playbook:      usecase.LIQUIDITY_SWEEP_REVERSAL,
				Direction:     usecase.LONG,
				Status:        usecase.TP2_HIT,
				CreatedAt:     now.Add(-30 * time.Minute),
				UpdatedAt:     now.Add(-20 * time.Minute),
				ClosedAt:      now.Add(-10 * time.Minute),
				EntryPrice:    100,
				TP1:           105,
				TP2:           110,
				StopLoss:      95,
				PnlPercentage: 7.5,
				Reason:        "WATCH_RECHECK_PROMOTION origin_watch_id=w1 | confirmed",
			},
		},
		watch: []usecase.WatchJournal{
			{ID: "w1", Status: usecase.WATCH_PROMOTED, CreatedAt: now.Add(-40 * time.Minute), UpdatedAt: now.Add(-10 * time.Minute)},
			{ID: "w2", Status: usecase.WATCH_RECHECK_INVALIDATED, CreatedAt: now.Add(-50 * time.Minute), UpdatedAt: now.Add(-15 * time.Minute), ClosedAt: now.Add(-15 * time.Minute)},
			{ID: "w3", Status: usecase.WATCH_RECHECK_EXPIRED, CreatedAt: now.Add(-60 * time.Minute), UpdatedAt: now.Add(-25 * time.Minute), ClosedAt: now.Add(-25 * time.Minute)},
		},
		audits: []usecase.DecisionAudit{
			{GeneratedAt: now.Add(-5 * time.Minute)},
		},
	}

	fb := usecase.NewFeedbackUsecase(usecase.NewStorageUsecase(repo))
	if err := fb.GenerateEvaluationReport(); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if repo.report == nil {
		t.Fatal("expected report to be saved")
	}
	if math.Abs(repo.report.Metrics["watch_to_promote_conversion_rate"]-((1.0/3.0)*100.0)) > 1e-9 {
		t.Fatalf("expected conversion rate 33.33..., got %v", repo.report.Metrics["watch_to_promote_conversion_rate"])
	}
	if repo.report.Metrics["promoted_win_rate"] != 100.0 {
		t.Fatalf("expected promoted_win_rate 100, got %v", repo.report.Metrics["promoted_win_rate"])
	}
	if repo.report.FreshnessMarkers["signal_journal"].Status == "" {
		t.Fatalf("expected signal_journal freshness marker, got %+v", repo.report.FreshnessMarkers)
	}
	if repo.report.FreshnessMarkers["decision_audit"].Status == "" {
		t.Fatalf("expected decision_audit freshness marker, got %+v", repo.report.FreshnessMarkers)
	}
}

// 5. Test Trend Pullback many SL with low ADX
func TestFeedback_TrendPullbackLowADX(t *testing.T) {
	// 15 signals
	journal := make([]usecase.SignalJournal, 15)
	for i := 0; i < 15; i++ {
		journal[i] = usecase.SignalJournal{
			ID:                      "tp_sl",
			Playbook:                "TREND_PULLBACK",
			Status:                  usecase.SL_HIT,
			ThresholdProfileSummary: "ADX was low",
			AIConfidence:            "HIGH",
		}
	}

	repo := &mockFeedbackStorageRepo{journal: journal}
	storage := usecase.NewStorageUsecase(repo)
	fb := usecase.NewFeedbackUsecase(storage)

	err := fb.GenerateEvaluationReport()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	report := repo.report
	foundTuning := false
	for _, rec := range report.Recommendations {
		if rec.Playbook == "TREND_PULLBACK" && rec.IssueType == "THRESHOLD_TUNING" {
			foundTuning = true
			if rec.SuggestedThreshold != "MinADX: 25" {
				t.Errorf("Expected suggested threshold to be MinADX: 25, got %s", rec.SuggestedThreshold)
			}
		}
	}
	if !foundTuning {
		t.Error("Expected THRESHOLD_TUNING recommendation for TREND_PULLBACK")
	}
}

// 6. Test Range Edge Reversal many SL during ADX expansion
func TestFeedback_RangeEdgeADXExpansion(t *testing.T) {
	// 15 signals
	journal := make([]usecase.SignalJournal, 15)
	for i := 0; i < 15; i++ {
		journal[i] = usecase.SignalJournal{
			ID:                      "re_sl",
			Playbook:                "RANGE_EDGE_REVERSAL",
			Status:                  usecase.SL_HIT,
			ThresholdProfileSummary: "adx expansion active",
			AIConfidence:            "HIGH",
		}
	}

	repo := &mockFeedbackStorageRepo{journal: journal}
	storage := usecase.NewStorageUsecase(repo)
	fb := usecase.NewFeedbackUsecase(storage)

	err := fb.GenerateEvaluationReport()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	report := repo.report
	foundTuning := false
	for _, rec := range report.Recommendations {
		if rec.Playbook == "RANGE_EDGE_REVERSAL" && rec.IssueType == "THRESHOLD_TUNING" {
			foundTuning = true
			if rec.SuggestedThreshold != "MaxADX: 22" {
				t.Errorf("Expected suggested threshold to be MaxADX: 22, got %s", rec.SuggestedThreshold)
			}
		}
	}
	if !foundTuning {
		t.Error("Expected THRESHOLD_TUNING recommendation for RANGE_EDGE_REVERSAL")
	}
}

// 7. Test Compression Breakout Retest many stale
func TestFeedback_CompressionBreakoutStale(t *testing.T) {
	// 15 expired signals
	journal := make([]usecase.SignalJournal, 15)
	for i := 0; i < 15; i++ {
		journal[i] = usecase.SignalJournal{
			ID:           "cb_stale",
			Playbook:     "COMPRESSION_BREAKOUT_RETEST",
			Status:       usecase.EXPIRED,
			AIConfidence: "HIGH",
		}
	}

	repo := &mockFeedbackStorageRepo{journal: journal}
	storage := usecase.NewStorageUsecase(repo)
	fb := usecase.NewFeedbackUsecase(storage)

	err := fb.GenerateEvaluationReport()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	report := repo.report
	foundTuning := false
	for _, rec := range report.Recommendations {
		if rec.Playbook == "COMPRESSION_BREAKOUT_RETEST" && rec.IssueType == "THRESHOLD_TUNING" {
			foundTuning = true
			if !strings.Contains(strings.ToLower(rec.SuggestedAction), "tighten") && !strings.Contains(strings.ToLower(rec.SuggestedAction), "retest") {
				t.Errorf("Expected compression quality tightening suggestion, got %s", rec.SuggestedAction)
			}
		}
	}
	if !foundTuning {
		t.Error("Expected THRESHOLD_TUNING recommendation for COMPRESSION_BREAKOUT_RETEST")
	}
}

// 8. Test Tier C underperforms under High Volatility
func TestFeedback_TierCHighVol(t *testing.T) {
	// 15 Tier C signals during HIGH_VOLATILITY
	journal := make([]usecase.SignalJournal, 15)
	for i := 0; i < 15; i++ {
		journal[i] = usecase.SignalJournal{
			ID:           "tier_c_chaos",
			Playbook:     "TREND_PULLBACK",
			Tier:         usecase.TierC,
			Status:       usecase.SL_HIT,
			MarketRegime: "HIGH_VOLATILITY",
			AIConfidence: "HIGH",
			MAE:          6.0,
		}
	}

	repo := &mockFeedbackStorageRepo{journal: journal}
	storage := usecase.NewStorageUsecase(repo)
	fb := usecase.NewFeedbackUsecase(storage)

	err := fb.GenerateEvaluationReport()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	report := repo.report
	foundPolicyRec := false
	for _, rec := range report.Recommendations {
		if rec.Tier == "TierC" && rec.IssueType == "POLICY_TUNING" {
			foundPolicyRec = true
			if !strings.Contains(rec.SuggestedAction, "Block Tier C execution") {
				t.Errorf("Expected block Tier C recommendation, got %s", rec.SuggestedAction)
			}
		}
	}
	if !foundPolicyRec {
		t.Error("Expected Tier C POLICY_TUNING recommendation")
	}
}

// 9. Test Low Volatility Expired
func TestFeedback_LowVolExpired(t *testing.T) {
	// 15 signals during LOW_VOLATILITY that expired
	journal := make([]usecase.SignalJournal, 15)
	for i := 0; i < 15; i++ {
		journal[i] = usecase.SignalJournal{
			ID:           "low_vol_exp",
			Playbook:     "TREND_PULLBACK",
			Status:       usecase.EXPIRED,
			MarketRegime: "LOW_VOLATILITY",
			AIConfidence: "HIGH",
		}
	}

	repo := &mockFeedbackStorageRepo{journal: journal}
	storage := usecase.NewStorageUsecase(repo)
	fb := usecase.NewFeedbackUsecase(storage)

	err := fb.GenerateEvaluationReport()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	report := repo.report
	foundTuning := false
	for _, rec := range report.Recommendations {
		if rec.MarketRegime == string(usecase.LOW_VOL) && rec.IssueType == "TARGET_TUNING" {
			foundTuning = true
			if !strings.Contains(rec.SuggestedAction, "Lower take-profit targets") {
				t.Errorf("Expected lower TP targets action, got %s", rec.SuggestedAction)
			}
		}
	}
	if !foundTuning {
		t.Error("Expected TARGET_TUNING recommendation for LOW_VOL")
	}
}

func TestFeedback_RegimeStatsCanonicalizeLegacyLabels(t *testing.T) {
	journal := []usecase.SignalJournal{
		{
			ID:           "legacy_low_vol",
			Playbook:     "TREND_PULLBACK",
			Status:       usecase.EXPIRED,
			MarketRegime: "LOW_VOLATILITY",
			AIConfidence: "HIGH",
		},
		{
			ID:           "current_low_vol",
			Playbook:     "TREND_PULLBACK",
			Status:       usecase.EXPIRED,
			MarketRegime: string(usecase.LOW_VOL),
			AIConfidence: "HIGH",
		},
		{
			ID:           "legacy_bullish",
			Playbook:     "TREND_PULLBACK",
			Status:       usecase.SL_HIT,
			MarketRegime: "BULLISH",
			Direction:    usecase.SHORT,
			AIConfidence: "HIGH",
		},
		{
			ID:           "current_supportive",
			Playbook:     "TREND_PULLBACK",
			Status:       usecase.SL_HIT,
			MarketRegime: string(usecase.ALT_SUPPORTIVE),
			Direction:    usecase.SHORT,
			AIConfidence: "HIGH",
		},
		{
			ID:           "reason_phrase_chop",
			Playbook:     "RANGE_EDGE_REVERSAL",
			Status:       usecase.TP2_HIT,
			MarketRegime: "CHOP_RANGE active - mean reversion only",
			Direction:    usecase.LONG,
			AIConfidence: "HIGH",
			EntryPrice:   100,
			TP1:          102,
			TP2:          104,
		},
	}

	repo := &mockFeedbackStorageRepo{journal: journal}
	storage := usecase.NewStorageUsecase(repo)
	fb := usecase.NewFeedbackUsecase(storage)

	if err := fb.GenerateEvaluationReport(); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	report := repo.report
	if got := report.RegimeStats[string(usecase.LOW_VOL)].TotalSignals; got != 2 {
		t.Fatalf("expected LOW_VOL regime stats to merge legacy/current labels into 2 signals, got %d", got)
	}
	if _, exists := report.RegimeStats["LOW_VOLATILITY"]; exists {
		t.Fatal("expected legacy LOW_VOLATILITY key to be canonicalized away")
	}
	if got := report.RegimeStats[string(usecase.ALT_SUPPORTIVE)].TotalSignals; got != 2 {
		t.Fatalf("expected ALT_SUPPORTIVE regime stats to merge legacy/current labels into 2 signals, got %d", got)
	}
	if _, exists := report.RegimeStats["BULLISH"]; exists {
		t.Fatal("expected legacy BULLISH key to be canonicalized away")
	}
	if got := report.RegimeStats[string(usecase.CHOP_RANGE)].TotalSignals; got != 1 {
		t.Fatalf("expected CHOP_RANGE reason phrase to canonicalize into 1 signal, got %d", got)
	}
}

func TestFeedback_ShortSupportiveRecommendationUsesCanonicalRegimeLabel(t *testing.T) {
	journal := make([]usecase.SignalJournal, 15)
	for i := 0; i < 15; i++ {
		journal[i] = usecase.SignalJournal{
			ID:           "supportive_short_sl",
			Playbook:     "TREND_PULLBACK",
			Direction:    usecase.SHORT,
			Status:       usecase.SL_HIT,
			MarketRegime: "BULLISH",
			AIConfidence: "HIGH",
			RR:           1.8,
		}
	}

	repo := &mockFeedbackStorageRepo{journal: journal}
	storage := usecase.NewStorageUsecase(repo)
	fb := usecase.NewFeedbackUsecase(storage)

	if err := fb.GenerateEvaluationReport(); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	report := repo.report
	for _, rec := range report.Recommendations {
		if rec.MetricName == "SHORT_SUPPORTIVE_SL_RATE" {
			if rec.MarketRegime != string(usecase.ALT_SUPPORTIVE) {
				t.Fatalf("expected canonical supportive regime label, got %s", rec.MarketRegime)
			}
			if rec.Direction != string(usecase.SHORT) {
				t.Fatalf("expected SHORT direction, got %s", rec.Direction)
			}
			return
		}
	}

	t.Fatal("expected SHORT_SUPPORTIVE_SL_RATE recommendation")
}

func TestFeedback_LongRiskOffRecommendationUsesCurrentModeSemantics(t *testing.T) {
	journal := make([]usecase.SignalJournal, 15)
	for i := 0; i < 15; i++ {
		journal[i] = usecase.SignalJournal{
			ID:           "risk_off_long_sl",
			Playbook:     "TREND_PULLBACK",
			Direction:    usecase.LONG,
			Status:       usecase.SL_HIT,
			MarketRegime: "BEARISH",
			AIConfidence: "HIGH",
			RR:           1.8,
		}
	}

	repo := &mockFeedbackStorageRepo{journal: journal}
	storage := usecase.NewStorageUsecase(repo)
	fb := usecase.NewFeedbackUsecase(storage)

	if err := fb.GenerateEvaluationReport(); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	report := repo.report
	for _, rec := range report.Recommendations {
		if rec.MetricName == "LONG_RISK_OFF_SL_RATE" {
			if rec.MarketRegime != string(usecase.RISK_OFF) {
				t.Fatalf("expected canonical risk-off regime label, got %s", rec.MarketRegime)
			}
			if rec.SuggestedThreshold != "LongMode: SWEEP_ONLY" {
				t.Fatalf("expected updated mode semantics, got %s", rec.SuggestedThreshold)
			}
			if !strings.Contains(rec.SuggestedAction, "SWEEP_ONLY") {
				t.Fatalf("expected suggested action to mention SWEEP_ONLY, got %s", rec.SuggestedAction)
			}
			return
		}
	}

	t.Fatal("expected LONG_RISK_OFF_SL_RATE recommendation")
}

func TestFeedback_LongRegimePlaybookDiagnosticsMergeLegacyRegimes(t *testing.T) {
	journal := []usecase.SignalJournal{
		{
			ID:            "long_low_vol_legacy",
			Playbook:      "TREND_PULLBACK",
			Direction:     usecase.LONG,
			Status:        usecase.SL_HIT,
			MarketRegime:  "LOW_VOLATILITY",
			AIConfidence:  "HIGH",
			RR:            1.4,
			MAE:           2.5,
			MFE:           0.8,
			PnlPercentage: -1.2,
		},
		{
			ID:           "long_low_vol_current",
			Playbook:     "TREND_PULLBACK",
			Direction:    usecase.LONG,
			Status:       usecase.EXPIRED,
			MarketRegime: string(usecase.LOW_VOL),
			AIConfidence: "HIGH",
			RR:           1.3,
			MAE:          1.8,
			MFE:          0.7,
		},
		{
			ID:            "long_supportive_win",
			Playbook:      "LIQUIDITY_SWEEP_REVERSAL",
			Direction:     usecase.LONG,
			Status:        usecase.TP2_HIT,
			MarketRegime:  string(usecase.ALT_SUPPORTIVE),
			AIConfidence:  "HIGH",
			RR:            2.1,
			MAE:           0.9,
			MFE:           3.4,
			TP1:           110,
			TP2:           115,
			EntryPrice:    100,
			PnlPercentage: 1.5,
		},
	}

	repo := &mockFeedbackStorageRepo{journal: journal}
	storage := usecase.NewStorageUsecase(repo)
	fb := usecase.NewFeedbackUsecase(storage)

	if err := fb.GenerateEvaluationReport(); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	report := repo.report
	foundMerged := false
	for _, stat := range report.LongRegimePlaybookStats {
		if stat.MarketRegime == string(usecase.LOW_VOL) && stat.Playbook == "TREND_PULLBACK" {
			foundMerged = true
			if stat.TotalSignals != 2 {
				t.Fatalf("expected canonical LOW_VOL long slice to merge into 2 signals, got %d", stat.TotalSignals)
			}
			if stat.SLRate <= 0 {
				t.Fatalf("expected merged long slice to retain SL data, got %+v", stat)
			}
		}
		if stat.MarketRegime == "LOW_VOLATILITY" {
			t.Fatalf("expected no legacy regime label in long diagnostics, got %+v", stat)
		}
	}
	if !foundMerged {
		t.Fatal("expected merged LOW_VOL TREND_PULLBACK long diagnostic slice")
	}
}

func TestFeedback_LongDirectionalDiagnosticRecommendation(t *testing.T) {
	journal := make([]usecase.SignalJournal, 0, 24)
	for i := 0; i < 12; i++ {
		journal = append(journal, usecase.SignalJournal{
			ID:            "long_chop_trendpullback_sl",
			Playbook:      "TREND_PULLBACK",
			Direction:     usecase.LONG,
			Status:        usecase.SL_HIT,
			MarketRegime:  string(usecase.CHOP_RANGE),
			AIConfidence:  "HIGH",
			RR:            1.3,
			MAE:           2.2,
			MFE:           0.6,
			PnlPercentage: -1.0,
		})
	}
	for i := 0; i < 12; i++ {
		journal = append(journal, usecase.SignalJournal{
			ID:            "long_supportive_sweep_win",
			Playbook:      "LIQUIDITY_SWEEP_REVERSAL",
			Direction:     usecase.LONG,
			Status:        usecase.TP2_HIT,
			MarketRegime:  string(usecase.ALT_SUPPORTIVE),
			AIConfidence:  "HIGH",
			RR:            2.0,
			MAE:           0.8,
			MFE:           2.8,
			TP1:           104,
			TP2:           108,
			EntryPrice:    100,
			PnlPercentage: 1.4,
		})
	}

	repo := &mockFeedbackStorageRepo{journal: journal}
	storage := usecase.NewStorageUsecase(repo)
	fb := usecase.NewFeedbackUsecase(storage)

	if err := fb.GenerateEvaluationReport(); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	report := repo.report
	for _, rec := range report.Recommendations {
		if rec.IssueType == "DIRECTIONAL_DIAGNOSTIC" && rec.Direction == string(usecase.LONG) && rec.Playbook == "TREND_PULLBACK" && rec.MarketRegime == string(usecase.CHOP_RANGE) {
			if rec.MetricName != "LONG_REGIME_PLAYBOOK_WIN_RATE" {
				t.Fatalf("expected metric name LONG_REGIME_PLAYBOOK_WIN_RATE, got %s", rec.MetricName)
			}
			return
		}
	}

	t.Fatal("expected DIRECTIONAL_DIAGNOSTIC recommendation for weak LONG CHOP_RANGE TREND_PULLBACK slice")
}

func TestFeedback_DisabledLongSliceRecommendationStaysKeepDisabled(t *testing.T) {
	journal := make([]usecase.SignalJournal, 0, 24)
	for i := 0; i < 12; i++ {
		journal = append(journal, usecase.SignalJournal{
			ID:            "long_default_compression_sl",
			Playbook:      "COMPRESSION_BREAKOUT_RETEST",
			Direction:     usecase.LONG,
			Status:        usecase.SL_HIT,
			MarketRegime:  string(usecase.DEFAULT),
			AIConfidence:  "HIGH",
			RR:            2.0,
			MAE:           1.1,
			MFE:           0.4,
			PnlPercentage: -1.0,
		})
	}
	for i := 0; i < 12; i++ {
		journal = append(journal, usecase.SignalJournal{
			ID:            "short_default_sweep_win",
			Playbook:      "LIQUIDITY_SWEEP_REVERSAL",
			Direction:     usecase.SHORT,
			Status:        usecase.TP2_HIT,
			MarketRegime:  string(usecase.DEFAULT),
			AIConfidence:  "HIGH",
			RR:            2.0,
			MAE:           0.5,
			MFE:           2.0,
			PnlPercentage: 1.2,
		})
	}

	repo := &mockFeedbackStorageRepo{journal: journal}
	storage := usecase.NewStorageUsecase(repo)
	fb := usecase.NewFeedbackUsecase(storage)

	if err := fb.GenerateEvaluationReport(); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	for _, rec := range repo.report.Recommendations {
		if rec.IssueType == "DIRECTIONAL_DIAGNOSTIC" &&
			rec.Direction == string(usecase.LONG) &&
			rec.Playbook == "COMPRESSION_BREAKOUT_RETEST" &&
			rec.MarketRegime == string(usecase.DEFAULT) {
			if rec.SuggestedThreshold != "KEEP_DISABLED" {
				t.Fatalf("expected KEEP_DISABLED for already blocked slice, got %s", rec.SuggestedThreshold)
			}
			if !strings.Contains(strings.ToLower(rec.SuggestedAction), "keep the current block") {
				t.Fatalf("expected action to preserve current block, got %s", rec.SuggestedAction)
			}
			if rec.PolicyMode != string(usecase.NORMAL) {
				t.Fatalf("expected policy mode NORMAL for DEFAULT regime, got %s", rec.PolicyMode)
			}
			return
		}
	}

	t.Fatal("expected disabled-slice diagnostic recommendation for DEFAULT LONG COMPRESSION_BREAKOUT_RETEST")
}

func TestFeedback_GateBugDetectsBlockingM5ExecutionViolation(t *testing.T) {
	audits := make([]usecase.DecisionAudit, 12)
	for i := 0; i < 12; i++ {
		audits[i] = usecase.DecisionAudit{
			Symbol:               "ETHUSDT",
			Playbook:             usecase.LIQUIDITY_SWEEP_REVERSAL,
			MarketRegime:         string(usecase.ALT_SUPPORTIVE),
			AIConfidence:         "HIGH",
			FinalStatus:          usecase.FINAL_EXECUTE,
			M5ConfirmationUsed:   true,
			M5ConfirmationMode:   string(usecase.M5ConfirmationSoftConfirm),
			M5ConfirmationStatus: string(usecase.M5ConfirmationFailed),
		}
	}

	repo := &mockFeedbackStorageRepo{audits: audits}
	storage := usecase.NewStorageUsecase(repo)
	fb := usecase.NewFeedbackUsecase(storage)

	if err := fb.GenerateEvaluationReport(); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	for _, rec := range repo.report.Recommendations {
		if rec.IssueType == "GATE_BUG" && strings.Contains(rec.EvidenceSummary, "M5 mode SOFT_CONFIRM and status FAILED") {
			return
		}
	}

	t.Fatal("expected M5 blocking-mode gate bug recommendation")
}

func TestFeedback_WatchOnlyM5ExecutionDoesNotRaiseGateBug(t *testing.T) {
	audits := make([]usecase.DecisionAudit, 12)
	for i := 0; i < 12; i++ {
		audits[i] = usecase.DecisionAudit{
			Symbol:               "SOLUSDT",
			Playbook:             usecase.TREND_PULLBACK,
			MarketRegime:         string(usecase.ALT_SUPPORTIVE),
			AIConfidence:         "HIGH",
			FinalStatus:          usecase.FINAL_EXECUTE,
			M5ConfirmationUsed:   true,
			M5ConfirmationMode:   string(usecase.M5ConfirmationWatchOnlyHint),
			M5ConfirmationStatus: string(usecase.M5ConfirmationFailed),
		}
	}

	repo := &mockFeedbackStorageRepo{audits: audits}
	storage := usecase.NewStorageUsecase(repo)
	fb := usecase.NewFeedbackUsecase(storage)

	if err := fb.GenerateEvaluationReport(); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	for _, rec := range repo.report.Recommendations {
		if rec.IssueType == "GATE_BUG" && strings.Contains(rec.EvidenceSummary, "M5 mode WATCH_ONLY_HINT and status FAILED") {
			t.Fatalf("did not expect watch-only M5 execution to be treated as gate bug: %+v", rec)
		}
	}
}

func TestFeedback_LongDiagnosticsCanonicalizeRegimeReasonPhrase(t *testing.T) {
	journal := []usecase.SignalJournal{
		{
			ID:            "reason_phrase_long",
			Playbook:      "LIQUIDITY_SWEEP_REVERSAL",
			Direction:     usecase.LONG,
			Status:        usecase.SL_HIT,
			MarketRegime:  "CHOP_RANGE active - mean reversion only",
			AIConfidence:  "HIGH",
			RR:            1.8,
			MAE:           1.4,
			MFE:           0.5,
			PnlPercentage: -0.9,
		},
		{
			ID:           "canonical_long",
			Playbook:     "LIQUIDITY_SWEEP_REVERSAL",
			Direction:    usecase.LONG,
			Status:       usecase.TP2_HIT,
			MarketRegime: string(usecase.CHOP_RANGE),
			AIConfidence: "HIGH",
			RR:           2.0,
			MAE:          0.6,
			MFE:          2.1,
			EntryPrice:   100,
			TP1:          103,
			TP2:          106,
		},
	}

	repo := &mockFeedbackStorageRepo{journal: journal}
	storage := usecase.NewStorageUsecase(repo)
	fb := usecase.NewFeedbackUsecase(storage)

	if err := fb.GenerateEvaluationReport(); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	report := repo.report
	for _, stat := range report.LongRegimePlaybookStats {
		if stat.MarketRegime == string(usecase.CHOP_RANGE) && stat.Playbook == "LIQUIDITY_SWEEP_REVERSAL" {
			if stat.TotalSignals != 2 {
				t.Fatalf("expected CHOP_RANGE reason phrase + canonical long slice to merge into 2 signals, got %d", stat.TotalSignals)
			}
			return
		}
	}

	t.Fatal("expected canonicalized CHOP_RANGE long diagnostic slice")
}

// 10. DataCompleteness verification
func TestFeedback_DataCompleteness(t *testing.T) {
	latestRes := &entity.LatestResult{
		Signals: []dto.SignalResponse{
			{Symbol: "BTCUSDT"},
		},
	}
	audits := []usecase.DecisionAudit{
		{Symbol: "BTCUSDT", AIConfidence: "HIGH"},
	}
	journal := []usecase.SignalJournal{
		{ID: "sig", Playbook: "TREND_PULLBACK", Status: usecase.TP1_HIT},
	}

	repo := &mockFeedbackStorageRepo{
		journal:   journal,
		latestRes: latestRes,
		audits:    audits,
	}
	storage := usecase.NewStorageUsecase(repo)
	fb := usecase.NewFeedbackUsecase(storage)

	err := fb.GenerateEvaluationReport()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	report := repo.report
	if !report.DataCompleteness.HasSignalJournal {
		t.Error("Expected HasSignalJournal to be true")
	}
	if !report.DataCompleteness.HasLatestResult {
		t.Error("Expected HasLatestResult to be true")
	}
	if !report.DataCompleteness.HasDecisionAudit {
		t.Error("Expected HasDecisionAudit to be true")
	}
	if !report.DataCompleteness.CanEvaluateAIWait {
		t.Error("Expected CanEvaluateAIWait to be true")
	}
}

// 11. Playbook Disable recommendation verification
func TestFeedback_PlaybookDisable(t *testing.T) {
	// 15 signals for TREND_PULLBACK with SL_HIT
	journal := make([]usecase.SignalJournal, 15)
	for i := 0; i < 15; i++ {
		journal[i] = usecase.SignalJournal{
			ID:           "tp_disable",
			Playbook:     "TREND_PULLBACK",
			Status:       usecase.SL_HIT,
			AIConfidence: "HIGH",
		}
	}

	repo := &mockFeedbackStorageRepo{journal: journal}
	storage := usecase.NewStorageUsecase(repo)
	fb := usecase.NewFeedbackUsecase(storage)

	err := fb.GenerateEvaluationReport()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	report := repo.report
	foundDisable := false
	for _, rec := range report.Recommendations {
		if rec.Playbook == "TREND_PULLBACK" && rec.IssueType == "PLAYBOOK_DISABLE" {
			foundDisable = true
			if !strings.Contains(rec.SuggestedAction, "Disable this playbook") {
				t.Errorf("Expected disable suggestion, got %s", rec.SuggestedAction)
			}
		}
	}
	if !foundDisable {
		t.Error("Expected PLAYBOOK_DISABLE recommendation for TREND_PULLBACK due to extreme failure rate")
	}
}

func TestFeedback_QuarantinesTimingAnomaliesFromEvaluation(t *testing.T) {
	original := usecase.SnapshotRuntimeSettings()
	t.Cleanup(func() { usecase.SetRuntimeSettings(original) })
	settings := original
	settings.MonitoringMaxHoldMinutes = 120
	usecase.SetRuntimeSettings(settings)

	now := time.Now().UTC()
	repo := &mockFeedbackStorageRepo{
		journal: []usecase.SignalJournal{
			{
				ID:         "valid_tp2",
				Symbol:     "BTCUSDT",
				Playbook:   "TREND_PULLBACK",
				Status:     usecase.TP2_HIT,
				EntryPrice: 100,
				TP2:        102,
				CreatedAt:  now.Add(-90 * time.Minute),
				ExpiresAt:  now.Add(30 * time.Minute),
				ClosedAt:   now.Add(-30 * time.Minute),
				TimeToTP2:  "45m0s",
			},
			{
				ID:         "invalid_tp2",
				Symbol:     "ETHUSDT",
				Playbook:   "TREND_PULLBACK",
				Status:     usecase.TP2_HIT,
				EntryPrice: 100,
				TP2:        103,
				CreatedAt:  now.Add(-48 * time.Hour),
				ExpiresAt:  now.Add(-46 * time.Hour),
				ClosedAt:   now.Add(-6 * time.Hour),
				TimeToTP1:  "1h30m0s",
				TimeToTP2:  "40h0m0s",
			},
		},
		watch: []usecase.WatchJournal{
			{
				ID:         "valid_watch",
				Symbol:     "SOLUSDT",
				Playbook:   "RANGE_EDGE_REVERSAL",
				Status:     usecase.VIRTUAL_TP2_HIT,
				EntryPrice: 50,
				TP2:        49,
				CreatedAt:  now.Add(-90 * time.Minute),
				ExpiresAt:  now.Add(30 * time.Minute),
				ClosedAt:   now.Add(-20 * time.Minute),
				TimeToTP1:  "20m0s",
				TimeToTP2:  "35m0s",
			},
			{
				ID:         "invalid_watch",
				Symbol:     "DOGEUSDT",
				Playbook:   "RANGE_EDGE_REVERSAL",
				Status:     usecase.VIRTUAL_TP2_HIT,
				EntryPrice: 10,
				TP2:        9.5,
				CreatedAt:  now.Add(-48 * time.Hour),
				ExpiresAt:  now.Add(-46 * time.Hour),
				ClosedAt:   now.Add(-6 * time.Hour),
				TimeToTP1:  "50m0s",
				TimeToTP2:  "16h0m0s",
			},
		},
	}

	storage := usecase.NewStorageUsecase(repo)
	fb := usecase.NewFeedbackUsecase(storage)

	if err := fb.GenerateEvaluationReport(); err != nil {
		t.Fatalf("GenerateEvaluationReport: %v", err)
	}

	report := repo.report
	if report == nil {
		t.Fatal("expected evaluation report")
	}

	if report.TotalSignals != 1 {
		t.Fatalf("expected only 1 clean finalized signal, got %d", report.TotalSignals)
	}
	if report.Metrics["excluded_signal_anomaly_count"] != 1 {
		t.Fatalf("expected 1 excluded signal anomaly, got %v", report.Metrics["excluded_signal_anomaly_count"])
	}
	if report.Metrics["excluded_watch_anomaly_count"] != 1 {
		t.Fatalf("expected 1 excluded watch anomaly, got %v", report.Metrics["excluded_watch_anomaly_count"])
	}
	if report.Metrics["watch_finalized"] != 1 {
		t.Fatalf("expected only 1 clean finalized watch, got %v", report.Metrics["watch_finalized"])
	}
	if !strings.Contains(report.Notes, "Journal sanity quarantine excluded 1 signal rows and 1 watch rows") {
		t.Fatalf("expected quarantine note, got %q", report.Notes)
	}
}

// TestFeedback_LegacyPnlFormulaIsRecorrectedForTP2Hit verifies that a TP2_HIT record
// stored with an old PnL formula (e.g., full 100% position at TP2, not 50/50 split)
// gets recalculated to the correct formula during evaluation when the deviation
// exceeds pnlFormulaMismatchThreshold (0.1%).
func TestFeedback_LegacyPnlFormulaIsRecorrectedForTP2Hit(t *testing.T) {
	now := time.Now().UTC()
	// Entry=100, TP1=105, TP2=110 → correct 50/50 PnL = 2.5 + 5.0 = 7.5%
	// Legacy formula stored a different value (e.g., full 100% at TP2 = 10%)
	legacyPnl := 10.0
	expectedPnl := 7.5 // realizedTP2Pnl(100, 105, 110)

	repo := &mockFeedbackStorageRepo{
		journal: []usecase.SignalJournal{
			{
				ID:            "legacy_tp2_hit",
				Playbook:      usecase.TREND_PULLBACK,
				Direction:     usecase.LONG,
				Status:        usecase.TP2_HIT,
				EntryPrice:    100,
				TP1:           105,
				TP2:           110,
				StopLoss:      95,
				TimeToTP1:     "15m0s",
				TimeToTP2:     "45m0s",
				PnlPercentage: legacyPnl,
				ClosedAt:      now.Add(-10 * time.Minute),
				ExpiresAt:     now.Add(-5 * time.Minute),
			},
		},
	}
	storage := usecase.NewStorageUsecase(repo)
	fb := usecase.NewFeedbackUsecase(storage)

	if err := fb.GenerateEvaluationReport(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.report == nil {
		t.Fatal("expected report to be saved")
	}
	// total_pnl_percentage should use the corrected value, not the legacy one
	got := repo.report.Metrics["total_pnl_percentage"]
	if math.Abs(got-expectedPnl) > 0.001 {
		t.Fatalf("expected corrected pnl=%.4f, got %.4f (legacy formula not corrected)", expectedPnl, got)
	}
}

// TestFeedback_QuarantinesTP1HitClosedAfterExpiry verifies that a TP1_HIT record
// whose closed_at is well beyond expires_at (anomalous timing from a batch restart)
// is excluded from evaluation metrics.
func TestFeedback_QuarantinesTP1HitClosedAfterExpiry(t *testing.T) {
	now := time.Now().UTC()
	// expires_at is 3 hours ago, but closed_at is 1 hour ago — 2h anomaly
	expiresAt := now.Add(-3 * time.Hour)
	closedAt := now.Add(-1 * time.Hour) // 2h after expiry — clearly anomalous

	repo := &mockFeedbackStorageRepo{
		journal: []usecase.SignalJournal{
			{
				ID:            "tp1_closed_after_expiry",
				Playbook:      usecase.TREND_PULLBACK,
				Direction:     usecase.LONG,
				Status:        usecase.TP1_HIT,
				EntryPrice:    100,
				TP1:           105,
				TP2:           110,
				StopLoss:      95,
				TimeToTP1:     "15m0s",
				PnlPercentage: 2.5,
				ClosedAt:      closedAt,
				ExpiresAt:     expiresAt,
			},
			{
				ID:            "clean_tp2_hit",
				Playbook:      usecase.TREND_PULLBACK,
				Direction:     usecase.LONG,
				Status:        usecase.TP2_HIT,
				EntryPrice:    100,
				TP1:           105,
				TP2:           110,
				StopLoss:      95,
				TimeToTP1:     "15m0s",
				TimeToTP2:     "45m0s",
				PnlPercentage: 7.5,
				ClosedAt:      now.Add(-10 * time.Minute),
				ExpiresAt:     now.Add(-5 * time.Minute),
			},
		},
	}
	storage := usecase.NewStorageUsecase(repo)
	fb := usecase.NewFeedbackUsecase(storage)

	if err := fb.GenerateEvaluationReport(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.report == nil {
		t.Fatal("expected report to be saved")
	}

	// The anomalous TP1_HIT should be quarantined, only clean_tp2_hit remains
	if repo.report.TotalSignals != 1 {
		t.Fatalf("expected 1 finalized signal (anomalous TP1_HIT quarantined), got %d", repo.report.TotalSignals)
	}
	if repo.report.Metrics["excluded_signal_anomaly_count"] != 1 {
		t.Fatalf("expected 1 excluded anomaly, got %v", repo.report.Metrics["excluded_signal_anomaly_count"])
	}
}

func TestFeedback_SetupMemorySlicesIncludeLatestDecisionBrief(t *testing.T) {
	now := time.Now().UTC()
	repo := &mockFeedbackStorageRepo{
		journal: []usecase.SignalJournal{
			{
				ID:            "slice_tp2",
				Symbol:        "SOLUSDT",
				Playbook:      usecase.TREND_PULLBACK,
				Direction:     usecase.LONG,
				MarketRegime:  string(usecase.ALT_SUPPORTIVE),
				Tier:          usecase.TierA,
				Status:        usecase.TP2_HIT,
				EntryPrice:    100,
				TP1:           103,
				TP2:           106,
				StopLoss:      97,
				RR:            2.0,
				PnlPercentage: 4.5,
				CreatedAt:     now.Add(-2 * time.Hour),
				UpdatedAt:     now.Add(-90 * time.Minute),
				ClosedAt:      now.Add(-80 * time.Minute),
				ExpiresAt:     now.Add(-70 * time.Minute),
				TimeToTP1:     "15m0s",
				TimeToTP2:     "40m0s",
			},
		},
		audits: []usecase.DecisionAudit{
			{
				Symbol:        "SOLUSDT",
				Direction:     usecase.LONG,
				Playbook:      usecase.TREND_PULLBACK,
				MarketRegime:  string(usecase.ALT_SUPPORTIVE),
				GeneratedAt:   now.Add(-60 * time.Minute),
				FinalStatus:   usecase.FINAL_WATCH,
				FinalReason:   "Need retest",
				DecisionBrief: "FINAL_WATCH | TREND_PULLBACK | ai=WAIT/HIGH | reason=Need retest",
			},
		},
	}

	fb := usecase.NewFeedbackUsecase(usecase.NewStorageUsecase(repo))
	if err := fb.GenerateEvaluationReport(); err != nil {
		t.Fatalf("GenerateEvaluationReport: %v", err)
	}
	if repo.report == nil {
		t.Fatal("expected report")
	}
	if len(repo.report.SetupMemorySlices) != 1 {
		t.Fatalf("expected 1 setup memory slice, got %d", len(repo.report.SetupMemorySlices))
	}
	slice := repo.report.SetupMemorySlices[0]
	if slice.Symbol != "SOLUSDT" || slice.Playbook != string(usecase.TREND_PULLBACK) || slice.Direction != string(usecase.LONG) {
		t.Fatalf("unexpected slice identity: %+v", slice)
	}
	if slice.MarketRegime != string(usecase.ALT_SUPPORTIVE) {
		t.Fatalf("expected canonical regime %q, got %q", usecase.ALT_SUPPORTIVE, slice.MarketRegime)
	}
	if slice.LastStatus != string(usecase.TP2_HIT) {
		t.Fatalf("expected last status TP2_HIT from finalized signal, got %q", slice.LastStatus)
	}
	if slice.LastDecisionBrief == "" || !strings.Contains(slice.LastDecisionBrief, "Need retest") {
		t.Fatalf("expected last decision brief, got %q", slice.LastDecisionBrief)
	}

	frontend := usecase.NormalizeEvaluationForFrontend(repo.report)
	if len(frontend.SetupMemorySlices) != 1 {
		t.Fatalf("expected frontend setup memory slices, got %d", len(frontend.SetupMemorySlices))
	}
	if frontend.SetupMemorySlices[0].LastDecisionBrief != slice.LastDecisionBrief {
		t.Fatalf("expected frontend decision brief %q, got %q", slice.LastDecisionBrief, frontend.SetupMemorySlices[0].LastDecisionBrief)
	}
	if len(frontend.LearningReviews) == 0 {
		t.Fatalf("expected review-only learning rows, got 0")
	}
	if !frontend.LearningReviews[0].ReviewOnly || !frontend.LearningReviews[0].DoNotAutoApply {
		t.Fatalf("expected review-only learning semantics, got %+v", frontend.LearningReviews[0])
	}
}
