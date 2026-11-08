package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/irgordon/kari/api/internal/core/domain"
	"github.com/irgordon/kari/api/internal/core/services"
)

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email,max=255"`
	Password string `json:"password" validate:"required,min=8,max=72"`
}

type AuthHandler struct {
	Service       domain.AuthService
	SecureCookies bool
	MFA           *services.MFAService
}

func NewAuthHandler(service domain.AuthService, secureCookies bool) *AuthHandler {
	return &AuthHandler{Service: service, SecureCookies: secureCookies}
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if json.NewDecoder(r.Body).Decode(&req) != nil || validate.Struct(req) != nil {
		writeJSON(w, 400, map[string]string{"message": "Enter a valid email and password"})
		return
	}
	tokens, user, err := h.Service.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		h.authError(w, err)
		return
	}
	h.setAuthCookies(w, tokens)
	if tokens.MFAChallenge != "" {
		writeJSON(w, 200, map[string]bool{"mfa_required": true})
		return
	}
	user.MFARequired = h.SecureCookies
	writeJSON(w, 200, map[string]interface{}{"user": user})
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("kari_refresh_token")
	if err != nil {
		writeJSON(w, 401, map[string]string{"message": "Please sign in again"})
		return
	}
	tokens, err := h.Service.RefreshTokens(r.Context(), cookie.Value)
	if err != nil {
		// A stale concurrent refresh must not clear a newer response's cookies.
		h.authError(w, err)
		return
	}
	h.setAuthCookies(w, tokens)
	writeJSON(w, 200, map[string]string{"message": "Session refreshed"})
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("kari_access_token")
	if err != nil {
		writeJSON(w, 401, map[string]string{"message": "Please sign in"})
		return
	}
	user, err := h.Service.CurrentUser(r.Context(), cookie.Value)
	if err != nil {
		h.authError(w, err)
		return
	}
	user.MFARequired = h.SecureCookies
	writeJSON(w, 200, map[string]interface{}{"user": user})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var refreshToken, accessToken string
	if cookie, err := r.Cookie("kari_refresh_token"); err == nil {
		refreshToken = cookie.Value
	}
	if cookie, err := r.Cookie("kari_access_token"); err == nil {
		accessToken = cookie.Value
	}
	if err := h.Service.Logout(r.Context(), refreshToken, accessToken); err != nil {
		h.authError(w, err)
		return
	}
	h.clearAuthCookies(w)
	writeJSON(w, 200, map[string]string{"message": "Signed out"})
}

func (h *AuthHandler) authError(w http.ResponseWriter, err error) {
	if errors.Is(err, domain.ErrMFAChallengeBusy) {
		w.Header().Set("Retry-After", "60")
		writeJSON(w, 429, map[string]string{"message": err.Error()})
		return
	}
	if errors.Is(err, domain.ErrInvalidCredentials) || errors.Is(err, domain.ErrNotFound) {
		writeJSON(w, 401, map[string]string{"message": "Invalid credentials or expired session"})
		return
	}
	writeJSON(w, 503, map[string]string{"message": "Sign-in service unavailable. Please try again."})
}

func (h *AuthHandler) setAuthCookies(w http.ResponseWriter, tokens *domain.TokenPair) {
	if tokens.MFAChallenge != "" {
		h.clearAuthCookies(w)
		http.SetCookie(w, &http.Cookie{Name: "kari_mfa_challenge", Value: tokens.MFAChallenge, Path: "/api/v1/auth", MaxAge: 300, HttpOnly: true, Secure: h.SecureCookies, SameSite: http.SameSiteStrictMode})
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "kari_mfa_challenge", Path: "/api/v1/auth", MaxAge: -1, HttpOnly: true, Secure: h.SecureCookies, SameSite: http.SameSiteStrictMode})
	for _, cookie := range []*http.Cookie{
		{Name: "kari_access_token", Value: tokens.AccessToken, Path: "/", Expires: time.Now().Add(15 * time.Minute)},
		{Name: "kari_refresh_token", Value: tokens.RefreshToken, Path: "/api/v1/auth", Expires: time.Now().Add(7 * 24 * time.Hour)},
	} {
		cookie.HttpOnly, cookie.Secure, cookie.SameSite = true, h.SecureCookies, http.SameSiteStrictMode
		http.SetCookie(w, cookie)
	}
	// Clear the legacy cookie path so it cannot shadow the current refresh cookie.
	http.SetCookie(w, &http.Cookie{Name: "kari_refresh_token", Path: "/api/v1/auth/refresh", MaxAge: -1,
		HttpOnly: true, Secure: h.SecureCookies, SameSite: http.SameSiteStrictMode})
}

func (h *AuthHandler) clearAuthCookies(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: "kari_mfa_challenge", Path: "/api/v1/auth", MaxAge: -1, HttpOnly: true, Secure: h.SecureCookies, SameSite: http.SameSiteStrictMode})
	for name, path := range map[string]string{"kari_access_token": "/", "kari_refresh_token": "/api/v1/auth"} {
		http.SetCookie(w, &http.Cookie{Name: name, Path: path, MaxAge: -1, Expires: time.Unix(0, 0),
			HttpOnly: true, Secure: h.SecureCookies, SameSite: http.SameSiteStrictMode})
	}
	http.SetCookie(w, &http.Cookie{Name: "kari_refresh_token", Path: "/api/v1/auth/refresh", MaxAge: -1,
		HttpOnly: true, Secure: h.SecureCookies, SameSite: http.SameSiteStrictMode})
}

func (h *AuthHandler) MFAAction(w http.ResponseWriter, r *http.Request) {
	if h.MFA == nil {
		writeJSON(w, 503, map[string]string{"message": "MFA is unavailable"})
		return
	}
	var req struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeJSON(w, 400, map[string]string{"message": "Invalid MFA request"})
		return
	}
	if r.URL.Path == "/api/v1/auth/mfa/complete" {
		cookie, err := r.Cookie("kari_mfa_challenge")
		if err != nil {
			h.authError(w, domain.ErrInvalidCredentials)
			return
		}
		tokens, user, err := h.MFA.Complete(r.Context(), cookie.Value, req.Code)
		if err != nil {
			h.authError(w, err)
			return
		}
		h.setAuthCookies(w, tokens)
		user.MFARequired = h.SecureCookies
		writeJSON(w, 200, map[string]interface{}{"user": user})
		return
	}
	cookie, err := r.Cookie("kari_access_token")
	if err != nil {
		h.authError(w, domain.ErrInvalidCredentials)
		return
	}
	user, err := h.Service.CurrentUser(r.Context(), cookie.Value)
	if err != nil {
		h.authError(w, err)
		return
	}
	if r.URL.Path == "/api/v1/auth/mfa/begin" {
		enrollment, err := h.MFA.Begin(r.Context(), user, req.Password)
		if err != nil {
			h.authError(w, err)
			return
		}
		writeJSON(w, 200, enrollment)
		return
	}
	enable := r.URL.Path == "/api/v1/auth/mfa/enable"
	tokens, codes, err := h.MFA.Change(r.Context(), user, req.Password, req.Code, enable)
	if err != nil {
		h.authError(w, err)
		return
	}
	h.setAuthCookies(w, tokens)
	user.MFAEnabled = enable
	user.MFARequired = h.SecureCookies
	writeJSON(w, 200, map[string]interface{}{"user": user, "recovery_codes": codes})
}
