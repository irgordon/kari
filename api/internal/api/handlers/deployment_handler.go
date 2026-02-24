package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"kari/api/internal/core/domain"
	"kari/api/internal/telemetry"
	"kari/api/internal/api/middleware"
)

type DeploymentHandler struct {
	repo   domain.DeploymentRepository
	crypto domain.CryptoService
	hub    *telemetry.Hub
}

func NewDeploymentHandler(repo domain.DeploymentRepository, crypto domain.CryptoService, hub *telemetry.Hub) *DeploymentHandler {
	return &DeploymentHandler{
		repo:   repo,
		crypto: crypto,
		hub:    hub,
	}
}

// CreateDeployment handles the initial POST request from the SvelteKit Wizard
func (h *DeploymentHandler) CreateDeployment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string `json:"name"`
		RepoURL      string `json:"repo_url"`
		Branch       string `json:"branch"`
		BuildCommand string `json:"build_command"`
		TargetPort   int    `json:"target_port"`
		SSHKey       string `json:"ssh_key"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Malformed request body", http.StatusBadRequest)
		return
	}

	// 🛡️ Zero-Trust: Identify the requesting user
	_, ok := r.Context().Value(middleware.UserKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// 🛡️ Cryptography: Encrypt the SSH Key before it hits the DB
	// We generate a temporary AppID or use an existing one to bind the AAD
	appID := uuid.New().String()
	var encryptedKey string
	if req.SSHKey != "" {
		enc, err := h.crypto.Encrypt(r.Context(), []byte(req.SSHKey), []byte(appID))
		if err != nil {
			http.Error(w, "Internal security error", http.StatusInternalServerError)
			return
		}
		encryptedKey = enc
	}

	// 🛡️ SLA: Persist the task as PENDING for the Worker to claim
	deployment := &domain.Deployment{
		ID:               uuid.New().String(),
		AppID:            appID,
		DomainName:       req.Name,
		RepoURL:          req.RepoURL,
		Branch:           req.Branch,
		BuildCommand:     req.BuildCommand,
		TargetPort:       req.TargetPort,
		EncryptedSSHKey:  encryptedKey,
		Status:           domain.StatusPending,
	}

	if err := h.repo.Save(r.Context(), deployment); err != nil {
		http.Error(w, "Failed to queue deployment", http.StatusInternalServerError)
		return
	}

	// Return the TraceID so the frontend can immediately subscribe to logs
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"trace_id": deployment.ID,
		"status":   "queued",
	})
}

// StreamLogs replaces the WebSocket implementation with SSE
func (h *DeploymentHandler) StreamLogs(w http.ResponseWriter, r *http.Request) {
	deploymentID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(deploymentID); err != nil {
		http.Error(w, "Invalid deployment ID", http.StatusBadRequest)
		return
	}

	// 🛡️ SLA: Establish SSE connection
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Subscribe to the Hub (broadcast from Worker)
	logChan := h.hub.Subscribe(deploymentID)
	defer h.hub.Unsubscribe(deploymentID, logChan)

	rc := http.NewResponseController(w)
	fmt.Fprintf(w, "event: connected\ndata: {\"status\": \"monitoring\"}\n\n")
	rc.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-logChan:
			// 🛡️ Logic: Format message as data chunk
			fmt.Fprintf(w, "data: %s\n\n", msg)
			if err := rc.Flush(); err != nil {
				return
			}
		}
	}
}
