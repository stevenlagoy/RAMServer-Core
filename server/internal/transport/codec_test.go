package transport

import (
	"bytes"
	"errors"
	"slices"
	"testing"

	"google.golang.org/protobuf/proto"

	pb "github.com/stevenlagoy/ramserver-core/server/gen/ramserver/v1"
)

func decodeServerMessage(t *testing.T, frame []byte) *pb.ServerMessage {
	t.Helper()
	var msg pb.ServerMessage
	if err := proto.Unmarshal(frame, &msg); err != nil {
		t.Fatalf("frame is not a ServerMessage: %v", err)
	}
	return &msg
}

func TestEncodersNeverProduceEmptyFrame(t *testing.T) {
	frames := map[string][]byte{
		"EncodeError":                EncodeError(0, errors.New("bad move")),
		"EncodeStateUpdate":          EncodeStateUpdate(0, StateUpdate{}),
		"EncodeMatchResult":          EncodeMatchResult(MatchResult{}),
		"EncodeLeaveSessionResponse": EncodeLeaveSessionResponse(0, ""),
	}
	for name, got := range frames {
		if len(got) == 0 {
			t.Errorf("%s returned an empty frame, which readers treat as a heartbeat", name)
		}
	}
}

func TestDecodeClientMessage(t *testing.T) {
	frame, err := proto.Marshal(&pb.ClientMessage{
		Sequence: 7,
		Payload: &pb.ClientMessage_ActionRequest{ActionRequest: &pb.ActionRequest{
			SessionId:            "s1",
			EncodedAction:        []byte("e2e4"),
			ExpectedStateVersion: 3,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	msg, err := DecodeClientMessage(frame)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Kind != KindAction || msg.Sequence != 7 || msg.SessionID != "s1" || msg.ExpectedStateVersion != 3 {
		t.Fatalf("unexpected decode: %+v", msg)
	}
	if msg.Action.ID != 7 || msg.Action.ActorID != "" || !bytes.Equal(msg.Action.Payload, []byte("e2e4")) {
		t.Fatalf("unexpected action: %+v", msg.Action)
	}
}

func TestDecodeClientMessageMalformed(t *testing.T) {
	assertMalformed := func(t *testing.T, err error) {
		t.Helper()
		var perr *Error
		if !errors.As(err, &perr) || perr.Code != CodeMalformedMessage {
			t.Fatalf("expected CodeMalformedMessage, got %v", err)
		}
	}

	_, err := DecodeClientMessage([]byte("move")) // truncated fixed32 field
	assertMalformed(t, err)

	frame, _ := proto.Marshal(&pb.ClientMessage{Sequence: 9}) // no payload
	msg, err := DecodeClientMessage(frame)
	assertMalformed(t, err)
	if msg.Sequence != 9 {
		t.Fatalf("expected sequence to survive a missing payload, got %d", msg.Sequence)
	}
}

func TestEncodeErrorUsesProtocolCode(t *testing.T) {
	perr := NewError(CodeOutOfTurn, "wait your turn")
	perr.SessionID = "s1"
	resp := decodeServerMessage(t, EncodeError(4, perr))
	if resp.GetInReplyTo() != 4 {
		t.Fatalf("in_reply_to = %d, want 4", resp.GetInReplyTo())
	}
	e := resp.GetErrorResponse()
	if e.GetCode() != CodeOutOfTurn || e.GetMessage() != "wait your turn" || e.GetSessionId() != "s1" {
		t.Fatalf("unexpected error response: %v", e)
	}

	plain := decodeServerMessage(t, EncodeError(0, errors.New("boom"))).GetErrorResponse()
	if plain.GetCode() != CodeInternal {
		t.Fatalf("plain error code = %v, want CodeInternal", plain.GetCode())
	}
}

func TestEncodeFallsBackOnInvalidUTF8(t *testing.T) {
	resp := decodeServerMessage(t, EncodeStateUpdate(5, StateUpdate{SessionID: "\xff"}))
	if resp.GetInReplyTo() != 5 || resp.GetErrorResponse().GetCode() != CodeInternal {
		t.Fatalf("expected internal error reply to 5, got %v", resp)
	}
}

func TestCompatibleVersion(t *testing.T) {
	for v, want := range map[string]bool{"1.0": true, "1.7": true, "2.0": false, "1": false, "": false} {
		if got := CompatibleVersion(v); got != want {
			t.Errorf("CompatibleVersion(%q) = %v, want %v", v, got, want)
		}
	}
}

func TestDecodeJoinSessionRequest(t *testing.T) {
	frame, err := proto.Marshal(&pb.ClientMessage{
		Sequence: 2,
		Payload: &pb.ClientMessage_JoinSessionRequest{JoinSessionRequest: &pb.JoinSessionRequest{
			GameId:        "chess",
			SessionId:     "s1",
			RequestedRole: "spectator",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	msg, err := DecodeClientMessage(frame)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Kind != KindJoinSession || msg.GameID != "chess" || msg.SessionID != "s1" || msg.RequestedRole != "spectator" {
		t.Fatalf("unexpected decode: %+v", msg)
	}
}

func TestEncodeStateUpdate(t *testing.T) {
	frame := EncodeStateUpdate(3, StateUpdate{
		SessionID: "s1",
		GameID:    "chess",
		Status:    StatusInProgress,
		Players: []SessionPlayer{
			{PlayerID: "p1", DisplayName: "Alice", Connected: true, Role: "player"},
			{PlayerID: "p2", DisplayName: "Bob", Role: "spectator"},
		},
		StateVersion:   7,
		EncodedView:    []byte("board"),
		ActivePlayerID: "p1",
		PlayerOrder:    []string{"p1"},
	})
	resp := decodeServerMessage(t, frame)
	state := resp.GetStateUpdate()
	if resp.GetInReplyTo() != 3 || state.GetSessionId() != "s1" || state.GetGameId() != "chess" ||
		state.GetStatus() != StatusInProgress || state.GetStateVersion() != 7 || !bytes.Equal(state.GetEncodedView(), []byte("board")) ||
		state.GetActivePlayerId() != "p1" || !slices.Equal(state.GetPlayerOrder(), []string{"p1"}) {
		t.Fatalf("unexpected state update: %v", resp)
	}
	players := state.GetPlayers()
	if len(players) != 2 {
		t.Fatalf("expected 2 players, got %v", players)
	}
	if p := players[0]; p.GetPlayerId() != "p1" || p.GetDisplayName() != "Alice" || !p.GetConnected() || p.GetRole() != "player" {
		t.Fatalf("unexpected first player: %v", p)
	}
	if p := players[1]; p.GetPlayerId() != "p2" || p.GetConnected() || p.GetRole() != "spectator" {
		t.Fatalf("unexpected second player: %v", p)
	}
}

// encodeAndDecode marshals msg as a client would and decodes it with DecodeClientMessage
func encodeAndDecode(t *testing.T, msg *pb.ClientMessage) ClientMessage {
	t.Helper()
	frame, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeClientMessage(frame)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func TestDecodeConnectRequest(t *testing.T) {
	msg := encodeAndDecode(t, &pb.ClientMessage{
		Sequence: 1,
		Payload: &pb.ClientMessage_ConnectRequest{ConnectRequest: &pb.ConnectRequest{
			ProtocolVersion: "1.0",
			ClientName:      "ramserver-java-client",
		}},
	})
	if msg.Kind != KindConnect || msg.Sequence != 1 || msg.ProtocolVersion != "1.0" || msg.ClientName != "ramserver-java-client" {
		t.Fatalf("unexpected decode: %+v", msg)
	}
}

func TestDecodeAuthRequest(t *testing.T) {
	msg := encodeAndDecode(t, &pb.ClientMessage{
		Sequence: 2,
		Payload: &pb.ClientMessage_AuthRequest{AuthRequest: &pb.AuthRequest{
			DisplayName:    "Alice",
			PlayerId:       "p1",
			ReconnectToken: "secret",
		}},
	})
	if msg.Kind != KindAuth || msg.Sequence != 2 || msg.DisplayName != "Alice" || msg.PlayerID != "p1" || msg.ReconnectToken != "secret" {
		t.Fatalf("unexpected decode: %+v", msg)
	}
}

func TestDecodeLeaveSessionRequest(t *testing.T) {
	msg := encodeAndDecode(t, &pb.ClientMessage{
		Sequence: 4,
		Payload:  &pb.ClientMessage_LeaveSessionRequest{LeaveSessionRequest: &pb.LeaveSessionRequest{SessionId: "s1"}},
	})
	if msg.Kind != KindLeaveSession || msg.Sequence != 4 || msg.SessionID != "s1" {
		t.Fatalf("unexpected decode: %+v", msg)
	}
}

func TestEncodeConnectResponse(t *testing.T) {
	resp := decodeServerMessage(t, EncodeConnectResponse(1, "127.0.0.1:5000"))
	if resp.GetInReplyTo() != 1 {
		t.Fatalf("in_reply_to = %d, want 1", resp.GetInReplyTo())
	}
	c := resp.GetConnectResponse()
	if c.GetConnectionId() != "127.0.0.1:5000" || c.GetServerProtocolVersion() != ProtocolVersion {
		t.Fatalf("unexpected connect response: %v", c)
	}
}

func TestEncodeAuthResponse(t *testing.T) {
	resp := decodeServerMessage(t, EncodeAuthResponse(2, "p1", "secret", "s1"))
	if resp.GetInReplyTo() != 2 {
		t.Fatalf("in_reply_to = %d, want 2", resp.GetInReplyTo())
	}
	a := resp.GetAuthResponse()
	if a.GetPlayerId() != "p1" || a.GetReconnectToken() != "secret" || a.GetSessionId() != "s1" {
		t.Fatalf("unexpected auth response: %v", a)
	}
}

func TestEncodeLeaveSessionResponse(t *testing.T) {
	resp := decodeServerMessage(t, EncodeLeaveSessionResponse(4, "s1"))
	if resp.GetInReplyTo() != 4 || resp.GetLeaveSessionResponse().GetSessionId() != "s1" {
		t.Fatalf("unexpected leave response: %v", resp)
	}
}

func TestEncodeMatchResult(t *testing.T) {
	resp := decodeServerMessage(t, EncodeMatchResult(MatchResult{
		SessionID:       "s1",
		WinnerPlayerIDs: []string{"p1"},
		Summaries: []PlayerActionSummary{
			{PlayerID: "p1", ActionsAccepted: 5, ActionsRejected: 1},
			{PlayerID: "p2", ActionsAccepted: 4},
		},
		Reason: EndForfeit,
	}))
	if resp.GetInReplyTo() != 0 {
		t.Fatalf("MatchResult must be unsolicited, got in_reply_to %d", resp.GetInReplyTo())
	}
	m := resp.GetMatchResult()
	if m.GetSessionId() != "s1" || !slices.Equal(m.GetWinnerPlayerIds(), []string{"p1"}) || m.GetDraw() || m.GetReason() != EndForfeit {
		t.Fatalf("unexpected match result: %v", m)
	}
	sums := m.GetSummaries()
	if len(sums) != 2 {
		t.Fatalf("expected 2 summaries, got %v", sums)
	}
	if s := sums[0]; s.GetPlayerId() != "p1" || s.GetActionsAccepted() != 5 || s.GetActionsRejected() != 1 {
		t.Fatalf("unexpected first summary: %v", s)
	}
	if s := sums[1]; s.GetPlayerId() != "p2" || s.GetActionsAccepted() != 4 || s.GetActionsRejected() != 0 {
		t.Fatalf("unexpected second summary: %v", s)
	}

	draw := decodeServerMessage(t, EncodeMatchResult(MatchResult{SessionID: "s1", Draw: true, Reason: EndCompleted})).GetMatchResult()
	if !draw.GetDraw() || len(draw.GetWinnerPlayerIds()) != 0 || draw.GetReason() != EndCompleted {
		t.Fatalf("unexpected draw result: %v", draw)
	}
}
