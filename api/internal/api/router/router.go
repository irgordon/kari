// api/internal/api/router/router.go
package router

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"kari/api/internal/api/handlers"
	auth_middleware "kari/api/internal/api/middleware"
)

// RouterConfig defines the strict dependencies required to build the API routing tree.
type RouterConfig struct {
	AuthHandler    *handlers.AuthHandler
	AppHandler     *handlers.AppHandler
	DomainHandler  *handlers.DomainHandler
	AuditHandler   *handlers.AuditHandler
	WSHandler      *handlers.WebSocketHandler
	AuthMiddleware *auth_middleware.AuthMiddleware
	Logger         *slog.Logger
}

// NewRouter constructs the Chi multiplexer, attaches global middleware, and wires all endpoints.
func NewRouter(cfg RouterConfig) *chi.Mux {
	r := chi.NewRouter()

	// =========================================================================
	// 1. Global Gateway Middleware Pipeline
	// =========================================================================

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(auth_middleware.StructuredLogger(cfg.Logger))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// 🛡️ Limit all incoming JSON requests to 1 Megabyte max (OOM Protection)
	r.Use(auth_middleware.MaxBytes(1_048_576))

	// 🛡️ In-memory token bucket rate limiting
	r.Use(auth_middleware.RateLimitMiddleware)

	// 🔒 Force all connections to use TLS/SSL and inject HSTS headers
	r.Use(auth_middleware.EnforceTLS)

	// Strict CORS Configuration
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://*", "http://localhost:5173"},
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
		// Public Routes (No Auth Required)
		// ---------------------------------------------------------------------
		r.Group(func(r chi.Router) {
			r.Post("/auth/login", cfg.AuthHandler.Login)
			r.Post("/auth/refresh", cfg.AuthHandler.Refresh)
			
			// Webhook now takes an {id} to isolate database lookups
			r.Post("/webhooks/github/{id}", cfg.AppHandler.HandleGitHubWebhook)
		})

		// ---------------------------------------------------------------------
		// Protected Routes (Requires a Valid JWT)
		// ---------------------------------------------------------------------
		r.Group(func(r chi.Router) {
			r.Use(cfg.AuthMiddleware.RequireAuthentication())

			// --- Domains & SSL ---
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

			// --- Applications & Deployments ---
			r.Route("/applications", func(r chi.Router) {
				r.With(cfg.AuthMiddleware.RequirePermission("applications", "read")).
					Get("/", cfg.AppHandler.List)
				
				r.With(cfg.AuthMiddleware.RequirePermission("applications", "write")).
					Post("/", cfg.AppHandler.Create)
				
				r.With(cfg.AuthMiddleware.RequirePermission("applications", "read")).
					Get("/{id}", cfg.AppHandler.GetByID)
				
				r.With(cfg.AuthMiddleware.RequirePermission("applications", "write")).
					Put("/{id}/env", cfg.AppHandler.UpdateEnv)
				
				r.With(cfg.AuthMiddleware.RequirePermission("applications", "deploy")).
					Post("/{id}/deploy", cfg.AppHandler.TriggerDeploy)
			})

			// --- Privacy-First Observability & Audit Logs ---
			r.With(cfg.AuthMiddleware.RequirePermission("audit_logs", "read")).
				Get("/audit", cfg.AuditHandler.HandleGetTenantLogs)

			r.With(cfg.AuthMiddleware.RequirePermission("server", "manage")).
				Get("/admin/alerts", cfg.AuditHandler.HandleGetAdminAlerts)

			// --- WebSocket Real-Time Terminal Streaming ---
			r.With(cfg.AuthMiddleware.RequirePermission("applications", "read")).
				Get("/ws/deployments/{trace_id}", cfg.WSHandler.StreamDeploymentLogs)
		})
	})

	r.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("pong"))
	})

	return r
}
