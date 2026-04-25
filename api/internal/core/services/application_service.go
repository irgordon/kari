package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/google/uuid"
	"kari/api/internal/core/domain"
	pb "kari/api/internal/grpc/rustagent"
)

type ApplicationService struct {
	repo        domain.ApplicationRepository
	auditRepo   domain.AuditRepository
	agentClient pb.SystemAgentClient
	logger      *slog.Logger
}

func NewApplicationService(
	repo domain.ApplicationRepository,
	audit domain.AuditRepository,
	agent pb.SystemAgentClient,
	logger *slog.Logger,
) *ApplicationService {
	return &ApplicationService{
		repo:        repo,
		auditRepo:   audit, // Fixed: was auditRepo: auditRepo
		agentClient: agent,
		logger:      logger,
	}
}

// Deploy triggers the GitOps workflow via the Rust Muscle
func (s *ApplicationService) Deploy(ctx context.Context, appID uuid.UUID, userID uuid.UUID) (<-chan string, error) {
	// 1. Fetch App & Verify Ownership (Zero-Trust IDOR Protection)
	app, err := s.repo.GetByID(ctx, appID, userID)
	if err != nil {
		return nil, fmt.Errorf("deploy unauthorized or app not found: %w", err)
	}

	// 2. Generate Trace Identity for the Action Center
	// Note: Fallback to current timestamp if request_start is missing from context
	reqStart, _ := ctx.Value("request_start").(int64)
	traceID := fmt.Sprintf("dep-%s-%d", app.ID.String()[:8], reqStart)

	s.logger.Info("Starting deployment",
		slog.String("app", app.Name),
		slog.String("trace_id", traceID))

	// 3. Prepare the gRPC Stream with the Rust Muscle
	stream, err := s.agentClient.StreamDeployment(ctx, &pb.DeployRequest{
		TraceId:      traceID,
		AppId:        app.ID.String(),
		DomainName:   app.DomainName,
		RepoUrl:      app.RepoURL,
		Branch:       app.Branch,
		BuildCommand: app.BuildCommand,
		EnvVars:      app.EnvVars,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to system agent: %w", err)
	}

	// 4. Async Log Pipeline (Memory-Safe Channel)
	logChan := make(chan string, 100)

	go func() {
		defer close(logChan)
		for {
			chunk, err := stream.Recv()
			if err == io.EOF {
				s.logger.Info("Deployment stream finished", slog.String("trace_id", traceID))
				break
			}
			if err != nil {
				s.logger.Error("Deployment stream interrupted", slog.Any("error", err))
				_ = s.auditRepo.CreateAlert(context.Background(), &domain.SystemAlert{
					Severity: "critical",
					Category: "deployment",
					Message:  fmt.Sprintf("Deployment failed for %s: %v", app.Name, err),
					Metadata: map[string]any{"trace_id": traceID},
				})
				break
			}
			select {
			case logChan <- chunk.Content:
			case <-time.After(2 * time.Second):
				s.logger.Warn("Deployment log send timed out (slow client)", slog.String("trace_id", traceID))
				// Optional: We can break here or just drop the log and continue
			}
		}
	}()

	return logChan, nil
}

// 🛡️ DeleteApplication enforces Rank-Based Ownership and OS-level cleanup
func (s *ApplicationService) DeleteApplication(ctx context.Context, appID uuid.UUID, actorID uuid.UUID, actorRank int) error {
	// 1. Fetch Target App with Internal Metadata (joins users to get owner_rank)
	app, err := s.repo.GetByIDWithMetadata(ctx, appID)
	if err != nil {
		return fmt.Errorf("application not found: %w", err)
	}

	// 🛡️ 2. Authority Logic: Actor must own the app OR have a superior (lower) rank than the owner
	isOwner := app.OwnerID == actorID
	hasSuperiorRank := actorRank < app.OwnerRank

	if !isOwner && !hasSuperiorRank {
		s.logger.Warn("Forbidden deletion attempt",
			slog.String("app_id", appID.String()),
			slog.String("actor", actorID.String()))
		return errors.New("forbidden: you do not have authority to delete this resource")
	}

	// 3. Audit the intent
	_ = s.auditRepo.CreateAlert(ctx, &domain.SystemAlert{
		Severity: "warning",
		Category: "lifecycle",
		Message:  fmt.Sprintf("Teardown of %s initiated by user rank %d", app.Name, actorRank),
		Metadata: map[string]any{"app_id": appID, "actor_id": actorID},
	})

	// 4. Invoke Rust Muscle (gRPC) for physical cleanup (systemd, nginx, directories)
	_, err = s.agentClient.DeleteDeployment(ctx, &pb.DeleteRequest{
		AppId:      app.ID.String(),
		DomainName: app.DomainName,
	})
	if err != nil {
		return fmt.Errorf("system agent failed to clean up resource: %w", err)
	}

	// 5. Atomic DB Deletion
	return s.repo.Delete(ctx, appID)
}
