package conditions_engine

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
)

// dialTestHub spins up a real WS test server, dials a client to it, and
// hands back (serverConn, clientConn, cleanup). The serverConn can be
// registered to a Hub for concurrency tests; the clientConn reads silently
// in a goroutine so server writes don't block.
func dialTestHub(t *testing.T) (*websocket.Conn, *websocket.Conn, func()) {
	t.Helper()
	upgrader := websocket.Upgrader{
		CheckOrigin: func(*http.Request) bool { return true },
	}
	connCh := make(chan *websocket.Conn, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		connCh <- conn
		// Drain client → server messages until closed.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		server.Close()
		t.Fatalf("dial: %v", err)
	}

	// Drain server → client so the server side's write buffer doesn't fill.
	go func() {
		for {
			if _, _, err := clientConn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	serverConn := <-connCh
	cleanup := func() {
		clientConn.Close()
		serverConn.Close()
		server.Close()
	}
	return serverConn, clientConn, cleanup
}

// TestHub_Broadcast_ConcurrentWritesNoPanic verifies that 50 concurrent
// Broadcast calls against a single registered connection don't panic with
// gorilla's "concurrent write to websocket connection" error. Before the
// fix, this test panics; after, it passes cleanly under -race.
func TestHub_Broadcast_ConcurrentWritesNoPanic(t *testing.T) {
	serverConn, _, cleanup := dialTestHub(t)
	defer cleanup()

	hub := NewHub()
	hub.Register("c1", "sess-1", serverConn)
	defer hub.Unregister("c1")

	envelope := WsEnvelopeWire{Type: "test", Ts: "x", Payload: nil}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hub.Broadcast(envelope)
		}()
	}
	wg.Wait()
	// No panic = pass. Race detector also covers the underlying
	// concurrent-access bug.
}

// TestHub_SendToClient_ConcurrentWritesNoPanic: same pattern for the
// per-session send path.
func TestHub_SendToClient_ConcurrentWritesNoPanic(t *testing.T) {
	serverConn, _, cleanup := dialTestHub(t)
	defer cleanup()

	hub := NewHub()
	hub.Register("c1", "sess-1", serverConn)
	defer hub.Unregister("c1")

	envelope := WsEnvelopeWire{Type: "test", Ts: "x", Payload: nil}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = hub.SendToClient("sess-1", envelope)
		}()
	}
	wg.Wait()
}

// TestHub_Broadcast_MixedWithSendToClient_NoPanic: stress the two write
// entry points concurrently against the same conn — the original panic
// reproduction scenario.
func TestHub_Broadcast_MixedWithSendToClient_NoPanic(t *testing.T) {
	serverConn, _, cleanup := dialTestHub(t)
	defer cleanup()

	hub := NewHub()
	hub.Register("c1", "sess-1", serverConn)
	defer hub.Unregister("c1")

	envelope := WsEnvelopeWire{Type: "test", Ts: "x", Payload: nil}

	var wg sync.WaitGroup
	for i := 0; i < 25; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); hub.Broadcast(envelope) }()
		go func() { defer wg.Done(); _ = hub.SendToClient("sess-1", envelope) }()
	}
	wg.Wait()
}
