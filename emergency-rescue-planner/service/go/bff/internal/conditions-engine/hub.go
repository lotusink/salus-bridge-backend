package conditions_engine

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Hub maintains the set of active WebSocket connections and broadcasts events.
// mu protects the clients / sessionClients maps; writeMu serialises all
// WriteMessage calls so concurrent Broadcast / SendToClient goroutines don't
// hit gorilla/websocket's "concurrent write to websocket connection" panic
// (the library requires writes on the same conn to be externally serialised).
type Hub struct {
	mu             sync.RWMutex
	writeMu        sync.Mutex
	clients        map[string]*websocket.Conn // clientID → conn
	sessionClients map[string]string          // volunteerSession → clientID
}

// NewHub creates an empty Hub.
func NewHub() *Hub {
	return &Hub{
		clients:        make(map[string]*websocket.Conn),
		sessionClients: make(map[string]string),
	}
}

// Register adds conn to the Hub under clientID. If volunteerSession is non-empty,
// the session→clientID mapping is stored for per-client sends.
func (h *Hub) Register(clientID string, volunteerSession string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[clientID] = conn
	if volunteerSession != "" {
		h.sessionClients[volunteerSession] = clientID
	}
}

// Unregister removes clientID from the Hub and closes its connection.
// Also removes any session→clientID mapping that referenced clientID.
// Safe to call for a non-existent clientID (no-op).
func (h *Hub) Unregister(clientID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if conn, ok := h.clients[clientID]; ok {
		conn.Close()
		delete(h.clients, clientID)
	}
	for sess, cid := range h.sessionClients {
		if cid == clientID {
			delete(h.sessionClients, sess)
			break
		}
	}
}

// Broadcast serialises envelope and sends it to all registered connections.
// Slow or dead connections are evicted: each write is bounded by a 5-second
// deadline. Failed connections are closed and removed after the send loop.
func (h *Hub) Broadcast(envelope WsEnvelopeWire) {
	data, err := json.Marshal(envelope)
	if err != nil {
		return
	}

	h.mu.RLock()
	ids := make([]string, 0, len(h.clients))
	conns := make([]*websocket.Conn, 0, len(h.clients))
	for id, conn := range h.clients {
		ids = append(ids, id)
		conns = append(conns, conn)
	}
	h.mu.RUnlock()

	h.writeMu.Lock()
	var failed []string
	for i, conn := range conns {
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			failed = append(failed, ids[i])
		}
		conn.SetWriteDeadline(time.Time{})
	}
	h.writeMu.Unlock()

	for _, id := range failed {
		h.Unregister(id)
	}
}

// SendToClient sends envelope to the single connection registered under volunteerSession.
// Returns an error if no connection is registered for that session, or if the write fails.
// On write failure the client is evicted.
func (h *Hub) SendToClient(volunteerSession string, envelope WsEnvelopeWire) error {
	data, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}

	h.mu.RLock()
	clientID, ok := h.sessionClients[volunteerSession]
	if !ok {
		h.mu.RUnlock()
		return fmt.Errorf("no connection for session %s", volunteerSession)
	}
	conn := h.clients[clientID]
	h.mu.RUnlock()

	h.writeMu.Lock()
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	writeErr := conn.WriteMessage(websocket.TextMessage, data)
	conn.SetWriteDeadline(time.Time{})
	h.writeMu.Unlock()
	if writeErr != nil {
		h.Unregister(clientID)
		return fmt.Errorf("write to client: %w", writeErr)
	}
	return nil
}

// CloseAll closes every connection and empties the Hub.
func (h *Hub) CloseAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, conn := range h.clients {
		conn.Close()
		delete(h.clients, id)
	}
	for sess := range h.sessionClients {
		delete(h.sessionClients, sess)
	}
}
