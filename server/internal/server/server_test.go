package server

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	pb "github.com/stevenlagoy/ramserver-core/server/gen/ramserver/v1"
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
// func dialTestClient(t *testing.T, addr string) net.Conn {
// 	t.Helper()
// 	conn, err := net.Dial("tcp", addr)
// 	if err != nil {
// 		t.Fatalf("dial %s: %v", addr, err)
// 	}
// 	t.Cleanup(func() { conn.Close() })
// 	return conn
// }

// readNonHeartbeatFrame reads frames until it gets a non-empty one or the connection's read deadline (set by the caller) expires.
// Heartbeats are zero-length by contract (see transport.ReadFrame), so this skips what a real client's read loop would skip.
// func readNonHeartbeatFrame(t *testing.T, conn net.Conn, r *bufio.Reader) ([]byte, error) {
// 	t.Helper()
// 	for {
// 		frame, err := transport.ReadFrame(r)
// 		if err != nil {
// 			return nil, err
// 		}
// 		if len(frame) > 0 {
// 			return frame, nil
// 		}
// 		// Heartbeat: reply immediately like a regular client
// 		if err := transport.WriteFrame(conn, nil); err != nil {
// 			return nil, err
// 		}
// 	}
// }

// func startTestServer(t *testing.T, cfg ServerConfig) string {
// 	t.Helper()
// 	ln, err := net.Listen("tcp", "127.0.0.1:0") // port 0: OS picks a free port, so tests don't collide
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	ctx, cancel := context.WithCancel(context.Background())
// 	done := make(chan struct{})
// 	go func() {
// 		defer close(done)
// 		Serve(ctx, ln, cfg)
// 	}()
// 	t.Cleanup(func() { cancel(); <-done })
// 	return ln.Addr().String()
// }

// func TestSilentClientTimesOut(t *testing.T) {
// 	cfg := testConfig()
// 	addr := startTestServer(t, cfg)

// 	conn, err := net.Dial("tcp", addr)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	defer conn.Close()

// 	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
// 	_, err = io.Copy(io.Discard, conn) // Returns once server closes the connection
// 	if err != nil && !errors.Is(err, io.EOF) {
// 		t.Fatalf("expected server to close the connection, got %v", err)
// 	}
// }

