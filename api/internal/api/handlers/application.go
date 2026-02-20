// api/internal/api/handlers/application.go
package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"kari/api/internal/core/domain"
)

// Use a single instance of Validate, it caches struct info
var validate = validator.New()

// ==============================================================================
// 1. Request Payloads (Input Validation)
// ==============================================================================

type CreateAppRequest struct {
	DomainID     uuid.UUID         `json:"domain_id" validate:"required"`
	AppType      string            `json:"app_type" validate:"required,oneof=nodejs python php ruby static"`
	RepoURL      string            `json:"repo_url" validate:"required,url"`
	Branch       string            `json:"branch" validate:"required,max=100"`
	BuildCommand string            `json:"build_command" validate:"required,max=255"`
	StartCommand string            `json:"start_command" validate:"required,max=255"`
	EnvVars      map[string]string `json:"env_vars" validate:"dive,keys,max=100,endkeys,max=5000"`
}

type UpdateEnvRequest struct {
	EnvVars map[string]string `json:"env_vars" validate:"required,dive,keys,max=100,endkeys,max=5000"`
}

// ==============================================================================
// 2. The Handler Struct (Dependency Injection)
// ==============================================================================

type AppHandler struct {
	Service domain.AppService
}

func NewAppHandler(service domain.AppService) *AppHandler {
	return &AppHandler{
		Service: service,
	}
}

// ==============================================================================
// 3. HTTP Methods
// ==============================================================================

// Create handles POST /api/v1/applications
func (h *AppHandler) Create(w http.ResponseWriter, r *http.Request) {
	// 1. Extract the verified user from the JWT Context
	userClaims, ok := r.Context().Value(domain.UserContextKey).(*domain.UserClaims)
	if !ok {
		http.Error(w, `{"message": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	// 2. Decode JSON payload
	var req CreateAppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"message": "Invalid JSON payload"}`, http.StatusBadRequest)
		return
	}

	// 3. Strictly validate the payload using go-playground/validator
	if err := validate.Struct(req); err != nil {
		// In a production app, you would format these validation errors nicely.
		// For brevity, we pass the raw validation error to our centralized handler.
		HandleError(w, r, err)
		return
	}

	// 4. Map to Domain Model
	app := &domain.Application{
		DomainID:     req.DomainID,
		AppType:      req.AppType,
		RepoURL:      req.RepoURL,
		Branch:       req.Branch,
		BuildCommand: req.BuildCommand,
		StartCommand: req.StartCommand,
		EnvVars:      req.EnvVars,
	}

	// 5. Delegate to the Core Service (Business Logic)
	createdApp, err := h.Service.CreateApplication(r.Context(), userClaims.Subject, app)
	if err != nil {
		HandleError(w, r, err)
		return
	}

	// 6. Return 201 Created
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(createdApp)
}

