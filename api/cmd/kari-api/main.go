package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	"github.com/irgordon/kari/api/internal/adapters"
	"github.com/irgordon/kari/api/internal/api/handlers"
	"github.com/irgordon/kari/api/internal/api/middleware"
	"github.com/irgordon/kari/api/internal/api/router"
	"github.com/irgordon/kari/api/internal/config"
	"github.com/irgordon/kari/api/internal/core/services"
	karidb "github.com/irgordon/kari/api/internal/db"
	"github.com/irgordon/kari/api/internal/db/postgres"
	agent "github.com/irgordon/kari/api/internal/grpc/rustagent"
	"github.com/irgordon/kari/api/internal/infrastructure/crypto"
	"github.com/irgordon/kari/api/internal/infrastructure/mail"
	"github.com/irgordon/kari/api/internal/telemetry"
	"github.com/irgordon/kari/api/internal/worker"
	"github.com/irgordon/kari/api/internal/workers"
)

func main() {
	// --- 1. Core Telemetry & Configuration ---
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
	logger.Info("🚀 Booting Karı Panel Brain...")
	cfg := config.Load()

	// --- 2. Outbound Infrastructure ---
	dbPool, err := postgres.NewPool(context.Background(), cfg.DatabaseURL)
	if err != nil {
		logger.Error("FATAL: DB failed", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()
	version, err := karidb.SchemaVersion(context.Background(), dbPool)
	plan, planErr := karidb.LoadMigrationPlan()
	if err != nil || planErr != nil || version != len(plan) {
		logger.Error("FATAL: Database must match this release; run the supported migration or restore procedure")
		os.Exit(1)
	}

	// 🛡️ gRPC Link to Rust Muscle over Unix Socket
	// Keepalive ensures the Brain detects a dead Muscle and triggers transport reconnection
	// when the Agent restarts and recreates the UDS.
	grpcDialer := func(ctx context.Context, addr string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", addr)
	}

	grpcConn, err := grpc.Dial(
		cfg.AgentSocket,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		// A filesystem endpoint is not a valid HTTP/2 authority. Peer identity is
		// still enforced by the native agent using the Unix connection credentials.
		grpc.WithAuthority("localhost"),
		grpc.WithContextDialer(grpcDialer),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second, // Send keepalive ping every 30s
			Timeout:             10 * time.Second, // Wait 10s for pong before marking dead
			PermitWithoutStream: true,             // Ping even when no active RPCs (for UDS reconnect)
		}),
	)
	if err != nil {
		logger.Error("FATAL: gRPC link failed", "error", err)
		os.Exit(1)
	}
	defer grpcConn.Close()
	agentClient := agent.NewSystemAgentClient(grpcConn)

	// --- 3. Transactional first-run setup ---
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	userRepo := postgres.NewUserRepo(dbPool)
	configured, err := userRepo.IsConfigured(context.Background())
	if err != nil {
		logger.Error("FATAL: Cannot check setup state", "error", err)
		os.Exit(1)
	}
	setupToken := cfg.SetupToken
	if !configured {
		if len(setupToken) < 32 {
			logger.Error("FATAL: Initialize a private KARI_SETUP_TOKEN with kari-admin init or install")
			os.Exit(1)
		}
		logger.Info("Open the frontend /setup page and retrieve the setup key through the local operator command. It expires in 15 minutes.")
	}
	setupHandler := handlers.NewSetupHandler(userRepo, logger, setupToken)
	if cfg.MasterKeyHex == "" && cfg.Environment == "development" {
		// Development-only fallback. Production always requires its persistent operator-supplied key.
		cfg.MasterKeyHex = "0000000000000000000000000000000000000000000000000000000000000000"
	}
	cryptoService, err := crypto.NewAESCryptoService(cfg.MasterKeyHex)
	if err != nil {
		logger.Error("FATAL: Cryptographic initialization failed", "error", err)
		os.Exit(1)
	}

	// Repositories
	appRepo := postgres.NewApplicationRepo(dbPool)
	deployRepo := postgres.NewPostgresDeploymentRepository(dbPool)
	auditRepo := postgres.NewAuditRepository(dbPool)

	// 🛡️ Global Telemetry Hub (Memory Bus)
	telemetryHub := telemetry.NewHub()

	// Services
	tokenService := services.NewTokenService(cfg.JWTSecret)
	authService := services.NewAuthService(userRepo, tokenService)
	mailer, err := mail.NewSMTP(cfg.Mail)
	if err != nil {
		logger.Error("FATAL: Invalid SMTP configuration", "error", err)
		os.Exit(1)
	}
	emailAccess, err := services.NewEmailAccess(userRepo, authService, mailer, cfg.PublicURL, cfg.Environment == "development", logger)
	if err != nil {
		logger.Error("FATAL: Invalid email access configuration", "error", err)
		os.Exit(1)
	}
	roleService := services.NewRoleService(userRepo, logger)

	// Handlers
	authHandler := handlers.NewAuthHandler(authService, cfg.Environment != "development")
	authHandler.MFA = &services.MFAService{Repo: userRepo, Auth: authService, Crypto: cryptoService}
	emailHandler := &handlers.EmailAccessHandler{Service: emailAccess, Auth: authHandler}
	deployHandler := handlers.NewDeploymentHandler(deployRepo, cryptoService, telemetryHub)
	sites := &services.Sites{DB: dbPool, Agent: agentClient, ACME: adapters.NewAcmeProvider(cfg, agentClient, logger), PanelURL: cfg.PublicURL}
	if err := sites.RecoverInterrupted(context.Background()); err != nil {
		logger.Error("FATAL: Cannot recover interrupted site operations", "error", err)
		os.Exit(1)
	}
	siteHandler := &handlers.SiteHandler{Service: sites}

	authMiddleware := middleware.NewAuthMiddleware(authService, roleService, userRepo, logger)

	// --- 5. Background Workers ---
	workerCtx, cancelWorkers := context.WithCancel(context.Background())
	defer cancelWorkers()
	go emailAccess.Run(workerCtx)

	// 🛡️ Deployment Worker: Claims tasks and orchestrates gRPC -> SSE
	deployWorker := worker.NewDeploymentWorker(deployRepo, agentClient, telemetryHub, logger)
	go deployWorker.Start(workerCtx)

	// 🩺 Health Prober: Background Muscle heartbeat (every 15s)
	healthProber := workers.NewHealthProber(agentClient, logger)
	go healthProber.Start(workerCtx)

	// App Availability Monitor
	appMonitor := workers.NewAppMonitor(appRepo, auditRepo, logger, 1*time.Minute)
	go appMonitor.Start(workerCtx)
	setupHandler.Auth = authHandler
	setupHandler.DatabaseReady = dbPool.Ping
	setupHandler.AgentReady = healthProber.IsHealthy
	setupHandler.AgentRequired = cfg.RequireAgent
	setupHandler.PublicURL = cfg.PublicURL
	setupHandler.EmailConfigured = emailAccess.Enabled()
	setupHandler.Version = cfg.Version

	// --- 6. HTTP Gateway ---
	mux := router.NewRouter(router.RouterConfig{
		AllowedOrigins: cfg.AllowedOrigins,
		TrustedProxies: cfg.TrustedProxies,
		AuthHandler:    authHandler,
		EmailHandler:   emailHandler,
		DeployHandler:  deployHandler,
		SiteHandler:    siteHandler,
		SetupHandler:   setupHandler,
		AuthMiddleware: authMiddleware,
		ReadinessCheck: newReadinessCheck(dbPool, healthProber, cfg.RequireAgent),
		Logger:         logger,
	})

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 4 * time.Minute,
	}

	// --- 7. Graceful Exit ---
	// (stop channel already created above for Setup lockdown)

	go func() {
		logger.Info("🌐 Kari Panel API active", "port", cfg.Port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("CRITICAL: Server crashed", "error", err)
			os.Exit(1)
		}
	}()

	<-stop
	logger.Info("🛑 Shutting down...")
	cancelWorkers() // Stop workers first to prevent new gRPC calls

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("ERROR: Forced shutdown", "error", err)
	}
	logger.Info("✅ Kari Panel Brain shutdown. Muscle Agent remains in jail.")
}

type databasePinger interface {
	Ping(context.Context) error
}

type agentHealth interface {
	IsHealthy() bool
}

func newReadinessCheck(database databasePinger, agentProbe agentHealth, requireAgent bool) func(context.Context) error {
	return func(ctx context.Context) error {
		if err := database.Ping(ctx); err != nil {
			return fmt.Errorf("database is not ready: %w", err)
		}
		if requireAgent && !agentProbe.IsHealthy() {
			return errors.New("native agent is not ready")
		}
		return nil
	}
}
