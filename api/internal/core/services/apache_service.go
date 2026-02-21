package services

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"kari/api/internal/core/domain"
	"kari/api/internal/grpc/rustagent"
)

type ApacheService struct {
	appRepo     domain.ApplicationRepository
	domainRepo  domain.DomainRepository
	agentClient rustagent.SystemAgentClient
	logger      *slog.Logger
}

func NewApacheService(
	appRepo domain.ApplicationRepository,
	domainRepo domain.DomainRepository,
	agent rustagent.SystemAgentClient,
	logger *slog.Logger,
) *ApacheService {
	return &ApacheService{
		appRepo:     appRepo,
		domainRepo:  domainRepo,
		agentClient: agent,
		logger:      logger,
	}
}

// AttachDomain binds a domain to an app and triggers the Rust Muscle to update Apache
func (s *ApacheService) AttachDomain(ctx context.Context, appID uuid.UUID, domainName string, port u16) error {
	s.logger.Info("Attaching domain", slog.String("domain", domainName), slog.String("app_id", appID.String()))

	// 1. Persist the intent in the DomainRepository
	err := s.domainRepo.Create(ctx, &domain.Domain{
		AppID:  appID,
		Name:   domainName,
		Status: "provisioning",
	})
	if err != nil {
		return fmt.Errorf("failed to record domain intent: %w", err)
	}

	// 🛡️ 2. gRPC Call to Muscle
	// We send the request to create a VirtualHost pointing to the app's internal port
	resp, err := s.agentClient.ManageProxy(ctx, &rustagent.ProxyRequest{
		Action:     rustagent.ProxyAction_CREATE,
		DomainName: domainName,
		TargetPort: uint32(port),
	})

	if err != nil || !resp.Success {
		s.logger.Error("Muscle failed to update Apache", slog.Any("error", err))
		_ = s.domainRepo.UpdateStatus(ctx, domainName, "failed")
		return fmt.Errorf("proxy configuration failed at the muscle layer")
	}

	// 3. Finalize state
	return s.domainRepo.UpdateStatus(ctx, domainName, "active")
}

// DetachDomain cleans up both the database and the remote Apache config
func (s *ApacheService) DetachDomain(ctx context.Context, domainName string) error {
	// 🛡️ Zero-Trust: Tell the Muscle to purge the VHost before we delete the DB record
	resp, err := s.agentClient.ManageProxy(ctx, &rustagent.ProxyRequest{
		Action:     rustagent.ProxyAction_DELETE,
		DomainName: domainName,
	})

	if err != nil || !resp.Success {
		return fmt.Errorf("failed to detach proxy: %v", err)
	}

	return s.domainRepo.Delete(ctx, domainName)
}
