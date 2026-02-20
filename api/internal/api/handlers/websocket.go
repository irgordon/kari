// api/internal/api/handlers/websocket.go
package handlers

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"

	"kari/api/internal/core/domain"
)

// ==============================================================================
// 1. WebSocket Configuration & Constants
// ==============================================================================

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer. (We only stream OUT, so inbound is tiny).
	maxMessageSize = 512
)

// We configure the Gorilla WebSocket upgrader.
// Security: Because this handler is protected by our global Chi AuthMiddleware,
// we already know the request has a valid HttpOnly session cookie and passed CORS.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// In production, you would strictly match this against your allowed frontend domains.
		// We return true here because the Chi router's CORS middleware already validated the Origin header.
		return true
	},
}

// ==============================================================================
// 2. The Handler Struct (Dependency Injection)
// ==============================================================================

type WebSocketHandler struct {
	Service domain.DeploymentStreamService
	Logger  *slog.Logger
}

func NewWebSocketHandler(service domain.DeploymentStreamService, logger *slog.Logger) *WebSocketHandler {
	return &WebSocketHandler{
		Service: service,
		Logger:  logger,
	}
}

// ==============================================================================
// 3. HTTP Methods (The Upgrader)
// ==============================================================================

// StreamDeploymentLogs handles GET /api/v1/ws/deployments/{trace_id}
func (h *WebSocketHandler) StreamDeploymentLogs(w http.ResponseWriter, r *http.Request) {
	// 1. Extract the verified user from the JWT Context
	// This physically prevents a tenant from guessing another tenant's trace_id and snooping on their logs.
	userClaims, ok := r.Context().Value(domain.UserContextKey).(*domain.UserClaims)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	traceID := chi.URLParam(r, "trace_id")
	if traceID == "" {
		http.Error(w, "Missing trace_id", http.StatusBadRequest)
		return
	}

	// 2. Upgrade the HTTP connection to a full-duplex WebSocket connection
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.Logger.Error("Failed to upgrade WebSocket connection", 
			slog.String("trace_id", traceID), 
			slog.String("error", err.Error()),
		)
		return
	}

	// 3. Subscribe to the Core Service for the log stream.
	// The service layer verifies that `userClaims.Subject` actually owns the application tied to this `traceID`.
	// It returns a read-only Go channel that will yield log chunks as they arrive from the Rust Agent.
	logChannel, err := h.Service.SubscribeToDeploymentLogs(r.Context(), traceID, userClaims.Subject)
	if err != nil {
		h.Logger.Warn("WebSocket subscription rejected", 
			slog.String("trace_id", traceID), 
			slog.String("error", err.Error()),
		)
		// Send a clean closure message to the frontend so the UI doesn't hang
		ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, err.Error()))
		ws.Close()
		return
	}

	// 4. Hand off the connection to the concurrent pump managers
	// We use two separate goroutines to handle reading and writing simultaneously.
	
	// The Read Pump handles incoming control messages (like Ping/Pong) to keep the connection alive.
	go h.readPump(ws, traceID)
	
	// The Write Pump takes the Go channel and streams it to the browser.
	// This blocks the current HTTP handler thread until the deployment finishes or the user closes the tab.
	h.writePump(ws, logChannel, traceID)
}

// ==============================================================================
// 4. The Write Pump (Streaming Logs to SvelteKit)
// ==============================================================================

func (h *WebSocketHandler) writePump(ws *websocket.Conn, logChannel <-chan domain.LogChunk, traceID string) {
	// Ensure the WebSocket is closed when this function exits to prevent memory leaks
	defer func() {
		ws.Close()
		h.Logger.Info("WebSocket write pump closed", slog.String("trace_id", traceID))
	}()

	// Ticker for sending periodic Ping messages to ensure the browser hasn't silently dropped off
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		// Case 1: We receive a new log chunk from the Go channel (originated from Rust)
		case chunk, ok := <-logChannel:
			ws.SetWriteDeadline(time.Now().Add(writeWait))
			
			if !ok {
				// The channel was closed by the Service layer. This means the deployment finished successfully.
				ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "Deployment completed"))
				return
			}

			// Serialize the chunk to JSON and push it over the WebSocket
			err := ws.WriteJSON(chunk)
			if err != nil {
				h.Logger.Error("Failed to write JSON to WebSocket", 
					slog.String("trace_id", traceID), 
					slog.String("error", err.Error()),
				)
				return // Drop the connection if writing fails (e.g., broken pipe)
			}

			// If the chunk itself signals EOF, we can gracefully close
			if chunk.IsEOF {
				ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "EOF reached"))
				return
			}

		// Case 2: The Ping ticker fires
		case <-ticker.C:
			ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				return // Browser disconnected, exit the loop
			}
		}
	}
}

// ==============================================================================
// 5. The Read Pump (Connection Keep-Alive)
// ==============================================================================

func (h *WebSocketHandler) readPump(ws *websocket.Conn, traceID string) {
	defer func() {
		ws.Close()
	}()

	// Configure limits and timeouts
	ws.SetReadLimit(maxMessageSize)
	ws.SetReadDeadline(time.Now().Add(pongWait))
	
	// Every time we receive a Pong from the browser, we reset the deadline
	ws.SetPongHandler(func(string) error { 
		ws.SetReadDeadline(time.Now().Add(pongWait))
		return nil 
	})

	// Enter an infinite loop reading messages. 
	// Since this is a one-way log stream, we don't actually care about text messages from the client.
	// We just read to process control messages (Pong/Close) and detect disconnects.
	for {
		_, _, err := ws.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				h.Logger.Warn("WebSocket closed unexpectedly", 
					slog.String("trace_id", traceID), 
					slog.String("error", err.Error()),
				)
			}
			break
		}
	}
}
