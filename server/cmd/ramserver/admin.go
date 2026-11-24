package main

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/stevenlagoy/ramserver-core/server/internal/admin"
	"github.com/stevenlagoy/ramserver-core/server/internal/server"
)

// Keeps recent logs for admin API
var logRing = captureLogs()

func captureLogs() *admin.LogRing {
	ring := admin.NewLogRing(2000)
	log.SetOutput(io.MultiWriter(os.Stderr, ring))
	return ring
}

// Starts the admin API if RAMSERVER_ADMIN_ADDR is set. A token file is needed.
//
// RAMSERVER_ADMIN_ADDR                 listen address, like 127.0.0.1:8081
// RAMSERVER_ADMIN_TOKENS_FILE          file of "<name> <sha256-hex>" lines
// RAMERVER_ADMIN_ALLOW_NONLOOPBACK=1   allow a non-loopback address
func startAdmin(ctx context.Context, hub *server.Hub) error {
	addr := os.Getenv("RAMSERVER_ADMIN_ADDR")
	if addr == "" {
		log.Println("Admin API disabled: set RAMSERVER_ADMIN_ADDR to enable")
		return nil
	}
	path := os.Getenv("RAMSERVER_ADMIN_TOKENS_FILE")
	if path == "" {
		return errors.New("RAMSERVER_ADMIN_TOKENS_FILE is required when RAMSERVER_ADMIN_ADDR is set")
	}
	tokens, err := admin.NewTokenStore(path)
	if err != nil {
		return err
	}
	ln, err := admin.Listen(addr, os.Getenv("RAMSERVER_ADMIN_ALLOW_NONLOOPBACK") == "1")
	if err != nil {
		return err
	}
	handler := admin.NewHandler(admin.Options{Tokens: tokens, Control: hub, Logs: logRing, Metrics: hub.Metrics})
	go func() {
		if err := admin.Serve(ctx, ln, handler); err != nil {
			log.Printf("admin: API stopped: %v", err)
		}
	}()
	return nil
}

// startCloudAdmin adds the admin HTTP handler to the Cloud Run WebSocket server. It uses no
// second listener or private port because Cloud Run provides HTTPS for the whole service.
func startCloudAdmin(hub *server.Hub) (http.Handler, error) {
	path := os.Getenv("RAMSERVER_ADMIN_TOKENS_FILE")
	if path == "" {
		log.Println("Admin API disabled: set RAMSERVER_ADMIN_TOKENS_FILE to enable")
		return nil, nil
	}
	tokens, err := admin.NewTokenStore(path)
	if err != nil {
		return nil, err
	}
	log.Printf("Admin API enabled over the Cloud Run HTTPS service")
	return admin.NewHandler(admin.Options{
		Tokens:  tokens,
		Control: hub,
		Logs:    logRing,
		Metrics: hub.Metrics,
	}), nil
}
