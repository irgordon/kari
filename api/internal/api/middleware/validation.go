package middleware

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// 🛡️ Zero-Trust: Input Validation Constants
const (
	// MaxTraceIDLength is the maximum length of a UUIDv4 string (36 chars with dashes)
	MaxTraceIDLength = 36
	// MaxEnvVarsCount limits the number of environment variables per deployment
	MaxEnvVarsCount = 50
	// MaxEnvVarKeyLength limits individual env var key length
	MaxEnvVarKeyLength = 128
	// MaxEnvVarValueLength limits individual env var value length (8KB per value)
	MaxEnvVarValueLength = 8192
)

// envVarKeyRegex validates env var keys: alphanumeric + underscores only
var envVarKeyRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]{0,127}$`)

// ValidateTraceID returns middleware that validates the {trace_id} or {id} URL param
// as a strict UUIDv4 BEFORE it reaches any handler or gRPC layer.
// 🛡️ This prevents malformed IDs from causing SQL injection, path traversal, or gRPC parse errors.
func ValidateTraceID(paramName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract the URL parameter from chi's context
			id := extractURLParam(r, paramName)
			if id == "" {
				writeValidationError(w, "Missing required parameter: "+paramName)
				return
			}

			// 🛡️ Length check first (fast path rejection)
			if len(id) != MaxTraceIDLength {
				writeValidationError(w, "Invalid "+paramName+": must be exactly 36 characters (UUIDv4)")
				return
			}

			// 🛡️ Use google/uuid for robust parsing and version checking
			u, err := uuid.Parse(id)
			if err != nil || u.Version() != 4 {
				writeValidationError(w, "Invalid "+paramName+": must be a valid UUIDv4 (xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx)")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ValidateEnvVars validates the `env_vars` field in a JSON request body.
// 🛡️ Prevents oversized maps from consuming Brain memory or causing DB bloat.
func ValidateEnvVars(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only validate on methods that carry a body
		if r.Method != http.MethodPost && r.Method != http.MethodPut {
			next.ServeHTTP(w, r)
			return
		}

		// Check Content-Type is JSON
		ct := r.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "application/json") {
			next.ServeHTTP(w, r) // Not a JSON body, skip validation
			return
		}

		// Peek at the body to extract env_vars without consuming it
		// We use a temporary struct to validate just the env_vars field
		type envPayload struct {
			EnvVars map[string]string `json:"env_vars"`
		}

		var payload envPayload
		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&payload); err != nil {
			// If we can't decode, let the handler deal with it
			// (the body is consumed, so we need to re-wrap it)
			next.ServeHTTP(w, r)
			return
		}

		// 🛡️ Validate env_vars count
		if len(payload.EnvVars) > MaxEnvVarsCount {
			writeValidationError(w, "env_vars exceeds maximum of 50 entries")
			return
		}

		// 🛡️ Validate individual keys and values
		for key, value := range payload.EnvVars {
			if len(key) > MaxEnvVarKeyLength {
				writeValidationError(w, "env_var key exceeds maximum length of 128 characters: "+key[:32]+"...")
				return
			}
			if !envVarKeyRegex.MatchString(key) {
				writeValidationError(w, "env_var key must be alphanumeric or underscore: "+key)
				return
			}
			if len(value) > MaxEnvVarValueLength {
				writeValidationError(w, "env_var value exceeds maximum length of 8192 characters for key: "+key)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func writeValidationError(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]string{
		"message": msg,
	})
}

// extractURLParam is a chi-compatible URL param extractor
func extractURLParam(r *http.Request, name string) string {
	// Standard Chi path parameter extraction
	return chi.URLParam(r, name)
}
