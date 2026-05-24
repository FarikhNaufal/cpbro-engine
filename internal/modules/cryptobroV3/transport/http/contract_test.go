package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"cpbro-engine/internal/modules/cryptobroV3/entity"
	"cpbro-engine/internal/modules/cryptobroV3/usecase"
	"github.com/gin-gonic/gin"
)

type contractStorageRepo struct {
	latest  *entity.LatestResult
	journal []usecase.SignalJournal
	audits  []usecase.DecisionAudit
}

func (m *contractStorageRepo) LoadLatestResult() (*entity.LatestResult, error) { return m.latest, nil }
func (m *contractStorageRepo) SaveLatestResult(res *entity.LatestResult) error {
	m.latest = res
	return nil
}
func (m *contractStorageRepo) LoadSignalHistory() (*entity.SignalHistory, error) {
	return &entity.SignalHistory{}, nil
}
func (m *contractStorageRepo) SaveSignalHistory(hist *entity.SignalHistory) error { return nil }
func (m *contractStorageRepo) LoadSignalJournal() ([]usecase.SignalJournal, error) {
	return m.journal, nil
}
func (m *contractStorageRepo) SaveSignalJournal(j []usecase.SignalJournal) error {
	m.journal = j
	return nil
}
func (m *contractStorageRepo) AppendSignalJournal(entry usecase.SignalJournal) error {
	m.journal = append(m.journal, entry)
	return nil
}
func (m *contractStorageRepo) LoadAIAuditCache() (*entity.AIAuditCache, error) {
	return &entity.AIAuditCache{CacheMap: map[string]entity.CachedAudit{}}, nil
}
func (m *contractStorageRepo) SaveAIAuditCache(cache *entity.AIAuditCache) error { return nil }
func (m *contractStorageRepo) LoadEvaluationReport() (*usecase.EvaluationReport, error) {
	return &usecase.EvaluationReport{}, nil
}
func (m *contractStorageRepo) SaveEvaluationReport(report *usecase.EvaluationReport) error {
	return nil
}
func (m *contractStorageRepo) LoadDecisionAudits() ([]usecase.DecisionAudit, error) {
	return m.audits, nil
}
func (m *contractStorageRepo) SaveDecisionAudits(a []usecase.DecisionAudit) error {
	m.audits = a
	return nil
}
func (m *contractStorageRepo) AppendDecisionAudit(entry usecase.DecisionAudit) error {
	m.audits = append(m.audits, entry)
	return nil
}

func TestAPIResponse_Contract_WrapperAlwaysPresent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &contractStorageRepo{}
	storageUC := usecase.NewStorageUsecase(repo)

	// Minimal handler: only validate /latest wrapper output and empty states.
	h := &Handler{
		storageUC: storageUC,
	}
	r := gin.New()
	r.GET("/latest", h.GetLatest)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/latest", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp APIResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Success != true {
		t.Fatalf("expected success=true, got %v", resp.Success)
	}
	if resp.Data == nil {
		t.Fatalf("expected data object, got null")
	}
	if resp.Errors == nil {
		t.Fatalf("expected errors array, got null")
	}
	_ = dto.Candle{}
}

func TestAPIResponse_Contract_Constraints(t *testing.T) {
	// Test 1 & 2: Empty LatestResult / nil arrays normalized to [] and numbers 0
	t.Run("Empty LatestResult normalization has correct arrays and numbers", func(t *testing.T) {
		res := usecase.NormalizeLatestResultForFrontend(nil)

		if res.TotalTickers != 0 || res.TotalUniversePass != 0 || res.TotalFinalExecute != 0 {
			t.Errorf("expected numeric fields to be 0, got TotalTickers=%d", res.TotalTickers)
		}

		if res.ExecuteSignals == nil || len(res.ExecuteSignals) != 0 {
			t.Errorf("expected ExecuteSignals to be empty slice [], got nil or length > 0")
		}

		if res.Watchlist == nil || len(res.Watchlist) != 0 {
			t.Errorf("expected Watchlist to be empty slice [], got nil or length > 0")
		}

		if res.Signals == nil || len(res.Signals) != 0 {
			t.Errorf("expected Signals to be empty slice [], got nil or length > 0")
		}

		if res.RejectedSummary == nil || len(res.RejectedSummary) != 0 {
			t.Errorf("expected RejectedSummary to be empty slice [], got nil or length > 0")
		}

		if res.PolicyRejectedSummary == nil || len(res.PolicyRejectedSummary) != 0 {
			t.Errorf("expected PolicyRejectedSummary to be empty slice [], got nil or length > 0")
		}
	})

	// Test 3: threshold_profile_summary nil menjadi {}
	t.Run("threshold_profile_summary nil becomes empty map", func(t *testing.T) {
		raw := &entity.LatestResult{
			SelectedThresholdProfileSummary: nil,
		}
		res := usecase.NormalizeLatestResultForFrontend(raw)

		if res.ThresholdProfileSummary == nil {
			t.Errorf("expected ThresholdProfileSummary to be empty map, got nil")
		}
		if len(res.ThresholdProfileSummary) != 0 {
			t.Errorf("expected ThresholdProfileSummary to have 0 items, got %d", len(res.ThresholdProfileSummary))
		}
	})

	// Test 4: arbiter_selected_details nil menjadi []
	t.Run("arbiter_selected_details nil becomes empty slice", func(t *testing.T) {
		raw := &entity.LatestResult{
			ArbiterSelectedDetails: nil,
		}
		res := usecase.NormalizeLatestResultForFrontend(raw)

		if res.ArbiterSelectedDetails == nil {
			t.Errorf("expected ArbiterSelectedDetails to be empty slice, got nil")
		}
		if len(res.ArbiterSelectedDetails) != 0 {
			t.Errorf("expected ArbiterSelectedDetails to have 0 items, got %d", len(res.ArbiterSelectedDetails))
		}
	})

	// Test 5: APIResponse error punya errors [] bukan null
	t.Run("APIResponse error has errors array and is not null", func(t *testing.T) {
		resp := fail("some error message", "error detail 1")
		if resp.Errors == nil {
			t.Errorf("expected errors array, got nil")
		}
		if len(resp.Errors) != 1 || resp.Errors[0] != "error detail 1" {
			t.Errorf("expected 1 error detail, got %v", resp.Errors)
		}

		respEmptyErr := fail("another error")
		if respEmptyErr.Errors == nil {
			t.Errorf("expected errors array, got nil")
		}
		if len(respEmptyErr.Errors) != 0 {
			t.Errorf("expected empty errors array, got %v", respEmptyErr.Errors)
		}
	})
}