func TestActionRejectedBeforeHandshake(t *testing.T) {
	cfg := testConfig()
	addr, stopServer := startTestServer(t, cfg)
	conn := dialTestClient(t, addr)
	defer stopServer()

	action, err := proto.Marshal(&pb.ClientMessage{
		Sequence: 1,
		Payload:  &pb.ClientMessage_ActionRequest{ActionRequest: &pb.ActionRequest{SessionId: "s1", EncodedAction: []byte("move")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := transport.WriteFrame(conn, action); err != nil {
		t.Fatal(err)
	}
	conn.SetReadDeadline(time.Now().Add(time.Second))
	frame, err := readNonHeartbeatFrame(t, conn, bufio.NewReader(conn))
	if err != nil {
		t.Fatalf("expected a response frame: %v", err)
	}
	var resp pb.ServerMessage
	if err := proto.Unmarshal(frame, &resp); err != nil {
		t.Fatalf("response is not a ServerMessage: %v", err)
	}
	if resp.GetInReplyTo() != 1 || resp.GetErrorResponse().GetCode() != pb.ErrorCode_ERROR_CODE_INVALID_STATE {
		t.Fatalf("expected ERROR_CODE_INVALID_STATE in reply to 1, got %v", &resp)
	}
}

func TestIPLimiterDoesNotLeakEntries(t *testing.T) {
	l := newIPLimiter()
	for i := 0; i < 1000; i++ {
		addr := &net.TCPAddr{IP: net.ParseIP(fmt.Sprintf("10.0.%d.%d", i/256, i%256)), Port: 12345}
		release, ok := l.allow(addr, 4)
		if !ok {
			t.Fatalf("connection %d unexpectedly rejected", i)
		}
		release()
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.counts) != 0 {
		t.Fatalf("expected no leftover entries after all releases, got %d", len(l.counts))
	}
}

// sendMessage marshals msg and writes it as one frame
func sendMessage(t *testing.T, conn net.Conn, msg *pb.ClientMessage) {
	t.Helper()
	frame, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	if err := transport.WriteFrame(conn, frame); err != nil {
		t.Fatal(err)
	}
}

// expectMessage reads the next non-heartbeat frame as a ServerMessage, failing after one second
func expectMessage(t *testing.T, conn net.Conn, r *bufio.Reader) *pb.ServerMessage {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(time.Second))
	frame, err := readNonHeartbeatFrame(t, conn, r)
	if err != nil {
		t.Fatalf("expected a response frame: %v", err)
	}
	var msg pb.ServerMessage
	if err := proto.Unmarshal(frame, &msg); err != nil {
		t.Fatalf("response is not a ServerMessage: %v", err)
	}
	return &msg
}

// authenticate dials addr and completes Connect and Auth, returning the connection, its reader, and the issued player_id
func authenticate(t *testing.T, addr, displayName string) (net.Conn, *bufio.Reader, string) {
	t.Helper()
	conn := dialTestClient(t, addr)
	r := bufio.NewReader(conn)
	sendMessage(t, conn, &pb.ClientMessage{
		Sequence: 1,
		Payload:  &pb.ClientMessage_ConnectRequest{ConnectRequest: &pb.ConnectRequest{ProtocolVersion: transport.ProtocolVersion}},
	})
	if resp := expectMessage(t, conn, r); resp.GetConnectResponse() == nil {
		t.Fatalf("expected ConnectResponse, got %v", resp)
	}
	sendMessage(t, conn, &pb.ClientMessage{
		Sequence: 2,
		Payload:  &pb.ClientMessage_AuthRequest{AuthRequest: &pb.AuthRequest{DisplayName: displayName}},
	})
	resp := expectMessage(t, conn, r)
	if resp.GetAuthResponse() == nil {
		t.Fatalf("expected AuthResponse, got %v", resp)
	}
	return conn, r, resp.GetAuthResponse().GetPlayerId()
}

func TestCreatorReceivesStateUpdate(t *testing.T) {
	addr, stopServer := startTestServer(t, testConfig())
	defer stopServer()
	conn, r, playerID := authenticate(t, addr, "Alice")

	sendMessage(t, conn, &pb.ClientMessage{
		Sequence: 5,
		Payload:  &pb.ClientMessage_JoinSessionRequest{JoinSessionRequest: &pb.JoinSessionRequest{GameId: "stub"}},
	})
	resp := expectMessage(t, conn, r)
	state := resp.GetStateUpdate()
	if state == nil || resp.GetInReplyTo() != 5 {
		t.Fatalf("expected StateUpdate in reply to 5, got %v", resp)
	}
	if state.GetSessionId() == "" || state.GetGameId() != "stub" || state.GetStatus() != pb.SessionStatus_SESSION_STATUS_WAITING {
		t.Fatalf("unexpected session fields: %v", state)
	}
	if players := state.GetPlayers(); len(players) != 1 || players[0].GetPlayerId() != playerID {
		t.Fatalf("expected creator %q as the only player, got %v", playerID, players)
	}
}

func TestJoinerAndCreatorBothReceiveStateUpdate(t *testing.T) {
	addr, stopServer := startTestServer(t, testConfig())
	defer stopServer()
	alice, aliceR, aliceID := authenticate(t, addr, "Alice")
	bob, bobR, bobID := authenticate(t, addr, "Bob")

	sendMessage(t, alice, &pb.ClientMessage{
		Sequence: 3,
		Payload:  &pb.ClientMessage_JoinSessionRequest{JoinSessionRequest: &pb.JoinSessionRequest{GameId: "stub"}},
	})
	sessionID := expectMessage(t, alice, aliceR).GetStateUpdate().GetSessionId()

	sendMessage(t, bob, &pb.ClientMessage{
		Sequence: 9,
		Payload:  &pb.ClientMessage_JoinSessionRequest{JoinSessionRequest: &pb.JoinSessionRequest{GameId: "stub", SessionId: sessionID}},
	})
	bobResp := expectMessage(t, bob, bobR)
	aliceResp := expectMessage(t, alice, aliceR)

	if bobResp.GetInReplyTo() != 9 {
		t.Fatalf("joiner's StateUpdate should reply to 9, got %v", bobResp)
	}
	if aliceResp.GetInReplyTo() != 0 {
		t.Fatalf("creator's copy should be unsolicited, got %v", aliceResp)
	}
	for _, resp := range []*pb.ServerMessage{bobResp, aliceResp} {
		players := resp.GetStateUpdate().GetPlayers()
		if len(players) != 2 || players[0].GetPlayerId() != aliceID || players[1].GetPlayerId() != bobID {
			t.Fatalf("expected players [%s %s] in join order, got %v", aliceID, bobID, players)
		}
		if players[0].GetDisplayName() != "Alice" || players[1].GetDisplayName() != "Bob" {
			t.Fatalf("expected display names [Alice Bob], got %v", players)
		}
	}
}

func TestJoinReportsAssignedRole(t *testing.T) {
	addr, stopServer := startTestServer(t, testConfig())
	defer stopServer()
	conn, r, _ := authenticate(t, addr, "Alice")

	sendMessage(t, conn, &pb.ClientMessage{
		Sequence: 3,
		Payload:  &pb.ClientMessage_JoinSessionRequest{JoinSessionRequest: &pb.JoinSessionRequest{GameId: "stub"}},
	})
	players := expectMessage(t, conn, r).GetStateUpdate().GetPlayers()
	if len(players) != 1 || players[0].GetRole() != "player" {
		t.Fatalf("expected one player with role \"player\", got %v", players)
	}
}

func TestJoinRejectsUnsupportedRequestedRole(t *testing.T) {
	addr, stopServer := startTestServer(t, testConfig())
	defer stopServer()
	conn, r, _ := authenticate(t, addr, "Alice")

	sendMessage(t, conn, &pb.ClientMessage{
		Sequence: 3,
		Payload: &pb.ClientMessage_JoinSessionRequest{JoinSessionRequest: &pb.JoinSessionRequest{
			GameId:        "stub",
			RequestedRole: "spectator", // the stub game only declares "player"
		}},
	})
	resp := expectMessage(t, conn, r)
	if resp.GetInReplyTo() != 3 || resp.GetErrorResponse().GetCode() != pb.ErrorCode_ERROR_CODE_INVALID_ARGUMENT {
		t.Fatalf("expected ERROR_CODE_INVALID_ARGUMENT in reply to 3, got %v", resp)
	}
}
