package server

import (
	"context"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/stevenlagoy/ramserver-core/server/game"
	"github.com/stevenlagoy/ramserver-core/server/internal/session"
	"github.com/stevenlagoy/ramserver-core/server/internal/transport"
)

func NewWebSocketHandler(ctx context.Context, config ServerConfig, games *game.Registry, adminHandler http.Handler) http.Handler {
	sessions := session.NewSessionStore(ctx)
	hub := config.Hub
	if hub == nil {
		hub = NewHub()
	}
	hub.attach(sessions)
	config.Hub = hub
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
	if adminHandler != nil {
		mux.Handle("/v1/", adminHandler)
	}

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
		client := NewClient(netConn, config, games, sessions)
		remove := hub.add(client)
		defer remove()
		client.Run(r.Context())
		log.Printf("WebSocket client disconnected: %s", r.RemoteAddr)
	})

	return mux
}
