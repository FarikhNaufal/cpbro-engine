package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/config"
	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"cpbro-engine/internal/modules/cryptobroV3/entity"
	transhttp "cpbro-engine/internal/modules/cryptobroV3/transport/http"
	"cpbro-engine/internal/modules/cryptobroV3/usecase"
	"github.com/stretchr/testify/assert"
)

// Mock Storage repo
type mockAPIStorageRepo struct {
	latestResult *entity.LatestResult
	history      *entity.SignalHistory
	journal      []usecase.SignalJournal
	evalReport   *usecase.EvaluationReport
	audits       []usecase.DecisionAudit
}

func (m *mockAPIStorageRepo) LoadLatestResult() (*entity.LatestResult, error) {
	if m.latestResult == nil {
		return nil, errors.New("no latest result mock found")
	}
	return m.latestResult, nil
}
func (m *mockAPIStorageRepo) SaveLatestResult(res *entity.LatestResult) error {
	m.latestResult = res
	return nil
}
func (m *mockAPIStorageRepo) LoadSignalHistory() (*entity.SignalHistory, error) {
	return m.history, nil
}
func (m *mockAPIStorageRepo) SaveSignalHistory(hist *entity.SignalHistory) error {
	m.history = hist
	return nil
}
func (m *mockAPIStorageRepo) LoadSignalJournal() ([]usecase.SignalJournal, error) {
	return m.journal, nil
}
func (m *mockAPIStorageRepo) SaveSignalJournal(journal []usecase.SignalJournal) error {
	m.journal = journal
	return nil
}
func (m *mockAPIStorageRepo) AppendSignalJournal(entry usecase.SignalJournal) error {
	m.journal = append(m.journal, entry)
	return nil
}
func (m *mockAPIStorageRepo) LoadAIAuditCache() (*entity.AIAuditCache, error) {
	return nil, nil
}
func (m *mockAPIStorageRepo) SaveAIAuditCache(cache *entity.AIAuditCache) error {
	return nil
}
func (m *mockAPIStorageRepo) LoadEvaluationReport() (*usecase.EvaluationReport, error) {
	if m.evalReport == nil {
		return nil, errors.New("no report mock found")
	}
	return m.evalReport, nil
}
func (m *mockAPIStorageRepo) SaveEvaluationReport(report *usecase.EvaluationReport) error {
	m.evalReport = report
	return nil
}
func (m *mockAPIStorageRepo) LoadDecisionAudits() ([]usecase.DecisionAudit, error) {
	return m.audits, nil
}
func (m *mockAPIStorageRepo) SaveDecisionAudits(audits []usecase.DecisionAudit) error {
	m.audits = audits
	return nil
}
func (m *mockAPIStorageRepo) AppendDecisionAudit(entry usecase.DecisionAudit) error {
	m.audits = append(m.audits, entry)
	if len(m.audits) > 1000 {
		m.audits = m.audits[len(m.audits)-1000:]
	}
	return nil
}

// Mock services for dependency injection
type mockAPIMarketDataProvider struct{}

func (m *mockAPIMarketDataProvider) FetchClosedCandles(ctx context.Context, symbol string, interval string, limit int) ([]dto.Candle, error) {
	return []dto.Candle{{Time: time.Now(), Open: 100.0, High: 101.0, Low: 99.0, Close: 100.0}}, nil
}
func (m *mockAPIMarketDataProvider) FetchLatestPrice(ctx context.Context, symbol string) (float64, error) {
	return 100.0, nil
}
func (m *mockAPIMarketDataProvider) FetchAllFuturesTickers24h(ctx context.Context) ([]dto.Ticker24h, error) {
	return []dto.Ticker24h{{Symbol: "SOLUSDT", LastPrice: 100.0, QuoteVolume: 100000.0}}, nil
}
func (m *mockAPIMarketDataProvider) FetchPremiumFundingRates(ctx context.Context) (map[string]float64, error) {
	return map[string]float64{"SOLUSDT": 0.0001}, nil
}
func (m *mockAPIMarketDataProvider) FetchOpenInterest(ctx context.Context, symbol string) (float64, error) {
	return 1000.0, nil
}
func (m *mockAPIMarketDataProvider) FetchHistoricalCandles(ctx context.Context, symbol string, interval string, startTime time.Time, endTime time.Time) ([]dto.Candle, error) {
	return []dto.Candle{{Time: startTime, Open: 100.0, High: 101.0, Low: 99.0, Close: 100.0}}, nil
}

type mockAPITestAIAuditor struct{}

func (m *mockAPITestAIAuditor) AuditCandidate(ctx context.Context, req dto.AIAuditRequest) (*dto.AIAuditResponse, error) {
	return &dto.AIAuditResponse{Symbol: req.Symbol, IsApproved: true, Decision: "CONFIRM"}, nil
}

