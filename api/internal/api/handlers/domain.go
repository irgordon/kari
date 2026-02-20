// api/internal/api/handlers/domain.go
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"kari/api/internal/core/domain"
)

// ==============================================================================
// 1. Request Payloads (Input Validation)
// ==============================================================================

type CreateDomainRequest struct {
	// fqdn tag ensures the user inputs a valid Fully Qualified Domain Name (e.g., app.example.com)
	// It prevents injection of malformed strings that could break the Nginx template.
	DomainName   string `json:"domain_name" validate:"required,fqdn,max=255"`
	DocumentRoot string `json:"document_root" validate:"required,max=512"`
}

// ==============================================================================
// 2. The Handler Struct (Dependency Injection)
// ==============================================================================

type DomainHandler struct {
	Service domain.DomainService
}

func NewDomainHandler(service domain.DomainService) *DomainHandler {
	return &DomainHandler{
		Service: service,
	}
}

// ==============================================================================
// 3. HTTP Methods
// ==============================================================================

// List handles GET /api/v1/domains
func (h *DomainHandler) List(w http.ResponseWriter, r *http.Request) {
	// 1. Extract the cryptographically verified user from the JWT Context
	userClaims, ok := r.Context().Value(domain.UserContextKey).(*domain.UserClaims)
	if !ok {
		http.Error(w, `{"message": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	// 2. Fetch the domains scoped exclusively to this user ID
	domains, err := h.Service.ListDomains(r.Context(), userClaims.Subject)
	if err != nil {
		HandleError(w, r, err)
		return
	}

	// 3. Return JSON array
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(domains)
}

// Create handles POST /api/v1/domains
func (h *DomainHandler) Create(w http.ResponseWriter, r *http.Request) {
	userClaims, ok := r.Context().Value(domain.UserContextKey).(*domain.UserClaims)
	if !ok {
		http.Error(w, `{"message": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req CreateDomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"message": "Invalid JSON payload"}`, http.StatusBadRequest)
		return
	}

	// Strict payload validation (fqdn check)
	if err := validate.Struct(req); err != nil {
		HandleError(w, r, err)
		return
	}

	// Map the validated request to our Domain model
	newDomain := &domain.Domain{
		UserID:       userClaims.Subject,
		DomainName:   req.DomainName,
		DocumentRoot: req.DocumentRoot,
		SSLStatus:    "none", // Default state
	}

	// The Service layer will insert this into Postgres AND instruct the Rust Agent
	// to generate and activate the Nginx reverse proxy configuration.
	createdDomain, err := h.Service.CreateDomain(r.Context(), newDomain)
	if err != nil {
		HandleError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(createdDomain)
}

// Delete handles DELETE /api/v1/domains/{id}
func (h *DomainHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userClaims, ok := r.Context().Value(domain.UserContextKey).(*domain.UserClaims)
	if !ok {
		http.Error(w, `{"message": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	domainIDStr := chi.URLParam(r, "id")
	domainID, err := uuid.Parse(domainIDStr)
	if err != nil {
		http.Error(w, `{"message": "Invalid domain ID format"}`, http.StatusBadRequest)
		return
	}

	// The Service layer enforces IDOR protection. It will verify that the user actually owns
	// this Domain ID before attempting to delete it from the database and instructing Rust 
	// to remove the Nginx configs.
	err = h.Service.DeleteDomain(r.Context(), domainID, userClaims.Subject)
	if err != nil {
		HandleError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent) // 204 No Content for successful deletion
}

// ProvisionSSL handles POST /api/v1/domains/{id}/ssl
// This manually triggers the Let's Encrypt generation flow for a specific domain.
func (h *DomainHandler) ProvisionSSL(w http.ResponseWriter, r *http.Request) {
	userClaims, ok := r.Context().Value(domain.UserContextKey).(*domain.UserClaims)
	if !ok {
		http.Error(w, `{"message": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	domainIDStr := chi.URLParam(r, "id")
	domainID, err := uuid.Parse(domainIDStr)
	if err != nil {
		http.Error(w, `{"message": "Invalid domain ID format"}`, http.StatusBadRequest)
		return
	}

	// Because SSL provisioning takes several seconds (verifying DNS, negotiating with Let's Encrypt),
	// we do not block the HTTP request. We trigger it asynchronously and return a 202 Accepted.
	
	// In a production environment, you might dispatch this to a Redis queue (e.g., Asynq or machinery).
	// For Karı's lightweight architecture, we dispatch it to a managed goroutine within the service.
	err = h.Service.TriggerSSLProvisioning(r.Context(), domainID, userClaims.Subject)
	if err != nil {
		// If the domain is already "active" or "renewing", the service will return an error here
		// instantly, before spawning the background task.
		HandleError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(`{"message": "SSL provisioning started. The domain status will update shortly."}`))
}
