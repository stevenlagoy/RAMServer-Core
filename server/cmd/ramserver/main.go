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
	"github.com/stevenlagoy/ramserver-core/server/internal/testgame"
)

const defaultAddr = "127.0.0.1:9000"

func main() {
	addr := os.Getenv("RAMSERVER_ADDR")
	if addr == "" {
		addr = defaultAddr
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
	games := game.NewRegistry()
	games.Register("stub", func() game.Game { return &testgame.StubGame{} })
	if err := server.Serve(ctx, listener, server.DefaultConfig(), games); err != nil {
		log.Fatal(err)
	}
}
