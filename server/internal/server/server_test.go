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

	"github.com/stevenlagoy/ramserver-core/server/game"
	"github.com/stevenlagoy/ramserver-core/server/internal/testgame"
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
func dialTestClient(t *testing.T, addr string) net.Conn {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("failed to dial %s: %v", addr, err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// readNonHeartbeatFrame reads frames until it gets a non-empty one or the connection's read deadline (set by the caller) expires.
// Heartbeats are zero-length by contract (see transport.ReadFrame), so this skips what a real client's read loop would skip.
func readNonHeartbeatFrame(t *testing.T, conn net.Conn, r *bufio.Reader) ([]byte, error) {
	t.Helper()
	for {
		frame, err := transport.ReadFrame(r)
		if err != nil {
			return nil, err
		}
		if len(frame) > 0 {
			return frame, nil
		}
		// Heartbeat: reply immediately like a regular client
		if err := transport.WriteFrame(conn, nil); err != nil {
			return nil, err
		}
	}
}

func startTestServer(t *testing.T, cfg ServerConfig) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0") // port 0: OS picks a free port, so tests don't collide
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		games := game.NewRegistry()
		games.Register("stub", func() game.Game { return &testgame.StubGame{} })
		Serve(ctx, ln, cfg, games)
	}()
	stop := func() {
		cancel()
		_ = ln.Close()
		<-done
	}
	return ln.Addr().String(), stop
}

func TestSilentClientTimesOut(t *testing.T) {
	cfg := testConfig()
	addr, stopServer := startTestServer(t, cfg)
	defer stopServer()

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
	auth := resp.GetAuthResponse()
	if auth == nil || resp.GetInReplyTo() != 2 {
		t.Fatalf("expected AuthResponse in reply to 2, got %v", resp)
	}
	if auth.GetPlayerId() == "" || auth.GetReconnectToken() == "" || auth.GetPlayerId() == auth.GetReconnectToken() {
		t.Fatalf("expected a server-issued player_id and a separate reconnect_token, got %v", auth)
	}
	return conn, r, auth.GetPlayerId()
}

// connectOnly dials addr and completes ConnectRequest but not AuthRequest
func connectOnly(t *testing.T, addr string) (net.Conn, *bufio.Reader) {
	t.Helper()
	conn := dialTestClient(t, addr)
	r := bufio.NewReader(conn)
	sendMessage(t, conn, connectRequest(1, transport.ProtocolVersion))
	if resp := expectMessage(t, conn, r); resp.GetConnectResponse() == nil {
		t.Fatalf("expected ConnectResponse, got %v", resp)
	}
	return conn, r
}

// expectError reads the next message and fails unless it is an ErrorResponse with code, replying to seq
func expectError(t *testing.T, conn net.Conn, r *bufio.Reader, seq uint64, code pb.ErrorCode) {
	t.Helper()
	resp := expectMessage(t, conn, r)
	if resp.GetInReplyTo() != seq || resp.GetErrorResponse().GetCode() != code {
		t.Fatalf("expected %v in reply to %d, got %v", code, seq, resp)
	}
}

func connectRequest(seq uint64, version string) *pb.ClientMessage {
	return &pb.ClientMessage{
		Sequence: seq,
		Payload:  &pb.ClientMessage_ConnectRequest{ConnectRequest: &pb.ConnectRequest{ProtocolVersion: version}},
	}
}

func authRequest(seq uint64, displayName string) *pb.ClientMessage {
	return &pb.ClientMessage{
		Sequence: seq,
		Payload:  &pb.ClientMessage_AuthRequest{AuthRequest: &pb.AuthRequest{DisplayName: displayName}},
	}
}

func joinRequest(seq uint64, gameID, sessionID string) *pb.ClientMessage {
	return &pb.ClientMessage{
		Sequence: seq,
		Payload:  &pb.ClientMessage_JoinSessionRequest{JoinSessionRequest: &pb.JoinSessionRequest{GameId: gameID, SessionId: sessionID}},
	}
}

