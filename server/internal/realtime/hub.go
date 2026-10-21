// Package realtime fans out events to UI clients, scoped by workspace.
package realtime

import (
	"sync"

	"github.com/collabos/collabos/server/internal/wsconn"
	"github.com/collabos/collabos/server/pkg/protocol"
)

// Hub tracks UI connections per workspace.
type Hub struct {
	mu   sync.RWMutex
	subs map[string]map[*wsconn.Conn]struct{}
}

// NewHub creates an empty hub.
func NewHub() *Hub { return &Hub{subs: map[string]map[*wsconn.Conn]struct{}{}} }

// Join subscribes c to a workspace until it closes.
func (h *Hub) Join(workspaceID string, c *wsconn.Conn) {
	h.mu.Lock()
	if h.subs[workspaceID] == nil {
		h.subs[workspaceID] = map[*wsconn.Conn]struct{}{}
	}
	h.subs[workspaceID][c] = struct{}{}
	h.mu.Unlock()
	go func() {
		<-c.Closed()
		h.mu.Lock()
		delete(h.subs[workspaceID], c)
		h.mu.Unlock()
	}()
}

// Broadcast sends e to every connection in the workspace.
func (h *Hub) Broadcast(workspaceID string, e protocol.Envelope) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.subs[workspaceID] {
		c.Send(e)
	}
}

// Count returns the subscriber count (tests/metrics).
func (h *Hub) Count(workspaceID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs[workspaceID])
}
