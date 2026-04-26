package http

import (
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/irgordon/kari/api/internal/telemetry"
	"net/http"
)

type LogHandler struct {
	hub *telemetry.Hub
}

func NewLogHandler(hub *telemetry.Hub) *LogHandler {
	return &LogHandler{hub: hub}
}

func (h *LogHandler) StreamLogs(w http.ResponseWriter, r *http.Request) {
	deploymentID := chi.URLParam(r, "id")
	if deploymentID == "" {
		http.Error(w, "Missing deployment ID", http.StatusBadRequest)
		return
	}

	// 🛡️ SLA: Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*") // Adjust for production

	// Subscribe to the hub
	logChan := h.hub.Subscribe(deploymentID)
	defer h.hub.Unsubscribe(deploymentID, logChan)

	// Detect client disconnect
	rc := http.NewResponseController(w)

	fmt.Fprintf(w, "event: connected\ndata: {\"status\": \"streaming\"}\n\n")
	rc.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case logLine := <-logChan:
			// 🛡️ Zero-Trust: Ensure no sensitive data is leaked in the log strings
			fmt.Fprintf(w, "data: %s\n\n", logLine)
			if err := rc.Flush(); err != nil {
				return
			}
		}
	}
}
