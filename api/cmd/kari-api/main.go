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

	"kari/api/internal/api/handlers"
	"kari/api/internal/api/middleware"
	"kari/api/internal/api/router"
	"kari/api/internal/config"
	"kari/api/internal/core/services"
	"kari/api/internal/db/postgres"
	"kari/api/internal/infrastructure/crypto"
	"kari/api/internal/telemetry" // 🛡️ Added: Telemetry Hub
	"kari/api/internal/worker"    // 🛡️ Added: Deployment Worker
	"kari/api/proto/agent"        // 🛡️ Fixed: Match your proto package
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
	grpcDialer := func(ctx context.Context, addr string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", addr)
	}
	
	grpcConn, err := grpc.Dial(
		cfg.AgentSocketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(grpcDialer),
	)
	if err != nil {
		logger.Error("FATAL: gRPC link failed", "error", err)
		os.Exit(1)
	}
	defer grpcConn.Close()
	agentClient := agent.NewSystemAgentClient(grpcConn)

	// --- 3. Hardened Dependency Injection ---
	cryptoService, _ := crypto.NewAESCryptoService(cfg.MasterKeyHex)
	
	// Repositories
	appRepo := postgres.NewApplicationRepository(dbPool)
	deployRepo := postgres.NewPostgresDeploymentRepository(dbPool) // 🛡️ New
	userRepo := postgres.NewUserRepository(dbPool)

	// 🛡️ Global Telemetry Hub (Memory Bus)
	telemetryHub := telemetry.NewHub()

	// Services
	authService := services.NewAuthService(userRepo, logger, cfg)
	
	// Handlers
	authHandler := handlers.NewAuthHandler(authService)
	// 🛡️ Updated DeploymentHandler with SSE and Task Queue capabilities
	deployHandler := handlers.NewDeploymentHandler(deployRepo, cryptoService, telemetryHub)
	
	authMiddleware := middleware.NewAuthMiddleware(authService, logger)

	// --- 4. Background Workers ---
	workerCtx, cancelWorkers := context.WithCancel(context.Background())
	defer cancelWorkers()

	// 🛡️ Deployment Worker: Claims tasks and orchestrates gRPC -> SSE
	deployWorker := worker.NewDeploymentWorker(deployRepo, cryptoService, agentClient, telemetryHub)
	go deployWorker.Start(workerCtx)

	// App Availability Monitor
	appMonitor := workers.NewAppMonitor(appRepo, logger, 1*time.Minute)
	go appMonitor.Start(workerCtx)

	// --- 5. HTTP Gateway ---
	mux := router.NewRouter(router.RouterConfig{
		AuthHandler:     authHandler,
		DeployHandler:   deployHandler, // 🛡️ Injected new handler
		AuthMiddleware:  authMiddleware,
		Logger:          logger,
	})

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	// --- 6. Graceful Exit ---
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

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

	shutdownCtx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("ERROR: Forced shutdown", "error", err)
	}
	logger.Info("✅ Kari Panel Brain shutdown. Muscle Agent remains in jail.")
}