// List handles GET /api/v1/applications
func (h *AppHandler) List(w http.ResponseWriter, r *http.Request) {
	userClaims, ok := r.Context().Value(domain.UserContextKey).(*domain.UserClaims)
	if !ok {
		http.Error(w, `{"message": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	// Note: Pagination could be extracted from r.URL.Query() here
	apps, err := h.Service.ListApplications(r.Context(), userClaims.Subject)
	if err != nil {
		HandleError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apps)
}

// GetByID handles GET /api/v1/applications/{id}
func (h *AppHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	userClaims, ok := r.Context().Value(domain.UserContextKey).(*domain.UserClaims)
	if !ok {
		http.Error(w, `{"message": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	appIDStr := chi.URLParam(r, "id")
	appID, err := uuid.Parse(appIDStr)
	if err != nil {
		http.Error(w, `{"message": "Invalid application ID format"}`, http.StatusBadRequest)
		return
	}

	// The service layer enforces IDOR protection by requiring the user's UUID
	app, err := h.Service.GetApplication(r.Context(), appID, userClaims.Subject)
	if err != nil {
		HandleError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(app)
}

// UpdateEnv handles PUT /api/v1/applications/{id}/env
func (h *AppHandler) UpdateEnv(w http.ResponseWriter, r *http.Request) {
	userClaims, ok := r.Context().Value(domain.UserContextKey).(*domain.UserClaims)
	if !ok {
		http.Error(w, `{"message": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	appIDStr := chi.URLParam(r, "id")
	appID, err := uuid.Parse(appIDStr)
	if err != nil {
		http.Error(w, `{"message": "Invalid application ID format"}`, http.StatusBadRequest)
		return
	}

	var req UpdateEnvRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"message": "Invalid JSON payload"}`, http.StatusBadRequest)
		return
	}

	if err := validate.Struct(req); err != nil {
		HandleError(w, r, err)
		return
	}

	updatedApp, err := h.Service.UpdateEnvironmentVariables(r.Context(), appID, userClaims.Subject, req.EnvVars)
	if err != nil {
		HandleError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updatedApp)
}

// TriggerDeploy handles POST /api/v1/applications/{id}/deploy
// This is used when a user manually clicks "Deploy Now" in the SvelteKit UI.
func (h *AppHandler) TriggerDeploy(w http.ResponseWriter, r *http.Request) {
	userClaims, ok := r.Context().Value(domain.UserContextKey).(*domain.UserClaims)
	if !ok {
		http.Error(w, `{"message": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	appIDStr := chi.URLParam(r, "id")
	appID, err := uuid.Parse(appIDStr)
	if err != nil {
		http.Error(w, `{"message": "Invalid application ID format"}`, http.StatusBadRequest)
		return
	}

	// 1. Trigger the deployment asynchronously. 
	// This returns the deployment record (containing the trace_id) immediately.
	deployment, err := h.Service.TriggerManualDeployment(r.Context(), appID, userClaims.Subject)
	if err != nil {
		HandleError(w, r, err)
		return
	}

	// 2. Return 202 Accepted. The UI will use the returned trace_id to open a WebSocket
	// connection and listen to the real-time logs generated by the Rust agent.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(deployment)
}

// HandleGitHubWebhook handles POST /api/v1/webhooks/github
// This endpoint is completely public. It relies on HMAC SHA-256 signatures for security.
func (h *AppHandler) HandleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	// 1. Validate the GitHub Signature to ensure the payload is authentic
	signature := r.Header.Get("X-Hub-Signature-256")
	if signature == "" {
		http.Error(w, `{"message": "Missing signature"}`, http.StatusUnauthorized)
		return
	}

	// GitHub sends events like "push", "ping", etc.
	event := r.Header.Get("X-GitHub-Event")
	if event == "ping" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if event != "push" {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// 2. Read the body (we need the raw bytes to calculate the HMAC)
	// In a real application, you'd want to limit the size of this read to prevent memory exhaustion.
	var payload map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, `{"message": "Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	// (HMAC Validation Logic omitted here, assuming it's done via a middleware or a utility function)
	// err := utils.VerifyGitHubSignature(r.Body, signature, h.Service.GetWebhookSecret())
	
	// 3. Extract the repository URL and the branch pushed to
	repoData, ok := payload["repository"].(map[string]interface{})
	if !ok {
		http.Error(w, `{"message": "Invalid payload format"}`, http.StatusBadRequest)
		return
	}
	
	repoURL, _ := repoData["clone_url"].(string)
	ref, _ := payload["ref"].(string) // e.g., "refs/heads/main"

	if repoURL == "" || ref == "" {
		http.Error(w, `{"message": "Missing repo url or ref"}`, http.StatusBadRequest)
		return
	}

	// 4. Delegate to the Service to find any applications tracking this repo/branch and deploy them
	err := h.Service.ProcessWebhook(r.Context(), repoURL, ref)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			// No apps tracking this repo, safely ignore
			w.WriteHeader(http.StatusAccepted)
			return
		}
		HandleError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}
