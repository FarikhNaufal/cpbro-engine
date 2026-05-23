package http

import (
	"cpbro-engine/internal/modules/cryptobroV3/config"
	"log/slog"

	docs "cpbro-engine/docs"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// SetupRouter creates and configures the Gin engine, registers middleware and all permitted routes
func SetupRouter(cfg *config.Config, h *Handler) *gin.Engine {
	// Set Gin mode
	if cfg.App.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	r := gin.New()

	// Register SRE Middlewares
	r.Use(RecoveryMiddleware())
	r.Use(LoggerMiddleware())

	// Public Health Endpoint
	r.GET("/health", h.GetHealth)

	// API v3 Route Group
	apiPrefix := cfg.Route.APIPrefix
	if apiPrefix == "" {
		apiPrefix = "/api/v3"
	}

	v3 := r.Group(apiPrefix)
	{
		// Health alias inside BasePath for Swagger doc alignment
		v3.GET("/health", h.GetHealth)

		v3.GET("/latest", h.GetLatest)
		v3.POST("/run", h.PostRun)
		v3.GET("/journal", h.GetJournal)
		v3.GET("/evaluation", h.GetEvaluation)

		// Optional Safe Endpoints
		if cfg.Route.EnableEvaluationRunEndpoint {
			v3.POST("/evaluation/run", h.PostEvaluationRun)
		}
		if cfg.Route.EnableDecisionAuditEndpoint {
			v3.GET("/decision-audit", h.GetDecisionAudit)
		}

		// Backtest Routes
		v3.POST("/backtest/run", h.PostBacktestRun)
		v3.GET("/backtest/reports", h.GetBacktestReports)
		v3.GET("/backtest/reports/:run_id", h.GetBacktestReportByID)
	}

	// Swagger Documentation Route (conditionally enabled)
	if cfg.Route.SwaggerEnabled {
		docs.SwaggerInfo.Title = "cryptobroV3 API"
		docs.SwaggerInfo.Description = "Alert-only crypto scanner API. No Binance order execution."
		docs.SwaggerInfo.Version = cfg.App.Version
		docs.SwaggerInfo.Host = cfg.Route.SwaggerHost
		docs.SwaggerInfo.BasePath = cfg.Route.SwaggerBasePath
		docs.SwaggerInfo.Schemes = []string{"http"}

		r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.NewHandler()))
		slog.Info("Swagger UI enabled", "url", "http://"+cfg.Route.SwaggerHost+"/swagger/index.html")
	}

	return r
}
