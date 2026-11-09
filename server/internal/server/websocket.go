package server

import (
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/stevenlagoy/ramserver-core/server/game"
	"github.com/stevenlagoy/ramserver-core/server/internal/session"
	"github.com/stevenlagoy/ramserver-core/server/internal/transport"
)

func NewWebSocketHandler(config ServerConfig, games *game.Registry) http.Handler {
	sessions := session.NewSessionStore()
	upgrader := websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
	}
	mux := http.NewServeMux()

	// Health endpoint
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("RAMServer-Core is running"))
	})

	// Websocket endpoint
	mux.HandleFunc("/connect", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("Websocket upgrade failed: %v", err)
			return
		}
		defer conn.Close()
		log.Printf("WebSocket client connected: %s", r.RemoteAddr)

		netConn := transport.NewWebSocketNetConn(conn)
		NewClient(netConn, config, games, sessions).Run(r.Context())
		log.Printf("WebSocket client disconnected: %s", r.RemoteAddr)
	})

	return mux
}
