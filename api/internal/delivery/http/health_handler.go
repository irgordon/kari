package http

import (
	"context"
	"net/http"
	"time"
	"kari/api/proto/agent"
)

type HealthHandler struct {
	agentClient agent.SystemAgentClient
}

func NewHealthHandler(client agent.SystemAgentClient) *HealthHandler {
	return &HealthHandler{agentClient: client}
}

func (h *HealthHandler) Check(w http.ResponseWriter, r *http.Request) {
	// 🛡️ SLA: Use a tight timeout for health checks
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	// Perform a low-impact gRPC call to verify the Muscle is awake
	// We'll use a standard 'Ping' or equivalent no-op command
	_, err := h.agentClient.GetSystemStatus(ctx, &agent.Empty{})
	
	if err != nil {
		// 🚨 FAIL: The Brain is up, but the Muscle is paralyzed
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("unhealthy: gRPC link to agent severed"))
		return
	}

	// ✅ PASS: Full system integrity verified
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("healthy"))
}
