package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/stevenlagoy/ramserver-core/server/game"
	"github.com/stevenlagoy/ramserver-core/server/internal/persistence"
	"github.com/stevenlagoy/ramserver-core/server/internal/server"
	"github.com/stevenlagoy/ramserver-core/server/rulesets/StubGame"
	"github.com/stevenlagoy/ramserver-core/server/rulesets/chess"
)

const defaultAddr = "127.0.0.1:9000"

func main() {
	addr := os.Getenv("RAMSERVER_ADDR")
	if addr == "" {
		addr = defaultAddr
	}

	dbPath := os.Getenv("RAMSERVER_DB_PATH")
	if dbPath == "" {
		dbPath = "data/ramserver.sqlite"
	}

	sqliteStore, err := persistence.Open(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize persistence: %v", err)
	}
	defer sqliteStore.Close()

	pendingDir := os.Getenv("RAMSERVER_PENDING_DIR")
	if pendingDir == "" {
		pendingDir = filepath.Join(filepath.Dir(dbPath), "pending")
	}

	store := persistence.NewRecoverableStore(sqliteStore, pendingDir)

	recoveryCtx, cancelRecovery := context.WithTimeout(
		context.Background(),
		30*time.Second,
	)
	if err := store.RecoverPending(recoveryCtx); err != nil {
		log.Printf("Pending match recovery incomplete: %v", err)
	}
	cancelRecovery()

	log.Printf("SQLite database initialized at %s", dbPath)

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
	games.Register("chess", func() game.Game { return &chess.Game{} })
	if os.Getenv("RAMSERVER_STUB_GAME") != "" {
		games.Register("stub", func() game.Game { return &StubGame.StubGame{} }) // for manual testing with leanclient
	}
	if err := server.Serve(ctx, listener, server.DefaultConfig(), games, store); err != nil {
		log.Fatal(err)
	}
}