type mockAPINotification struct{}

func (m *mockAPINotification) SendSignalMessage(ctx context.Context, msg string) error {
	return nil
}
func (m *mockAPINotification) SendOpsMessage(ctx context.Context, msg string) error {
	return nil
}

func TestAPIRoutes(t *testing.T) {
	// Initialize Mock Repo
	mockRepo := &mockAPIStorageRepo{
		latestResult: &entity.LatestResult{
			ScanID: "20260520120000",
		},
		journal: []usecase.SignalJournal{
			{Symbol: "SOLUSDT", Playbook: usecase.TREND_PULLBACK, Status: usecase.MONITORING},
		},
		evalReport: &usecase.EvaluationReport{
			GeneratedAt:  time.Now(),
			TotalSignals: 1,
			Metrics: map[string]float64{
				"winrate": 1.0,
			},
		},
		audits: []usecase.DecisionAudit{
			{Symbol: "SOLUSDT", FinalStatus: usecase.FINAL_EXECUTE},
		},
	}

	storageUC := usecase.NewStorageUsecase(mockRepo)
	marketDataUC := usecase.NewMarketDataUsecase(&mockAPIMarketDataProvider{})
	marketPolicyUC := usecase.NewMarketPolicyUsecase()
	universeUC := usecase.NewUniverseUsecase()
	strategySelectorUC := usecase.NewStrategySelectorUsecase()
	playbookEligibilityUC := usecase.NewPlaybookEligibilityUsecase()
	playbookQuantEngineUC := usecase.NewPlaybookQuantEngineUsecase()
	scoringUC := usecase.NewScoringUsecase()
	candidateArbiterUC := usecase.NewCandidateArbiterUsecase()
	localGateUC := usecase.NewLocalGateUsecase()
	aiCandidateSelectorUC := usecase.NewAICandidateSelectorUsecase(60.0)
	aiAuditorUC := usecase.NewAIAuditorUsecase(&mockAPITestAIAuditor{}, storageUC)
	planReconciliationUC := usecase.NewPlanReconciliationUsecase()
	stalenessUC := usecase.NewStalenessUsecase(30 * time.Minute)
	finalGateUC := usecase.NewFinalGateUsecase()
	conflictResolverUC := usecase.NewConflictResolverUsecase()
	signalNotificationUC := usecase.NewSignalNotificationUsecase(&mockAPINotification{}, storageUC)
	opsNotificationUC := usecase.NewOpsNotificationUsecase(&mockAPINotification{})
	monitoringUC := usecase.NewMonitoringUsecase(&mockAPIMarketDataProvider{}, storageUC)
	feedbackUC := usecase.NewFeedbackUsecase(storageUC)

	scannerUC := usecase.NewScannerUsecase(
		marketDataUC,
		marketPolicyUC,
		universeUC,
		strategySelectorUC,
		playbookEligibilityUC,
		playbookQuantEngineUC,
		scoringUC,
		candidateArbiterUC,
		localGateUC,
		aiCandidateSelectorUC,
		aiAuditorUC,
		planReconciliationUC,
		stalenessUC,
		finalGateUC,
		conflictResolverUC,
		signalNotificationUC,
		opsNotificationUC,
		monitoringUC,
		feedbackUC,
		storageUC,
	)

	backtestUC := usecase.NewBacktestEngineUsecase(
		&mockAPIMarketDataProvider{},
		marketPolicyUC,
		universeUC,
		strategySelectorUC,
		playbookEligibilityUC,
		playbookQuantEngineUC,
		scoringUC,
		candidateArbiterUC,
		localGateUC,
		aiCandidateSelectorUC,
		aiAuditorUC,
		planReconciliationUC,
		stalenessUC,
		finalGateUC,
		conflictResolverUC,
		storageUC,
		".",
	)

	observabilityUC := usecase.NewObservabilityUsecase(&mockAPIMarketDataProvider{}, &mockAPITestAIAuditor{}, &mockAPINotification{}, ".")
	cfg, _ := config.LoadConfigFromEnv()
	var testScannerRunning atomic.Bool
	handler := transhttp.NewHandler(scannerUC, feedbackUC, storageUC, backtestUC, observabilityUC, ".", &testScannerRunning)
	router := transhttp.SetupRouter(cfg, handler)

	t.Run("GET /health", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/health", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"status":"UP"`)
		assert.Contains(t, w.Body.String(), `"mode":"alert-only"`)
		// Check that secrets are not exposed in health response
		assert.NotContains(t, w.Body.String(), "key")
		assert.NotContains(t, w.Body.String(), "token")
	})

	t.Run("Forbidden Routes return 404", func(t *testing.T) {
		forbiddenPaths := []string{
			"/api/v3/order",
			"/api/v3/execute",
			"/api/v3/close",
			"/api/v3/leverage",
			"/api/v3/margin",
			"/api/v3/apply-recommendation",
			"/api/v3/auto-tune",
			"/api/v3/auto-apply",
			"/api/v3/binance/order",
		}

		for _, path := range forbiddenPaths {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("POST", path, nil)
			router.ServeHTTP(w, req)
			assert.Equal(t, http.StatusNotFound, w.Code, "Path %s must be not registered", path)
		}
	})

	t.Run("GET /api/v3/latest", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v3/latest", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"success":true`)
		assert.Contains(t, w.Body.String(), `"scan_id":"20260520120000"`)
	})

	t.Run("GET /api/v3/journal", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v3/journal", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"success":true`)
		assert.Contains(t, w.Body.String(), `"symbol":"SOLUSDT"`)
	})

	t.Run("GET /api/v3/evaluation", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v3/evaluation", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"success":true`)
		assert.Contains(t, w.Body.String(), `"total_signals":1`)
	})

	t.Run("GET /api/v3/decision-audit", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v3/decision-audit", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"success":true`)
		assert.Contains(t, w.Body.String(), `"SOLUSDT"`)
	})

	t.Run("GET /api/v3/backtest/reports", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v3/backtest/reports", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"success":true`)
	})

	t.Run("GET /api/v3/health alias", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v3/health", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"status":"UP"`)
	})
}

