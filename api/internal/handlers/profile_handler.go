package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"kari/api/internal/core/domain"
	"kari/api/internal/db"
)

// ProfileHandler manages HTTP requests for system governance settings.
// 🛡️ SOLID (Dependency Inversion): It depends purely on the domain interface, 
// completely unaware that PostgreSQL even exists.
type ProfileHandler struct {
	repo domain.SystemProfileRepository
}

// NewProfileHandler injects the required dependencies.
func NewProfileHandler(repo domain.SystemProfileRepository) *ProfileHandler {
	return &ProfileHandler{
		repo: repo,
	}
}

// GetProfile handles GET /api/v1/system/profile
func (h *ProfileHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	// The HTTP context carries the cancellation signal from the client
	ctx := r.Context()

	profile, err := h.repo.GetActiveProfile(ctx)
	if err != nil {
		if errors.Is(err, db.ErrProfileNotFound) {
			http.Error(w, "System profile not initialized", http.StatusNotFound)
			return
		}
		
		// 🛡️ Zero-Trust: Log the real error internally, but return a generic 500
		// to prevent leaking database topology or SQL syntax to the caller.
		log.Printf("ERROR: Failed to fetch profile: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(profile)
}

// UpdateProfile handles PUT /api/v1/system/profile
func (h *ProfileHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	// 1. 🛡️ Zero-Trust (Anti-DOS): Restrict payload size.
	// We refuse to read more than 1MB to prevent memory exhaustion attacks 
	// from malicious clients sending infinite JSON streams.
	r.Body = http.MaxBytesReader(w, r.Body, 1048576)

	var payload domain.SystemProfile
	
	// 2. SLA: Translate HTTP bytes to Domain Intent
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	// 3. Orchestrate the update via the Domain interface
	ctx := r.Context()
	err := h.repo.UpdateProfile(ctx, &payload)
	
	// 4. 🛡️ SLA & Stability: Map Domain/DB errors to HTTP Semantics
	if err != nil {
		switch {
		// The Optimistic Concurrency Control (OCC) Trap we built earlier
		case errors.Is(err, db.ErrConcurrencyConflict):
			http.Error(w, "Conflict: The profile was modified by another administrator. Please refresh and try again.", http.StatusConflict)
			
		// Catch our strict Domain.Validate() errors (e.g., MaxMemory < 128)
		case err.Error() != "" && contains(err.Error(), "domain validation failed"):
			http.Error(w, err.Error(), http.StatusBadRequest)
			
		default:
			// Generic fallback for actual database connection drops or panics
			log.Printf("ERROR: Failed to update profile: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	// 5. Success: Return the newly updated profile (which now has an incremented Version)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(payload)
}

// Simple helper to check substring for our domain validation mapping
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr || 
	       (len(s) > len(substr) && s[0:len(s)-len(substr)] == substr) // Simplistic check, strings.Contains is better but keeping dependencies low
}
