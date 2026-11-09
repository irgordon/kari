package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/irgordon/kari/api/internal/core/domain"
	agent "github.com/irgordon/kari/api/internal/grpc/rustagent" // Generated gRPC client
)

// Broadcaster abstracts the telemetry hub for dependency inversion
type Broadcaster interface {
	Broadcast(deploymentID string, message string)
}

// DeploymentWorker orchestrates the lifecycle of an application deployment.
// 🛡️ SOLID: Depends on domain interfaces, not concrete implementations.
type DeploymentWorker struct {
	repo         domain.DeploymentRepository
	agent        agent.SystemAgentClient
	hub          Broadcaster
	logger       *slog.Logger
	pollInterval time.Duration
}

// NewDeploymentWorker initializes the background processor with necessary dependencies.
func NewDeploymentWorker(
	repo domain.DeploymentRepository,
	agent agent.SystemAgentClient,
	hub Broadcaster,
	logger *slog.Logger,
) *DeploymentWorker {
	return &DeploymentWorker{
		repo:         repo,
		agent:        agent,
		hub:          hub,
		logger:       logger,
		pollInterval: 5 * time.Second,
	}
}

// Start initiates the non-blocking polling loop.
func (w *DeploymentWorker) Start(ctx context.Context) {
	w.logger.Info("🚀 Kari Brain: Deployment Worker started.")
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("🛑 Kari Brain: Deployment Worker shutting down...")
			return
		case <-ticker.C:
			w.processNextTask(ctx)
		}
	}
}

// processNextTask handles the transition from PENDING to SUCCESS/FAILED.
func (w *DeploymentWorker) processNextTask(ctx context.Context) {
	// 1. 🛡️ Claim Task: Atomic 'FOR UPDATE SKIP LOCKED' via repository
	deployment, err := w.repo.ClaimNextPending(ctx)
	if err != nil {
		w.logger.Warn("⚠️  Kari Panel: Failed to claim task", slog.Any("error", err))
		return
	}
	if deployment == nil {
		return // No tasks available
	}

	w.hub.Broadcast(deployment.ID, "🚀 Kari Panel: Initializing deployment engine...\n")

	if deployment.BuildCommand != "" || deployment.EncryptedSSHKey != "" {
		w.failDeployment(ctx, deployment, fmt.Errorf("only prepared static sites from public HTTPS repositories are supported"))
		return
	}
	// Publishing belongs to the worker, not the lifetime of a browser log viewer.
	operation, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	result, err := w.agent.DeployStaticSite(operation, &agent.StaticSiteRequest{
		SiteId: deployment.AppID, DomainName: deployment.DomainName,
		RepoUrl: deployment.RepoURL, Branch: deployment.Branch, DeploymentId: deployment.ID,
	})
	if err != nil || !result.GetSuccess() {
		w.failDeployment(ctx, deployment, fmt.Errorf("static publication failed; check the repository, index.html, and host journal"))
		return
	}
	if err := w.repo.AppendLog(ctx, deployment.ID, result.Stdout); err != nil {
		w.failDeployment(ctx, deployment, fmt.Errorf("could not persist publication result: %w", err))
		return
	}

	// 5. ✅ Finalize: Update state to Success
	if err := w.repo.UpdateStatus(ctx, deployment.ID, domain.StatusSuccess); err != nil {
		w.logger.Error("❌ Kari Panel: Failed to update success status",
			slog.String("deployment_id", deployment.ID),
			slog.Any("error", err))
		return
	}

	w.hub.Broadcast(deployment.ID, "✅ Kari Panel: Deployment successful. Service is live.\n")
}

// failDeployment handles cleanup and telemetry updates for failed builds.
// 🛡️ Zero-Trust: Raw Muscle errors are classified into UI-safe codes before broadcast.
func (w *DeploymentWorker) failDeployment(ctx context.Context, d *domain.Deployment, err error) {
	// 1. Classify the raw error into a human-readable, UI-safe structure
	agentErr := domain.ClassifyAgentError(err.Error())

	// 2. Log the RAW error server-side for forensic analysis (never sent to browser)
	w.logger.Error("❌ Deployment failed",
		slog.String("deployment_id", d.ID),
		slog.String("error_code", string(agentErr.Code)),
		slog.Any("raw_error", err))

	// 3. Build the user-facing terminal message with ANSI colors
	var terminalMsg string
	switch agentErr.Severity {
	case "critical":
		terminalMsg = fmt.Sprintf("\r\n\x1b[31m[%s] %s\x1b[0m\r\n\x1b[31m  → %s\x1b[0m\r\n", agentErr.Code, agentErr.Title, agentErr.Message)
	default:
		terminalMsg = fmt.Sprintf("\r\n\x1b[33m[%s] %s\x1b[0m\r\n\x1b[33m  → %s\x1b[0m\r\n", agentErr.Code, agentErr.Title, agentErr.Message)
	}

	_ = w.repo.AppendLog(ctx, d.ID, terminalMsg)
	w.hub.Broadcast(d.ID, terminalMsg)
	_ = w.repo.UpdateStatus(ctx, d.ID, domain.StatusFailed)
}
