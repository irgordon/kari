package worker

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"kari/api/internal/core/domain"
	"kari/api/internal/core/services"
	"kari/api/internal/telemetry"
	"kari/api/proto/agent" // Generated gRPC client
)

// DeploymentWorker orchestrates the lifecycle of an application deployment.
type DeploymentWorker struct {
	repo         domain.DeploymentRepository
	crypto       services.CryptoService
	agent        agent.SystemAgentClient
	hub          *telemetry.Hub
	pollInterval time.Duration
}

// NewDeploymentWorker initializes the background processor with necessary dependencies.
func NewDeploymentWorker(
	repo domain.DeploymentRepository,
	crypto services.CryptoService,
	agent agent.SystemAgentClient,
	hub *telemetry.Hub,
) *DeploymentWorker {
	return &DeploymentWorker{
		repo:         repo,
		crypto:       crypto,
		agent:        agent,
		hub:          hub,
		pollInterval: 5 * time.Second,
	}
}

// Start initiates the non-blocking polling loop.
func (w *DeploymentWorker) Start(ctx context.Context) {
	log.Println("🚀 Kari Brain: Deployment Worker started.")
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("🛑 Kari Brain: Deployment Worker shutting down...")
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
		log.Printf("⚠️  Kari Panel: Failed to claim task: %v", err)
		return
	}
	if deployment == nil {
		return // No tasks available
	}

	w.hub.Broadcast(deployment.ID, "🚀 Kari Panel: Initializing deployment engine...\n")

	// 2. 🛡️ Zero-Trust: Decrypt SSH Key (Transient Memory Only)
	var sshKey string
	if deployment.EncryptedSSHKey != "" {
		// AssociatedData binds this key to the specific AppID for tamper protection
		decrypted, err := w.crypto.Decrypt(ctx, deployment.EncryptedSSHKey, []byte(deployment.AppID))
		if err != nil {
			w.failDeployment(ctx, deployment, fmt.Errorf("security: failed to decrypt deploy key: %w", err))
			return
		}
		sshKey = string(decrypted)
		
		// Hygiene: Best effort to clear the string from memory after gRPC call (later)
		// Go strings are immutable, but we ensure no permanent pointers remain.
	}

	// 3. 📡 Connect to the Muscle (Rust Agent)
	// Context is passed to allow cancellation if the worker shuts down.
	stream, err := w.agent.StreamDeployment(ctx, &agent.DeployRequest{
		AppId:        deployment.AppID,
		DomainName:   deployment.DomainName,
		RepoUrl:      deployment.RepoURL,
		Branch:       deployment.Branch,
		BuildCommand: deployment.BuildCommand,
		Port:         int32(deployment.TargetPort),
		SshKey:       &sshKey, // Injected for transient use
		TraceId:      deployment.ID,
	})

	if err != nil {
		w.failDeployment(ctx, deployment, fmt.Errorf("network: agent unreachable: %w", err))
		return
	}

	// 4. 🚰 Telemetry Loop: Pipe logs from Agent -> DB & Hub
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break // Deployment finished successfully
		}
		if err != nil {
			w.failDeployment(ctx, deployment, fmt.Errorf("execution: stream interrupted: %w", err))
			return
		}

		// 🛡️ SLA Visibility: Concurrent persistence and real-time broadcast
		// We ignore errors on logging to ensure the deployment continues even if DB is under load.
		_ = w.repo.AppendLog(ctx, deployment.ID, chunk.Content)
		w.hub.Broadcast(deployment.ID, chunk.Content)
	}

	// 5. ✅ Finalize: Update state to Success
	if err := w.repo.UpdateStatus(ctx, deployment.ID, domain.StatusSuccess); err != nil {
		log.Printf("❌ Kari Panel: Failed to update success status for %s: %v", deployment.ID, err)
		return
	}

	w.hub.Broadcast(deployment.ID, "✅ Kari Panel: Deployment successful. Service is live.\n")
}

// failDeployment handles cleanup and telemetry updates for failed builds.
func (w *DeploymentWorker) failDeployment(ctx context.Context, d *domain.Deployment, err error) {
	errMsg := fmt.Sprintf("\n❌ ERROR: %v\n", err)
	log.Printf("❌ Kari Panel: Deployment %s failed: %v", d.ID, err)

	_ = w.repo.AppendLog(ctx, d.ID, errMsg)
	w.hub.Broadcast(d.ID, errMsg)
	_ = w.repo.UpdateStatus(ctx, d.ID, domain.StatusFailed)
}
