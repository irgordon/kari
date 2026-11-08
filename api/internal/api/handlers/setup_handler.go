package handlers

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/irgordon/kari/api/internal/core/domain"
	"github.com/irgordon/kari/api/internal/core/utils"
	"golang.org/x/crypto/bcrypt"
)

type SetupRepository interface {
	IsConfigured(context.Context) (bool, error)
	CompleteSetup(context.Context, string, string) error
}

type SetupHandler struct {
	repo            SetupRepository
	logger          *slog.Logger
	token           string
	expires         time.Time
	Auth            *AuthHandler
	DatabaseReady   func(context.Context) error
	AgentReady      func() bool
	AgentRequired   bool
	PublicURL       string
	EmailConfigured bool
	Version         string
}

func NewSetupHandler(repo SetupRepository, logger *slog.Logger, token string) *SetupHandler {
	return &SetupHandler{repo: repo, logger: logger, token: token, expires: time.Now().Add(15 * time.Minute)}
}

func (h *SetupHandler) Status(w http.ResponseWriter, r *http.Request) {
	configured, err := h.repo.IsConfigured(r.Context())
	if err != nil {
		writeJSON(w, 503, map[string]string{"message": "Cannot check setup status. Please try again."})
		return
	}
	writeJSON(w, 200, map[string]bool{"configured": configured})
}

func (h *SetupHandler) Onboarding(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("kari_access_token")
	if err != nil || h.Auth == nil {
		writeJSON(w, 401, map[string]string{"message": "Sign in first"})
		return
	}
	user, err := h.Auth.Service.CurrentUser(r.Context(), cookie.Value)
	if err != nil {
		h.Auth.authError(w, err)
		return
	}
	databaseReady := h.DatabaseReady != nil && h.DatabaseReady(r.Context()) == nil
	agentReady := h.AgentReady != nil && h.AgentReady()
	checks := map[string]bool{"administrator": true, "https": strings.HasPrefix(h.PublicURL, "https://") || !h.Auth.SecureCookies,
		"database": databaseReady, "native_agent": agentReady, "verified_email": h.EmailConfigured && user.EmailVerified, "mfa": user.MFAEnabled && user.MFAVerified}
	complete := true
	for name, ok := range checks {
		if name == "native_agent" && !h.AgentRequired {
			continue
		}
		complete = complete && ok
	}
	writeJSON(w, 200, map[string]interface{}{"checks": checks, "complete": complete, "version": h.Version, "agent_required": h.AgentRequired})
}

func (h *SetupHandler) SetupGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/health" || path == "/ready" || path == "/api/v1/setup/status" {
			next.ServeHTTP(w, r)
			return
		}
		configured, err := h.repo.IsConfigured(r.Context())
		if err != nil {
			writeJSON(w, 503, map[string]string{"message": "Database unavailable"})
			return
		}
		setupPath := strings.HasPrefix(path, "/api/v1/setup/")
		if configured && setupPath {
			writeJSON(w, 409, map[string]string{"message": "Setup is already complete"})
			return
		}
		if !configured && !setupPath {
			writeJSON(w, 409, map[string]string{"message": "Complete administrator setup first", "code": "setup_required"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *SetupHandler) SetupAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provided := r.Header.Get("X-Setup-Token")
		if h.token == "" || time.Now().After(h.expires) || subtle.ConstantTimeCompare([]byte(provided), []byte(h.token)) != 1 {
			writeJSON(w, 401, map[string]string{"message": "Invalid or expired setup token. Ask your operator to retrieve the private key or restart the API if it has expired."})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *SetupHandler) Finalize(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"admin_email"`
		Password string `json:"admin_password"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&req) != nil {
		writeJSON(w, 400, map[string]string{"message": "Invalid setup request"})
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if err := validate.Var(req.Email, "required,email,max=255"); err != nil {
		writeJSON(w, 400, map[string]string{"message": "Enter a valid email address"})
		return
	}
	if err := utils.ValidatePasswordComplexity(req.Password); err != nil {
		writeJSON(w, 400, map[string]string{"message": err.Error()})
		return
	}
	if len(req.Password) > 72 {
		writeJSON(w, 400, map[string]string{"message": "Password must be at most 72 bytes"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeJSON(w, 500, map[string]string{"message": "Could not create administrator"})
		return
	}
	err = h.repo.CompleteSetup(r.Context(), req.Email, string(hash))
	if errors.Is(err, domain.ErrConflict) {
		writeJSON(w, 409, map[string]string{"message": "Setup is already complete"})
		return
	}
	if err != nil {
		h.logger.Error("Administrator setup failed", "error", err)
		writeJSON(w, 503, map[string]string{"message": "Could not save administrator. Please try again."})
		return
	}
	writeJSON(w, 201, map[string]string{"message": "Administrator created. Sign in to continue."})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
