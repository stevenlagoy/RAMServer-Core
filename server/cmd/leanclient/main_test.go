package main

import (
	"bytes"
	"context"
	"github.com/stevenlagoy/ramserver-core/server/rulesets/StubGame"
	"net"
	"testing"
	"time"

	"github.com/stevenlagoy/ramserver-core/server/game"
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
		{"play stub", "", func(m *pb.ClientMessage) bool {
			j := m.GetJoinSessionRequest()
			return j.GetGameId() == "stub" && j.GetSessionId() == "" && !j.GetCreateNew() && !j.GetPrivateSession()
		}},
		{"create stub", "", func(m *pb.ClientMessage) bool {
			j := m.GetJoinSessionRequest()
			return j.GetGameId() == "stub" && j.GetSessionId() == "" && j.GetCreateNew() && !j.GetPrivateSession()
		}},
		{"create stub private", "", func(m *pb.ClientMessage) bool {
			j := m.GetJoinSessionRequest()
			return j.GetGameId() == "stub" && j.GetCreateNew() && j.GetPrivateSession()
		}},
		{"list", "", func(m *pb.ClientMessage) bool {
			return m.GetListSessionsRequest() != nil && m.GetListSessionsRequest().GetGameId() == ""
		}},
		{"list chess", "", func(m *pb.ClientMessage) bool { return m.GetListSessionsRequest().GetGameId() == "chess" }},
		{"history", "s1", func(m *pb.ClientMessage) bool { return m.GetMatchHistoryRequest().GetSessionId() == "s1" }},
		{"history s2", "", func(m *pb.ClientMessage) bool { return m.GetMatchHistoryRequest().GetSessionId() == "s2" }},
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
		{"ready", "s1", func(m *pb.ClientMessage) bool {
			r := m.GetSetReadyRequest()
			return r.GetSessionId() == "s1" && r.GetReady()
		}},
		{"ready false", "s1", func(m *pb.ClientMessage) bool {
			r := m.GetSetReadyRequest()
			return r != nil && !r.GetReady()
		}},
		{"legal", "s1", func(m *pb.ClientMessage) bool { return m.GetLegalActionsRequest().GetSessionId() == "s1" }},
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
	for _, line := range []string{
		"auth", "auth Alice p1", "play", "play stub extra", "create", "create stub extra", "create stub private extra",
		"join stub", "join stub s1 extra", "leave", "action move", "action", "ready", "legal",
		"list chess extra", "history", "history s1 extra", "raw", "dance",
	} {
		if _, _, err := parseCommand(line, ""); err == nil {
			t.Errorf("parseCommand(%q) succeeded with no session, want an error", line)
		}
	}
}

// startTestServer starts a real server on a free port and returns its address
func startTestServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		games := game.NewRegistry()
		games.Register("stub", func() game.Game { return &StubGame.StubGame{} })
		server.Serve(ctx, ln, server.DefaultConfig(), games)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return ln.Addr().String()
}

// dial connects to addr, failing the test if the connection takes more than five seconds
func dial(t *testing.T, addr string) net.Conn {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	return conn
}

// dialTestServer starts a real server and returns a connection to it
func dialTestServer(t *testing.T) net.Conn {
	t.Helper()
	return dial(t, startTestServer(t))
}

// authedClient connects to addr and completes the handshake as displayName
func authedClient(t *testing.T, addr, displayName string) *client {
	t.Helper()
	c := newClient(dial(t, addr))
	if _, err := c.handshake(transport.ProtocolVersion, displayName); err != nil {
		t.Fatal(err)
	}
	return c
}

// runAndRead runs one command and returns the server's next message
func runAndRead(t *testing.T, c *client, line string) *pb.ServerMessage {
	t.Helper()
	if err := c.runCommand(line); err != nil {
		t.Fatalf("%s: %v", line, err)
	}
	resp, err := c.next()
	if err != nil {
		t.Fatalf("%s: %v", line, err)
	}
	return resp
}

// The protocol 1.2 commands against a real server: a private session is not listed and not joined by play,
// list and history get their responses, and those responses have their own log labels
func TestNewCommandsAgainstServer(t *testing.T) {
	addr := startTestServer(t)
	alice := authedClient(t, addr, "Alice")
	bob := authedClient(t, addr, "Bob")

	private := runAndRead(t, alice, "create stub private").GetStateUpdate().GetSessionId()
	if private == "" {
		t.Fatal("create stub private did not create a session")
	}

	resp := runAndRead(t, bob, "list stub")
	if resp.GetListSessionsResponse() == nil || serverMessageKind(resp) != "SESSIONS" {
		t.Fatalf("expected a SESSIONS response, got %s %v", serverMessageKind(resp), resp)
	}
	if sessions := resp.GetListSessionsResponse().GetSessions(); len(sessions) != 0 {
		t.Fatalf("expected the private session to be unlisted, got %v", sessions)
	}

	if joined := runAndRead(t, bob, "play stub").GetStateUpdate().GetSessionId(); joined == "" || joined == private {
		t.Fatalf("play stub joined %q, want a new session, not the private %q", joined, private)
	}

	resp = runAndRead(t, alice, "history")
	if resp.GetMatchHistoryResponse().GetSessionId() != private || serverMessageKind(resp) != "HISTORY" {
		t.Fatalf("expected a HISTORY response for %q, got %s %v", private, serverMessageKind(resp), resp)
	}
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
