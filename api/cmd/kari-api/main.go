package main

import (
	"context"
	"errors"
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

	"kari/api/internal/api/handlers"
	"kari/api/internal/api/middleware"
	"kari/api/internal/api/router"
	"kari/api/internal/config"
	"kari/api/internal/core/services"
	"kari/api/internal/db/postgres"
	"kari/api/internal/infrastructure/crypto"
	"kari/api/internal/telemetry"
	"kari/api/internal/worker"
	"kari/api/internal/workers"
	agent "kari/api/proto/kari/agent/v1"
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

	// 🛡️ gRPC Link to Rust Muscle over Unix Socket
	// Keepalive ensures the Brain detects a dead Muscle and triggers transport reconnection
	// when the Agent restarts and recreates the UDS.
	grpcDialer := func(ctx context.Context, addr string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", addr)
	}

	grpcConn, err := grpc.Dial(
		cfg.AgentSocket,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
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

	// --- 3. Setup Mode Detection ---
	// 🛡️ The Setup Guard determines whether the system is configured.
	// In setup mode, crypto and DB are not yet available.
	lockPath := "/opt/kari/setup.lock"

	// Signal channel for shutdown (used by setup lockdown)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	setupHandler := handlers.NewSetupHandler(
		agentClient, logger, cfg.JWTSecret, lockPath,
		func() { stop <- syscall.SIGTERM }, // Shutdown trigger for lockdown
	)

	// 🛡️ If in setup mode, print the transient setup token and skip crypto/DB init
	if !setupHandler.IsLocked() {
		setupToken, tokenErr := handlers.GenerateSetupToken(cfg.JWTSecret)
		if tokenErr != nil {
			logger.Error("FATAL: Cannot generate setup token", "error", tokenErr)
			os.Exit(1)
		}
		logger.Info("🔧 SETUP MODE: System is unconfigured. Access the wizard at:")
		logger.Info("   http://localhost:"+cfg.Port+"/setup?token="+setupToken)
	}

	// --- 4. Hardened Dependency Injection ---
	// 🛡️ Zero-Trust: Crypto failure at boot is FATAL (only after setup).
	var cryptoService *crypto.AESCryptoService
	if setupHandler.IsLocked() {
		cryptoService, err = crypto.NewAESCryptoService(cfg.MasterKeyHex)
		if err != nil {
			logger.Error("FATAL: Cryptographic initialization failed", "error", err)
			os.Exit(1)
		}
	}

	// Repositories
	appRepo := postgres.NewApplicationRepository(dbPool)
	deployRepo := postgres.NewPostgresDeploymentRepository(dbPool)
	userRepo := postgres.NewUserRepository(dbPool)

	// 🛡️ Global Telemetry Hub (Memory Bus)
	telemetryHub := telemetry.NewHub()

	// Services
	authService := services.NewAuthService(userRepo, logger, cfg)

	// Handlers
	authHandler := handlers.NewAuthHandler(authService)
	deployHandler := handlers.NewDeploymentHandler(deployRepo, cryptoService, telemetryHub)

	authMiddleware := middleware.NewAuthMiddleware(authService, logger)

	// --- 5. Background Workers ---
	workerCtx, cancelWorkers := context.WithCancel(context.Background())
	defer cancelWorkers()

	// 🛡️ Deployment Worker: Claims tasks and orchestrates gRPC -> SSE
	deployWorker := worker.NewDeploymentWorker(deployRepo, cryptoService, agentClient, telemetryHub, logger)
	go deployWorker.Start(workerCtx)

	// 🩺 Health Prober: Background Muscle heartbeat (every 15s)
	healthProber := workers.NewHealthProber(agentClient, logger)
	go healthProber.Start(workerCtx)

	// App Availability Monitor
	appMonitor := workers.NewAppMonitor(appRepo, logger, 1*time.Minute)
	go appMonitor.Start(workerCtx)

	// --- 6. HTTP Gateway ---
	mux := router.NewRouter(router.RouterConfig{
		AllowedOrigins:  cfg.AllowedOrigins,
		AuthHandler:     authHandler,
		DeployHandler:   deployHandler,
		SetupHandler:    setupHandler,
		AuthMiddleware:  authMiddleware,
		Logger:          logger,
	})

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
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
