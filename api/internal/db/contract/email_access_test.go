//go:build integration

package contract_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	stdmail "net/mail"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/irgordon/kari/api/internal/api/handlers"
	"github.com/irgordon/kari/api/internal/api/middleware"
	"github.com/irgordon/kari/api/internal/api/router"
	"github.com/irgordon/kari/api/internal/core/services"
	"github.com/irgordon/kari/api/internal/infrastructure/mail"
	"golang.org/x/crypto/bcrypt"
)

// A local SMTP socket captures actual protocol delivery; no mail service or
// repository mocks replace the boundaries exercised by this journey.
func accountMailSink(t *testing.T) (string, <-chan string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	requireNoError(t, err)
	messages := make(chan string, 8)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.SetDeadline(time.Now().Add(10 * time.Second))
			protocol := textproto.NewConn(conn)
			protocol.PrintfLine("220 local SMTP")
			for {
				line, err := protocol.ReadLine()
				if err != nil {
					break
				}
				switch {
				case strings.HasPrefix(line, "EHLO"), strings.HasPrefix(line, "HELO"):
					protocol.PrintfLine("250 localhost")
				case strings.HasPrefix(line, "MAIL"), strings.HasPrefix(line, "RCPT"):
					protocol.PrintfLine("250 OK")
				case line == "DATA":
					protocol.PrintfLine("354 Send message")
					body, err := protocol.ReadDotBytes()
					if err == nil {
						messages <- string(body)
					}
					protocol.PrintfLine("250 Accepted")
				case line == "QUIT":
					protocol.PrintfLine("221 Bye")
				}
				if line == "QUIT" {
					break
				}
			}
			conn.Close()
		}
	}()
	t.Cleanup(func() { listener.Close(); <-done })
	return listener.Addr().String(), messages
}

