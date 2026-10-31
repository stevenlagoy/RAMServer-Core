package session

import (
	"context"
	"errors"
	"github.com/stevenlagoy/ramserver-core/server/rulesets/StubGame"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/stevenlagoy/ramserver-core/server/game"
	pb "github.com/stevenlagoy/ramserver-core/server/gen/ramserver/v1"
	"github.com/stevenlagoy/ramserver-core/server/internal/transport"
	"github.com/stevenlagoy/ramserver-core/server/rulesets/chess"
)

// fakeSender records every frame a Session sends to one member
type fakeSender struct{ frames chan []byte }

func newFakeSender() *fakeSender { return &fakeSender{frames: make(chan []byte, 16)} }

func (f *fakeSender) Send(b []byte) { f.frames <- b }

func (f *fakeSender) next(t *testing.T) *pb.ServerMessage {
	t.Helper()
	select {
	case b := <-f.frames:
		var msg pb.ServerMessage
		if err := proto.Unmarshal(b, &msg); err != nil {
			t.Fatal(err)
		}
		return &msg
	case <-time.After(time.Second):
		t.Fatal("expected a message from the session")
		return nil
	}
}

// oneSeatGame allows a single player and nothing else
type oneSeatGame struct{ StubGame.StubGame }

func (g *oneSeatGame) Roles() []game.RoleSpec {
	return []game.RoleSpec{{Name: game.RolePlayer, Min: 1, Max: 1}}
}

// TestSessionPipeline_JoinReadyLeave starts a session with a mock owner and mock sender to verify joining a session,
// tests users readying up, and a user leaving.
func TestSessionPipeline_JoinReadyLeave(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ownerSender := newFakeSender()
	clientSender := newFakeSender()

	g := &StubGame.StubGame{}

	sess := NewSession("session-1", "TestGame", g, true)

	go sess.Run(ctx)

	errOwnerJoin := sess.Join(ctx, "owner-1", "Alice", 2, ownerSender)
	if errOwnerJoin != nil {
		t.Fatalf("unexpected error when owner-1 joined: %v", errOwnerJoin)
	}

	errJoin := sess.Join(ctx, "client-2", "Bob", 2, clientSender)
	if errJoin != nil {
		t.Fatalf("unexpected error when client-2 joined: %v", errJoin)
	}

	if len(sess.members) != 2 {
		t.Errorf("expected 2 members in sessions, got %d", len(sess.members))
	}

	if _, exists := sess.members["client-2"]; !exists {
		t.Error("expected client-2 to exist in member map")
	}

	_ = ownerSender.next(t)

	// Ready State Check
	sess.SetReady(ctx, "owner-1", true)
	sess.SetReady(ctx, "client-2", true)

	_ = ownerSender.next(t)
	_ = clientSender.next(t)

	// Client Leave Check
	// TODO: Clarify leave behavior in standup
	sess.Leave(ctx, "client-2")

	//if len(sess.members) != 1 {
	//	t.Errorf("expected 1 member left in session, got %d", len(sess.members))
	//}

	leaveMsg := ownerSender.next(t)
	if leaveMsg == nil {
		t.Error("expected owner to receive state update after client-2 left")
	}
}

// TestSessionJoin_RejectFull
func TestSessionJoin_RejectFull(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ownerSender := newFakeSender()
	clientSender := newFakeSender()
	rejectSender := newFakeSender()

	g := &StubGame.StubGame{}

	sess := NewSession("session-1", "TestGame", g, true)

	go sess.Run(ctx)

	errOwnerJoin := sess.Join(ctx, "owner-1", "Alice", 1, ownerSender)
	if errOwnerJoin != nil {
		t.Fatalf("unexpected error when owner-1 joined: %v", errOwnerJoin)
	}

	errJoin := sess.Join(ctx, "client-2", "Bob", 2, clientSender)
	if errJoin != nil {
		t.Fatalf("unexpected error when client-2 joined: %v", errJoin)
	}

	if len(sess.members) != 2 {
		t.Errorf("expected 2 members in sessions, got %d", len(sess.members))
	}

	if _, exists := sess.members["client-2"]; !exists {
		t.Error("expected client-2 to exist in member map")
	}

	errReject := sess.Join(ctx, "client-3", "Bill", 3, rejectSender)
	if errReject == nil {
		t.Error("expected error when client joins full session")
	}
}

func startSession(t *testing.T, g game.Game) (*Session, context.Context) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s := NewSession("s1", "test", g, true)
	go s.Run(ctx)
	return s, ctx
}

func TestRejectedJoinDoesNotEnterTurnOrder(t *testing.T) {
	s, ctx := startSession(t, &oneSeatGame{})
	a := newFakeSender()
	if err := s.Join(ctx, "a", "", 1, a); err != nil {
		t.Fatal(err)
	}
	a.next(t)
	if err := s.Join(ctx, "b", "", 2, newFakeSender()); !errors.Is(err, ErrLobbyFull) {
		t.Fatalf("err = %v, want ErrLobbyFull", err)
	}

	s.SetReady(ctx, "a", true) // starts the match; turn order is copied from s.order
	state := a.next(t).GetStateUpdate()
	if state.GetStatus() != pb.SessionStatus_SESSION_STATUS_IN_PROGRESS {
		t.Fatalf("expected match to start, got %v", state)
	}
	if order := state.GetPlayerOrder(); len(order) != 1 || order[0] != "a" {
		t.Fatalf("player_order = %v, want [a]", order)
	}
}

