package adapters

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"
	"github.com/irgordon/kari/api/internal/config"
	pb "github.com/irgordon/kari/api/internal/grpc/rustagent"
)

type KariUser struct {
	Email        string
	Registration *registration.Resource
	key          crypto.PrivateKey
}

func (u *KariUser) GetEmail() string                        { return u.Email }
func (u *KariUser) GetRegistration() *registration.Resource { return u.Registration }
func (u *KariUser) GetPrivateKey() crypto.PrivateKey        { return u.key }

type KariChallengeProvider struct {
	ctx         context.Context
	AgentClient pb.SystemAgentClient
	SiteID      string
}

func (p *KariChallengeProvider) Present(domain, token, key string) error {
	return p.change(domain, token, key, false)
}
func (p *KariChallengeProvider) CleanUp(domain, token, key string) error {
	return p.change(domain, token, key, true)
}
func (p *KariChallengeProvider) change(domain, token, key string, remove bool) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(p.ctx), 10*time.Second)
	defer cancel()
	response, err := p.AgentClient.SetSiteChallenge(ctx, &pb.SiteChallenge{SiteId: p.SiteID, DomainName: domain, Token: token, KeyAuthorization: key, Remove: remove})
	if err == nil && !response.Success {
		return errors.New("agent rejected ACME challenge")
	}
	return err
}

type AcmeProvider struct {
	Config      *config.Config
	AgentClient pb.SystemAgentClient
	Logger      *slog.Logger
}

func NewAcmeProvider(cfg *config.Config, agent pb.SystemAgentClient, logger *slog.Logger) *AcmeProvider {
	return &AcmeProvider{cfg, agent, logger}
}

// Requests carry the operation context even inside Lego's polling and HTTP calls.
type acmeTransport struct{ ctx context.Context }

func (t acmeTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return http.DefaultTransport.RoundTrip(r.WithContext(t.ctx))
}

func (p *AcmeProvider) ProvisionSiteCertificate(ctx context.Context, site, email, hostname string) (time.Time, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return time.Time{}, err
	}
	user := KariUser{Email: email, key: key}
	cfg := lego.NewConfig(&user)
	if p.Config.AcmeDirectoryUrl != "" {
		cfg.CADirURL = p.Config.AcmeDirectoryUrl
	}
	cfg.HTTPClient = &http.Client{Timeout: 20 * time.Second, Transport: acmeTransport{ctx}}
	cfg.Certificate.Timeout = 90 * time.Second
	client, err := lego.NewClient(cfg)
	if err != nil {
		return time.Time{}, err
	}
	provider := &KariChallengeProvider{ctx: ctx, AgentClient: p.AgentClient, SiteID: site}
	if err = client.Challenge.SetHTTP01Provider(provider); err != nil {
		return time.Time{}, err
	}
	user.Registration, err = client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
	if err != nil {
		return time.Time{}, err
	}
	cert, err := client.Certificate.Obtain(certificate.ObtainRequest{Domains: []string{hostname}, Bundle: true})
	if err != nil {
		return time.Time{}, err
	}
	defer clear(cert.PrivateKey)
	block, _ := pem.Decode(cert.Certificate)
	if block == nil {
		return time.Time{}, errors.New("ACME returned invalid PEM")
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}, err
	}
	response, err := p.AgentClient.ActivateSiteTLS(ctx, &pb.SiteCertificate{SiteId: site, DomainName: hostname, FullchainPem: cert.Certificate, PrivkeyPem: cert.PrivateKey})
	if err == nil && !response.Success {
		err = errors.New("agent rejected TLS activation")
	}
	return leaf.NotAfter, err
}
