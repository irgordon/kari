package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/irgordon/kari/api/internal/core/domain"
)

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email,max=255"`
	Password string `json:"password" validate:"required,min=8,max=72"`
}

type AuthHandler struct {
	Service       domain.AuthService
	SecureCookies bool
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
	if errors.Is(err, domain.ErrInvalidCredentials) || errors.Is(err, domain.ErrNotFound) {
		writeJSON(w, 401, map[string]string{"message": "Invalid credentials or expired session"})
		return
	}
	writeJSON(w, 503, map[string]string{"message": "Sign-in service unavailable. Please try again."})
}

func (h *AuthHandler) setAuthCookies(w http.ResponseWriter, tokens *domain.TokenPair) {
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
	for name, path := range map[string]string{"kari_access_token": "/", "kari_refresh_token": "/api/v1/auth"} {
		http.SetCookie(w, &http.Cookie{Name: name, Path: path, MaxAge: -1, Expires: time.Unix(0, 0),
			HttpOnly: true, Secure: h.SecureCookies, SameSite: http.SameSiteStrictMode})
	}
	http.SetCookie(w, &http.Cookie{Name: "kari_refresh_token", Path: "/api/v1/auth/refresh", MaxAge: -1,
		HttpOnly: true, Secure: h.SecureCookies, SameSite: http.SameSiteStrictMode})
}
