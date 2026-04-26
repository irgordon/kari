package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/irgordon/kari/api/internal/core/services"
)

// LoginRequest defines the expected shape of the inbound authentication payload.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// AuthHandler bridges the HTTP edge and the core authentication logic.
type AuthHandler struct {
	authService *services.AuthService
}

// NewAuthHandler injects the required business logic orchestrator.
func NewAuthHandler(authService *services.AuthService) *AuthHandler {
	return &AuthHandler{
		authService: authService,
	}
}

// Login handles POST /api/v1/auth/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	// 1. 🛡️ Anti-DOS: Cap payload size at 10KB to prevent memory exhaustion
	r.Body = http.MaxBytesReader(w, r.Body, 10240)

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request payload", http.StatusBadRequest)
		return
	}

	// 2. Orchestrate business logic (which includes our timing-attack defenses)
	tokenPair, _, err := h.authService.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		// 🛡️ Information Obfuscation: Generic 401 prevents enumeration
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	// 3. 🛡️ Zero-Trust Network Boundary: Bake the cookies
	h.setSecureCookies(w, tokenPair.AccessToken, tokenPair.RefreshToken)

	// Return a clean 200 OK. Notice we DO NOT return the tokens in the JSON body.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "authenticated"})
}

// Logout handles POST /api/v1/auth/logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// 1. 🛡️ State Destruction: Overwrite the cookies with immediate expiration
	http.SetCookie(w, &http.Cookie{
		Name:     "kari_access_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1, // Instructs browser to delete immediately
		Expires:  time.Unix(0, 0),
	})

	http.SetCookie(w, &http.Cookie{
		Name:     "kari_refresh_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})

	w.WriteHeader(http.StatusOK)
}

// setSecureCookies applies strict browser security policies to the session tokens.
func (h *AuthHandler) setSecureCookies(w http.ResponseWriter, accessToken, refreshToken string) {
	// 🛡️ Access Token (Short-lived, contains RBAC state)
	http.SetCookie(w, &http.Cookie{
		Name:     "kari_access_token",
		Value:    accessToken,
		Path:     "/",
		HttpOnly: true,                    // 🛡️ XSS Protection: JS cannot read this
		Secure:   true,                    // 🛡️ MITM Protection: Only sent over HTTPS
		SameSite: http.SameSiteStrictMode, // 🛡️ CSRF Protection: Never sent cross-origin
		MaxAge:   15 * 60,                 // 15 Minutes
	})

	// 🛡️ Refresh Token (Long-lived, opaque database pointer)
	http.SetCookie(w, &http.Cookie{
		Name:     "kari_refresh_token",
		Value:    refreshToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   7 * 24 * 60 * 60, // 7 Days
	})
}
