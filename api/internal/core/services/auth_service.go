package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/irgordon/kari/api/internal/core/domain"
	"golang.org/x/crypto/bcrypt"
)

// Precomputed valid bcrypt hash keeps unknown-user and wrong-password paths comparable.
var dummyBcryptHash = []byte("$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy")

type AuthService struct {
	repo         domain.SessionRepository
	tokenService *TokenService
}

func NewAuthService(repo domain.SessionRepository, ts *TokenService) *AuthService {
	return &AuthService{repo: repo, tokenService: ts}
}

func refreshHash(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func (s *AuthService) sessionTokens(user *domain.User, sessionID uuid.UUID) (*domain.TokenPair, error) {
	access, err := s.tokenService.GenerateSessionAccessToken(user, sessionID)
	if err != nil {
		return nil, err
	}
	entropy := make([]byte, 32)
	if _, err := rand.Read(entropy); err != nil {
		return nil, err
	}
	return &domain.TokenPair{AccessToken: access, RefreshToken: base64.RawURLEncoding.EncodeToString(entropy)}, nil
}

func (s *AuthService) GenerateTokenPair(ctx context.Context, user *domain.User) (*domain.TokenPair, error) {
	sessionID := uuid.New()
	tokens, err := s.sessionTokens(user, sessionID)
	if err != nil {
		return nil, err
	}
	err = s.repo.CreateSession(ctx, &domain.Session{ID: sessionID, UserID: user.ID,
		RefreshHash: refreshHash(tokens.RefreshToken), ExpiresAt: time.Now().Add(7 * 24 * time.Hour)})
	return tokens, err
}

func (s *AuthService) Login(ctx context.Context, email, password string) (*domain.TokenPair, *domain.User, error) {
	user, err := s.repo.GetByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		_ = bcrypt.CompareHashAndPassword(dummyBcryptHash, []byte(password))
		if errors.Is(err, domain.ErrNotFound) {
			return nil, nil, domain.ErrInvalidCredentials
		}
		return nil, nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil || !user.IsActive {
		return nil, nil, domain.ErrInvalidCredentials
	}
	tokens, err := s.GenerateTokenPair(ctx, user)
	return tokens, user, err
}

func (s *AuthService) RefreshTokens(ctx context.Context, refreshToken string) (*domain.TokenPair, error) {
	session, err := s.repo.FindSession(ctx, refreshHash(refreshToken))
	if err != nil {
		return nil, err
	}
	user, err := s.repo.GetByID(ctx, session.UserID)
	if err != nil {
		return nil, err
	}
	if !user.IsActive {
		return nil, domain.ErrInvalidCredentials
	}
	tokens, err := s.sessionTokens(user, session.ID)
	if err != nil {
		return nil, err
	}
	// Compare-and-swap ensures only one concurrent refresh can consume this token.
	err = s.repo.RotateSession(ctx, session.ID, session.RefreshHash, refreshHash(tokens.RefreshToken))
	return tokens, err
}

func (s *AuthService) ValidateAccessToken(ctx context.Context, token string) (*domain.UserClaims, error) {
	claims, err := s.tokenService.ValidateAccessToken(token)
	if err != nil {
		return nil, domain.ErrInvalidCredentials
	}
	active, err := s.repo.SessionActive(ctx, claims.SessionID, claims.UserID)
	if err != nil {
		return nil, err
	}
	if !active {
		return nil, domain.ErrInvalidCredentials
	}
	// Read current permissions so role changes take effect without waiting for JWT expiry.
	user, err := s.repo.GetByID(ctx, claims.UserID)
	if err != nil {
		return nil, err
	}
	claims.Permissions = user.Permissions
	return claims, nil
}

func (s *AuthService) CurrentUser(ctx context.Context, token string) (*domain.User, error) {
	claims, err := s.ValidateAccessToken(ctx, token)
	if err != nil {
		return nil, err
	}
	return s.repo.GetByID(ctx, claims.UserID)
}

func (s *AuthService) Logout(ctx context.Context, refreshToken, accessToken string) error {
	if refreshToken == "" && accessToken == "" {
		return nil
	}
	var sessionID, userID uuid.UUID
	// A signed access token identifies the session even if refresh just rotated.
	if claims, err := s.tokenService.ValidateAccessToken(accessToken); err == nil {
		sessionID, userID = claims.SessionID, claims.UserID
	}
	return s.repo.RevokeSession(ctx, refreshHash(refreshToken), sessionID, userID)
}
