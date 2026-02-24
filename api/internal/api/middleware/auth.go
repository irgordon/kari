package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	// "github.com/go-chi/chi/v5/middleware"
	"golang.org/x/time/rate"

	"kari/api/internal/core/domain"
)

type AuthMiddleware struct {
	AuthService domain.AuthService
	RoleService domain.RoleService
	UserRepo    domain.UserRepository // 🛡️ Added for Real-time Zero-Trust checks
	Logger      *slog.Logger
	visitors    sync.Map // 🛡️ Thread-safe Map for high-concurrency scaling
}

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func NewAuthMiddleware(authService domain.AuthService, roleService domain.RoleService, userRepo domain.UserRepository, logger *slog.Logger) *AuthMiddleware {
	m := &AuthMiddleware{
		AuthService: authService,
		RoleService: roleService,
		UserRepo:    userRepo,
		Logger:      logger,
	}
	// Start cleanup worker as a managed method, not a global init
	go m.cleanupVisitors()
	return m
}

// ==============================================================================
// 1. Identity & Zero-Trust Access
// ==============================================================================

func (m *AuthMiddleware) RequireAuthentication(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenString := m.extractToken(r)

		if tokenString == "" {
			http.Error(w, `{"message": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		claims, err := m.AuthService.ValidateAccessToken(r.Context(), tokenString)
		if err != nil {
			http.Error(w, `{"message": "Invalid token"}`, http.StatusUnauthorized)
			return
		}

		// 🛡️ Zero-Trust: Verify user is still active in the DB (Ghost Token Prevention)
		user, err := m.UserRepo.GetByID(r.Context(), claims.UserID)
		if err != nil || !user.IsActive {
			m.Logger.Warn("Attempted access with ghost token", slog.String("user_id", claims.UserID.String()))
			http.Error(w, `{"message": "Account suspended"}`, http.StatusForbidden)
			return
		}

		ctx := context.WithValue(r.Context(), domain.UserContextKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ==============================================================================
// 2. Performance & DoS Protection
// ==============================================================================

func (m *AuthMiddleware) RateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 🛡️ Use X-Real-IP for proxy compatibility
		ip := r.Header.Get("X-Real-IP")
		if ip == "" {
			ip = r.RemoteAddr
		}

		v, _ := m.visitors.LoadOrStore(ip, &visitor{
			limiter:  rate.NewLimiter(rate.Limit(10), 30),
			lastSeen: time.Now(),
		})
		
		vis := v.(*visitor)
		vis.lastSeen = time.Now()

		if !vis.limiter.Allow() {
			http.Error(w, `{"message": "Rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (m *AuthMiddleware) cleanupVisitors() {
	ticker := time.NewTicker(time.Minute)
	for range ticker.C {
		m.visitors.Range(func(key, value interface{}) bool {
			if time.Since(value.(*visitor).lastSeen) > 3*time.Minute {
				m.visitors.Delete(key)
			}
			return true
		})
	}
}

// ... [EnforceTLS and StructuredLogger remain as helper functions] ...

// ==============================================================================
// 3. 🛡️ JWT Scope Enforcement (Stateless RBAC)
// ==============================================================================

// RequirePermission returns middleware that checks if the authenticated user's JWT
// contains a specific permission string (format: "resource:action").
// This is the stateless guard — the DB check already happened in RequireAuthentication.
func (m *AuthMiddleware) RequirePermission(resource, action string) func(http.Handler) http.Handler {
	required := resource + ":" + action
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := m.claimsFromContext(r.Context())
			if claims == nil {
				http.Error(w, `{"message": "Identity context missing"}`, http.StatusUnauthorized)
				return
			}

			// 🛡️ SLA: Check if the user's JWT-embedded permissions include the required scope
			if !hasPermission(claims.Permissions, required) {
				m.Logger.Warn("🛡️ Scope violation: user lacks required permission",
					slog.String("user_id", claims.UserID.String()),
					slog.String("required", required),
					slog.Any("granted", claims.Permissions))
				http.Error(w, `{"message": "Forbidden: insufficient scope"}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireScope blocks operators whose JWT permissions do NOT include ANY of the listed scopes.
// Use this for high-privilege endpoints (deploy, teardown) to enforce "view" vs "manage" separation.
//
// Example: RequireScope("applications:deploy", "applications:teardown")
func (m *AuthMiddleware) RequireScope(scopes ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := m.claimsFromContext(r.Context())
			if claims == nil {
				http.Error(w, `{"message": "Identity context missing"}`, http.StatusUnauthorized)
				return
			}

			for _, scope := range scopes {
				if hasPermission(claims.Permissions, scope) {
					next.ServeHTTP(w, r)
					return
				}
			}

			m.Logger.Warn("🛡️ Scope enforcement: view-only user attempted mutating action",
				slog.String("user_id", claims.UserID.String()),
				slog.Any("required_scopes", scopes))
			http.Error(w, `{"message": "Forbidden: your account scope does not allow this action"}`, http.StatusForbidden)
		})
	}
}

// claimsFromContext safely extracts the authenticated user claims from the request context.
func (m *AuthMiddleware) claimsFromContext(ctx context.Context) *domain.UserClaims {
	val := ctx.Value(domain.UserContextKey)
	if val == nil {
		return nil
	}
	claims, ok := val.(*domain.UserClaims)
	if !ok {
		return nil
	}
	return claims
}

// hasPermission checks if the permissions slice contains the target string.
func hasPermission(permissions []string, target string) bool {
	for _, p := range permissions {
		if p == target || p == "*" {
			return true
		}
	}
	return false
}

func (m *AuthMiddleware) extractToken(r *http.Request) string {
	if cookie, err := r.Cookie("kari_access_token"); err == nil {
		return cookie.Value
	}
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}
	return ""
}
