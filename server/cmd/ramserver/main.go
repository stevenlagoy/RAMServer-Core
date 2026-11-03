package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/stevenlagoy/ramserver-core/server/game"
	"github.com/stevenlagoy/ramserver-core/server/internal/server"
	"github.com/stevenlagoy/ramserver-core/server/internal/session"
	"github.com/stevenlagoy/ramserver-core/server/rulesets/chess"
)

const defaultPort = "8080"

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	addr := os.Getenv("RAMSERVER_ADDR")
	if addr == "" {
		addr = ":" + port
	}

	// Create a TCP listener
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal("Error creating listener:", err)
	}
	defer listener.Close()

	// Context for coordinating shutdown; cancelled on SIGINT/SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	// Debug: enable AutoReady for new players
	session.AutoReady = os.Getenv("RAMSERVER_AUTOREADY") != ""
	games := game.NewRegistry()
	games.Register("chess", func() game.Game { return &chess.Game{} })
	if err := server.Serve(ctx, listener, server.DefaultConfig(), games); err != nil {
		log.Fatal(err)
	}
}
