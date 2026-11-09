// Package integration plays real games through the server core, one file per game, to check the messages players receive.
// Ruleset unit tests stay with each ruleset, and the server's own tests use the stub game.
package integration

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

// startChessMatch runs a chess session, joins "white" then "black" (the first to join plays white),
// readies both, and returns their recorders and first in-progress StateUpdates
func startChessMatch(t *testing.T) (*session.Session, context.Context, *recorder, *recorder, []*pb.StateUpdate) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s := session.NewSession("s1", chess.Name, &chess.Game{}, true)
	go s.Run(ctx)

	white, black := newRecorder(), newRecorder()
	if err := s.Join(ctx, "white", "", 1, white); err != nil {
		t.Fatal(err)
	}
	if err := s.Join(ctx, "black", "", 1, black); err != nil {
		t.Fatal(err)
	}
	s.SetReady(ctx, "white", true, 2)
	s.SetReady(ctx, "black", true, 2)
	var started []*pb.StateUpdate
	for _, r := range []*recorder{white, black} {
		var state *pb.StateUpdate
		for state.GetStatus() != pb.SessionStatus_SESSION_STATUS_IN_PROGRESS {
			state = r.next(t).GetStateUpdate()
		}
		started = append(started, state)
	}
	return s, ctx, white, black, started
}

func TestViewIsTheCurrentPosition(t *testing.T) {
	const startFEN = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"
	s, ctx, white, black, started := startChessMatch(t)
	for _, state := range started {
		if got := string(state.GetEncodedView()); got != startFEN {
			t.Fatalf("starting view = %q, want %q", got, startFEN)
		}
	}

	s.Submit(ctx, game.Action{
		ID:      5,
		ActorID: "white",
		Payload: []byte("e2e4"),
	}, s.ID, started[0].GetStateVersion(), white)
	const afterE4 = "rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 1"
	for _, r := range []*recorder{white, black} {
		if got := string(r.next(t).GetStateUpdate().GetEncodedView()); got != afterE4 {
			t.Fatalf("view after e2e4 = %q, want %q", got, afterE4)
		}
	}
}

func TestCheckmateEndsTheSessionMatch(t *testing.T) {
	s, ctx, white, black, started := startChessMatch(t)
	stateVersion := started[0].GetStateVersion()

	// Fool's mate: 1. f3 e5 2. g4 Qh4#
	moves := []struct{ actor, uci string }{
		{"white", "f2f3"}, {"black", "e7e5"}, {"white", "g2g4"}, {"black", "d8h4"},
	}

	senders := map[string]*recorder{
		"white": white,
		"black": black,
	}

	for i, m := range moves {
		seq := uint64(10 + i)

		s.Submit(ctx, game.Action{
			ID:      seq,
			ActorID: m.actor,
			Payload: []byte(m.uci),
		}, s.ID, stateVersion, senders[m.actor])

		for _, r := range []*recorder{white, black} {
			got := r.next(t)
			update := got.GetStateUpdate()

			if update == nil {
				t.Fatalf("move %s: expected a StateUpdate, got %v", m.uci, got)
			}

			stateVersion = update.GetStateVersion()
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

func TestResigningOutOfTurnEndsTheMatch(t *testing.T) {
	s, ctx, white, black, _ := startChessMatch(t)

	// It is white's turn, and black resigns anyway
	s.Submit(ctx, game.Action{ID: 5, ActorID: "black", Payload: []byte("resign")})
	for _, r := range []*recorder{white, black} {
		if got := r.next(t); got.GetStateUpdate() == nil {
			t.Fatalf("expected a StateUpdate for the resignation, got %v", got)
		}
	}
	for name, r := range map[string]*recorder{"white": white, "black": black} {
		got := r.next(t)
		result := got.GetMatchResult()
		if result == nil || got.GetInReplyTo() != 0 {
			t.Fatalf("%s: expected an unsolicited MatchResult, got %v", name, got)
		}
		if result.GetDraw() || len(result.GetWinnerPlayerIds()) != 1 || result.GetWinnerPlayerIds()[0] != "white" ||
			result.GetTerminationReason() != "RESIGNATION" || result.GetTurns() != 1 {
			t.Fatalf("%s: expected white to win by resignation, got %v", name, result)
		}
	}
}

func TestStateUpdatesCarryCheckAnnotations(t *testing.T) {
	s, ctx, white, black, _ := startChessMatch(t)

	// 1. e4 f5 2. Qh5+ checks the black king through the square f7 which the f-pawn left
	moves := []struct{ actor, uci string }{{"white", "e2e4"}, {"black", "f7f5"}, {"white", "d1h5"}}
	var last *pb.StateUpdate
	for i, m := range moves {
		s.Submit(ctx, game.Action{ID: uint64(10 + i), ActorID: m.actor, Payload: []byte(m.uci)})
		for _, r := range []*recorder{white, black} {
			last = r.next(t).GetStateUpdate()
		}
	}
	want := map[string]string{"plies": "3", "last_move": "d1h5", "check": "black", "checkers": "h5", "king_unsafe": "f7"}
	for key, value := range want {
		if got := last.GetAnnotations()[key]; got != value {
			t.Errorf("annotation %s = %q, want %q", key, got, value)
		}
	}
	if _, offered := last.GetAnnotations()["draw_offer"]; offered {
		t.Error("a draw offer is reported although none was made")
	}
}
