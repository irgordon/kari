package services

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/google/uuid"
	"kari/api/internal/core/domain"
	pb "kari/api/proto/kari/agent/v1"
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
		auditRepo:   auditRepo,
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
	traceID := fmt.Sprintf("dep-%s-%d", app.ID.String()[:8], ctx.Value("request_start").(int64))
	
	s.logger.Info("Starting deployment", 
		slog.String("app", app.Name), 
		slog.String("trace_id", traceID))

	// 3. Prepare the gRPC Stream with the Rust Muscle
	stream, err := s.agentClient.StreamDeployment(ctx, &pb.DeployRequest{
		TraceId:      traceID,
		AppId:        app.ID.String(),
		DomainName:   app.DomainName, // From Joined query
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
				// Log to Action Center via AuditRepo
				_ = s.auditRepo.CreateAlert(context.Background(), &domain.SystemAlert{
					Severity: "critical",
					Category: "deployment",
					Message:  fmt.Sprintf("Deployment failed for %s: %v", app.Name, err),
					Metadata: map[string]any{"trace_id": traceID},
				})
				break
			}
			
			// Push to channel for WebSocket delivery
			logChan <- chunk.Content
		}
	}()

	return logChan, nil
}