func TestSwaggerRouteEnabled(t *testing.T) {
	// Swagger should be mounted when SwaggerEnabled=true
	t.Setenv("SWAGGER_ENABLED", "true")
	t.Setenv("SWAGGER_HOST", "localhost:9999")
	t.Setenv("SWAGGER_BASE_PATH", "/api/v3")

	cfg, _ := config.LoadConfigFromEnv()
	assert.True(t, cfg.Route.SwaggerEnabled)

	mockRepo := &mockAPIStorageRepo{}
	storageUC := usecase.NewStorageUsecase(mockRepo)
	feedbackUC := usecase.NewFeedbackUsecase(storageUC)
	observabilityUC := usecase.NewObservabilityUsecase(&mockAPIMarketDataProvider{}, &mockAPITestAIAuditor{}, &mockAPINotification{}, ".")

	backtestUC := usecase.NewBacktestEngineUsecase(
		&mockAPIMarketDataProvider{},
		usecase.NewMarketPolicyUsecase(),
		usecase.NewUniverseUsecase(),
		usecase.NewStrategySelectorUsecase(),
		usecase.NewPlaybookEligibilityUsecase(),
		usecase.NewPlaybookQuantEngineUsecase(),
		usecase.NewScoringUsecase(),
		usecase.NewCandidateArbiterUsecase(),
		usecase.NewLocalGateUsecase(),
		usecase.NewAICandidateSelectorUsecase(60.0),
		usecase.NewAIAuditorUsecase(&mockAPITestAIAuditor{}, storageUC),
		usecase.NewPlanReconciliationUsecase(),
		usecase.NewStalenessUsecase(30*time.Minute),
		usecase.NewFinalGateUsecase(),
		usecase.NewConflictResolverUsecase(),
		storageUC,
		".",
	)

	scannerUC := usecase.NewScannerUsecase(
		usecase.NewMarketDataUsecase(&mockAPIMarketDataProvider{}),
		usecase.NewMarketPolicyUsecase(),
		usecase.NewUniverseUsecase(),
		usecase.NewStrategySelectorUsecase(),
		usecase.NewPlaybookEligibilityUsecase(),
		usecase.NewPlaybookQuantEngineUsecase(),
		usecase.NewScoringUsecase(),
		usecase.NewCandidateArbiterUsecase(),
		usecase.NewLocalGateUsecase(),
		usecase.NewAICandidateSelectorUsecase(60.0),
		usecase.NewAIAuditorUsecase(&mockAPITestAIAuditor{}, storageUC),
		usecase.NewPlanReconciliationUsecase(),
		usecase.NewStalenessUsecase(30*time.Minute),
		usecase.NewFinalGateUsecase(),
		usecase.NewConflictResolverUsecase(),
		usecase.NewSignalNotificationUsecase(&mockAPINotification{}, storageUC),
		usecase.NewOpsNotificationUsecase(&mockAPINotification{}),
		usecase.NewMonitoringUsecase(&mockAPIMarketDataProvider{}, storageUC),
		feedbackUC,
		storageUC,
	)

	var testRunning atomic.Bool
	handler := transhttp.NewHandler(scannerUC, feedbackUC, storageUC, backtestUC, observabilityUC, ".", &testRunning)
	router := transhttp.SetupRouter(cfg, handler)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/swagger/doc.json", nil)
	router.ServeHTTP(w, req)

	// ginSwagger serves the OpenAPI spec at /swagger/doc.json
	assert.Equal(t, http.StatusOK, w.Code,
		"expected swagger doc.json to be served when enabled, got %d", w.Code)
	assert.Contains(t, w.Body.String(), "cryptobroV3 API",
		"swagger doc.json must contain the API title")
}

