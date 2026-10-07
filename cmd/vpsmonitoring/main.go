package main

import (
	"context"
	"log"
	"strconv"
	"time"

	"gofr.dev/pkg/gofr"

	agentController "vpsmonitoring-backend/internal/agent/controller"
	agentMiddleware "vpsmonitoring-backend/internal/agent/middleware"
	agentRepository "vpsmonitoring-backend/internal/agent/repository"
	agentService "vpsmonitoring-backend/internal/agent/service"
	agentWorker "vpsmonitoring-backend/internal/agent/worker"
	alertController "vpsmonitoring-backend/internal/alert/controller"
	alertEvaluator "vpsmonitoring-backend/internal/alert/evaluator"
	alertRepository "vpsmonitoring-backend/internal/alert/repository"
	alertService "vpsmonitoring-backend/internal/alert/service"
	appController "vpsmonitoring-backend/internal/application/controller"
	appModels "vpsmonitoring-backend/internal/application/models"
	appRepository "vpsmonitoring-backend/internal/application/repository"
	appService "vpsmonitoring-backend/internal/application/service"
	"vpsmonitoring-backend/internal/auth/controller"
	"vpsmonitoring-backend/internal/auth/middleware"
	"vpsmonitoring-backend/internal/auth/repository"
	"vpsmonitoring-backend/internal/auth/service"
	"vpsmonitoring-backend/internal/config"
	discoveryController "vpsmonitoring-backend/internal/discovery/controller"
	discoveryService "vpsmonitoring-backend/internal/discovery/service"
	healthController "vpsmonitoring-backend/internal/health/controller"
	healthRepository "vpsmonitoring-backend/internal/health/repository"
	healthService "vpsmonitoring-backend/internal/health/service"
	tokenController "vpsmonitoring-backend/internal/installation_token/controller"
	tokenRepository "vpsmonitoring-backend/internal/installation_token/repository"
	tokenService "vpsmonitoring-backend/internal/installation_token/service"
	metricController "vpsmonitoring-backend/internal/metric/controller"
	metricRepository "vpsmonitoring-backend/internal/metric/repository"
	metricService "vpsmonitoring-backend/internal/metric/service"
	processCache "vpsmonitoring-backend/internal/process/cache"
	processController "vpsmonitoring-backend/internal/process/controller"
	processService "vpsmonitoring-backend/internal/process/service"
	serverController "vpsmonitoring-backend/internal/server/controller"
	serverRepository "vpsmonitoring-backend/internal/server/repository"
	serverService "vpsmonitoring-backend/internal/server/service"
	systemdCache "vpsmonitoring-backend/internal/systemd/cache"
	systemdController "vpsmonitoring-backend/internal/systemd/controller"
	systemdService "vpsmonitoring-backend/internal/systemd/service"
)

type DatabaseHealth struct {
	Status     string `json:"status"`
	Connected  bool   `json:"connected"`
	Provider   string `json:"provider"`
	Database   string `json:"database"`
	ORM        string `json:"orm"`
	LatencyMs  int64  `json:"latency_ms"`
	ServerTime string `json:"server_time,omitempty"`
	Version    string `json:"version,omitempty"`
	Error      string `json:"error,omitempty"`
}

type HealthResponse struct {
	Status    string         `json:"status"`
	Service   string         `json:"service"`
	ORM       string         `json:"orm"`
	Timestamp string         `json:"timestamp"`
	Database  DatabaseHealth `json:"database"`
}

