package services

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/irgordon/kari/api/internal/core/domain"
	"github.com/irgordon/kari/api/internal/grpc/rustagent"
)

type SslService struct {
	repo        domain.SslRepository
	agentClient rustagent.SystemAgentClient
	logger      *slog.Logger
}

type SSLService = SslService

func (s *SslService) ProvisionCertificate(ctx context.Context, userID uuid.UUID, domainID uuid.UUID) error {
	_ = ctx
	_ = userID
	_ = domainID
	return fmt.Errorf("ssl certificate provisioning is not wired to the current agent API")
}

// ProvisionCert orchestrates the platform-independent ACME flow
func (s *SslService) ProvisionCert(ctx context.Context, domainName string, email string) error {
	s.logger.Info("Initiating ACME handshake", slog.String("domain", domainName))

	_ = ctx
	_ = email
	_ = s.repo
	_ = s.agentClient
	return fmt.Errorf("ssl provisioning is not wired to the current agent API")
}
