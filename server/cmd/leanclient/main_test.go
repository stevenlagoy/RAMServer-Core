package main

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"

	pb "github.com/stevenlagoy/ramserver-core/server/gen/ramserver/v1"
	"github.com/stevenlagoy/ramserver-core/server/internal/server"
	"github.com/stevenlagoy/ramserver-core/server/internal/transport"
)

func TestFrameRoundTrip(t *testing.T) {
	want := []byte("hello server")

	var buf bytes.Buffer

	if err := writeFrame(&buf, want); err != nil {
		t.Fatalf("writeFrame() error = %v", err)
	}

	got, err := readFrame(&buf)
	if err != nil {
		t.Fatalf("readFrame() error = %v", err)
	}

	if !bytes.Equal(got, want) {
		t.Fatalf("readFrame() = %q, want %q", got, want)
	}
}

func TestParseCommand(t *testing.T) {
	tests := []struct {
		line    string
		session string
		check   func(*pb.ClientMessage) bool
	}{
		{"connect", "", func(m *pb.ClientMessage) bool {
			return m.GetConnectRequest().GetProtocolVersion() == transport.ProtocolVersion
		}},
		{"connect 2.0", "", func(m *pb.ClientMessage) bool { return m.GetConnectRequest().GetProtocolVersion() == "2.0" }},
		{"auth Alice", "", func(m *pb.ClientMessage) bool {
			a := m.GetAuthRequest()
			return a.GetDisplayName() == "Alice" && a.GetPlayerId() == ""
		}},
		{"auth Alice p1 secret", "", func(m *pb.ClientMessage) bool {
			a := m.GetAuthRequest()
			return a.GetPlayerId() == "p1" && a.GetReconnectToken() == "secret"
		}},
		{"create stub", "", func(m *pb.ClientMessage) bool {
			j := m.GetJoinSessionRequest()
			return j.GetGameId() == "stub" && j.GetSessionId() == ""
		}},
		{"join stub s1", "", func(m *pb.ClientMessage) bool {
			j := m.GetJoinSessionRequest()
			return j.GetGameId() == "stub" && j.GetSessionId() == "s1"
		}},
		{"leave", "s1", func(m *pb.ClientMessage) bool { return m.GetLeaveSessionRequest().GetSessionId() == "s1" }},
		{"leave s2", "s1", func(m *pb.ClientMessage) bool { return m.GetLeaveSessionRequest().GetSessionId() == "s2" }},
		{"action e2 e4", "s1", func(m *pb.ClientMessage) bool {
			a := m.GetActionRequest()
			return a.GetSessionId() == "s1" && string(a.GetEncodedAction()) == "e2 e4"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			msg, raw, err := parseCommand(tt.line, tt.session)
			if err != nil || raw != nil || !tt.check(msg) {
				t.Fatalf("parseCommand(%q) = %v, %q, %v", tt.line, msg, raw, err)
			}
		})
	}
}

func TestParseCommandRaw(t *testing.T) {
	msg, raw, err := parseCommand("raw not protobuf", "")
	if err != nil || msg != nil || string(raw) != "not protobuf" {
		t.Fatalf("parseCommand(raw) = %v, %q, %v", msg, raw, err)
	}
}

func TestParseCommandErrors(t *testing.T) {
	for _, line := range []string{"auth", "auth Alice p1", "create", "create stub extra", "join stub", "join stub s1 extra", "leave", "action move", "action", "raw", "dance"} {
		if _, _, err := parseCommand(line, ""); err == nil {
			t.Errorf("parseCommand(%q) succeeded with no session, want an error", line)
		}
	}
}

// dialTestServer starts a real server on a free port and returns a connection to it
func dialTestServer(t *testing.T) net.Conn {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		server.Serve(ctx, ln, server.DefaultConfig(), nil)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	return conn
}

// Runs the client against a real server: handshake, create a session, then leave it
func TestClientAgainstServer(t *testing.T) {
	conn := dialTestServer(t)
	c := newClient(conn)

	playerID, err := c.handshake(transport.ProtocolVersion, "Alice")
	if err != nil || playerID == "" {
		t.Fatalf("handshake() = %q, %v", playerID, err)
	}

	if err := c.runCommand("create stub"); err != nil {
		t.Fatal(err)
	}
	resp, err := c.next()
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetStateUpdate() == nil || c.currentSession() == "" {
		t.Fatalf("expected a StateUpdate that sets the current session, got %v", resp)
	}

	if err := c.runCommand("leave"); err != nil {
		t.Fatal(err)
	}
	resp, err = c.next()
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetLeaveSessionResponse() == nil || c.currentSession() != "" {
		t.Fatalf("expected LeaveSessionResponse that clears the current session, got %v", resp)
	}
}

func TestHandshakeRejectsIncompatibleVersion(t *testing.T) {
	conn := dialTestServer(t)

	if _, err := newClient(conn).handshake("2.0", "Alice"); err == nil {
		t.Fatal("expected handshake with protocol 2.0 to fail")
	}
}
