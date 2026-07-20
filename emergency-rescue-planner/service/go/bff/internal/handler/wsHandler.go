package handler

import (
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	// CheckOrigin returns true unconditionally — REPLACE WITH ALLOWLIST BEFORE PRODUCTION.
	// CORS middleware does not cover WebSocket upgrades; this is the only origin gate on /ws.
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// WsHandler upgrades the HTTP request to a WebSocket connection and echoes
// every received text message back to the client. Used for connection testing.
func WsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("ws disconnected:", err)
		return
	}
	defer func(c *websocket.Conn) {
		_ = c.Close()
	}(conn)

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Println("disconnected:", err)
			break
		}
		log.Println("Receive", string(msg))

		_ = conn.WriteMessage(websocket.TextMessage, []byte("Receive: "+string(msg)))
	}
}
