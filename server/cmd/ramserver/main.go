package main

import (
	"context"
	"log"
	"net"
	"os/signal"
	"syscall"

	"github.com/stevenlagoy/ramserver-core/server/internal/server"
)

func main() {
	// Create a TCP listener
	listener, err := net.Listen("tcp", "127.0.0.1:8080") // Bind to loopback to avoid firewall prompt - change later to allow LAN
	if err != nil {
		log.Fatal("Error creating listener:", err)
	}
	defer listener.Close()

	// Context for coordinating shutdown; cancelled on SIGINT/SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := server.Serve(ctx, listener, server.DefaultConfig()); err != nil {
		log.Fatal(err)
	}
}