func leaveRequest(seq uint64, sessionID string) *pb.ClientMessage {
	return &pb.ClientMessage{
		Sequence: seq,
		Payload:  &pb.ClientMessage_LeaveSessionRequest{LeaveSessionRequest: &pb.LeaveSessionRequest{SessionId: sessionID}},
	}
}

// createSession sends a JoinSessionRequest with no session_id and returns the new session's id
func createSession(t *testing.T, conn net.Conn, r *bufio.Reader, seq uint64) string {
	t.Helper()
	sendMessage(t, conn, joinRequest(seq, "stub", ""))
	resp := expectMessage(t, conn, r)
	if resp.GetStateUpdate() == nil || resp.GetInReplyTo() != seq {
		t.Fatalf("expected StateUpdate in reply to %d, got %v", seq, resp)
	}
	return resp.GetStateUpdate().GetSessionId()
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

func TestHandshakeIssuesUniqueIdentities(t *testing.T) {
	addr, stopServer := startTestServer(t, testConfig())
	defer stopServer()
	_, _, alice := authenticate(t, addr, "Alice")
	_, _, alsoAlice := authenticate(t, addr, "Alice") // same display name, different identity
	if alice == alsoAlice {
		t.Fatalf("two connections were issued the same player_id %q", alice)
	}
}

func TestIncompatibleVersionDisconnects(t *testing.T) {
	cfg := testConfig()
	cfg.ReadTimeout = 5 * time.Second // long, so only the version check can close the connection
	cfg.FirstMessageTimeout = 5 * time.Second
	cfg.CloseTimeout = 100 * time.Millisecond
	addr, stopServer := startTestServer(t, cfg)
	defer stopServer()
	conn := dialTestClient(t, addr)
	r := bufio.NewReader(conn)

	sendMessage(t, conn, connectRequest(1, "2.0"))
	expectError(t, conn, r, 1, pb.ErrorCode_ERROR_CODE_INVALID_PROTOCOL_VERSION)

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.Copy(io.Discard, r); err != nil {
		t.Fatalf("expected the server to close the connection, got %v", err)
	}
}

func TestMessagesBeforeAuthRejected(t *testing.T) {
	addr, stopServer := startTestServer(t, testConfig())
	defer stopServer()

	conn := dialTestClient(t, addr)
	r := bufio.NewReader(conn)
	sendMessage(t, conn, authRequest(1, "Alice")) // Auth before Connect
	expectError(t, conn, r, 1, pb.ErrorCode_ERROR_CODE_INVALID_STATE)

	conn, r = connectOnly(t, addr)
	sendMessage(t, conn, joinRequest(2, "stub", "")) // session message before Auth
	expectError(t, conn, r, 2, pb.ErrorCode_ERROR_CODE_INVALID_STATE)
	sendMessage(t, conn, connectRequest(3, transport.ProtocolVersion)) // second Connect
	expectError(t, conn, r, 3, pb.ErrorCode_ERROR_CODE_INVALID_STATE)
}

func TestHandshakeMessagesAfterAuthRejected(t *testing.T) {
	addr, stopServer := startTestServer(t, testConfig())
	defer stopServer()
	conn, r, _ := authenticate(t, addr, "Alice")

	sendMessage(t, conn, connectRequest(3, transport.ProtocolVersion))
	expectError(t, conn, r, 3, pb.ErrorCode_ERROR_CODE_INVALID_STATE)
	sendMessage(t, conn, authRequest(4, "Alice"))
	expectError(t, conn, r, 4, pb.ErrorCode_ERROR_CODE_INVALID_STATE)
}

func TestAuthRequiresDisplayName(t *testing.T) {
	addr, stopServer := startTestServer(t, testConfig())
	defer stopServer()
	conn, r := connectOnly(t, addr)

	sendMessage(t, conn, authRequest(2, ""))
	expectError(t, conn, r, 2, pb.ErrorCode_ERROR_CODE_INVALID_ARGUMENT)
}

func TestLeaveSession(t *testing.T) {
	addr, stopServer := startTestServer(t, testConfig())
	defer stopServer()
	alice, aliceR, aliceID := authenticate(t, addr, "Alice")
	bob, bobR, _ := authenticate(t, addr, "Bob")
	sessionID := createSession(t, alice, aliceR, 3)
	sendMessage(t, bob, joinRequest(3, "stub", sessionID))
	expectMessage(t, bob, bobR)     // Bob's join reply
	expectMessage(t, alice, aliceR) // Alice's copy of the join broadcast

	sendMessage(t, bob, leaveRequest(4, sessionID))
	resp := expectMessage(t, bob, bobR)
	if resp.GetInReplyTo() != 4 || resp.GetLeaveSessionResponse().GetSessionId() != sessionID {
		t.Fatalf("expected LeaveSessionResponse for %q in reply to 4, got %v", sessionID, resp)
	}
	players := expectMessage(t, alice, aliceR).GetStateUpdate().GetPlayers()
	if len(players) != 1 || players[0].GetPlayerId() != aliceID {
		t.Fatalf("expected only Alice left in the session, got %v", players)
	}
}

func TestJoinSessionErrors(t *testing.T) {
	addr, stopServer := startTestServer(t, testConfig())
	defer stopServer()
	alice, aliceR, _ := authenticate(t, addr, "Alice")
	sessionID := createSession(t, alice, aliceR, 3)
	bob, bobR, _ := authenticate(t, addr, "Bob")

	tests := []struct {
		name string
		msg  *pb.ClientMessage
		code pb.ErrorCode
	}{
		{"empty game_id", joinRequest(10, "", ""), pb.ErrorCode_ERROR_CODE_INVALID_ARGUMENT},
		{"unknown game", joinRequest(11, "nope", ""), pb.ErrorCode_ERROR_CODE_UNKNOWN_GAME},
		{"unknown session", joinRequest(12, "stub", "session-missing"), pb.ErrorCode_ERROR_CODE_SESSION_NOT_FOUND},
		{"session for another game", joinRequest(13, "chess", sessionID), pb.ErrorCode_ERROR_CODE_SESSION_NOT_FOUND},
		{"leave without a session", leaveRequest(14, sessionID), pb.ErrorCode_ERROR_CODE_NOT_IN_SESSION},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sendMessage(t, bob, tt.msg)
			expectError(t, bob, bobR, tt.msg.GetSequence(), tt.code)
		})
	}
}

