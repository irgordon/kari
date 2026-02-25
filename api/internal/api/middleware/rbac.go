package middleware

import (
	"context"
	// "errors"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"kari/api/internal/core/domain"
)

type contextKey string

const (
	UserKey contextKey = "user_id"
	RoleKey contextKey = "role_rank"
)

type RBACMiddleware struct {
	repo      domain.UserRepository
	jwtSecret []byte
}

func NewRBACMiddleware(repo domain.UserRepository, secret string) *RBACMiddleware {
	return &RBACMiddleware{
		repo:      repo,
		jwtSecret: []byte(secret),
	}
}

func (m *RBACMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenStr := m.extractToken(r)

		if tokenStr == "" {
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}

		claims := &jwt.RegisteredClaims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
			return m.jwtSecret, nil
		})

		if err != nil || !token.Valid {
			http.Error(w, "Invalid session", http.StatusUnauthorized)
			return
		}

		userID, err := uuid.Parse(claims.Subject)
		if err != nil {
			http.Error(w, "Malformed subject", http.StatusUnauthorized)
			return
		}

		// 🛡️ Zero-Trust: Real-time DB check with eager loading of Role
		user, err := m.repo.GetByID(r.Context(), userID)
		if err != nil || !user.IsActive {
			http.Error(w, "Account inactive", http.StatusForbidden)
			return
		}

		ctx := context.WithValue(r.Context(), UserKey, user.ID)
		ctx = context.WithValue(ctx, RoleKey, user.Role.Rank)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m *RBACMiddleware) RequirePermission(resource, action string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 🛡️ Safe context retrieval
			val := r.Context().Value(UserKey)
			if val == nil {
				http.Error(w, "Identity context missing", http.StatusInternalServerError)
				return
			}
			userID := val.(uuid.UUID)

			// Consult the Dynamic RBAC Store
			hasPerm, err := m.repo.HasPermission(r.Context(), userID, resource, action)
			if err != nil || !hasPerm {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// 🛡️ Platform Agnostic Token Extraction
func (m *RBACMiddleware) extractToken(r *http.Request) string {
	// 1. Check Authorization Header (CLI/Patty flow)
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	// 2. Check Cookie (SvelteKit flow) - Standardized name
	if cookie, err := r.Cookie("kari_access_token"); err == nil {
		return cookie.Value
	}
	return ""
}
