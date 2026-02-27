package services_test

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kari/api/internal/core/domain"
	"kari/api/internal/core/services"
)

const (
	testSecret = "super-secret-key-for-testing-purposes-1234567890"
)

func TestTokenService_GenerateTokenPair(t *testing.T) {
	// 1. Setup
	tokenService := services.NewTokenService(testSecret)
	userID := uuid.New()
	user := &domain.User{
		ID:    userID,
		Email: "test@kari.dev",
		Rank:  "admin",
		Permissions: []string{"read:users", "write:users"},
	}

	// 2. Execution
	accessTokenString, refreshTokenString, err := tokenService.GenerateTokenPair(user)

	// 3. Verification
	require.NoError(t, err)
	assert.NotEmpty(t, accessTokenString)
	assert.NotEmpty(t, refreshTokenString)

	// 3a. Verify Access Token Claims
	token, err := jwt.ParseWithClaims(accessTokenString, &services.KariClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(testSecret), nil
	})
	require.NoError(t, err)
	require.True(t, token.Valid)

	claims, ok := token.Claims.(*services.KariClaims)
	require.True(t, ok)

	assert.Equal(t, "access", claims.TokenType)
	assert.Equal(t, userID.String(), claims.Subject)
	assert.Equal(t, "kari-brain", claims.Issuer)
	assert.Equal(t, "test@kari.dev", claims.Email)
	assert.Equal(t, "admin", claims.Rank)
	assert.Equal(t, []string{"read:users", "write:users"}, claims.Permissions)

	// Verify Expiration (approx 15 mins)
	expectedExp := time.Now().Add(15 * time.Minute)
	assert.WithinDuration(t, expectedExp, claims.ExpiresAt.Time, 5*time.Second)

	// 3b. Verify Refresh Token Claims
	refreshToken, err := jwt.ParseWithClaims(refreshTokenString, &services.KariClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(testSecret), nil
	})
	require.NoError(t, err)
	require.True(t, refreshToken.Valid)

	refreshClaims, ok := refreshToken.Claims.(*services.KariClaims)
	require.True(t, ok)

	assert.Equal(t, "refresh", refreshClaims.TokenType)
	assert.Equal(t, userID.String(), refreshClaims.Subject)
	assert.Equal(t, "kari-brain", refreshClaims.Issuer)
	assert.NotEmpty(t, refreshClaims.ID) // JTI should be present

	// Verify Expiration (approx 7 days)
	expectedRefreshExp := time.Now().Add(7 * 24 * time.Hour)
	assert.WithinDuration(t, expectedRefreshExp, refreshClaims.ExpiresAt.Time, 5*time.Second)
}

func TestTokenService_VerifyRefreshToken(t *testing.T) {
	tokenService := services.NewTokenService(testSecret)
	userID := uuid.New()
	user := &domain.User{
		ID:    userID,
		Email: "test@kari.dev",
	}

	// Generate valid pair
	accessToken, refreshToken, _ := tokenService.GenerateTokenPair(user)

	t.Run("Valid Refresh Token", func(t *testing.T) {
		uid, err := tokenService.VerifyRefreshToken(refreshToken)
		require.NoError(t, err)
		assert.Equal(t, userID, uid)
	})

	t.Run("Invalid: Use Access Token as Refresh Token", func(t *testing.T) {
		uid, err := tokenService.VerifyRefreshToken(accessToken)
		assert.Error(t, err)
		assert.Equal(t, uuid.Nil, uid)
		assert.Contains(t, err.Error(), "invalid token type")
	})

	t.Run("Invalid: Wrong Secret", func(t *testing.T) {
		// Sign a token with a different secret
		otherService := services.NewTokenService("wrong-secret-key")
		_, otherRefresh, _ := otherService.GenerateTokenPair(user)

		uid, err := tokenService.VerifyRefreshToken(otherRefresh)
		assert.Error(t, err)
		assert.Equal(t, uuid.Nil, uid)
		assert.Contains(t, err.Error(), "signature is invalid")
	})

	t.Run("Invalid: Malformed Token", func(t *testing.T) {
		uid, err := tokenService.VerifyRefreshToken("not.a.valid.token")
		assert.Error(t, err)
		assert.Equal(t, uuid.Nil, uid)
	})
}
