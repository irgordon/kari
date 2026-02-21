package services

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/lego"
	"kari/api/internal/core/domain"
	"kari/api/internal/grpc/rustagent"
)

type SslService struct {
	repo        domain.SslRepository
	agentClient rustagent.SystemAgentClient
	logger      *slog.Logger
}

// ProvisionCert orchestrates the platform-independent ACME flow
func (s *SslService) ProvisionCert(ctx context.Context, domainName string, email string) error {
	s.logger.Info("Initiating ACME handshake", slog.String("domain", domainName))

	// 1. Setup ACME User (stored in DB/Vault)
	user := &AcmeUser{Email: email}
	config := lego.NewConfig(user)
	client, _ := lego.NewClient(config)

	// 🛡️ 2. Platform Agnostic Challenge Provider
	// We inject the gRPC client into the provider. 
	// The provider sends the "intent" to the Rust Muscle.
	provider := &MuscleChallengeProvider{
		agent:  s.agentClient,
		domain: domainName,
	}
	
	err := client.Challenge.SetHTTP01Provider(provider)
	if err != nil {
		return fmt.Errorf("provider_setup_failed: %w", err)
	}

	// 3. Obtain Certificate
	request := certificate.ObtainRequest{Domains: []string{domainName}, Bundle: true}
	certs, err := client.Certificate.Obtain(request)
	if err != nil {
		return fmt.Errorf("acme_obtainment_failed: %w", err)
	}

	// 🛡️ 4. Unified Installation
	// The Muscle receives the PEM bytes and installs them into the
	// platform-specific paths (e.g., /etc/ssl/ or /etc/pki/)
	_, err = s.agentClient.InstallCertificate(ctx, &rustagent.SslInstallRequest{
		DomainName:   domainName,
		FullchainPem: certs.Certificate,
		PrivkeyPem:   certs.PrivateKey,
	})

	return s.repo.MarkAsSecure(ctx, domainName, certs.Expiry)
}
