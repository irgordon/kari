// api/internal/api/router/router.go
package router

import (
	"context"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/irgordon/kari/api/internal/api/handlers"
	auth_middleware "github.com/irgordon/kari/api/internal/api/middleware"
)

// RouterConfig defines the strict dependencies required to build the API routing tree.
type RouterConfig struct {
	AllowedOrigins []string
	TrustedProxies []netip.Prefix
	AuthHandler    *handlers.AuthHandler
	EmailHandler   *handlers.EmailAccessHandler
	AppHandler     *handlers.AppHandler
	DomainHandler  *handlers.DomainHandler
	AuditHandler   *handlers.AuditHandler
	WSHandler      *handlers.WebSocketHandler
	SetupHandler   *handlers.SetupHandler
	AuthMiddleware *auth_middleware.AuthMiddleware
	DeployHandler  *handlers.DeploymentHandler
	ReadinessCheck func(context.Context) error
	Logger         *slog.Logger
}

// NewRouter constructs the Chi multiplexer, attaches global middleware, and wires all endpoints.
func NewRouter(cfg RouterConfig) *chi.Mux {
	r := chi.NewRouter()

	// =========================================================================
	// 1. Global Gateway Middleware Pipeline
	// =========================================================================

	r.Use(middleware.RequestID)
	r.Use(auth_middleware.StructuredLogger(cfg.Logger))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// 🛡️ Limit all incoming JSON requests to 1 Megabyte max (OOM Protection)
	r.Use(auth_middleware.MaxBytes(1_048_576))

	// 🔒 Force all connections to use TLS/SSL and inject HSTS headers
	r.Use(auth_middleware.EnforceTLS)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "GET" || r.Method == "HEAD" || r.Method == "OPTIONS" || strings.HasPrefix(r.URL.Path, "/api/v1/webhooks/") {
				next.ServeHTTP(w, r)
				return
			}
			origin := r.Header.Get("Origin")
			parsed, err := url.Parse(origin)
			allowed := err == nil && parsed.Host != "" && parsed.Host == r.Host && (parsed.Scheme == "http" || parsed.Scheme == "https")
			for _, configured := range cfg.AllowedOrigins {
				allowed = allowed || origin == configured
			}
			if cfg.AuthHandler.SecureCookies && !strings.HasPrefix(origin, "https://") {
				allowed = false
			}
			if !allowed {
				w.Header().Set("Content-Type", "application/json")
				http.Error(w, `{"message":"Request origin is not allowed"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	})

	r.Use(auth_middleware.RateLimitMiddleware(cfg.TrustedProxies))

	// Strict CORS Configuration
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.AllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "X-Hub-Signature-256", "X-GitHub-Event"},
		ExposedHeaders:   []string{"Link", "Set-Cookie"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// =========================================================================
	// 2. API v1 Routing Tree
	// =========================================================================

	r.Route("/api/v1", func(r chi.Router) {

		// ---------------------------------------------------------------------
		// Setup routes are gated by the database installation marker.
		// ---------------------------------------------------------------------
		if cfg.SetupHandler != nil {
			r.Get("/setup/status", cfg.SetupHandler.Status)
			r.Route("/setup", func(r chi.Router) {
				r.Use(cfg.SetupHandler.SetupAuth)
				r.Post("/finalize", cfg.SetupHandler.Finalize)
			})
		}

		// ---------------------------------------------------------------------
		// Public Routes (No Auth Required)
		// ---------------------------------------------------------------------
		r.Group(func(r chi.Router) {
			r.Post("/auth/login", cfg.AuthHandler.Login)
			r.Post("/auth/refresh", cfg.AuthHandler.Refresh)
			r.Post("/auth/logout", cfg.AuthHandler.Logout)
			r.Get("/auth/me", cfg.AuthHandler.Me)
			if cfg.EmailHandler != nil {
				r.Get("/auth/email/status", cfg.EmailHandler.Status)
				r.Post("/auth/email/request", cfg.EmailHandler.Request)
				r.Post("/auth/email/verification/request", cfg.EmailHandler.RequestVerification)
				r.Post("/auth/email/verify", cfg.EmailHandler.Confirm)
				r.Post("/auth/email/reset", cfg.EmailHandler.Confirm)
				r.Post("/auth/email/magic", cfg.EmailHandler.Confirm)
			}

			if cfg.AppHandler != nil {
				r.Post("/webhooks/github/{id}", cfg.AppHandler.HandleGitHubWebhook)
			}
		})

		// ---------------------------------------------------------------------
		// Protected Routes (Requires a Valid JWT)
		// ---------------------------------------------------------------------
		r.Group(func(r chi.Router) {
			r.Use(cfg.AuthMiddleware.RequireAuthentication)

			// --- Mutating Method Guard (Stateless RBAC) ---
			// 🛡️ Zero-Trust: Even if a specific route forgets a RequirePermission check,
			// this global guard ensures view-only operators can NEVER mutate state.
			r.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					if req.Method == http.MethodPost || req.Method == http.MethodPut ||
						req.Method == http.MethodDelete || req.Method == http.MethodPatch {

						// The scopes that permit mutation
						guard := cfg.AuthMiddleware.RequireScope(
							"domains:write", "domains:delete",
							"applications:write", "applications:deploy", "applications:delete",
							"server:manage",
						)
						guard(next).ServeHTTP(w, req)
						return
					}
					next.ServeHTTP(w, req)
				})
			})

			if cfg.DomainHandler != nil {
				r.Route("/domains", func(r chi.Router) {
					r.With(cfg.AuthMiddleware.RequirePermission("domains", "read")).
						Get("/", cfg.DomainHandler.List)

					r.With(cfg.AuthMiddleware.RequirePermission("domains", "write")).
						Post("/", cfg.DomainHandler.Create)

					r.With(cfg.AuthMiddleware.RequirePermission("domains", "delete")).
						Delete("/{id}", cfg.DomainHandler.Delete)

					r.With(cfg.AuthMiddleware.RequirePermission("domains", "write")).
						Post("/{id}/ssl", cfg.DomainHandler.ProvisionSSL)
				})
			}

			if cfg.AppHandler != nil {
				r.Route("/applications", func(r chi.Router) {
					r.With(cfg.AuthMiddleware.RequirePermission("applications", "read")).
						Get("/", cfg.AppHandler.List)

					r.With(cfg.AuthMiddleware.RequirePermission("applications", "write")).
						Post("/", cfg.AppHandler.Create)

					r.With(cfg.AuthMiddleware.RequirePermission("applications", "read")).
						Get("/{id}", cfg.AppHandler.GetByID)

					r.With(cfg.AuthMiddleware.RequirePermission("applications", "write")).
						With(auth_middleware.ValidateEnvVars).
						Put("/{id}/env", cfg.AppHandler.UpdateEnv)

					r.With(cfg.AuthMiddleware.RequirePermission("applications", "deploy")).
						Post("/{id}/deploy", cfg.AppHandler.TriggerDeploy)
				})
			}

			if cfg.AuditHandler != nil {
				r.With(cfg.AuthMiddleware.RequirePermission("audit_logs", "read")).
					Get("/audit", cfg.AuditHandler.HandleGetTenantLogs)

				r.With(cfg.AuthMiddleware.RequirePermission("server", "manage")).
					Get("/admin/alerts", cfg.AuditHandler.HandleGetAdminAlerts)
			}

			if cfg.WSHandler != nil {
				r.With(cfg.AuthMiddleware.RequirePermission("applications", "read")).
					With(auth_middleware.ValidateTraceID("trace_id")).
					Get("/ws/deployments/{trace_id}", cfg.WSHandler.StreamDeploymentLogs)
			}
		})
	})

	registerHealthRoutes(r, cfg.ReadinessCheck)

	r.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("pong"))
	})

	// 🛡️ Setup Guard: Wraps the entire router to enforce setup-first flow
	if cfg.SetupHandler != nil {
		guardedRouter := chi.NewRouter()
		guardedRouter.Use(cfg.SetupHandler.SetupGuard)
		guardedRouter.Mount("/", r)
		return guardedRouter
	}

	return r
}

func registerHealthRoutes(router chi.Router, check func(context.Context) error) {
	router.Get("/health", handleLiveness)
	router.Get("/ready", handleReadiness(check))
}

func handleLiveness(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"alive"}`))
}

func handleReadiness(check func(context.Context) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if check == nil || check(r.Context()) == nil {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ready"}`))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"not_ready"}`))
	}
}
