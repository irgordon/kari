// api/internal/api/handlers/application.go
package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	// "errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"kari/api/internal/core/domain"
	"kari/api/internal/core/utils"
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
	userClaims, ok := r.Context().Value(domain.UserContextKey).(*domain.UserClaims)
	if !ok {
		http.Error(w, `{"message": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req CreateAppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"message": "Invalid JSON payload"}`, http.StatusBadRequest)
		return
	}

	if err := validate.Struct(req); err != nil {
		HandleError(w, r, err)
		return
	}

	app := &domain.Application{
		DomainID:     req.DomainID,
		AppType:      req.AppType,
		RepoURL:      req.RepoURL,
		Branch:       req.Branch,
		BuildCommand: req.BuildCommand,
		StartCommand: req.StartCommand,
		EnvVars:      req.EnvVars,
	}

	createdApp, err := h.Service.CreateApplication(r.Context(), userClaims.Subject, app)
	if err != nil {
		HandleError(w, r, err)
		return
	}

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

	deployment, err := h.Service.TriggerManualDeployment(r.Context(), appID, userClaims.Subject)
	if err != nil {
		HandleError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(deployment)
}

// HandleGitHubWebhook handles POST /api/v1/webhooks/github/{id}
func (h *AppHandler) HandleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	// 1. Parse the Application ID from the URL
	appIDStr := chi.URLParam(r, "id")
	appID, err := uuid.Parse(appIDStr)
	if err != nil {
		http.Error(w, `{"message": "Invalid application ID"}`, http.StatusBadRequest)
		return
	}

	// 2. Fetch the Application (and its decrypted webhook secret)
	app, err := h.Service.GetApplicationSystem(r.Context(), appID)
	if err != nil {
		http.Error(w, `{"message": "Not found"}`, http.StatusNotFound)
		return
	}

	// 3. Read the RAW bytes for cryptographic HMAC validation (Safe due to MaxBytes middleware)
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"message": "Failed to read body"}`, http.StatusInternalServerError)
		return
	}

	// Re-populate the body so json.NewDecoder can read it later
	r.Body = io.NopCloser(bytes.NewBuffer(rawBody))

	// 4. Validate the HMAC Signature (Fails instantly if forged)
	signature := r.Header.Get("X-Hub-Signature-256")
	if err := utils.VerifyGitHubSignature(rawBody, signature, app.WebhookSecret); err != nil {
		// Log the attack attempt, but return a generic 401
		// h.Service.Logger.Warn("Forged Webhook", ...) 
		http.Error(w, `{"message": "Unauthorized: Invalid signature"}`, http.StatusUnauthorized)
		return
	}

	// 5. Ignore non-push events
	event := r.Header.Get("X-GitHub-Event")
	if event == "ping" {
		w.WriteHeader(http.StatusOK)
		return
	}
	if event != "push" {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// 6. Safely decode the JSON payload
	var payload map[string]interface{}
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		http.Error(w, `{"message": "Invalid JSON payload"}`, http.StatusBadRequest)
		return
	}

	ref, _ := payload["ref"].(string)

	// 7. Check if the push was to the specific branch this app is tracking
	expectedRef := "refs/heads/" + app.Branch
	if ref != expectedRef {
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"message": "Ignored: push to untracked branch"}`))
		return
	}

	// 8. Trigger the GitOps Deployment asynchronously
	go func() {
		_ = h.Service.TriggerSystemDeployment(context.Background(), appID)
	}()

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(`{"message": "Deployment triggered successfully"}`))
}
