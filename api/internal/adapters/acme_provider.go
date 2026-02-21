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

	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge/http01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"

	"kari/api/internal/config"
	"kari/api/internal/grpc/rustagent"
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
func (u *KariUser) GetPrivateKey() crypto.PrivateKey       { return u.key }

// ==============================================================================
// 2. Custom gRPC Challenge Provider
// ==============================================================================

/**
 * KariChallengeProvider implements the lego.Provider interface.
 * Instead of writing to a local disk, it asks the Rust Agent to write the 
 * challenge token to the web root via gRPC.
 */
type KariChallengeProvider struct {
	AgentClient rustagent.SystemAgentClient
	WebRoot     string // e.g., "/var/www/html"
}

func (p *KariChallengeProvider) Present(domain, token, keyAuth string) error {
	path := http01.ChallengePath(token)
	fullPath := fmt.Sprintf("%s/%s", p.WebRoot, path)

	_, err := p.AgentClient.WriteSystemFile(context.Background(), &rustagent.FileWriteRequest{
		AbsolutePath: fullPath,
		Content:      []byte(keyAuth),
		Owner:        "www-data",
		Group:        "www-data",
		FileMode:     "0644",
	})
	return err
}

func (p *KariChallengeProvider) CleanUp(domain, token, keyAuth string) error {
	path := http01.ChallengePath(token)
	fullPath := fmt.Sprintf("%s/%s", p.WebRoot, path)

	_, err := p.AgentClient.ExecutePackageCommand(context.Background(), &rustagent.PackageRequest{
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
	AgentClient rustagent.SystemAgentClient
	Logger      *slog.Logger
}

func NewAcmeProvider(cfg *config.Config, agent rustagent.SystemAgentClient, logger *slog.Logger) *AcmeProvider {
	return &AcmeProvider{
		Config:      cfg,
		AgentClient: agent,
		Logger:      logger,
	}
}

func (p *AcmeProvider) ProvisionCertificate(ctx context.Context, email, domainName string) (*certificate.Resource, error) {
	p.Logger.Info("Starting ACME certificate provision", slog.String("domain", domainName))

	// 1. Setup private key for the ACME Account
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate account key: %w", err)
	}

	user := KariUser{
		Email: email,
		key:   privateKey,
	}

	// 2. Initialize Lego Client
	config := lego.NewConfig(&user)
	// Platform Agnostic: Use production or staging based on environment config
	config.CADirURL = "https://acme-v02.api.letsencrypt.org/directory"
	
	client, err := lego.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create lego client: %w", err)
	}

	// 3. Configure HTTP-01 Challenge via our gRPC Proxy
	provider := &KariChallengeProvider{
		AgentClient: p.AgentClient,
		WebRoot:     "/var/www/html", // Standard global webroot for challenges
	}
	err = client.Challenge.SetHTTP01Provider(provider)
	if err != nil {
		return nil, fmt.Errorf("failed to set http01 provider: %w", err)
	}

	// 4. Register Account & Agree to Terms
	reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
	if err != nil {
		return nil, fmt.Errorf("failed to register ACME account: %w", err)
	}
	user.Registration = reg

	// 5. Request Certificate
	request := certificate.ObtainRequest{
		Domains: []string{domainName},
		Bundle:  true,
	}
	
	certificates, err := client.Certificate.Obtain(request)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain certificate for %s: %w", domainName, err)
	}

	// 6. Securely Install via Rust Agent (The final memory-safe write)
	// We pass the bytes to our InstallCertificate gRPC call, which utilizes 
	// the zero-copy/secrecy logic in Rust.
	_, err = p.AgentClient.InstallCertificate(ctx, &rustagent.SslPayload{
		DomainName:   domainName,
		FullchainPem: certificates.Certificate,
		PrivkeyPem:   certificates.PrivateKey,
	})
	if err != nil {
		return nil, fmt.Errorf("agent failed to install certificate: %w", err)
	}

	p.Logger.Info("✅ SSL Certificate successfully provisioned and installed", slog.String("domain", domainName))
	return certificates, nil
}
