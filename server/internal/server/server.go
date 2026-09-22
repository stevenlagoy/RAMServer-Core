package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/stevenlagoy/ramserver-core/server/internal/game"
	"github.com/stevenlagoy/ramserver-core/server/internal/match"
)

/*
https://medium.com/@diasmashikovnasa/tcp-listeners-and-servers-in-golang-a-beginners-guide-9032a9e2eeeb
https://okanexe.medium.com/the-complete-guide-to-tcp-ip-connections-in-golang-1216dae27b5a
https://oneuptime.com/blog/post/2026-01-25-concurrent-tcp-server-10k-connections-go/view
*/

type ServerConfig struct {
	MaxConnections      int
	ReadTimeout         time.Duration
	FirstMessageTimeout time.Duration
	WriteTimeout        time.Duration
	CloseTimeout        time.Duration
	PingInterval        time.Duration
	OutQueueSize        int
	MaxConnectionsOneIP int32
}

func DefaultConfig() ServerConfig {
	return ServerConfig{
		MaxConnections:      16, // Scale as #matches * 6 + some headroom
		ReadTimeout:         30 * time.Second,
		FirstMessageTimeout: 5 * time.Second, // Grace period for a client's first frame
		WriteTimeout:        10 * time.Second,
		CloseTimeout:        10 * time.Second,
		PingInterval:        10 * time.Second, // Keep under ReadTimeout
		OutQueueSize:        32,               // Messages queued per client before it's dropped as too slow
		MaxConnectionsOneIP: 4,
	}
}

func allowIP(ipCounts *sync.Map, addr net.Addr, max int32) (release func(), ok bool) {
	host, _, _ := net.SplitHostPort(addr.String())
	val, _ := ipCounts.LoadOrStore(host, new(int32))
	counter := val.(*int32)
	if atomic.AddInt32(counter, 1) > max {
		atomic.AddInt32(counter, -1)
		return nil, false
	}
	return func() { atomic.AddInt32(counter, -1) }, true
}

func Serve(ctx context.Context, listener net.Listener, config ServerConfig) error {
	var ipCounts sync.Map // string -> *int32, or a mutex-guarded map

	// Semaphore to limit connections
	sem := make(chan struct{}, config.MaxConnections)
	var wg sync.WaitGroup

	store := match.NewStore()
	defer store.Wait() // Let in-flight matches finish before Serve returns

	// Single hard-coded match until real matchmaking exists.
	// Client still needs to look this ID up, so pass it through, not just the *Match.
	defaultMatch, err := store.Create(ctx, "default", &game.StubGame{})
	if err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		log.Println("Shutdown signal received")
		listener.Close() // Unblock Accept()
	}()

	fmt.Println("Server is running on port 8080")

	var backoff time.Duration // grows while Accept fails

	for {
		// Accept incoming connections
		conn, err := listener.Accept() // blocks until connection arrives
		if err != nil {
			// Check if graceful shutdown
			select {
			case <-ctx.Done():
				log.Println("Waiting for connections to close...")
				waitTimeout(&wg, config.CloseTimeout)
				log.Println("Server stopped")
				return nil
			default:
				log.Printf("Error accepting connection: %v", err)
				if backoff == 0 {
					backoff = 5 * time.Millisecond
				} else {
					backoff *= 2
				}
				if backoff > time.Second {
					backoff = time.Second
				}
				time.Sleep(backoff)
				continue
			}
		}
		backoff = 0

		release, ok := allowIP(&ipCounts, conn.RemoteAddr(), config.MaxConnectionsOneIP)
		if !ok {
			conn.Close()
			continue
		}

		// Try to acquire a semaphore slot; reject instead of blocking the accept loop
		select {
		case sem <- struct{}{}:
		default:
			conn.SetWriteDeadline(time.Now().Add(time.Second))
			conn.Write([]byte("server full\n"))
			conn.Close()
			release()
			continue
		}
		wg.Add(1)

		// Handle client connection
		go func(ctx context.Context, conn net.Conn) {
			defer wg.Done()
			defer func() { <-sem }() // Release slot when done
			defer release()
			NewClient(conn, defaultMatch, config).Run(ctx)
		}(ctx, conn)
	}
}

func waitTimeout(wg *sync.WaitGroup, d time.Duration) {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(d):
		log.Println("Timed out waiting for connections; exiting")
	}
}
