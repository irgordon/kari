package http

import (
	"context"
	"fmt"
	"net/http"
	"time"

	agent "github.com/irgordon/kari/api/internal/grpc/rustagent"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type HealthHandler struct {
	agentClient agent.SystemAgentClient
}

func NewHealthHandler(client agent.SystemAgentClient) *HealthHandler {
	return &HealthHandler{agentClient: client}
}

func (h *HealthHandler) Check(w http.ResponseWriter, r *http.Request) {
	// 🛡️ SLA: 2 seconds is the "Hard Boundary" for a reactive orchestration boot
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	// 🛡️ Zero-Trust: Prevent upstream proxies from caching health status
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	// 📡 Perform the "Heartbeat" probe to the Muscle
	_, err := h.agentClient.GetSystemStatus(ctx, &agent.Empty{})

	if err != nil {
		st, ok := status.FromError(err)

		// 🚨 Forensic Analysis: Why did the link fail?
		errorMessage := "unhealthy: gRPC link severed"
		if ok {
			switch st.Code() {
			case codes.DeadlineExceeded:
				errorMessage = "unhealthy: agent response timed out (high load)"
			case codes.Unavailable:
				errorMessage = "unhealthy: agent unreachable (socket missing or agent down)"
			case codes.PermissionDenied:
				errorMessage = "unhealthy: peer credential verification failed"
			default:
				errorMessage = fmt.Sprintf("unhealthy: agent error (%s)", st.Code())
			}
		}

		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, errorMessage)
		return
	}

	// ✅ PASS: The Brain and Muscle are synchronized
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "healthy")
}
