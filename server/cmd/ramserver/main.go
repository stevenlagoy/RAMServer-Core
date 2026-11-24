package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/stevenlagoy/ramserver-core/server/game"
	"github.com/stevenlagoy/ramserver-core/server/internal/server"
	"github.com/stevenlagoy/ramserver-core/server/rulesets/chess"
)

const defaultAddr = "127.0.0.1:8080"

func main() {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer stop()

	games := game.NewRegistry()
	games.Register("chess", func() game.Game {
		return &chess.Game{}
	})

	// Cloud Run supplies PORT. Use HTTP/WebSocket mode
	if port := os.Getenv("PORT"); port != "" {
		addr := ":" + port
		hub := server.NewHub()
		cfg := server.DefaultConfig()
		cfg.Hub = hub
		adminHandler, err := startCloudAdmin(hub)
		if err != nil {
			log.Fatal("Error starting the Cloud Run admin API:", err)
		}

		httpServer := &http.Server{
			Addr:              addr,
			Handler:           server.NewWebSocketHandler(ctx, cfg, games, adminHandler),
			ReadHeaderTimeout: 10 * time.Second,
		}

		go func() {
			log.Printf("HTTP/WebSocket server listening on %s", addr)

			if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatalf("HTTP server failed: %v", err)
			}
		}()

		<-ctx.Done()
		log.Println("Shutdown signal received")

		shutdownCtx, cancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer cancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP shutdown error: %v", err)
		}

		return
	}

	// Local TCP server mode
	addr := os.Getenv("RAMSERVER_ADDR")
	if addr == "" {
		addr = defaultAddr
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal("Error creating listener:", err)
	}
	defer listener.Close()

	hub := server.NewHub()
	if err := startAdmin(ctx, hub); err != nil {
		log.Fatal("Error starting the admin API:", err)
	}
	cfg := server.DefaultConfig()
	cfg.Hub = hub
	if err := server.Serve(ctx, listener, cfg, games); err != nil {
		log.Fatal(err)
	}
}
