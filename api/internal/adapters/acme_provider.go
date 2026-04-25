// api/internal/adapters/acme_provider.go
package adapters

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge/http01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"

	"kari/api/internal/config"
	// Assuming the generated protobuf package is aliased as pb
	pb "kari/api/internal/grpc/rustagent"
)

// ==============================================================================
// 1. Kari User Implementation (Required by Lego)
// ==============================================================================

type KariUser struct {
	Email        string
	Registration *registration.Resource
	key          crypto.PrivateKey
}

func (u *KariUser) GetEmail() string                        { return u.Email }
func (u *KariUser) GetRegistration() *registration.Resource { return u.Registration }
func (u *KariUser) GetPrivateKey() crypto.PrivateKey        { return u.key }

// ==============================================================================
// 2. Custom gRPC Challenge Provider
// ==============================================================================

type KariChallengeProvider struct {
	ctx         context.Context // Preserves cancellation SLA
	AgentClient pb.SystemAgentClient
	WebRoot     string
	WebUser     string // Injected dynamically
	WebGroup    string // Injected dynamically
}

func (p *KariChallengeProvider) Present(domain, token, keyAuth string) error {
	// 🛡️ Zero-Trust Input Validation
	if strings.Contains(token, "..") || strings.Contains(token, "/") {
		return fmt.Errorf("SECURITY VIOLATION: Invalid ACME token format")
	}

	path := http01.ChallengePath(token)
	fullPath := fmt.Sprintf("%s/%s", p.WebRoot, path)

	// Pass the parent context with a hard timeout to prevent hanging the Muscle
	ctx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
	defer cancel()

	_, err := p.AgentClient.WriteSystemFile(ctx, &pb.FileWriteRequest{
		AbsolutePath: fullPath,
		Content:      []byte(keyAuth),
		Owner:        p.WebUser,
		Group:        p.WebGroup,
		FileMode:     "0644",
	})
	return err
}

func (p *KariChallengeProvider) CleanUp(domain, token, keyAuth string) error {
	if strings.Contains(token, "..") || strings.Contains(token, "/") {
		return fmt.Errorf("SECURITY VIOLATION: Invalid ACME token format")
	}

	path := http01.ChallengePath(token)
	fullPath := fmt.Sprintf("%s/%s", p.WebRoot, path)

	ctx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
	defer cancel()

	_, err := p.AgentClient.ExecutePackageCommand(ctx, &pb.PackageRequest{
		Command: "rm",
		Args:    []string{"-f", fullPath},
	})
	return err
}

// ==============================================================================
// 3. The ACME Adapter Implementation
// ==============================================================================

type AcmeProvider struct {
	Config      *config.Config
	AgentClient pb.SystemAgentClient
	Logger      *slog.Logger
}

func NewAcmeProvider(cfg *config.Config, agent pb.SystemAgentClient, logger *slog.Logger) *AcmeProvider {
	return &AcmeProvider{
		Config:      cfg,
		AgentClient: agent,
		Logger:      logger,
	}
}

func (p *AcmeProvider) ProvisionCertificate(ctx context.Context, email, domainName string) (*certificate.Resource, error) {
	p.Logger.Info("Starting ACME certificate provision", slog.String("domain", domainName))

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate account key: %w", err)
	}

	user := KariUser{
		Email: email,
		key:   privateKey,
	}

	legoCfg := lego.NewConfig(&user)

	// 🛡️ Environment Agnostic: URL injected via configuration
	if p.Config.AcmeDirectoryUrl != "" {
		legoCfg.CADirURL = p.Config.AcmeDirectoryUrl
	}

	client, err := lego.NewClient(legoCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create lego client: %w", err)
	}

	// 🛡️ Platform Agnostic: Injected User/Group and WebRoot
	provider := &KariChallengeProvider{
		ctx:         ctx,
		AgentClient: p.AgentClient,
		WebRoot:     p.Config.WebRoot,
		WebUser:     p.Config.WebUser,
		WebGroup:    p.Config.WebGroup,
	}

	err = client.Challenge.SetHTTP01Provider(provider)
	if err != nil {
		return nil, fmt.Errorf("failed to set http01 provider: %w", err)
	}

	reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
	if err != nil {
		return nil, fmt.Errorf("failed to register ACME account: %w", err)
	}
	user.Registration = reg

	request := certificate.ObtainRequest{
		Domains: []string{domainName},
		Bundle:  true,
	}

	certificates, err := client.Certificate.Obtain(request)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain certificate for %s: %w", domainName, err)
	}

	_, err = p.AgentClient.InstallCertificate(ctx, &pb.SslPayload{
		DomainName:   domainName,
		FullchainPem: certificates.Certificate,
		PrivkeyPem:   certificates.PrivateKey,
	})

	// 🛡️ Memory Safety: Best-Effort Plaintext Zeroing in Go
	// We physically overwrite the byte array with zeros so it is destroyed
	// before the Garbage Collector even runs.
	for i := range certificates.PrivateKey {
		certificates.PrivateKey[i] = 0
	}

	if err != nil {
		return nil, fmt.Errorf("agent failed to install certificate: %w", err)
	}

	p.Logger.Info("✅ SSL Certificate successfully provisioned and installed", slog.String("domain", domainName))
	return certificates, nil
}