// Joins refused by the session reach clients as ERROR_CODE_SESSION_NOT_JOINABLE. Tested here
// because a match cannot yet be started over the protocol.
func TestJoinAfterMatchStartsIsNotJoinable(t *testing.T) {
	s, ctx := startSession(t, &StubGame.StubGame{})
	a := newFakeSender()
	if err := s.Join(ctx, "a", "", 1, a); err != nil {
		t.Fatal(err)
	}
	a.next(t)
	s.SetReady(ctx, "a", true)
	if state := a.next(t).GetStateUpdate(); state.GetStatus() != pb.SessionStatus_SESSION_STATUS_IN_PROGRESS {
		t.Fatalf("expected match to start, got %v", state)
	}

	err := s.Join(ctx, "b", "", 2, newFakeSender())
	var perr *transport.ProtocolError
	if !errors.Is(err, ErrAlreadyStarted) || !errors.As(err, &perr) || perr.Code != transport.CodeSessionNotJoinable {
		t.Fatalf("err = %v, want ErrAlreadyStarted with CodeSessionNotJoinable", err)
	}
}

func TestFullLobbyIsNotJoinable(t *testing.T) {
	var perr *transport.ProtocolError
	if !errors.As(ErrLobbyFull, &perr) || perr.Code != transport.CodeSessionNotJoinable {
		t.Fatalf("ErrLobbyFull has code %v, want CodeSessionNotJoinable", perr)
	}
}

// Tests that a MatchResult is received in a real game implementation.
func TestChessCheckmateEndsTheMatch(t *testing.T) {
	s, ctx := startSession(t, &chess.Game{})
	white, black := newFakeSender(), newFakeSender()
	if err := s.Join(ctx, "white", "", 1, white); err != nil { // first to join plays white
		t.Fatal(err)
	}
	if err := s.Join(ctx, "black", "", 1, black); err != nil {
		t.Fatal(err)
	}
	s.SetReady(ctx, "white", true)
	s.SetReady(ctx, "black", true)
	for _, f := range []*fakeSender{white, black} {
		for f.next(t).GetStateUpdate().GetStatus() != pb.SessionStatus_SESSION_STATUS_IN_PROGRESS {
		}
	}

	// Fool's mate: 1. f3 e5 2. g4 Qh4#
	moves := []struct{ actor, uci string }{
		{"white", "f2f3"}, {"black", "e7e5"}, {"white", "g2g4"}, {"black", "d8h4"},
	}
	for i, m := range moves {
		seq := uint64(10 + i)
		s.Submit(ctx, game.Action{ID: seq, ActorID: m.actor, Payload: []byte(m.uci)})
		for _, f := range []*fakeSender{white, black} {
			if got := f.next(t); got.GetStateUpdate() == nil {
				t.Fatalf("move %s: expected a StateUpdate, got %v", m.uci, got)
			}
		}
	}

	for name, f := range map[string]*fakeSender{"white": white, "black": black} {
		got := f.next(t)
		result := got.GetMatchResult()
		if result == nil || got.GetInReplyTo() != 0 {
			t.Fatalf("%s: expected an unsolicited MatchResult after checkmate, got %v", name, got)
		}
		if result.GetDraw() || len(result.GetWinnerPlayerIds()) != 1 || result.GetWinnerPlayerIds()[0] != "black" {
			t.Fatalf("%s: expected black to win, got %v", name, result)
		}
	}
}

// secretGame shows each member only their own secret, like a hand of cards
type secretGame struct{ StubGame.StubGame }

func (g *secretGame) ViewFor(_ game.GameState, memberID string) []byte {
	return []byte("secret-of-" + memberID)
}

// startMatch joins each id, readies everyone, and returns each member's sender and first in-progress StateUpdate
func startMatch(t *testing.T, s *Session, ctx context.Context, ids ...string) (map[string]*fakeSender, map[string]*pb.StateUpdate) {
	t.Helper()
	senders := make(map[string]*fakeSender, len(ids))
	for _, id := range ids {
		senders[id] = newFakeSender()
		if err := s.Join(ctx, id, "", 1, senders[id]); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range ids {
		s.SetReady(ctx, id, true)
	}
	started := make(map[string]*pb.StateUpdate, len(ids))
	for id, f := range senders {
		for started[id].GetStatus() != pb.SessionStatus_SESSION_STATUS_IN_PROGRESS {
			started[id] = f.next(t).GetStateUpdate()
		}
	}
	return senders, started
}

func TestEachMemberReceivesOnlyTheirOwnView(t *testing.T) {
	s, ctx := startSession(t, &secretGame{})
	_, started := startMatch(t, s, ctx, "a", "b")
	for id, state := range started {
		if got, want := string(state.GetEncodedView()), "secret-of-"+id; got != want {
			t.Fatalf("%s's encoded_view = %q, want %q", id, got, want)
		}
	}
}

func TestChessViewIsTheCurrentPosition(t *testing.T) {
	const startFEN = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"
	s, ctx := startSession(t, &chess.Game{})
	senders, started := startMatch(t, s, ctx, "white", "black") // first to join plays white
	for id, state := range started {
		if got := string(state.GetEncodedView()); got != startFEN {
			t.Fatalf("%s's starting view = %q, want %q", id, got, startFEN)
		}
	}

	s.Submit(ctx, game.Action{ID: 5, ActorID: "white", Payload: []byte("e2e4")})
	const afterE4 = "rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 1"
	for _, f := range senders {
		if got := string(f.next(t).GetStateUpdate().GetEncodedView()); got != afterE4 {
			t.Fatalf("view after e2e4 = %q, want %q", got, afterE4)
		}
	}
}
