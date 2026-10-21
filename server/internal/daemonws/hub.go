// Package daemonws tracks daemon connections, one per daemon id.
package daemonws

import (
	"sync"

	"github.com/collabos/collabos/server/internal/wsconn"
	"github.com/collabos/collabos/server/pkg/protocol"
)

// Hub maps daemon ids to their live connection.
type Hub struct {
	mu    sync.RWMutex
	conns map[string]*wsconn.Conn
}

// NewHub creates an empty hub.
func NewHub() *Hub { return &Hub{conns: map[string]*wsconn.Conn{}} }

// Register replaces any previous connection for the daemon.
func (h *Hub) Register(daemonID string, c *wsconn.Conn) {
	h.mu.Lock()
	if old := h.conns[daemonID]; old != nil && old != c {
		old.Close()
	}
	h.conns[daemonID] = c
	h.mu.Unlock()
}

// Unregister removes c if it is still the current connection; reports whether it was.
func (h *Hub) Unregister(daemonID string, c *wsconn.Conn) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.conns[daemonID] == c {
		delete(h.conns, daemonID)
		return true
	}
	return false
}

// Online reports whether the daemon is connected.
func (h *Hub) Online(daemonID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.conns[daemonID] != nil
}

// Send delivers e to a daemon; false if offline.
func (h *Hub) Send(daemonID string, e protocol.Envelope) bool {
	h.mu.RLock()
	c := h.conns[daemonID]
	h.mu.RUnlock()
	if c == nil {
		return false
	}
	c.Send(e)
	return true
}

// Kick closes the daemon's live connection (device token revoked).
func (h *Hub) Kick(daemonID string) {
	h.mu.RLock()
	c := h.conns[daemonID]
	h.mu.RUnlock()
	if c != nil {
		c.Close()
	}
}
