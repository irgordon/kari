package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/irgordon/kari/api/internal/core/domain"
	"github.com/irgordon/kari/api/internal/core/services"
)

type EmailAccessHandler struct {
	Service *services.EmailAccess
	Auth    *AuthHandler
}

func (h *EmailAccessHandler) Status(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]bool{"enabled": h.Service.Enabled()})
}

func (h *EmailAccessHandler) Request(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email   string `json:"email"`
		Purpose string `json:"purpose"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || validate.Var(req.Email, "required,email,max=255") != nil || (req.Purpose != "reset" && req.Purpose != "magic") {
		writeJSON(w, 400, map[string]string{"message": "Enter a valid email and requested action"})
		return
	}
	if err := h.Service.Request(r.Context(), req.Email, req.Purpose, uuid.Nil); err != nil {
		h.failure(w, err)
		return
	}
	writeJSON(w, 202, map[string]string{"message": "If this is an eligible verified account, an email will arrive shortly. Check spam and wait at least one minute before requesting another link."})
}

func (h *EmailAccessHandler) currentUser(r *http.Request) (*domain.User, error) {
	cookie, err := r.Cookie("kari_access_token")
	if err != nil {
		return nil, domain.ErrInvalidCredentials
	}
	return h.Auth.Service.CurrentUser(r.Context(), cookie.Value)
}

func (h *EmailAccessHandler) RequestVerification(w http.ResponseWriter, r *http.Request) {
	user, err := h.currentUser(r)
	if err != nil {
		h.Auth.authError(w, err)
		return
	}
	if err = h.Service.Request(r.Context(), user.Email, "verify", user.ID); err != nil {
		h.failure(w, err)
		return
	}
	writeJSON(w, 202, map[string]string{"message": "Verification requested. Check your mailbox, then confirm the link while signed in to this account."})
}

func (h *EmailAccessHandler) Confirm(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeJSON(w, 400, map[string]string{"message": "Invalid email action"})
		return
	}
	switch r.URL.Path {
	case "/api/v1/auth/email/verify":
		user, err := h.currentUser(r)
		if err != nil {
			h.Auth.authError(w, err)
			return
		}
		if err = h.Service.Verify(r.Context(), user.ID, req.Token); err != nil {
			h.failure(w, err)
			return
		}
		user.EmailVerified = true
		user.MFARequired = h.Auth.SecureCookies
		writeJSON(w, 200, map[string]interface{}{"message": "Email verified", "user": user})
	case "/api/v1/auth/email/reset":
		if err := h.Service.Reset(r.Context(), req.Token, req.Password); err != nil {
			h.failure(w, err)
			return
		}
		h.Auth.clearAuthCookies(w)
		writeJSON(w, 200, map[string]string{"message": "Password updated. All previous sessions and email links have been revoked. Sign in with your new password."})
	case "/api/v1/auth/email/magic":
		tokens, user, err := h.Service.MagicLogin(r.Context(), req.Token)
		if err != nil {
			h.failure(w, err)
			return
		}
		h.Auth.setAuthCookies(w, tokens)
		if tokens.MFAChallenge != "" {
			writeJSON(w, 200, map[string]bool{"mfa_required": true})
			return
		}
		user.MFARequired = h.Auth.SecureCookies
		writeJSON(w, 200, map[string]interface{}{"user": user})
	default:
		writeJSON(w, 404, map[string]string{"message": "Unknown email action"})
	}
}

func (h *EmailAccessHandler) failure(w http.ResponseWriter, err error) {
	if errors.Is(err, domain.ErrMFAChallengeBusy) {
		h.Auth.authError(w, err)
		return
	}
	switch {
	case errors.Is(err, services.ErrEmailDisabled):
		writeJSON(w, 503, map[string]string{"message": "Email delivery is not configured. Contact your administrator."})
	case errors.Is(err, services.ErrEmailBusy):
		w.Header().Set("Retry-After", "60")
		writeJSON(w, 429, map[string]string{"message": "Too many email requests. Please try again later."})
	case errors.Is(err, domain.ErrInvalidCredentials), errors.Is(err, domain.ErrNotFound):
		writeJSON(w, 400, map[string]string{"message": "This link is invalid, expired, already used, or belongs to a different account. Request a new link."})
	case errors.Is(err, services.ErrPasswordPolicy):
		writeJSON(w, 400, map[string]string{"message": err.Error()})
	default:
		writeJSON(w, 503, map[string]string{"message": "Account service unavailable. Please try again."})
	}
}
