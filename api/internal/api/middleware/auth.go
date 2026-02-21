// api/internal/api/middleware/auth.go
package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/time/rate"

	"kari/api/internal/core/domain"
)

// ==============================================================================
// 1. Dependency Injection Struct
// ==============================================================================

type AuthMiddleware struct {
	AuthService domain.AuthService
	RoleService domain.RoleService
	Logger      *slog.Logger
}

func NewAuthMiddleware(authService domain.AuthService, roleService domain.RoleService, logger *slog.Logger) *AuthMiddleware {
	return &AuthMiddleware{
		AuthService: authService,
		RoleService: roleService,
		Logger:      logger,
	}
}

// ==============================================================================
// 2. Security & Protocol Enforcers (Platform Agnostic)
// ==============================================================================

// EnforceTLS ensures no plaintext traffic interacts with the API.
// It detects 'X-Forwarded-Proto' to remain compatible with Nginx, Caddy, or Cloudflare proxies.
func EnforceTLS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isHTTPS := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"

		// Allow localhost DX bypass for development
		if !isHTTPS && !strings.HasPrefix(r.Host, "localhost:") && !strings.HasPrefix(r.Host, "127.0.0.1:") {
			target := "https://" + r.Host + r.URL.RequestURI()
			http.Redirect(w, r, target, http.StatusMovedPermanently)
			return
		}

		// Security Headers: HSTS, Clickjacking protection, and Content-Sniffing prevention
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")

		next.ServeHTTP(w, r)
	})
}

// MaxBytes protects against memory-exhaustion attacks by capping the request size.
func MaxBytes(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}

// ==============================================================================
// 3. Identity & Access Management (IAM)
// ==============================================================================

// RequireAuthentication extracts the JWT from HttpOnly cookies or Authorization headers.
func (m *AuthMiddleware) RequireAuthentication() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var tokenString string

			// 1. Priority: Secure HttpOnly Cookie (Browser/SPA flow)
			if cookie, err := r.Cookie("kari_access_token"); err == nil {
				tokenString = cookie.Value
			} else {
				// 2. Fallback: Bearer Token (CLI/API flow)
				authHeader := r.Header.Get("Authorization")
				if strings.HasPrefix(authHeader, "Bearer ") {
					tokenString = strings.TrimPrefix(authHeader, "Bearer ")
				}
			}

			if tokenString == "" {
				m.Logger.Debug("Authentication failed: No token provided")
				http.Error(w, `{"message": "Unauthorized: Missing token"}`, http.StatusUnauthorized)
				return
			}

			// 3. SLA: Delegate cryptographic validation to AuthService
			claims, err := m.AuthService.ValidateAccessToken(r.Context(), tokenString)
			if err != nil {
				m.Logger.Warn("Invalid token attempt", slog.String("error", err.Error()))
				http.Error(w, `{"message": "Unauthorized: Invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			// 4. Inject Claims into Context for downstream handlers/RBAC checks
			ctx := context.WithValue(r.Context(), domain.UserContextKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequirePermission checks the user's role against the requested resource:action.
func (m *AuthMiddleware) RequirePermission(resource, action string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userClaims, ok := r.Context().Value(domain.UserContextKey).(*domain.UserClaims)
			if !ok {
				http.Error(w, `{"message": "Unauthorized"}`, http.StatusUnauthorized)
				return
			}

			// SOLID: Delegate permission lookup to RoleService
			hasPerm, err := m.RoleService.RoleHasPermission(r.Context(), userClaims.RoleID, resource, action)
			if err != nil || !hasPerm {
				m.Logger.Warn("Forbidden access attempt", 
					slog.String("user", userClaims.Email),
					slog.String("resource", resource),
					slog.String("action", action),
				)
				http.Error(w, `{"message": "Forbidden: insufficient permissions"}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ==============================================================================
// 4. In-Memory Rate Limiting (DoS Protection)
// ==============================================================================

var (
	visitors = make(map[string]*visitor)
	mu       sync.Mutex
)

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// Cleanup old visitors to prevent memory leaks in long-running Go processes.
func init() {
	go func() {
		for {
			time.Sleep(time.Minute)
			mu.Lock()
			for ip, v := range visitors {
				if time.Since(v.lastSeen) > 3*time.Minute {
					delete(visitors, ip)
				}
			}
			mu.Unlock()
		}
	}()
}

func RateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Use chi's RealIP to ensure we aren't limiting the reverse proxy's IP
		ip := r.RemoteAddr

		mu.Lock()
		v, exists := visitors[ip]
		if !exists {
			v = &visitor{limiter: rate.NewLimiter(10, 30)}
			visitors[ip] = v
		}
		v.lastSeen = time.Now()
		limiter := v.limiter
		mu.Unlock()

		if !limiter.Allow() {
			http.Error(w, `{"message": "Too many requests"}`, http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// ==============================================================================
// 5. Observability (Structured Logging)
// ==============================================================================

func StructuredLogger(logger *slog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			// Log every request with its Trace ID (Request ID) for audit trails
			logger.Info("HTTP Access",
				slog.String("trace_id", middleware.GetReqID(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", ww.Status()),
				slog.Duration("latency", time.Since(start)),
				slog.String("ip", r.RemoteAddr),
			)
		})
	}
}
