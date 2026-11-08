package middleware

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	// "github.com/go-chi/chi/v5/middleware"
	"golang.org/x/time/rate"

	"github.com/irgordon/kari/api/internal/core/domain"
)

type AuthMiddleware struct {
	AuthService domain.AuthService
	RoleService domain.RoleService
	UserRepo    domain.UserRepository // 🛡️ Added for Real-time Zero-Trust checks
	Logger      *slog.Logger
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

func StructuredLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			startedAt := time.Now()
			next.ServeHTTP(w, r)
			logger.Info("http request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Duration("duration", time.Since(startedAt)),
			)
		})
	}
}

func MaxBytes(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}

func RateLimitMiddleware(trustedProxies []netip.Prefix) func(http.Handler) http.Handler {
	type key struct {
		client netip.Addr
		scope  string
	}
	var mu sync.Mutex
	visitors := make(map[key]*visitor)
	lastCleanup := time.Now()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodOptions || (r.Method == http.MethodGet && (r.URL.Path == "/health" || r.URL.Path == "/ready")) {
				next.ServeHTTP(w, r)
				return
			}
			client, err := rateLimitClientIP(r, trustedProxies)
			if err != nil {
				http.Error(w, `{"message":"Invalid client address"}`, http.StatusBadRequest)
				return
			}
			scope := "read"
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				scope = "write"
			}
			if strings.HasPrefix(r.URL.Path, "/api/v1/auth/") || r.URL.Path == "/api/v1/setup/finalize" {
				scope = "auth"
			}
			now := time.Now()
			mu.Lock()
			if now.Sub(lastCleanup) >= time.Minute {
				for k, v := range visitors {
					if now.Sub(v.lastSeen) > 3*time.Minute {
						delete(visitors, k)
					}
				}
				lastCleanup = now
			}
			k := key{client: client, scope: scope}
			v := visitors[k]
			if v == nil && len(visitors) < 4096 {
				v = &visitor{limiter: rate.NewLimiter(rate.Limit(10), 30)}
				visitors[k] = v
			}
			allowed := false
			if v != nil {
				v.lastSeen = now
				allowed = v.limiter.AllowN(now, 1)
			}
			mu.Unlock()
			if !allowed {
				w.Header().Set("Retry-After", "1")
				w.Header().Set("Cache-Control", "no-store")
				http.Error(w, `{"message":"Rate limit exceeded"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Only a configured TCP peer may assert a different client address. Source ports
// and caller-supplied forwarding chains never define independent rate budgets.
func rateLimitClientIP(r *http.Request, trustedProxies []netip.Prefix) (netip.Addr, error) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return netip.Addr{}, err
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, err
	}
	peer = peer.Unmap().WithZone("")
	for _, prefix := range trustedProxies {
		if prefix.Contains(peer) {
			client, err := netip.ParseAddr(r.Header.Get("X-Real-IP"))
			return client.Unmap().WithZone(""), err
		}
	}
	return peer, nil
}

func EnforceTLS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
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
