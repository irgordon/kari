//go:build integration

package contract_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/irgordon/kari/api/internal/api/handlers"
	"github.com/irgordon/kari/api/internal/api/middleware"
	"github.com/irgordon/kari/api/internal/api/router"
	"github.com/irgordon/kari/api/internal/core/services"
	"golang.org/x/crypto/bcrypt"
)

func TestFirstRunAndAuthentication(t *testing.T) {
	suite := newContractSuite(t)
	t.Cleanup(suite.pool.Close)
	suite.reset(t)
	ctx := context.Background()
	requireNoError(t, execStatement(ctx, suite.pool, "DELETE FROM installation"))
	t.Cleanup(func() { _, _ = suite.pool.Exec(ctx, "DELETE FROM installation") })
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	const secret = "integration-signing-secret-only-32-characters"
	const password = "Correct-Horse-42!"
	auth := services.NewAuthService(suite.users, services.NewTokenService(secret))
	server := httptest.NewTLSServer(router.NewRouter(router.RouterConfig{
		AuthHandler:  handlers.NewAuthHandler(auth, true),
		SetupHandler: handlers.NewSetupHandler(suite.users, logger, "local-operator-capability"), Logger: logger,
		AuthMiddleware: middleware.NewAuthMiddleware(auth, services.NewRoleService(suite.users, logger), suite.users, logger),
	}))
	t.Cleanup(server.Close)
	client := server.Client()
	client.Jar, _ = cookiejar.New(nil)
	request := func(method, path, body, token string, want int) *http.Response {
		t.Helper()
		req, err := http.NewRequest(method, server.URL+path, strings.NewReader(body))
		requireNoError(t, err)
		req.Header.Set("Origin", server.URL)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Setup-Token", token)
		response, err := client.Do(req)
		requireNoError(t, err)
		if response.StatusCode != want {
			output, _ := io.ReadAll(response.Body)
			response.Body.Close()
			t.Fatalf("%s %s = %d, want %d: %s", method, path, response.StatusCode, want, output)
		}
		t.Cleanup(func() { response.Body.Close() })
		return response
	}
	setupBody := fmt.Sprintf(`{"admin_email":"Admin@Example.test","admin_password":%q}`, password)
	request("POST", "/api/v1/setup/finalize", setupBody, "wrong-token", 401)
	request("POST", "/api/v1/setup/finalize", `{"admin_email":"bad","admin_password":"weak"}`, "local-operator-capability", 400)
	request("GET", "/api/v1/auth/me", "", "", 409)
	_, err := suite.pool.Exec(ctx, "UPDATE roles SET name = 'temporarily-missing' WHERE name = 'superadmin'")
	requireNoError(t, err)
	request("POST", "/api/v1/setup/finalize", setupBody, "local-operator-capability", 503)
	assertCount(t, suite.pool, "SELECT COUNT(*) FROM installation", 0)
	_, err = suite.pool.Exec(ctx, "UPDATE roles SET name = 'superadmin' WHERE name = 'temporarily-missing'")
	requireNoError(t, err)
	// Both requests cross HTTP and PostgreSQL; exactly one may install.
	var outcomes [2]int
	var wg sync.WaitGroup
	for index := range outcomes {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			req, _ := http.NewRequest("POST", server.URL+"/api/v1/setup/finalize", strings.NewReader(setupBody))
			req.Header.Set("Origin", server.URL)
			req.Header.Set("X-Setup-Token", "local-operator-capability")
			response, err := client.Do(req)
			if err == nil {
				outcomes[index] = response.StatusCode
				response.Body.Close()
			}
		}(index)
	}
	wg.Wait()
	if !((outcomes[0] == 201 && outcomes[1] == 409) || (outcomes[1] == 201 && outcomes[0] == 409)) {
		t.Fatalf("concurrent setup = %v", outcomes)
	}
	assertCount(t, suite.pool, "SELECT COUNT(*) FROM users", 1)
	assertCount(t, suite.pool, "SELECT COUNT(*) FROM installation", 1)
	request("POST", "/api/v1/setup/finalize", setupBody, "local-operator-capability", 409)
	user, err := suite.users.GetByEmail(ctx, "admin@example.test")
	requireNoError(t, err)
	requireNoError(t, bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)))
	if len(user.Permissions) != 9 {
		t.Fatalf("administrator permissions = %v", user.Permissions)
	}
	request("POST", "/api/v1/auth/login", `{"email":"admin@example.test","password":"wrong-password"}`, "", 401)
	login := request("POST", "/api/v1/auth/login", fmt.Sprintf(`{"email":"admin@example.test","password":%q}`, password), "", 200)
	output, err := io.ReadAll(login.Body)
	requireNoError(t, err)
	if strings.Contains(string(output), "password") || strings.Contains(string(output), "token") {
		t.Fatal("login exposed sensitive fields")
	}
	var access, refresh string
	for _, cookie := range login.Cookies() {
		if cookie.MaxAge < 0 {
			continue
		}
		if !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
			t.Fatalf("unsafe cookie %s", cookie.Name)
		}
		if cookie.Name == "kari_access_token" {
			access = cookie.Value
		}
		if cookie.Name == "kari_refresh_token" {
			refresh = cookie.Value
		}
	}
	if access == "" || refresh == "" {
		t.Fatal("missing session cookies")
	}
	hash := sha256.Sum256([]byte(refresh))
	session, err := suite.users.FindSession(ctx, hex.EncodeToString(hash[:]))
	requireNoError(t, err)
	if session.RefreshHash == refresh || time.Until(session.ExpiresAt) < 6*24*time.Hour {
		t.Fatal("invalid refresh persistence or expiry")
	}
	request("GET", "/api/v1/auth/me", "", "", 200)
	expired := services.KariClaims{SessionID: session.ID.String(), TokenType: "access", RegisteredClaims: jwt.RegisteredClaims{
		Subject: user.ID.String(), Issuer: "kari-brain", ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute))}}
	expiredToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, expired).SignedString([]byte(secret))
	requireNoError(t, err)
	base, _ := http.NewRequest("GET", server.URL, nil)
	client.Jar.SetCookies(base.URL, []*http.Cookie{{Name: "kari_access_token", Value: expiredToken, Path: "/", Secure: true}})
	request("GET", "/api/v1/auth/me", "", "", 401)
	request("POST", "/api/v1/auth/refresh", "{}", "", 200)
	request("GET", "/api/v1/auth/me", "", "", 200)
	if _, err := auth.RefreshTokens(ctx, refresh); err == nil {
		t.Fatal("rotated refresh token replay accepted")
	}
	tokens, err := auth.GenerateTokenPair(ctx, user)
	requireNoError(t, err)
	successes := make(chan bool, 2)
	for i := 0; i < 2; i++ {
		go func() { _, err := auth.RefreshTokens(ctx, tokens.RefreshToken); successes <- err == nil }()
	}
	first, second := <-successes, <-successes
	if first == second {
		t.Fatal("concurrent refresh did not have exactly one winner")
	}
	foreign, _ := http.NewRequest("POST", server.URL+"/api/v1/auth/logout", nil)
	foreign.Header.Set("Origin", "https://attacker.example")
	response, err := client.Do(foreign)
	requireNoError(t, err)
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatalf("cross-origin logout = %d", response.StatusCode)
	}
	request("POST", "/api/v1/auth/logout", "{}", "", 200)
	request("GET", "/api/v1/auth/me", "", "", 401)
	if _, err := auth.ValidateAccessToken(ctx, access); err == nil {
		t.Fatal("logout left captured access token valid")
	}
	tokens, err = auth.GenerateTokenPair(ctx, user)
	requireNoError(t, err)
	requireNoError(t, auth.Logout(ctx, tokens.RefreshToken, tokens.AccessToken))
	if _, err := auth.RefreshTokens(ctx, tokens.RefreshToken); err == nil {
		t.Fatal("revoked refresh accepted")
	}
	tokens, err = auth.GenerateTokenPair(ctx, user)
	requireNoError(t, err)
	rotated, err := auth.RefreshTokens(ctx, tokens.RefreshToken)
	requireNoError(t, err)
	// Logout may arrive carrying the previous refresh after another tab rotated it.
	requireNoError(t, auth.Logout(ctx, tokens.RefreshToken, tokens.AccessToken))
	if _, err := auth.ValidateAccessToken(ctx, rotated.AccessToken); err == nil {
		t.Fatal("logout after concurrent rotation left session active")
	}
	if _, err := auth.RefreshTokens(ctx, rotated.RefreshToken); err == nil {
		t.Fatal("logout after concurrent rotation left refresh valid")
	}
	tokens, err = auth.GenerateTokenPair(ctx, user)
	requireNoError(t, err)
	_, err = suite.pool.Exec(ctx, "UPDATE users SET is_active = FALSE WHERE id = $1", user.ID)
	requireNoError(t, err)
	if _, err := auth.ValidateAccessToken(ctx, tokens.AccessToken); err == nil {
		t.Fatal("disabled account access accepted")
	}
	if _, err := auth.RefreshTokens(ctx, tokens.RefreshToken); err == nil {
		t.Fatal("disabled account refresh accepted")
	}
	_, err = suite.pool.Exec(ctx, "UPDATE users SET is_active = TRUE, role_id = $2 WHERE id = $1", user.ID, viewerRoleID)
	requireNoError(t, err)
	claims, err := auth.ValidateAccessToken(ctx, tokens.AccessToken)
	requireNoError(t, err)
	for _, permission := range claims.Permissions {
		if permission == "applications:deploy" {
			t.Fatal("demotion retained deployment permission")
		}
	}
	_, err = suite.pool.Exec(ctx, "UPDATE auth_sessions SET expires_at = NOW() - INTERVAL '1 second'")
	requireNoError(t, err)
	if _, err := auth.RefreshTokens(ctx, tokens.RefreshToken); err == nil {
		t.Fatal("expired refresh accepted")
	}
	if _, err := auth.ValidateAccessToken(ctx, tokens.AccessToken); err == nil {
		t.Fatal("expired session access accepted")
	}
}
