package server

import (
	"context"
	"errors"
	"log"
	"net"
	"sync"
	"time"

	"github.com/stevenlagoy/ramserver-core/server/game"
	"github.com/stevenlagoy/ramserver-core/server/internal/session"
	"github.com/stevenlagoy/ramserver-core/server/internal/transport"
)

/*
https://medium.com/@diasmashikovnasa/tcp-listeners-and-servers-in-golang-a-beginners-guide-9032a9e2eeeb
https://okanexe.medium.com/the-complete-guide-to-tcp-ip-connections-in-golang-1216dae27b5a
https://oneuptime.com/blog/post/2026-01-25-concurrent-tcp-server-10k-connections-go/view
*/

// Configuration values for a server
type ServerConfig struct {
	MaxConnections      int           // Maximum number of client connections the server can support
	ReadTimeout         time.Duration // Duration after which a read from a client will time out
	FirstMessageTimeout time.Duration // Duration after which a client handshake will time out
	WriteTimeout        time.Duration // Duration after which a write to a client will time out
	CloseTimeout        time.Duration // Duration after which a request to close the connection will time out
	PingInterval        time.Duration // Interval between keep-alive pings sent to a connection
	OutQueueSize        int           // Number of messages which can be in flight to a client at once: exceeding this will cause the client to be dropped
	MaxConnectionsOneIP int32         // Maximum connections allowed from the same IP address
}

// Get the default configuration for a medium-size server
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

// ipLimiter caps concurrent connections per source IP
type ipLimiter struct {
	mu     sync.Mutex
	counts map[string]int32
}

func newIPLimiter() *ipLimiter {
	return &ipLimiter{counts: make(map[string]int32)}
}

// Request a new allowed connection to the given IP address
func (l *ipLimiter) allow(addr net.Addr, max int32) (release func(), ok bool) {
	host, _, _ := net.SplitHostPort(addr.String())

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.counts[host] >= max {
		return nil, false
	}
	l.counts[host]++

	return func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.counts[host]--
		if l.counts[host] <= 0 {
			delete(l.counts, host) // Prevent unbounded growith from IP churn
		}
	}, true
}

// Start the server and accept incoming connections to the given listener
func Serve(ctx context.Context, listener net.Listener, config ServerConfig, games *game.Registry) error {
	ipCounts := newIPLimiter()

	// Semaphore to limit connections
	sem := make(chan struct{}, config.MaxConnections)
	var wg sync.WaitGroup

	sessions := session.NewSessionStore()

	go func() {
		<-ctx.Done()
		log.Println("Shutdown signal received")
		listener.Close() // Unblock Accept()
	}()

	log.Printf("Server is running on %s", listener.Addr())

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

		release, ok := ipCounts.allow(conn.RemoteAddr(), config.MaxConnectionsOneIP)
		if !ok {
			conn.Close()
			continue
		}

		// Try to acquire a semaphore slot; reject instead of blocking the accept loop
		select {
		case sem <- struct{}{}:
		default:
			conn.SetWriteDeadline(time.Now().Add(time.Second))
			transport.WriteFrame(conn, transport.EncodeReject(0, errors.New("server full")))
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
			NewClient(conn, config, games, sessions).Run(ctx)
		}(ctx, conn)
	}
}

// Wait for connections to terminate during shutdown and return if waited too long
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
