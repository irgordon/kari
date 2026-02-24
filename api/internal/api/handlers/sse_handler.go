package handlers

import (
	"context"
	"fmt"
	"net/http"
	// "time"

	"github.com/gin-gonic/gin"
	agent "kari/api/proto/kari/agent/v1"
	"log/slog"
)

/**
 * 🛡️ SLA: StreamDeploymentLogs
 * Relays real-time build/runtime logs from the Muscle Agent to the Frontend.
 * Implements manual flushing and context cancellation to prevent memory leaks.
 */
func StreamDeploymentLogs(grpcClient agent.SystemAgentClient, logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.Param("trace_id")
		if traceID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Missing trace_id"})
			return
		}

		// 🛡️ Set headers for SSE
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("Connection", "keep-alive")
		c.Writer.Header().Set("Transfer-Encoding", "chunked")

		// 🛡️ Zero-Trust: Link gRPC lifetime to HTTP request lifetime
		ctx, cancel := context.WithCancel(c.Request.Context())
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
					fmt.Fprintf(c.Writer, "data: [SYSTEM] Stream closed by Muscle Agent\n\n")
					c.Writer.Flush()
					return
				}

				// 🛡️ Format as SSE data
				// We use "data: " prefix and double newline as per spec
				_, err = fmt.Fprintf(c.Writer, "data: %s\n\n", chunk.Content)
				if err != nil {
					logger.Warn("Failed to write to SSE client", "trace_id", traceID, "error", err)
					return
				}

				// Force push the buffer to the frontend
				c.Writer.Flush()
			}
		}
	}
}