func TestSwaggerRouteDisabled(t *testing.T) {
	// Swagger should NOT be mounted when SwaggerEnabled=false
	t.Setenv("SWAGGER_ENABLED", "false")

	cfg, _ := config.LoadConfigFromEnv()
	assert.False(t, cfg.Route.SwaggerEnabled)

	mockRepo := &mockAPIStorageRepo{}
	storageUC := usecase.NewStorageUsecase(mockRepo)
	feedbackUC := usecase.NewFeedbackUsecase(storageUC)
	observabilityUC := usecase.NewObservabilityUsecase(&mockAPIMarketDataProvider{}, &mockAPITestAIAuditor{}, &mockAPINotification{}, ".")

	backtestUC := usecase.NewBacktestEngineUsecase(
		&mockAPIMarketDataProvider{},
		usecase.NewMarketPolicyUsecase(),
		usecase.NewUniverseUsecase(),
		usecase.NewStrategySelectorUsecase(),
		usecase.NewPlaybookEligibilityUsecase(),
		usecase.NewPlaybookQuantEngineUsecase(),
		usecase.NewScoringUsecase(),
		usecase.NewCandidateArbiterUsecase(),
		usecase.NewLocalGateUsecase(),
		usecase.NewAICandidateSelectorUsecase(60.0),
		usecase.NewAIAuditorUsecase(&mockAPITestAIAuditor{}, storageUC),
		usecase.NewPlanReconciliationUsecase(),
		usecase.NewStalenessUsecase(30*time.Minute),
		usecase.NewFinalGateUsecase(),
		usecase.NewConflictResolverUsecase(),
		storageUC,
		".",
	)

	scannerUC := usecase.NewScannerUsecase(
		usecase.NewMarketDataUsecase(&mockAPIMarketDataProvider{}),
		usecase.NewMarketPolicyUsecase(),
		usecase.NewUniverseUsecase(),
		usecase.NewStrategySelectorUsecase(),
		usecase.NewPlaybookEligibilityUsecase(),
		usecase.NewPlaybookQuantEngineUsecase(),
		usecase.NewScoringUsecase(),
		usecase.NewCandidateArbiterUsecase(),
		usecase.NewLocalGateUsecase(),
		usecase.NewAICandidateSelectorUsecase(60.0),
		usecase.NewAIAuditorUsecase(&mockAPITestAIAuditor{}, storageUC),
		usecase.NewPlanReconciliationUsecase(),
		usecase.NewStalenessUsecase(30*time.Minute),
		usecase.NewFinalGateUsecase(),
		usecase.NewConflictResolverUsecase(),
		usecase.NewSignalNotificationUsecase(&mockAPINotification{}, storageUC),
		usecase.NewOpsNotificationUsecase(&mockAPINotification{}),
		usecase.NewMonitoringUsecase(&mockAPIMarketDataProvider{}, storageUC),
		feedbackUC,
		storageUC,
	)

	var testRunning atomic.Bool
	handler := transhttp.NewHandler(scannerUC, feedbackUC, storageUC, backtestUC, observabilityUC, ".", &testRunning)
	router := transhttp.SetupRouter(cfg, handler)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/swagger/index.html", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code,
		"swagger route should return 404 when disabled")
}

func TestSwaggerJSONSafety(t *testing.T) {
	// Programmatic check: swagger.json must not contain forbidden paths or secrets
	data, err := os.ReadFile("../../docs/swagger.json")
	if err != nil {
		t.Skip("swagger.json not generated yet, skipping safety check")
	}

	var doc map[string]any
	err = json.Unmarshal(data, &doc)
	assert.NoError(t, err, "swagger.json should be valid JSON")

	// Check no forbidden route paths
	paths, ok := doc["paths"].(map[string]any)
	if ok {
		forbiddenRoutes := []string{
			"/order", "/execute", "/close", "/leverage", "/margin",
			"/auto-tune", "/apply-recommendation", "/auto-apply",
			"/binance/order",
		}
		for _, fr := range forbiddenRoutes {
			_, exists := paths[fr]
			assert.False(t, exists, "forbidden route %s must not exist in swagger.json", fr)
		}
	}

	// Check no secrets in the raw JSON content
	content := string(data)
	secretKeywords := []string{"API_KEY", "API_SECRET", "BOT_TOKEN", "CHAT_ID", "GEMINI_KEY", "BINANCE_SECRET"}
	for _, kw := range secretKeywords {
		assert.NotContains(t, content, kw, "swagger.json must not contain secret keyword: %s", kw)
	}
}
