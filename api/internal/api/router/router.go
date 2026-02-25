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
	AllowedOrigins []string
	AuthHandler    *handlers.AuthHandler
	AppHandler     *handlers.AppHandler
	DomainHandler  *handlers.DomainHandler
	AuditHandler   *handlers.AuditHandler
	WSHandler      *handlers.WebSocketHandler
	SetupHandler   *handlers.SetupHandler
	AuthMiddleware *auth_middleware.AuthMiddleware
	DeployHandler  *handlers.DeploymentHandler
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
		// Setup Wizard Routes (Only accessible before setup.lock exists)
		// ---------------------------------------------------------------------
		if cfg.SetupHandler != nil {
			r.Route("/setup", func(r chi.Router) {
				r.Use(cfg.SetupHandler.SetupAuth)
				r.Get("/test-muscle", cfg.SetupHandler.TestMuscle)
				r.Post("/test-db", cfg.SetupHandler.TestDB)
				r.Post("/generate-key", cfg.SetupHandler.GenerateKey)
				r.Post("/finalize", cfg.SetupHandler.Finalize)
			})
		}

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
					With(auth_middleware.ValidateEnvVars).
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
				With(auth_middleware.ValidateTraceID("trace_id")).
				Get("/ws/deployments/{trace_id}", cfg.WSHandler.StreamDeploymentLogs)
		})
	})

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
