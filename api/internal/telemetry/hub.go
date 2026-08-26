package telemetry

import (
	"context"
	"sync"
)

// Hub manages active log streams for the Kari Panel.
// 🛡️ SLA: Implements backpressure (drop-on-full) and hanging-stream cancellation.
type Hub struct {
	mu          sync.RWMutex
	subscribers map[string][]chan string      // deploymentID -> list of client channels
	cancels     map[string]context.CancelFunc // deploymentID -> cancel func for gRPC stream
}

func NewHub() *Hub {
	return &Hub{
		subscribers: make(map[string][]chan string),
		cancels:     make(map[string]context.CancelFunc),
	}
}

// RegisterCancel stores a cancellation function for a deployment's gRPC stream.
// The DeploymentWorker calls this before starting the stream, enabling the Hub
// to signal teardown when the last SSE consumer disconnects.
func (h *Hub) RegisterCancel(deploymentID string, cancel context.CancelFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cancels[deploymentID] = cancel
}

// Subscribe adds a new UI client to a deployment log stream.
func (h *Hub) Subscribe(deploymentID string) chan string {
	h.mu.Lock()
	defer h.mu.Unlock()

	ch := make(chan string, 100) // Buffer to prevent slow clients from blocking the worker
	h.subscribers[deploymentID] = append(h.subscribers[deploymentID], ch)
	return ch
}

// Unsubscribe removes a client channel.
// 🛡️ Hanging-Stream Prevention: If this was the LAST subscriber, fire the gRPC cancel
// so the Muscle stops streaming logs to a ghost consumer.
func (h *Hub) Unsubscribe(deploymentID string, ch chan string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	subs := h.subscribers[deploymentID]
	for i, sub := range subs {
		if sub == ch {
			h.subscribers[deploymentID] = append(subs[:i], subs[i+1:]...)
			close(ch)
			break
		}
	}

	// 🛡️ If no subscribers remain, cancel the gRPC stream to free Muscle CPU
	if len(h.subscribers[deploymentID]) == 0 {
		if cancel, ok := h.cancels[deploymentID]; ok {
			cancel()
			delete(h.cancels, deploymentID)
		}
		delete(h.subscribers, deploymentID)
	}
}

// HasSubscribers returns true if at least one UI client is listening.
func (h *Hub) HasSubscribers(deploymentID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subscribers[deploymentID]) > 0
}

// Broadcast sends a log chunk to all listeners of a deployment.
// 🛡️ SLA: Uses select+default to drop messages for slow clients (backpressure).
func (h *Hub) Broadcast(deploymentID string, message string) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if subs, ok := h.subscribers[deploymentID]; ok {
		for _, ch := range subs {
			select {
			case ch <- message:
			default: // Drop message if buffer is full to preserve SLA stability
			}
		}
	}
}
