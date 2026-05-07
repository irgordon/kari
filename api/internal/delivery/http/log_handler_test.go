package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"kari/api/internal/telemetry"
	"github.com/stretchr/testify/assert"
)

func TestLogHandler_StreamLogs_NoWildcardCORS(t *testing.T) {
	hub := telemetry.NewHub()
	handler := NewLogHandler(hub)

	req, _ := http.NewRequest("GET", "/api/v1/logs/123", nil)
	rr := httptest.NewRecorder()

	// Use a cancellable context to break out of the SSE infinite loop
	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)

	// Start a goroutine to cancel the context after a short delay
	// This allows the handler to set headers and enter the loop before being stopped
	go func() {
		cancel()
	}()

	handler.StreamLogs(rr, req)

	assert.Equal(t, "text/event-stream", rr.Header().Get("Content-Type"))
	assert.Equal(t, "", rr.Header().Get("Access-Control-Allow-Origin"), "Wildcard CORS header should not be set")
}
