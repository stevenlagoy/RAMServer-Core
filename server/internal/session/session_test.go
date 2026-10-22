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
	s, ctx := startSession(t, &testgame.StubGame{})
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