func TestEmailAccessJourney(t *testing.T) {
	suite := newContractSuite(t)
	t.Cleanup(suite.pool.Close)
	suite.reset(t)
	ctx, cancel := context.WithCancel(context.Background())
	const password = "Owner-Password-42!"
	const newPassword = "New-Owner-Password-84!"
	ownerID := insertUser(t, suite.pool, superadminRoleID, "email-owner")
	otherID := insertUser(t, suite.pool, superadminRoleID, "email-other")
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	requireNoError(t, err)
	_, err = suite.pool.Exec(ctx, "UPDATE users SET password_hash = $1 WHERE id = $2", string(hash), ownerID)
	requireNoError(t, err)
	address, messages := accountMailSink(t)
	host, port, _ := net.SplitHostPort(address)
	portNumber, _ := strconv.Atoi(port)
	mailer, err := mail.NewSMTP(mail.Config{Host: host, Port: portNumber, From: "panel@example.test", Development: true})
	requireNoError(t, err)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	auth := services.NewAuthService(suite.users, services.NewTokenService("email-test-signing-secret-not-for-production"))
	if _, err = services.NewEmailAccess(suite.users, auth, mailer, "http://public.example", true, logger); err == nil {
		t.Fatal("non-HTTPS public origin accepted")
	}
	access, err := services.NewEmailAccess(suite.users, auth, mailer, "https://panel.example", false, logger)
	requireNoError(t, err)
	workerDone := make(chan struct{})
	go func() { defer close(workerDone); access.Run(ctx) }()
	t.Cleanup(func() { cancel(); <-workerDone })
	authHandler := handlers.NewAuthHandler(auth, true)
	server := httptest.NewTLSServer(router.NewRouter(router.RouterConfig{AuthHandler: authHandler,
		EmailHandler: &handlers.EmailAccessHandler{Service: access, Auth: authHandler}, Logger: logger,
		AuthMiddleware: middleware.NewAuthMiddleware(auth, services.NewRoleService(suite.users, logger), suite.users, logger)}))
	t.Cleanup(server.Close)
	client := server.Client()
	client.Jar, _ = cookiejar.New(nil)
	call := func(client *http.Client, method, path, body string, want int) *http.Response {
		t.Helper()
		req, _ := http.NewRequest(method, server.URL+path, strings.NewReader(body))
		req.Header.Set("Origin", server.URL)
		req.Header.Set("Content-Type", "application/json")
		response, err := client.Do(req)
		requireNoError(t, err)
		if response.StatusCode != want {
			t.Fatalf("%s %s = %d, want %d", method, path, response.StatusCode, want)
		}
		t.Cleanup(func() { response.Body.Close() })
		return response
	}
	linkToken := func(purpose string) string {
		t.Helper()
		select {
		case raw := <-messages:
			message, err := stdmail.ReadMessage(strings.NewReader(raw))
			requireNoError(t, err)
			body, err := io.ReadAll(quotedprintable.NewReader(message.Body))
			requireNoError(t, err)
			for _, field := range strings.Fields(string(body)) {
				if strings.HasPrefix(field, "https://panel.example/") {
					link, err := url.Parse(field)
					requireNoError(t, err)
					if link.Path != "/auth/email/"+purpose || link.RawQuery != "" {
						t.Fatalf("incorrect email purpose or leaked token query: %s", link.Path)
					}
					return strings.TrimPrefix(link.Fragment, "token=")
				}
			}
			t.Fatal("email did not contain its configured-origin link")
		case <-time.After(5 * time.Second):
			t.Fatal("SMTP message not delivered")
		}
		return ""
	}
	request := func(email, purpose string) {
		call(client, "POST", "/api/v1/auth/email/request", fmt.Sprintf(`{"email":%q,"purpose":%q}`, email, purpose), 202)
	}
	request("unknown@example.test", "magic")
	request("email-owner@example.test", "magic")
	call(client, "POST", "/api/v1/auth/email/verification/request", "{}", 401)
	call(client, "POST", "/api/v1/auth/login", fmt.Sprintf(`{"email":"email-owner@example.test","password":%q}`, password), 200)
	call(client, "POST", "/api/v1/auth/email/verification/request", "{}", 202)
	verification := linkToken("verify") // Earlier unknown/unverified requests must not send mail.
	var stored string
	requireNoError(t, suite.pool.QueryRow(ctx, "SELECT token_hash FROM account_email_tokens WHERE user_id = $1 AND purpose = 'verify'", ownerID).Scan(&stored))
	digest := sha256.Sum256([]byte(verification))
	if stored != hex.EncodeToString(digest[:]) || stored == verification {
		t.Fatal("email capability not stored as a hash")
	}
	call(client, "GET", "/api/v1/auth/email/verify", "", 405)
	other, err := suite.users.GetByID(ctx, otherID)
	requireNoError(t, err)
	otherTokens, err := auth.GenerateTokenPair(ctx, other)
	requireNoError(t, err)
	otherClient := &http.Client{Transport: client.Transport}
	otherClient.Jar, _ = cookiejar.New(nil)
	base, _ := url.Parse(server.URL)
	otherClient.Jar.SetCookies(base, []*http.Cookie{{Name: "kari_access_token", Value: otherTokens.AccessToken, Path: "/", Secure: true}})
	verifyBody := fmt.Sprintf(`{"token":%q}`, verification)
	call(otherClient, "POST", "/api/v1/auth/email/verify", verifyBody, 400)
	call(client, "POST", "/api/v1/auth/email/verify", verifyBody, 200)
	call(client, "POST", "/api/v1/auth/email/verify", verifyBody, 400)
	owner, err := suite.users.GetByID(ctx, ownerID)
	requireNoError(t, err)
	if !owner.EmailVerified {
		t.Fatal("verification did not persist")
	}
	request(owner.Email, "magic")
	magic := linkToken("magic")
	call(client, "POST", "/api/v1/auth/email/reset", fmt.Sprintf(`{"token":%q,"password":%q}`, magic, newPassword), 400)
	var wg sync.WaitGroup
	outcomes := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest("POST", server.URL+"/api/v1/auth/email/magic", strings.NewReader(fmt.Sprintf(`{"token":%q}`, magic)))
			req.Header.Set("Origin", server.URL)
			response, err := client.Do(req)
			if err != nil {
				outcomes <- 0
				return
			}
			outcomes <- response.StatusCode
			response.Body.Close()
		}()
	}
	wg.Wait()
	close(outcomes)
	success, rejected := 0, 0
	for status := range outcomes {
		if status == 200 {
			success++
		}
		if status == 400 {
			rejected++
		}
	}
	if success != 1 || rejected != 1 {
		t.Fatalf("magic link consumption = %d successes, %d rejections", success, rejected)
	}
	beforeReset, err := auth.GenerateTokenPair(ctx, owner)
	requireNoError(t, err)
	request(owner.Email, "magic") // Same account cooldown must suppress this email.
	request(owner.Email, "reset")
	expired := linkToken("reset")
	_, err = suite.pool.Exec(ctx, "UPDATE account_email_tokens SET expires_at = NOW() - INTERVAL '1 second', requested_at = NOW() - INTERVAL '61 seconds' WHERE user_id = $1 AND purpose = 'reset'", ownerID)
	requireNoError(t, err)
	call(client, "POST", "/api/v1/auth/email/reset", fmt.Sprintf(`{"token":%q,"password":%q}`, expired, newPassword), 400)
	request(owner.Email, "reset")
	reset := linkToken("reset")
	call(client, "POST", "/api/v1/auth/email/reset", fmt.Sprintf(`{"token":%q,"password":"weak"}`, reset), 400)
	resetBody := fmt.Sprintf(`{"token":%q,"password":%q}`, reset, newPassword)
	call(client, "POST", "/api/v1/auth/email/reset", resetBody, 200)
	call(client, "POST", "/api/v1/auth/email/reset", resetBody, 400)
	if _, err = auth.ValidateAccessToken(ctx, beforeReset.AccessToken); err == nil {
		t.Fatal("reset left access session valid")
	}
	if _, err = auth.RefreshTokens(ctx, beforeReset.RefreshToken); err == nil {
		t.Fatal("reset left refresh valid")
	}
	if _, err = auth.GenerateTokenPair(ctx, owner); err == nil {
		t.Fatal("old password snapshot created a new valid session after reset")
	}
	call(client, "POST", "/api/v1/auth/login", fmt.Sprintf(`{"email":%q,"password":%q}`, owner.Email, password), 401)
	call(client, "POST", "/api/v1/auth/login", fmt.Sprintf(`{"email":%q,"password":%q}`, owner.Email, newPassword), 200)
}
