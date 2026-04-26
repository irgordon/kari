package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	agent "github.com/irgordon/kari/api/internal/grpc/rustagent"
)

/**
 * 🛡️ SLA: StreamDeploymentLogs
 * Relays real-time build/runtime logs from the Muscle Agent to the Frontend.
 * Implements manual flushing and context cancellation to prevent memory leaks.
 */
func StreamDeploymentLogs(grpcClient agent.SystemAgentClient, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		traceID := chi.URLParam(r, "trace_id")
		if traceID == "" {
			http.Error(w, `{"error":"Missing trace_id"}`, http.StatusBadRequest)
			return
		}

		// 🛡️ Set headers for SSE
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Transfer-Encoding", "chunked")

		// 🛡️ Zero-Trust: Link gRPC lifetime to HTTP request lifetime
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		// 📡 Dialing the Muscle for the log stream
		stream, err := grpcClient.StreamDeployment(ctx, &agent.DeployRequest{
			TraceId: traceID,
		})
		if err != nil {
			logger.Error("Failed to initiate gRPC log stream", "trace_id", traceID, "error", err)
			return
		}

		logger.Info("SSE connection established", "trace_id", traceID)

		// 🛡️ Continuous Relay Loop
		for {
			select {
			case <-ctx.Done():
				logger.Info("SSE client disconnected, closing stream", "trace_id", traceID)
				return
			default:
				chunk, err := stream.Recv()
				if err != nil {
					// Handle EOF or Stream Termination
					fmt.Fprintf(w, "data: [SYSTEM] Stream closed by Muscle Agent\n\n")
					flush(w)
					return
				}

				// 🛡️ Format as SSE data
				// We use "data: " prefix and double newline as per spec
				_, err = fmt.Fprintf(w, "data: %s\n\n", chunk.Content)
				if err != nil {
					logger.Warn("Failed to write to SSE client", "trace_id", traceID, "error", err)
					return
				}

				// Force push the buffer to the frontend
				flush(w)
			}
		}
	}
}

func flush(w http.ResponseWriter) {
	flusher, ok := w.(http.Flusher)
	if ok {
		flusher.Flush()
	}
}
