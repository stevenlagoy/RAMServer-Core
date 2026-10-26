package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/stevenlagoy/ramserver-core/server/game"
	pb "github.com/stevenlagoy/ramserver-core/server/gen/ramserver/v1"
	"github.com/stevenlagoy/ramserver-core/server/internal/testgame"
	"github.com/stevenlagoy/ramserver-core/server/internal/transport"
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
type oneSeatGame struct{ testgame.StubGame }

func (g *oneSeatGame) Roles() []game.RoleSpec {
	return []game.RoleSpec{{Name: game.RolePlayer, Min: 1, Max: 1}}
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

	s.SetReady(ctx, "a", true, 2) // starts the match; turn order is copied from s.order
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
	s, ctx := startSession(t, &testgame.StubGame{})
	a := newFakeSender()
	if err := s.Join(ctx, "a", "", 1, a); err != nil {
		t.Fatal(err)
	}
	a.next(t)
	s.SetReady(ctx, "a", true, 2)
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

// The stub ends after four actions with its own result type, which the core must pass on through the
// MatchResult interface rather than asserting a concrete type
func TestMatchEndBroadcastsMatchResult(t *testing.T) {
	s, ctx := startSession(t, &testgame.StubGame{})
	a, b := newFakeSender(), newFakeSender()
	if err := s.Join(ctx, "a", "", 1, a); err != nil {
		t.Fatal(err)
	}
	if err := s.Join(ctx, "b", "", 1, b); err != nil {
		t.Fatal(err)
	}
	s.SetReady(ctx, "a", true, 2)
	s.SetReady(ctx, "b", true, 2)
	for _, f := range []*fakeSender{a, b} {
		for f.next(t).GetStateUpdate().GetStatus() != pb.SessionStatus_SESSION_STATUS_IN_PROGRESS {
		}
	}

	for i, actor := range []string{"a", "b", "a", "b"} { // the stub alternates turns in join order
		s.Submit(ctx, game.Action{ID: uint64(10 + i), ActorID: actor, Payload: []byte("NoOp")})
		for _, f := range []*fakeSender{a, b} {
			if got := f.next(t); got.GetStateUpdate() == nil {
				t.Fatalf("action %d: expected a StateUpdate, got %v", i, got)
			}
		}
	}

	for name, f := range map[string]*fakeSender{"a": a, "b": b} {
		got := f.next(t)
		result := got.GetMatchResult()
		if result == nil || got.GetInReplyTo() != 0 {
			t.Fatalf("%s: expected an unsolicited MatchResult, got %v", name, got)
		}
		if !result.GetDraw() || len(result.GetWinnerPlayerIds()) != 0 {
			t.Fatalf("%s: expected a draw, got %v", name, result)
		}
	}
}

// secretGame shows each member only their own secret, like a hand of cards
type secretGame struct{ testgame.StubGame }

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
		s.SetReady(ctx, id, true, 2)
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
