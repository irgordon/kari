package services

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/irgordon/kari/api/internal/core/domain"
)

// KariClaims holds the stateless authorization data.
type KariClaims struct {
	Rank        string   `json:"rank,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
	Email       string   `json:"email,omitempty"`
	TokenType   string   `json:"token_type"` // 🛡️ SLA: Distinguish between 'access' and 'refresh'
	jwt.RegisteredClaims
}

// TokenService orchestrates cryptographic identity for the Brain.
type TokenService struct {
	secret []byte
}

// NewTokenService creates a new symmetric-key token service.
func NewTokenService(secret string) *TokenService {
	return &TokenService{secret: []byte(secret)}
}

// GenerateTokenPair mints both the short-lived access token and the long-lived refresh token.
func (s *TokenService) GenerateTokenPair(user *domain.User) (string, string, error) {
	now := time.Now()
	// 🛡️ Stability: 5-second clock skew allowance for distributed systems
	nbf := jwt.NewNumericDate(now.Add(-5 * time.Second))

	// 1. 🛡️ Mint Access Token (15 Minutes) - Contains full RBAC data
	accessClaims := KariClaims{
		Rank:        user.Rank,
		Permissions: user.Permissions,
		Email:       user.Email,
		TokenType:   "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID.String(),
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: nbf,
			Issuer:    "kari-brain",
		},
	}
	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	signedAccess, err := accessToken.SignedString(s.secret)
	if err != nil {
		return "", "", fmt.Errorf("failed to sign access token: %w", err)
	}

	// 2. 🛡️ Mint Refresh Token (7 Days) - Stripped down, purely for session renewal
	refreshClaims := KariClaims{
		TokenType: "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID.String(),
			ExpiresAt: jwt.NewNumericDate(now.Add(7 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: nbf,
			Issuer:    "kari-brain",
			ID:        uuid.New().String(), // JTI for potential database revocation
		},
	}
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	signedRefresh, err := refreshToken.SignedString(s.secret)
	if err != nil {
		return "", "", fmt.Errorf("failed to sign refresh token: %w", err)
	}

	return signedAccess, signedRefresh, nil
}

func (s *TokenService) GenerateAccessToken(user *domain.User) (string, error) {
	accessToken, _, err := s.GenerateTokenPair(user)
	return accessToken, err
}

func (s *TokenService) ValidateAccessToken(tokenString string) (*domain.UserClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &KariClaims{}, func(token *jwt.Token) (interface{}, error) {
		return s.secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer("kari-brain"), jwt.WithExpirationRequired())
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*KariClaims)
	if !ok || !token.Valid || claims.TokenType != "access" {
		return nil, fmt.Errorf("invalid access token")
	}
	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return nil, err
	}
	return &domain.UserClaims{
		UserID:      userID,
		Subject:     userID,
		Permissions: claims.Permissions,
	}, nil
}

// VerifyRefreshToken validates the signature, expiry, algorithm, issuer, and token type.
func (s *TokenService) VerifyRefreshToken(tokenString string) (uuid.UUID, error) {
	// 🛡️ Zero-Trust: We utilize v5's parser options to strictly enforce cryptographic boundaries
	token, err := jwt.ParseWithClaims(tokenString, &KariClaims{}, func(token *jwt.Token) (interface{}, error) {
		return s.secret, nil
	},
		jwt.WithValidMethods([]string{"HS256"}), // Explicitly reject HS512, none, RS256, etc.
		jwt.WithIssuer("kari-brain"),            // Explicitly reject tokens minted by other services
		jwt.WithExpirationRequired(),            // Reject tokens missing the 'exp' claim
	)

	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid token signature, expired, or failed claim validation: %w", err)
	}

	claims, ok := token.Claims.(*KariClaims)
	if !ok || !token.Valid {
		return uuid.Nil, fmt.Errorf("invalid token claims structure")
	}

	// 🛡️ Explicitly prevent an Access token from being used as a Refresh token
	if claims.TokenType != "refresh" {
		return uuid.Nil, fmt.Errorf("invalid token type: expected refresh, got %s", claims.TokenType)
	}

	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return uuid.Nil, fmt.Errorf("malformed subject claim: not a valid UUID")
	}

	return userID, nil
}
