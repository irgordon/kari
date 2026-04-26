package services

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/irgordon/kari/api/internal/core/domain"
	"github.com/irgordon/kari/api/internal/grpc/rustagent"
)

const unsupportedProxyManagement = "apache proxy management is not supported by the current agent API"

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
func (s *ApacheService) AttachDomain(ctx context.Context, appID uuid.UUID, domainName string, port int) error {
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

	_ = port
	_ = s.agentClient
	_ = s.domainRepo.UpdateStatus(ctx, domainName, "failed")
	return fmt.Errorf(unsupportedProxyManagement)
}

// DetachDomain cleans up both the database and the remote Apache config
func (s *ApacheService) DetachDomain(ctx context.Context, domainName string) error {
	_ = s.agentClient
	_ = s.domainRepo.Delete(ctx, domainName)
	return fmt.Errorf(unsupportedProxyManagement)
}
