package server

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stevenlagoy/ramserver-core/server/internal/transport"
)

func testConfig() ServerConfig {
	return ServerConfig{
		MaxConnections:      16,
		ReadTimeout:         200 * time.Millisecond,
		FirstMessageTimeout: 200 * time.Millisecond,
		WriteTimeout:        time.Second,
		CloseTimeout:        time.Second,
		PingInterval:        50 * time.Millisecond,
		OutQueueSize:        8,
		MaxConnectionsOneIP: 8,
	}
}

// dialTestClient connects to addr and fails the test immediately if the dial fails, so call sites don't need their own error handling.
func dialTestClient(t *testing.T, addr string) net.Conn {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// readNonHeartbeatFrame reads frames until it gets a non-empty one or the connection's read deadline (set by the caller) expires.
// Heartbeats are zero-length by contract (see transport.ReadFrame), so this skips what a real client's read loop would skip.
func readNonHeartbeatFrame(t *testing.T, r *bufio.Reader) ([]byte, error) {
	t.Helper()
	for {
		frame, err := transport.ReadFrame(r)
		if err != nil {
			return nil, err
		}
		if len(frame) > 0 {
			return frame, nil
		}
	}
}

func startTestServer(t *testing.T, cfg ServerConfig) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0") // port 0: OS picks a free port, so tests don't collide
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		Serve(ctx, ln, cfg)
	}()
	t.Cleanup(func() { cancel(); <-done })
	return ln.Addr().String()
}

func TestSilentClientTimesOut(t *testing.T) {
	cfg := testConfig()
	addr := startTestServer(t, cfg)

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err = io.Copy(io.Discard, conn) // Returns once server closes the connection
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("expected server to close the connection, got %v", err)
	}
}

func TestBroadcastReachesAllClients(t *testing.T) {
	cfg := testConfig()
	addr := startTestServer(t, cfg)

	a := dialTestClient(t, addr)
	b := dialTestClient(t, addr)

	// Give both connections a moment to register with the match before acting.
	time.Sleep(50 * time.Millisecond)

	if err := transport.WriteFrame(a, []byte("move")); err != nil {
		t.Fatal(err)
	}

	for _, conn := range []net.Conn{a, b} {
		conn.SetReadDeadline(time.Now().Add(time.Second))
		reader := bufio.NewReader(conn)
		frame, err := readNonHeartbeatFrame(t, reader)
		if err != nil {
			t.Fatalf("expected a state frame: %v", err)
		}
		if len(frame) == 0 {
			t.Fatal("state frame was empty")
		}
	}
}
