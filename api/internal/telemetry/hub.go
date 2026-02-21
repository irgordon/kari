package telemetry

import (
	"sync"
)

// Hub manages active log streams for the Kari Panel
type Hub struct {
	mu          sync.RWMutex
	subscribers map[string][]chan string // deploymentID -> list of client channels
}

func NewHub() *Hub {
	return &Hub{
		subscribers: make(map[string][]chan string),
	}
}

// Subscribe adds a new UI client to a deployment log stream
func (h *Hub) Subscribe(deploymentID string) chan string {
	h.mu.Lock()
	defer h.mu.Unlock()

	ch := make(chan string, 100) // Buffer to prevent slow clients from blocking the worker
	h.subscribers[deploymentID] = append(h.subscribers[deploymentID], ch)
	return ch
}

// Unsubscribe removes a client channel
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
}

// Broadcast sends a log chunk to all listeners of a deployment
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
