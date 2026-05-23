package usecase_test

import (
	"strings"
	"testing"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"cpbro-engine/internal/modules/cryptobroV3/entity"
	"cpbro-engine/internal/modules/cryptobroV3/usecase"
)

type mockFeedbackStorageRepo struct {
	journal   []usecase.SignalJournal
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

	if report.DataCompleteness.HasSignalJournal || report.DataCompleteness.HasDecisionAudit {
		t.Error("Expected completeness flags to be false")
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
			if !strings.Contains(rec.SuggestedAction, "AllowBreakoutCandleEntry to false") {
				t.Errorf("Expected retest action suggestion, got %s", rec.SuggestedAction)
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
		if rec.MarketRegime == "LOW_VOLATILITY" && rec.IssueType == "TARGET_TUNING" {
			foundTuning = true
			if !strings.Contains(rec.SuggestedAction, "Lower take-profit targets") {
				t.Errorf("Expected lower TP targets action, got %s", rec.SuggestedAction)
			}
		}
	}
	if !foundTuning {
		t.Error("Expected TARGET_TUNING recommendation for LOW_VOLATILITY")
	}
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
