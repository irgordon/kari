package services

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/irgordon/kari/api/internal/core/domain"
	"github.com/irgordon/kari/api/internal/core/utils"
	"github.com/irgordon/kari/api/internal/db/postgres"
	"github.com/irgordon/kari/api/internal/infrastructure/mail"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrEmailDisabled  = errors.New("email access is not configured")
	ErrEmailBusy      = errors.New("email requests are busy; try again later")
	ErrPasswordPolicy = errors.New("password policy")
)

type emailRequest struct {
	email, purpose string
	actor          uuid.UUID
}
type EmailAccess struct {
	repo     *postgres.UserRepo
	auth     *AuthService
	mailer   *mail.SMTP
	baseURL  string
	logger   *slog.Logger
	requests chan emailRequest
}

func NewEmailAccess(repo *postgres.UserRepo, auth *AuthService, mailer *mail.SMTP, publicURL string, development bool, logger *slog.Logger) (*EmailAccess, error) {
	if mailer != nil {
		base, err := url.Parse(publicURL)
		if err != nil || len(publicURL) > 2048 || base.Hostname() == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || (base.Path != "" && base.Path != "/") {
			return nil, errors.New("public URL must be an absolute origin without credentials, query, or fragment")
		}
		ip := net.ParseIP(base.Hostname())
		local := base.Hostname() == "localhost" || (ip != nil && ip.IsLoopback())
		if base.Scheme != "https" && !(development && base.Scheme == "http" && local) {
			return nil, errors.New("public email URL requires HTTPS")
		}
	}
	return &EmailAccess{repo: repo, auth: auth, mailer: mailer, baseURL: strings.TrimRight(publicURL, "/"), logger: logger, requests: make(chan emailRequest, 64)}, nil
}

func (s *EmailAccess) Enabled() bool { return s.mailer != nil }

func (s *EmailAccess) Request(ctx context.Context, email, purpose string, actor uuid.UUID) error {
	if !s.Enabled() {
		return ErrEmailDisabled
	}
	if purpose != "magic" && purpose != "reset" && purpose != "verify" {
		return domain.ErrInvalidCredentials
	}
	if purpose == "verify" && actor == uuid.Nil {
		return domain.ErrInvalidCredentials
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case s.requests <- emailRequest{strings.ToLower(strings.TrimSpace(email)), purpose, actor}:
		return nil
	default:
		return ErrEmailBusy
	}
}

// An owned, bounded worker keeps SMTP and account-existence timing out of public
// request responses. One request means one delivery attempt, not a retry framework.
func (s *EmailAccess) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case request := <-s.requests:
			if err := s.deliver(ctx, request); err != nil && ctx.Err() == nil {
				// SMTP responses can contain provider-supplied content. Never log tokens or recipients.
				s.logger.Error("Account email delivery failed", "purpose", request.purpose)
			}
		}
	}
}

func (s *EmailAccess) deliver(ctx context.Context, request emailRequest) error {
	user, err := s.repo.GetByEmail(ctx, request.email)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !user.IsActive || (request.purpose == "verify" && request.actor != user.ID) || (request.purpose != "verify" && !user.EmailVerified) {
		return nil
	}
	entropy := make([]byte, 32)
	if _, err = rand.Read(entropy); err != nil {
		return err
	}
	token := base64.RawURLEncoding.EncodeToString(entropy)
	issued, err := s.repo.IssueEmailToken(ctx, user, request.purpose, refreshHash(token))
	if err != nil || !issued {
		return err
	}
	link := s.baseURL + "/auth/email/" + request.purpose + "#token=" + token
	subject := map[string]string{"verify": "Verify your Karı email", "reset": "Reset your Karı password", "magic": "Sign in to Karı"}[request.purpose]
	body := subject + "\n\nOpen this link and confirm the action:\n" + link + "\n\nThis link expires in 15 minutes and can be used once. If you did not request it, ignore this email.\n"
	return s.mailer.Send(ctx, user.Email, subject, body)
}

func (s *EmailAccess) Verify(ctx context.Context, actor uuid.UUID, token string) error {
	if len(token) != 43 {
		return domain.ErrInvalidCredentials
	}
	return s.repo.ConsumeEmailToken(ctx, refreshHash(token), "verify", actor, "", nil)
}

func (s *EmailAccess) Reset(ctx context.Context, token, password string) error {
	if len(password) > 72 {
		return fmt.Errorf("%w: use at most 72 bytes", ErrPasswordPolicy)
	}
	if err := utils.ValidatePasswordComplexity(password); err != nil {
		return fmt.Errorf("%w: %v", ErrPasswordPolicy, err)
	}
	if len(token) != 43 {
		return domain.ErrInvalidCredentials
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.repo.ConsumeEmailToken(ctx, refreshHash(token), "reset", uuid.Nil, string(hash), nil)
}

func (s *EmailAccess) MagicLogin(ctx context.Context, token string) (*domain.TokenPair, *domain.User, error) {
	if len(token) != 43 {
		return nil, nil, domain.ErrInvalidCredentials
	}
	user, err := s.repo.EmailTokenUser(ctx, refreshHash(token), "magic")
	if err != nil {
		return nil, nil, err
	}
	tokens, session, err := s.auth.prepareLogin(user)
	if err != nil {
		return nil, nil, err
	}
	err = s.repo.ConsumeEmailToken(ctx, refreshHash(token), "magic", uuid.Nil, "", session)
	return tokens, user, err
}
