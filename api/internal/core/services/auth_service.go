package services

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"kari/api/internal/config"
	"kari/api/internal/core/domain"
)

type AuthService struct {
	repo   domain.UserRepository
	config *config.Config
}

type KariClaims struct {
	Email string `json:"email"`
	Rank  int    `json:"rank"`
	jwt.RegisteredClaims
}

func NewAuthService(repo domain.UserRepository, cfg *config.Config) *AuthService {
	return &AuthService{repo: repo, config: cfg}
}

// GenerateTokenPair issues tokens with higher entropy for the refresh side
func (s *AuthService) GenerateTokenPair(ctx context.Context, user *domain.User) (string, string, error) {
	// 1. Access Token
	accessToken, err := s.generateAccessToken(user)
	if err != nil {
		return "", "", err
	}

	// 🛡️ 2. Secure Refresh Token Generation
	// Instead of UUID, we use 32 bytes of random data (Base64 encoded)
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("failed to generate entropy: %w", err)
	}
	refreshToken := base64.URLEncoding.EncodeToString(b)

	// 🛡️ Use the passed-in context to honor request timeouts
	err = s.repo.UpdateRefreshToken(ctx, user.ID, refreshToken)
	if err != nil {
		return "", "", fmt.Errorf("failed to persist refresh token: %w", err)
	}

	return accessToken, refreshToken, nil
}

func (s *AuthService) Login(ctx context.Context, email, password string) (string, string, error) {
	user, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		return "", "", errors.New("invalid credentials")
	}

	// Constant-time check
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", "", errors.New("invalid credentials")
	}

	if !user.IsActive {
		return "", "", errors.New("account suspended")
	}

	return s.GenerateTokenPair(ctx, user)
}

// ... generateAccessToken remains the same ...
