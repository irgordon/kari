// api/internal/api/handlers/auth.go
package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/irgordon/kari/api/internal/core/domain"
)

// ==============================================================================
// 1. Request Payloads (Input Validation)
// ==============================================================================

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email,max=255"`
	Password string `json:"password" validate:"required,min=8,max=72"`
}

// ==============================================================================
// 2. The Handler Struct (Dependency Injection)
// ==============================================================================

type AuthHandler struct {
	Service domain.AuthService
}

func NewAuthHandler(service domain.AuthService) *AuthHandler {
	return &AuthHandler{
		Service: service,
	}
}

// ==============================================================================
// 3. HTTP Methods
// ==============================================================================

// Login handles POST /api/v1/auth/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	// 1. Decode JSON payload
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"message": "Invalid JSON payload"}`, http.StatusBadRequest)
		return
	}

	// 2. Validate format (e.g., prevent massive strings that could cause bcrypt to consume too much CPU)
	if err := validate.Struct(req); err != nil {
		HandleError(w, r, err)
		return
	}

	// 3. Delegate to the Core Service
	// The service handles fetching the user, verifying the bcrypt password hash,
	// and generating the cryptographic JWT strings.
	tokenPair, user, err := h.Service.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		// If credentials are bad, the service returns domain.ErrInvalidCredentials.
		// HandleError will map this to a friendly 401 Unauthorized message.
		HandleError(w, r, err)
		return
	}

	// 4. Secure by Design: Set HttpOnly Cookies
	// We DO NOT return the tokens in the JSON body. We attach them as strict cookies.
	h.setAuthCookies(w, tokenPair)

	// 5. Return safe user data to the frontend (no passwords, no tokens)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	// We return the user struct so the SvelteKit UI can instantly display their name/role
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Login successful",
		"user": map[string]interface{}{
			"id":       user.ID,
			"username": user.Username,
			"email":    user.Email,
			"role_id":  user.RoleID,
		},
	})
}

// Refresh handles POST /api/v1/auth/refresh
// This is called silently by SvelteKit's hooks.server.ts when the access token expires.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	// 1. Extract the Refresh Token strictly from the cookie, ignoring the request body
	refreshCookie, err := r.Cookie("kari_refresh_token")
	if err != nil {
		http.Error(w, `{"message": "Missing refresh token"}`, http.StatusUnauthorized)
		return
	}

	// 2. Delegate to the Core Service to validate the refresh token and issue a new pair
	tokenPair, err := h.Service.RefreshTokens(r.Context(), refreshCookie.Value)
	if err != nil {
		// If the refresh token is expired, revoked, or manipulated, we wipe the cookies.
		h.clearAuthCookies(w)
		http.Error(w, `{"message": "Session expired. Please log in again."}`, http.StatusUnauthorized)
		return
	}

	// 3. Set the newly minted cookies
	h.setAuthCookies(w, tokenPair)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message": "Tokens refreshed successfully"}`))
}

// Logout handles POST /api/v1/auth/logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// Optionally, you can extract the refresh token and tell the database to blacklist it here

	// Issue expired cookies to the browser to physically delete them
	h.clearAuthCookies(w)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message": "Logged out successfully"}`))
}

// ==============================================================================
// 4. Internal Helpers (Cookie Management)
// ==============================================================================

// setAuthCookies abstracts the strict security flags required for session cookies in 2026.
func (h *AuthHandler) setAuthCookies(w http.ResponseWriter, tokens *domain.TokenPair) {
	// Access Token: Short-lived (e.g., 15 minutes)
	http.SetCookie(w, &http.Cookie{
		Name:     "kari_access_token",
		Value:    tokens.AccessToken,
		Path:     "/",
		Expires:  time.Now().Add(15 * time.Minute),
		HttpOnly: true,                    // JavaScript cannot read this (XSS protection)
		Secure:   true,                    // Only sent over HTTPS
		SameSite: http.SameSiteStrictMode, // Prevents Cross-Site Request Forgery (CSRF)
	})

	// Refresh Token: Long-lived (e.g., 7 days)
	http.SetCookie(w, &http.Cookie{
		Name:     "kari_refresh_token",
		Value:    tokens.RefreshToken,
		Path:     "/api/v1/auth/refresh", // ONLY sent to the refresh endpoint to minimize exposure
		Expires:  time.Now().Add(7 * 24 * time.Hour),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

// clearAuthCookies forces the browser to delete the session cookies immediately.
func (h *AuthHandler) clearAuthCookies(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "kari_access_token",
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0), // Expired in 1970
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     "kari_refresh_token",
		Value:    "",
		Path:     "/api/v1/auth/refresh",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}