func TestPlayerCannotJoinTwoSessions(t *testing.T) {
	addr, stopServer := startTestServer(t, testConfig())
	defer stopServer()
	alice, aliceR, _ := authenticate(t, addr, "Alice")
	bob, bobR, _ := authenticate(t, addr, "Bob")
	aliceSession := createSession(t, alice, aliceR, 3)
	bobSession := createSession(t, bob, bobR, 3)

	sendMessage(t, alice, joinRequest(4, "stub", ""))
	expectError(t, alice, aliceR, 4, pb.ErrorCode_ERROR_CODE_ALREADY_IN_SESSION)
	sendMessage(t, alice, joinRequest(5, "stub", bobSession))
	expectError(t, alice, aliceR, 5, pb.ErrorCode_ERROR_CODE_ALREADY_IN_SESSION)

	sendMessage(t, alice, leaveRequest(6, bobSession)) // a session Alice is not a member of
	expectError(t, alice, aliceR, 6, pb.ErrorCode_ERROR_CODE_NOT_IN_SESSION)
	sendMessage(t, alice, &pb.ClientMessage{
		Sequence: 7,
		Payload:  &pb.ClientMessage_ActionRequest{ActionRequest: &pb.ActionRequest{SessionId: bobSession, EncodedAction: []byte("move")}},
	})
	expectError(t, alice, aliceR, 7, pb.ErrorCode_ERROR_CODE_NOT_IN_SESSION)

	sendMessage(t, alice, leaveRequest(8, aliceSession)) // still a member of her own session
	if resp := expectMessage(t, alice, aliceR); resp.GetLeaveSessionResponse() == nil {
		t.Fatalf("expected Alice to still be in her own session, got %v", resp)
	}
}
