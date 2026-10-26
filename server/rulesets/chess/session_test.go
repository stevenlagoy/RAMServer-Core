package chess_test

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/stevenlagoy/ramserver-core/server/game"
	pb "github.com/stevenlagoy/ramserver-core/server/gen/ramserver/v1"
	"github.com/stevenlagoy/ramserver-core/server/internal/session"
	"github.com/stevenlagoy/ramserver-core/server/rulesets/chess"
)

// Plays chess through a real server session, so the ruleset is checked against how the core
// actually drives a game. The server's own tests use the stub game instead.

// recorder collects the messages a session sends to one player
type recorder struct{ frames chan []byte }

func newRecorder() *recorder { return &recorder{frames: make(chan []byte, 16)} }

func (r *recorder) Send(b []byte) { r.frames <- b }

func (r *recorder) next(t *testing.T) *pb.ServerMessage {
	t.Helper()
	select {
	case b := <-r.frames:
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

func TestCheckmateEndsTheSessionMatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s := session.NewSession("s1", chess.Name, &chess.Game{}, true)
	go s.Run(ctx)

	white, black := newRecorder(), newRecorder()
	if err := s.Join(ctx, "white", "", 1, white); err != nil { // first to join plays white
		t.Fatal(err)
	}
	if err := s.Join(ctx, "black", "", 1, black); err != nil {
		t.Fatal(err)
	}
	s.SetReady(ctx, "white", true, 2)
	s.SetReady(ctx, "black", true, 2)
	for _, r := range []*recorder{white, black} {
		for r.next(t).GetStateUpdate().GetStatus() != pb.SessionStatus_SESSION_STATUS_IN_PROGRESS {
		}
	}

	// Fool's mate: 1. f3 e5 2. g4 Qh4#
	moves := []struct{ actor, uci string }{
		{"white", "f2f3"}, {"black", "e7e5"}, {"white", "g2g4"}, {"black", "d8h4"},
	}
	for i, m := range moves {
		s.Submit(ctx, game.Action{ID: uint64(10 + i), ActorID: m.actor, Payload: []byte(m.uci)})
		for _, r := range []*recorder{white, black} {
			if got := r.next(t); got.GetStateUpdate() == nil {
				t.Fatalf("move %s: expected a StateUpdate, got %v", m.uci, got)
			}
		}
	}

	for name, r := range map[string]*recorder{"white": white, "black": black} {
		got := r.next(t)
		result := got.GetMatchResult()
		if result == nil || got.GetInReplyTo() != 0 {
			t.Fatalf("%s: expected an unsolicited MatchResult after checkmate, got %v", name, got)
		}
		if result.GetDraw() || len(result.GetWinnerPlayerIds()) != 1 || result.GetWinnerPlayerIds()[0] != "black" {
			t.Fatalf("%s: expected black to win, got %v", name, result)
		}
	}
}
