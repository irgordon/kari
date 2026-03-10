package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
)

func TestValidateTraceID(t *testing.T) {
	tests := []struct {
		name       string
		paramName  string
		paramValue string
		wantStatus int
	}{
		{
			name:       "valid UUIDv4",
			paramName:  "id",
			paramValue: "550e8400-e29b-41d4-a716-446655440000",
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid UUID version (v1)",
			paramName:  "id",
			paramValue: "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "malformed UUID",
			paramName:  "id",
			paramValue: "not-a-uuid",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty UUID",
			paramName:  "id",
			paramValue: "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "UUID too long",
			paramName:  "id",
			paramValue: "550e8400-e29b-41d4-a716-446655440000-extra",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "uppercase valid UUIDv4",
			paramName:  "id",
			paramValue: "550E8400-E29B-41D4-A716-446655440000",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := ValidateTraceID(tt.paramName)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest("GET", "/", nil)

			// Chi path value simulation
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add(tt.paramName, tt.paramValue)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			assert.Equal(t, tt.wantStatus, rr.Code)
		})
	}
}

func TestValidateEnvVars(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		body       interface{}
		wantStatus int
	}{
		{
			name:   "valid env vars",
			method: "POST",
			body: map[string]interface{}{
				"env_vars": map[string]string{
					"DATABASE_URL": "postgres://localhost",
					"PORT":         "8080",
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "invalid key format",
			method: "POST",
			body: map[string]interface{}{
				"env_vars": map[string]string{
					"database-url": "postgres://localhost",
				},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "key too long",
			method: "POST",
			body: map[string]interface{}{
				"env_vars": map[string]string{
					"A123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789": "value",
				},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "too many env vars",
			method: "POST",
			body: func() interface{} {
				vars := make(map[string]string)
				for i := 0; i < 51; i++ {
					vars[string(rune('A'+i))] = "value"
				}
				return map[string]interface{}{"env_vars": vars}
			}(),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "non-POST/PUT methods skip validation",
			method: "GET",
			body:   nil,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := ValidateEnvVars(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			var reqBody []byte
			if tt.body != nil {
				reqBody, _ = json.Marshal(tt.body)
			}

			req := httptest.NewRequest(tt.method, "/", bytes.NewBuffer(reqBody))
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			assert.Equal(t, tt.wantStatus, rr.Code)
		})
	}
}
