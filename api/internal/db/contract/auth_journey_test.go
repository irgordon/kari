//go:build integration

package contract_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/irgordon/kari/api/internal/api/handlers"
	"github.com/irgordon/kari/api/internal/api/middleware"
	"github.com/irgordon/kari/api/internal/api/router"
	"github.com/irgordon/kari/api/internal/core/services"
	karidb "github.com/irgordon/kari/api/internal/db"
	"github.com/irgordon/kari/api/internal/infrastructure/crypto"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pquerna/otp/totp"
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

func TestMigrationRequiresRestorableDatabaseBackup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	suite := newContractSuite(t)
	t.Cleanup(suite.pool.Close)
	address, err := url.Parse(os.Getenv("KARI_TEST_DATABASE_URL"))
	requireNoError(t, err)
	if address.Hostname() != "127.0.0.1" || address.Path != "/kari_contract" {
		t.Fatal("upgrade fixture requires the disposable contract database")
	}
	_, err = suite.pool.Exec(ctx, "CREATE DATABASE kari_upgrade_guard")
	requireNoError(t, err)
	address.Path = "/kari_upgrade_guard"
	pool, err := pgxpool.New(ctx, address.String())
	requireNoError(t, err)
	t.Cleanup(func() {
		pool.Close()
		_, _ = suite.pool.Exec(context.Background(), "DROP DATABASE kari_upgrade_guard WITH (FORCE)")
	})
	plan, err := karidb.LoadMigrationPlan()
	requireNoError(t, err)
	_, err = pool.Exec(ctx, `CREATE TABLE schema_migrations(version BIGINT PRIMARY KEY,name TEXT NOT NULL UNIQUE,checksum TEXT NOT NULL,applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP)`)
	requireNoError(t, err)
	// Construct the previously shipped schema from its unchanged canonical SQL.
	for _, migration := range plan[:6] {
		_, err = pool.Exec(ctx, migration.SQL)
		requireNoError(t, err)
		_, err = pool.Exec(ctx, `INSERT INTO schema_migrations(version,name,checksum) VALUES($1,$2,$3)`, migration.Version, migration.Name, migration.Checksum)
		requireNoError(t, err)
	}
	root, err := filepath.Abs("../../../..")
	requireNoError(t, err)
	run := func(proof string) ([]byte, error) {
		command := exec.CommandContext(ctx, "go", "run", "./api/cmd/migrate")
		command.Dir = root
		command.Env = append(os.Environ(), "DATABASE_URL="+address.String(), "KARI_MIGRATION_BACKUP="+proof)
		return command.CombinedOutput()
	}
	output, err := run("")
	if err == nil || !strings.Contains(string(output), "verified control-plane backup") {
		t.Fatalf("unbacked migration was not rejected: %v", err)
	}
	version, err := karidb.SchemaVersion(ctx, pool)
	requireNoError(t, err)
	if version != 6 {
		t.Fatal("rejected upgrade modified the database")
	}
	containers, err := exec.CommandContext(ctx, "docker", "ps", "--filter", "name=kari-contract-", "--format", "{{.ID}} {{.Ports}}").Output()
	requireNoError(t, err)
	container := ""
	for _, line := range strings.Split(string(containers), "\n") {
		if strings.Contains(line, "127.0.0.1:"+address.Port()+"->5432") {
			container = strings.Fields(line)[0]
		}
	}
	if container == "" {
		t.Fatal("cannot identify owned disposable PostgreSQL container")
	}
	dir := t.TempDir()
	file, err := os.Create(filepath.Join(dir, "database.dump"))
	requireNoError(t, err)
	dump := exec.CommandContext(ctx, "docker", "exec", container, "pg_dump", "--username=kari", "--dbname=kari_upgrade_guard", "--format=custom")
	dump.Stdout = file
	requireNoError(t, dump.Run())
	requireNoError(t, file.Close())
	data, err := os.ReadFile(filepath.Join(dir, "database.dump"))
	requireNoError(t, err)
	hash := sha256.Sum256(data)
	proof, err := json.Marshal(karidb.BackupProof{SchemaVersion: 6, Version: "fixture-previous", Files: map[string]string{"database.dump": hex.EncodeToString(hash[:])}})
	requireNoError(t, err)
	proofPath := filepath.Join(dir, "backup.json")
	requireNoError(t, os.WriteFile(proofPath, proof, 0600))
	output, err = run(proofPath)
	if err != nil {
		t.Fatalf("backed migration failed: %v: %s", err, output)
	}
	version, err = karidb.SchemaVersion(ctx, pool)
	requireNoError(t, err)
	if version != 8 {
		t.Fatal("backed migration did not reach current schema")
	}
}

