package services

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"kari/api/internal/core/domain"
)

// 🛡️ Custom Claims containing our business logic
type KariClaims struct {
	Rank        string   `json:"rank"`
	Permissions []string `json:"permissions"`
	Email       string   `json:"email"`
	jwt.RegisteredClaims
}

type TokenService struct {
	secret []byte
}

func NewTokenService(secret string) *TokenService {
	return &TokenService{secret: []byte(secret)}
}

// GenerateAccessToken mints a short-lived token containing the user's exact access rights
func (s *TokenService) GenerateAccessToken(user *domain.User) (string, error) {
	// 🛡️ SLA: The payload is now the Source of Truth for the UI
	claims := KariClaims{
		Rank:        user.Rank,
		Permissions: user.Permissions,
		Email:       user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID.String(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "kari-brain",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	
	// 🛡️ Cryptographic Sealing
	// If the user tries to modify their 'Rank' to 'admin', this signature becomes invalid.
	signedToken, err := token.SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("failed to sign access token: %w", err)
	}

	return signedToken, nil
}
