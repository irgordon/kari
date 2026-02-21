package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"kari/api/internal/core/domain"
	"kari/api/internal/core/services"
)

type AuthHandler struct {
	tokenService *services.TokenService
	userRepo     domain.UserRepository // To fetch fresh data during rotation
}

func NewAuthHandler(ts *services.TokenService, ur domain.UserRepository) *AuthHandler {
	return &AuthHandler{
		tokenService: ts,
		userRepo:     ur,
	}
}

// Refresh handles the Silent Refresh flow
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	// 1. Extract the Refresh Cookie
	cookie, err := r.Cookie("kari_refresh_token")
	if err != nil {
		http.Error(w, `{"message": "No refresh token provided"}`, http.StatusUnauthorized)
		return
	}

	// 2. 🛡️ Cryptographic Verification
	userID, err := h.tokenService.VerifyRefreshToken(cookie.Value)
	if err != nil {
		// If the refresh token is expired or tampered with, clear the dead cookie
		h.clearCookies(w)
		http.Error(w, `{"message": "Session expired, please log in again"}`, http.StatusUnauthorized)
		return
	}

	// 3. 🛡️ SLA: Fetch Fresh State
	// We MUST hit the database here. If an admin revoked this user's access
	// 5 minutes ago, this stops them from getting a new 15-minute access token.
	user, err := h.userRepo.FindByID(r.Context(), userID)
	if err != nil || !user.IsActive {
		h.clearCookies(w)
		http.Error(w, `{"message": "Account suspended or not found"}`, http.StatusUnauthorized)
		return
	}

	// 4. Generate New Token Pair (Token Rotation)
	newAccess, newRefresh, err := h.tokenService.GenerateTokenPair(user)
	if err != nil {
		http.Error(w, `{"message": "Failed to generate session"}`, http.StatusInternalServerError)
		return
	}

	// 5. Securely set the new cookies
	h.setAuthCookies(w, newAccess, newRefresh)

	// Return a 200 OK so SvelteKit knows it can proceed
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "refreshed"})
}

// 🛡️ Helper: Apply Zero-Trust cookie policies
func (h *AuthHandler) setAuthCookies(w http.ResponseWriter, accessToken, refreshToken string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "kari_access_token",
		Value:    accessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   true, // Requires HTTPS in production
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(15 * time.Minute.Seconds()), 
	})

	http.SetCookie(w, &http.Cookie{
		Name:     "kari_refresh_token",
		Value:    refreshToken,
		Path:     "/api/v1/auth/refresh", // 🛡️ SLA: Only send this cookie to the refresh endpoint!
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(7 * 24 * time.Hour.Seconds()), 
	})
}

// 🛡️ Helper: Clean up cookies on failure
func (h *AuthHandler) clearCookies(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "kari_access_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "kari_refresh_token",
		Value:    "",
		Path:     "/api/v1/auth/refresh",
		HttpOnly: true,
		MaxAge:   -1,
	})
}
