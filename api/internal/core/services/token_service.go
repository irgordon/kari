package services

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"kari/api/internal/core/domain"
)

// KariClaims holds the stateless authorization data
type KariClaims struct {
	Rank        string   `json:"rank,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
	Email       string   `json:"email,omitempty"`
	TokenType   string   `json:"token_type"` // 🛡️ SLA: Distinguish between 'access' and 'refresh'
	jwt.RegisteredClaims
}

type TokenService struct {
	secret []byte
}

func NewTokenService(secret string) *TokenService {
	return &TokenService{secret: []byte(secret)}
}

// GenerateTokenPair mints both the short-lived access token and the long-lived refresh token
func (s *TokenService) GenerateTokenPair(user *domain.User) (string, string, error) {
	// 1. 🛡️ Mint Access Token (15 Minutes) - Contains full RBAC data
	accessClaims := KariClaims{
		Rank:        user.Rank,
		Permissions: user.Permissions,
		Email:       user.Email,
		TokenType:   "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID.String(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "kari-brain",
		},
	}
	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	signedAccess, err := accessToken.SignedString(s.secret)
	if err != nil {
		return "", "", fmt.Errorf("failed to sign access token: %w", err)
	}

	// 2. 🛡️ Mint Refresh Token (7 Days) - Only contains the Subject ID
	refreshClaims := KariClaims{
		TokenType: "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID.String(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "kari-brain",
			ID:        uuid.New().String(), // JTI for potential database revocation tracking
		},
	}
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	signedRefresh, err := refreshToken.SignedString(s.secret)
	if err != nil {
		return "", "", fmt.Errorf("failed to sign refresh token: %w", err)
	}

	return signedAccess, signedRefresh, nil
}

// VerifyRefreshToken validates the signature, expiry, and token type
func (s *TokenService) VerifyRefreshToken(tokenString string) (uuid.UUID, error) {
	token, err := jwt.ParseWithClaims(tokenString, &KariClaims{}, func(token *jwt.Token) (interface{}, error) {
		// 🛡️ Zero-Trust: Force the signing method check
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.secret, nil
	})

	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid token signature or expired: %w", err)
	}

	claims, ok := token.Claims.(*KariClaims)
	if !ok || !token.Valid {
		return uuid.Nil, fmt.Errorf("invalid token claims")
	}

	// 🛡️ Explicitly prevent an Access token from being used as a Refresh token
	if claims.TokenType != "refresh" {
		return uuid.Nil, fmt.Errorf("invalid token type: expected refresh")
	}

	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return uuid.Nil, fmt.Errorf("malformed subject claim")
	}

	return userID, nil
}