func main() {
	// 1. Initialize GoFr application
	app := gofr.New()

	// 2. Load and validate configurations (fail-fast in production if secrets are missing)
	cfg, err := config.LoadConfig(app.Config)
	if err != nil {
		log.Fatalf("[FATAL] Configuration error: %v", err)
	}

	log.Printf("[INFO] runtime configuration loaded for environment: %s", cfg.Env)

	// 3. Initialize GORM with PostgreSQL and run AutoMigrate (managed within auth repository layer)
	gormDB, err := repository.InitGORM(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("[FATAL] GORM initialization failed: %v", err)
	}

	// 4. Initialize Auth Layered Architecture (Repository -> Service -> Controller)
	userRepo := repository.NewUserRepository(gormDB)
	authService := service.NewAuthService(userRepo, cfg.JWTSecret)
	authController := controller.NewAuthController(authService)

	// 5. Initialize Server Layered Architecture (Phase 2A)
	serverRepo := serverRepository.NewServerRepository(gormDB)
	if err := serverRepo.AutoMigrate(); err != nil {
		log.Fatalf("[FATAL] Server AutoMigrate failed: %v", err)
	}
	serverServ := serverService.NewServerService(serverRepo)
	serverCtrl := serverController.NewServerController(serverServ)

	// 6. Initialize Installation Token Layered Architecture (Phase 2B)
	tokenRepo := tokenRepository.NewInstallationTokenRepository(gormDB)
	if err := tokenRepo.AutoMigrate(); err != nil {
		log.Fatalf("[FATAL] Installation Token AutoMigrate failed: %v", err)
	}
	tokenServ := tokenService.NewInstallationTokenService(tokenRepo, serverRepo, "https://get.vpspulse.dev/agent.sh")
	tokenCtrl := tokenController.NewInstallationTokenController(tokenServ)

	// 7. Initialize Agent Layered Architecture (Phase 2C.1 & 2C.3)
	agentRepo := agentRepository.NewAgentRepository(gormDB)
	if err := agentRepo.AutoMigrate(); err != nil {
		log.Fatalf("[FATAL] Agent AutoMigrate failed: %v", err)
	}
	agentServ := agentService.NewAgentService(agentRepo, tokenServ)
	agentCtrl := agentController.NewAgentController(agentServ)

	// 8. Initialize Metric Layered Architecture (Phase 2C.7)
	metricRepo := metricRepository.NewMetricRepository(gormDB)
	if err := metricRepo.AutoMigrate(); err != nil {
		log.Fatalf("[FATAL] Metric AutoMigrate failed: %v", err)
	}
	metricServ := metricService.NewMetricService(metricRepo, serverRepo)
	metricCtrl := metricController.NewMetricController(metricServ)

	// 8b. Initialize Process Monitoring Architecture (Phase 3.4B.2 & 3.4.11)
	// Process snapshots are stored exclusively in memory (no DB).
	procCache := processCache.NewProcessCache()
	procServ := processService.NewProcessService(procCache, serverServ)
	procCtrl := processController.NewProcessController(procServ)

	// 8c. Initialize Systemd Architecture (Phase 3.5A)
	sysCache := systemdCache.NewSystemdCache()
	sysServ := systemdService.NewSystemdService(sysCache, serverServ)
	sysCtrl := systemdController.NewSystemdController(sysServ)

	// 8d. Initialize Health Check Architecture (Phase 3.5B.1)
	healthRepo := healthRepository.NewHealthRepository(gormDB)
	if err := healthRepo.AutoMigrate(); err != nil {
		log.Fatalf("[FATAL] Health AutoMigrate failed: %v", err)
	}
	healthCache := healthService.NewHealthResultCache()
	healthServ := healthService.NewHealthService(healthRepo, serverServ, healthCache)
	healthCtrl := healthController.NewHealthController(healthServ)

	// 8e. Initialize Discovery Architecture (Phase 3.5C.6)
	discoveryCache := discoveryService.NewDiscoveryCache()
	discoveryServ := discoveryService.NewDiscoveryService(discoveryCache, serverServ)
	discoveryCtrl := discoveryController.NewDiscoveryController(discoveryServ)

	// 8f. Initialize Application Architecture (Phase 3.6.2)
	if err := gormDB.AutoMigrate(
		&appModels.Application{},
		&appModels.ApplicationMetric{},
		&appModels.ApplicationEvent{},
	); err != nil {
		log.Fatalf("[FATAL] Application AutoMigrate failed: %v", err)
	}
	appRepo := appRepository.NewApplicationRepository(gormDB)
	appServ := appService.NewApplicationService(appRepo, serverServ, gormDB)
	appCtrl := appController.NewApplicationController(appServ)

	telemetryRepo := appRepository.NewTelemetryRepository(gormDB)
	telemetryServ := appService.NewTelemetryService(telemetryRepo, appRepo)
	telemetryCtrl := appController.NewTelemetryController(telemetryServ)

	// Phase 3.6.14: Application Telemetry Query API (read-only, user JWT)
	telemetryQueryRepo := appRepository.NewTelemetryQueryRepository(gormDB)
	telemetryQueryServ := appService.NewTelemetryQueryService(telemetryQueryRepo, appRepo, serverServ)
	telemetryQueryCtrl := appController.NewTelemetryQueryController(telemetryQueryServ)

	// Phase 4.1: Alert Rule foundation (configuration CRUD only; no evaluation).
	alertRuleRepo := alertRepository.NewAlertRuleRepository(gormDB)
	if err := alertRuleRepo.AutoMigrate(); err != nil {
		log.Fatalf("[FATAL] Alert Rule AutoMigrate failed: %v", err)
	}
	alertRuleServ := alertService.NewAlertRuleService(alertRuleRepo, serverServ, appRepo)
	alertRuleCtrl := alertController.NewAlertRuleController(alertRuleServ)
	alertRepo := alertRepository.NewAlertRepository(gormDB)
	if err := alertRepo.AutoMigrate(); err != nil {
		log.Fatalf("[FATAL] Alert AutoMigrate failed: %v", err)
	}
	alertPersistence := alertService.NewAlertPersistenceService(alertRepo)
	alertQueryServ := alertService.NewAlertQueryService(alertRepo, serverServ, appRepo)
	alertCtrl := alertController.NewAlertController(alertQueryServ)
	alertIntervalSec, _ := strconv.Atoi(app.Config.GetOrDefault("ALERT_EVALUATION_INTERVAL_SECONDS", "30"))
	if alertIntervalSec < 5 || alertIntervalSec > 300 {
		alertIntervalSec = 30
	}
	alertEval := alertEvaluator.New(alertRuleRepo, alertEvaluator.NewGORMSnapshotProvider(gormDB), time.Duration(alertIntervalSec)*time.Second)
	alertEval.SetResultSink(alertPersistence)

	// 9. Initialize and start Offline Detection Worker (Phase 2C.4)
	offlineThresholdSec, _ := strconv.Atoi(app.Config.GetOrDefault("AGENT_OFFLINE_THRESHOLD_SECONDS", "90"))
	workerIntervalSec, _ := strconv.Atoi(app.Config.GetOrDefault("AGENT_WORKER_INTERVAL_SECONDS", "30"))
	if offlineThresholdSec <= 0 {
		offlineThresholdSec = 90
	}
	if workerIntervalSec <= 0 {
		workerIntervalSec = 30
	}

	offlineWorker := agentWorker.NewOfflineWorker(agentServ, &agentWorker.OfflineWorkerConfig{
		CheckInterval:    time.Duration(workerIntervalSec) * time.Second,
		OfflineThreshold: time.Duration(offlineThresholdSec) * time.Second,
	})
	workerCtx, cancelWorker := context.WithCancel(context.Background())
	defer cancelWorker()
	offlineWorker.Start(workerCtx)
	alertEval.Start(workerCtx)

	// 10. Attach JWT & Role-Based Access Control middleware (User Auth)
	app.UseMiddleware(middleware.JWTRoleMiddleware(authService))

	// 11. Attach Agent Authentication middleware (Agent Auth - Phase 2C.2)
	app.UseMiddleware(agentMiddleware.AgentAuthMiddleware(agentServ))

	// 12. Health check endpoints
	healthHandler := func(ctx *gofr.Context) (any, error) {
		start := time.Now()
		sqlDB, err := gormDB.DB()
		if err != nil {
			return HealthResponse{
				Status:    "DEGRADED",
				Service:   "vpsmonitoring-backend",
				ORM:       "GORM v1.31.2",
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Database: DatabaseHealth{
					Status:    "DEGRADED",
					Connected: false,
					Provider:  "Neon PostgreSQL",
					ORM:       "GORM",
					Error:     err.Error(),
				},
			}, nil
		}

		queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		var (
			currentTime time.Time
			version     string
			dbName      string
		)

		err = sqlDB.QueryRowContext(queryCtx, "SELECT NOW(), version(), current_database();").Scan(&currentTime, &version, &dbName)
		latency := time.Since(start).Milliseconds()

		if err != nil {
			return HealthResponse{
				Status:    "DEGRADED",
				Service:   "vpsmonitoring-backend",
				ORM:       "GORM v1.31.2",
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Database: DatabaseHealth{
					Status:    "DEGRADED",
					Connected: false,
					Provider:  "Neon PostgreSQL",
					ORM:       "GORM",
					Database:  "neondb",
					LatencyMs: latency,
					Error:     err.Error(),
				},
			}, nil
		}

		return HealthResponse{
			Status:    "HEALTHY",
			Service:   "vpsmonitoring-backend",
			ORM:       "GORM v1.31.2 (Postgres)",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Database: DatabaseHealth{
				Status:     "HEALTHY",
				Connected:  true,
				Provider:   "Neon PostgreSQL",
				Database:   dbName,
				ORM:        "GORM",
				LatencyMs:  latency,
				ServerTime: currentTime.UTC().Format(time.RFC3339),
				Version:    version,
			},
		}, nil
	}

	app.GET("/check-health", healthHandler)
	app.GET("/health", healthHandler)
	app.GET("/api/db-status", healthHandler)

	// 13. Authentication Routes (Public)
	app.POST("/api/v1/auth/login", authController.Login)
	app.POST("/api/v1/auth/register", authController.Register)

	// 14. Admin Routes (Protected: ADMIN role required)
	app.GET("/api/v1/admin/admins", authController.GetAdmins)
	app.POST("/api/v1/admin/admins", authController.CreateAdmin)
	app.PUT("/api/v1/admin/admins/{id}", authController.UpdateAdmin)
	app.PATCH("/api/v1/admin/admins/{id}/status", authController.UpdateStatus)

	// 15. User Routes (Protected: USER or ADMIN role)
	app.GET("/api/v1/user/profile", authController.GetProfile)

	// 16. Server Management Routes (Phase 2A - Protected: USER or ADMIN role)
	app.GET("/api/v1/user/servers", serverCtrl.ListServers)
	app.POST("/api/v1/user/servers", serverCtrl.CreateServer)
	app.GET("/api/v1/user/servers/{id}", serverCtrl.GetServer)
	app.PUT("/api/v1/user/servers/{id}", serverCtrl.UpdateServer)
	app.DELETE("/api/v1/user/servers/{id}", serverCtrl.DeleteServer)
	app.GET("/api/v1/user/servers/{serverId}/metrics/latest", metricCtrl.GetLatest)
	app.GET("/api/v1/user/servers/{serverId}/metrics/history", metricCtrl.GetHistory)
	app.GET("/api/v1/user/servers/{serverId}/metrics/aggregate", metricCtrl.GetAggregate)
	app.GET("/api/v1/user/servers/{serverId}/processes", procCtrl.GetProcesses)
	app.GET("/api/v1/user/servers/{serverId}/services", sysCtrl.GetServices) // Phase 3.5A
	app.GET("/api/v1/user/servers/{serverId}/discovery", discoveryCtrl.GetDiscovery)

	// Phase 3.5B User APIs
	app.POST("/api/v1/user/servers/{serverId}/health-configs", healthCtrl.CreateConfig)
	app.GET("/api/v1/user/servers/{serverId}/health-configs", healthCtrl.ListConfigs)
	app.DELETE("/api/v1/user/servers/{serverId}/health-configs/{configId}", healthCtrl.DeleteConfig)
	app.GET("/api/v1/user/servers/{serverId}/health-checks", healthCtrl.GetHealthChecks)

	// Phase 3.6.2 Application CRUD
	app.POST("/api/v1/user/servers/{serverId}/applications", appCtrl.Create)
	app.GET("/api/v1/user/servers/{serverId}/applications", appCtrl.List)
	app.GET("/api/v1/user/servers/{serverId}/applications/{appId}", appCtrl.Get)
	app.PUT("/api/v1/user/servers/{serverId}/applications/{appId}", appCtrl.Update)
	app.DELETE("/api/v1/user/servers/{serverId}/applications/{appId}", appCtrl.Delete)

	// 17. Installation Token Routes (Phase 2B - Protected: USER or ADMIN role)
	app.POST("/api/v1/user/servers/{id}/tokens", tokenCtrl.CreateToken)
	app.GET("/api/v1/user/servers/{id}/tokens", tokenCtrl.GetTokenStatus)

	// 18. Agent Routes (Phase 2C.1, 2C.3, 2C.7)
	// Registration: Public / token-authenticated
	app.POST("/api/v1/agent/register", agentCtrl.Register)
	// Heartbeat: Protected by AgentAuthMiddleware (Authorization: Bearer <AGENT_CREDENTIAL>)
	app.POST("/api/v1/agent/heartbeat", agentCtrl.Heartbeat)
	// Metrics Ingestion: Protected by AgentAuthMiddleware (Authorization: Bearer <AGENT_CREDENTIAL>)
	app.POST("/api/v1/agent/metrics", metricCtrl.Ingest)
	// Process Snapshot Ingestion: Protected by AgentAuthMiddleware (Phase 3.4B.2)
	app.POST("/api/v1/agents/{agentId}/processes", procCtrl.IngestSnapshot)
	// Systemd Snapshot Ingestion: Protected by AgentAuthMiddleware (Phase 3.5A)
	app.POST("/api/v1/agents/{id}/services", sysCtrl.IngestSnapshot)
	// Health Check Config Sync: Protected by AgentAuthMiddleware (Phase 3.5B.2)
	app.GET("/api/v1/agents/{agentId}/config", healthCtrl.GetAgentConfig)
	// Health Check Results Ingestion: Protected by AgentAuthMiddleware (Phase 3.5B.4)
	app.POST("/api/v1/agents/{agentId}/health-checks", healthCtrl.IngestHealthChecks)
	// Application Config Sync: Protected by AgentAuthMiddleware (Phase 3.6.4)
	app.GET("/api/v1/agents/{agentId}/applications/config", appCtrl.GetAgentConfig)
	// Application Discovery Ingestion: Protected by AgentAuthMiddleware (Phase 3.5C.6)
	app.POST("/api/v1/agents/{agentId}/discovery", discoveryCtrl.IngestDiscovery)
	// Application Telemetry Ingestion: Protected by AgentAuthMiddleware (Phase 3.6.12)
	app.POST("/api/v1/agents/{agentId}/applications/telemetry", telemetryCtrl.IngestTelemetry)

	// Application Telemetry Query API: Protected by JWT User Auth (Phase 3.6.14)
	app.GET("/api/v1/user/servers/{serverId}/applications/{appId}/telemetry/latest", telemetryQueryCtrl.GetLatest)
	app.GET("/api/v1/user/servers/{serverId}/applications/{appId}/telemetry/history", telemetryQueryCtrl.GetHistory)
	app.GET("/api/v1/user/servers/{serverId}/applications/{appId}/events", telemetryQueryCtrl.GetEvents)

	// Phase 4.1 Alert Rule CRUD (normal user/admin JWT only).
	app.POST("/api/v1/user/alert-rules", alertRuleCtrl.Create)
	app.GET("/api/v1/user/alert-rules", alertRuleCtrl.List)
	app.GET("/api/v1/user/alert-rules/{id}", alertRuleCtrl.Get)
	app.PUT("/api/v1/user/alert-rules/{id}", alertRuleCtrl.Update)
	app.PATCH("/api/v1/user/alert-rules/{id}/status", alertRuleCtrl.UpdateStatus)
	app.DELETE("/api/v1/user/alert-rules/{id}", alertRuleCtrl.Delete)

	// Phase 4.4 persisted Alert APIs (read-only, normal user/admin JWT only).
	app.GET("/api/v1/user/alerts", alertCtrl.List)
	app.GET("/api/v1/user/alerts/{id}", alertCtrl.Get)
	app.GET("/api/v1/user/servers/{serverId}/alerts", alertCtrl.ListByServer)
	app.GET("/api/v1/user/servers/{serverId}/applications/{applicationId}/alerts", alertCtrl.ListByApplication)

	// 19. Start the GoFr server
	app.Run()
}