func TestMFAAndEmergencyRecovery(t *testing.T) {
	suite := newContractSuite(t)
	t.Cleanup(suite.pool.Close)
	suite.reset(t)
	ctx := context.Background()
	id := insertUser(t, suite.pool, superadminRoleID, "mfa-owner")
	const password = "Owner-Password-42!"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	requireNoError(t, err)
	_, err = suite.pool.Exec(ctx, `UPDATE users SET password_hash=$2,email_verified_at=NOW() WHERE id=$1`, id, string(hash))
	requireNoError(t, err)
	auth := services.NewAuthService(suite.users, services.NewTokenService("mfa-test-signing-secret-not-for-production"))
	cipher, err := crypto.NewAESCryptoService(strings.Repeat("01", 32))
	requireNoError(t, err)
	mfa := &services.MFAService{Repo: suite.users, Auth: auth, Crypto: cipher}
	user, err := suite.users.GetByID(ctx, id)
	requireNoError(t, err)
	old, err := auth.GenerateTokenPair(ctx, user)
	requireNoError(t, err)
	enrollment, err := mfa.Begin(ctx, user, password)
	requireNoError(t, err)
	state, err := suite.users.MFAState(ctx, id)
	requireNoError(t, err)
	if state.Pending == enrollment.Secret || state.Pending == "" || !strings.HasPrefix(enrollment.QR, "data:image/png;base64,") {
		t.Fatal("MFA secret was not encrypted or enrollment image missing")
	}
	code, err := totp.GenerateCode(enrollment.Secret, time.Now())
	requireNoError(t, err)
	current, codes, err := mfa.Change(ctx, user, password, code, true)
	requireNoError(t, err)
	if len(codes) != 10 {
		t.Fatal("missing single-use recovery codes")
	}
	if _, err = auth.ValidateAccessToken(ctx, old.AccessToken); err == nil {
		t.Fatal("MFA enrollment left pre-MFA session valid")
	}
	claims, err := auth.ValidateAccessToken(ctx, current.AccessToken)
	requireNoError(t, err)
	if !claims.MFAVerified {
		t.Fatal("enrollment did not establish verified MFA session")
	}
	pending, _, err := auth.Login(ctx, user.Email, password)
	requireNoError(t, err)
	if pending.AccessToken != "" || pending.RefreshToken != "" || pending.MFAChallenge == "" {
		t.Fatal("password bypassed MFA")
	}
	if _, _, err = mfa.Complete(ctx, pending.MFAChallenge, "invalid"); err == nil {
		t.Fatal("invalid OTP accepted")
	}
	first, _, err := mfa.Complete(ctx, pending.MFAChallenge, codes[0])
	requireNoError(t, err)
	if _, err = auth.ValidateAccessToken(ctx, first.AccessToken); err != nil {
		t.Fatal("recovery-code login did not establish valid session")
	}
	refreshed, err := auth.RefreshTokens(ctx, first.RefreshToken)
	requireNoError(t, err)
	claims, err = auth.ValidateAccessToken(ctx, refreshed.AccessToken)
	requireNoError(t, err)
	if !claims.MFAVerified {
		t.Fatal("refresh lost the established second factor")
	}
	pending, _, err = auth.Login(ctx, user.Email, password)
	requireNoError(t, err)
	if _, _, err = mfa.Complete(ctx, pending.MFAChallenge, codes[0]); err == nil {
		t.Fatal("recovery code reused")
	}
	code, err = totp.GenerateCode(enrollment.Secret, time.Now().Add(30*time.Second))
	requireNoError(t, err)
	_, _, err = mfa.Complete(ctx, pending.MFAChallenge, code)
	requireNoError(t, err)
	pending, _, err = auth.Login(ctx, user.Email, password)
	requireNoError(t, err)
	if _, _, err = mfa.Complete(ctx, pending.MFAChallenge, code); err == nil {
		t.Fatal("authenticator time step reused")
	}
	_, _, err = mfa.Complete(ctx, pending.MFAChallenge, codes[1])
	requireNoError(t, err)
	user, err = suite.users.GetByID(ctx, id)
	requireNoError(t, err)
	raw := strings.Repeat("m", 43)
	digest := sha256.Sum256([]byte(raw))
	_, err = suite.users.IssueEmailToken(ctx, user, "magic", hex.EncodeToString(digest[:]))
	requireNoError(t, err)
	email, err := services.NewEmailAccess(suite.users, auth, nil, "", true, slog.New(slog.NewTextHandler(io.Discard, nil)))
	requireNoError(t, err)
	magic, _, err := email.MagicLogin(ctx, raw)
	requireNoError(t, err)
	if magic.AccessToken != "" || magic.MFAChallenge == "" {
		t.Fatal("magic link bypassed MFA")
	}
	_, _, err = mfa.Complete(ctx, magic.MFAChallenge, codes[2])
	requireNoError(t, err)
	raw = strings.Repeat("r", 43)
	digest = sha256.Sum256([]byte(raw))
	_, err = suite.users.IssueEmailToken(ctx, user, "reset", hex.EncodeToString(digest[:]))
	requireNoError(t, err)
	requireNoError(t, email.Reset(ctx, raw, "Replacement-Password-84!"))
	user, err = suite.users.GetByID(ctx, id)
	requireNoError(t, err)
	if !user.MFAEnabled {
		t.Fatal("password reset disabled MFA")
	}
	if _, err = auth.ValidateAccessToken(ctx, current.AccessToken); err == nil {
		t.Fatal("password reset left old MFA session valid")
	}
	hash, err = bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	requireNoError(t, err)
	requireNoError(t, suite.users.EmergencyRecover(ctx, user.Email, string(hash), true))
	user, err = suite.users.GetByID(ctx, id)
	requireNoError(t, err)
	if user.MFAEnabled {
		t.Fatal("explicit local MFA recovery did not remove lost factor")
	}
	pair, _, err := auth.Login(ctx, user.Email, password)
	requireNoError(t, err)
	if pair.AccessToken == "" {
		t.Fatal("local emergency recovery did not restore account access")
	}
	_, err = suite.pool.Exec(ctx, `INSERT INTO schema_migrations(version,name,checksum) VALUES(999,'future.sql','unknown')`)
	requireNoError(t, err)
	if _, err = karidb.SchemaVersion(ctx, suite.pool); err == nil {
		t.Fatal("newer database accepted by older executable")
	}
	_, err = suite.pool.Exec(ctx, `DELETE FROM schema_migrations WHERE version=999`)
	requireNoError(t, err)
}
