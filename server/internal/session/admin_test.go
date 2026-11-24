package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stevenlagoy/ramserver-core/server/game"
	pb "github.com/stevenlagoy/ramserver-core/server/gen/ramserver/v1"
	"github.com/stevenlagoy/ramserver-core/server/internal/admin"
	"github.com/stevenlagoy/ramserver-core/server/rulesets/StubGame"
)

func TestInfoReportsMembersAndActionCounts(t *testing.T) {
	s, ctx := startSession(t, &StubGame.StubGame{})
	startMatch(t, s, ctx, "a", "b")
	s.Submit(ctx, game.Action{ID: 1, ActorID: "a", Payload: []byte("NoOp")}, s.ID, 0, newFakeSender())
	s.Submit(ctx, game.Action{ID: 2, ActorID: "a", Payload: []byte("NoOp")}, s.ID, 0, newFakeSender()) // out of turn: it is b's move

	info, err := s.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != "s1" || info.Status != "in_progress" || info.Turns != 1 || len(info.Members) != 2 {
		t.Fatalf("unexpected info: %+v", info)
	}
	a, b := info.Members[0], info.Members[1]
	if a.PlayerID != "a" || a.ActionsAccepted != 1 || a.ActionsRejected != 1 || !a.Ready {
		t.Fatalf("unexpected member a: %+v", a)
	}
	if b.PlayerID != "b" || b.ActionsAccepted != 0 || b.ActionsRejected != 0 {
		t.Fatalf("unexpected member b: %+v", b)
	}
}

func TestInfoIsNotActivity(t *testing.T) {
	s, ctx := startSession(t, &StubGame.StubGame{})
	first, err := s.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	second, _ := s.Info(ctx)
	if !first.LastActivity.Equal(second.LastActivity) {
		t.Fatal("inspecting a session must not count as activity, or polling would keep idle sessions alive")
	}
	if first.Created.IsZero() {
		t.Fatal("Created is not set")
	}
}

func TestInfoGivesUpOnAWedgedSession(t *testing.T) {
	s := NewSession("wedged", "test", &StubGame.StubGame{}, true) // Run is never started, so nothing answers
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := s.Info(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want a deadline error", err)
	}
	if err := s.Close(ctx, "stuck"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close err = %v, want a deadline error", err)
	}
}

func TestCloseAbortsMatchAndNotifiesEveryMember(t *testing.T) {
	s, ctx := startSession(t, &StubGame.StubGame{})
	senders, _ := startMatch(t, s, ctx, "a", "b")

	if err := s.Close(ctx, "misbehaving"); err != nil {
		t.Fatal(err)
	}
	if !s.IsClosed() {
		t.Fatal("IsClosed is false after Close")
	}
	for id, f := range senders {
		result := f.next(t).GetMatchResult()
		if result == nil || result.GetReason() != pb.MatchEndReason_MATCH_END_REASON_ABORTED || result.GetDraw() {
			t.Fatalf("%s: expected an aborted MatchResult, got %v", id, result)
		}
		state := f.next(t).GetStateUpdate()
		if state.GetStatus() != pb.SessionStatus_SESSION_STATUS_CLOSED || len(state.GetEncodedView()) != 0 || state.GetActivePlayerId() != "" {
			t.Fatalf("%s: expected a closed StateUpdate without game state, got %v", id, state)
		}
	}

	info, _ := s.Info(ctx)
	if info.Status != "closed" || len(info.Members) != 0 {
		t.Fatalf("unexpected info after close: %+v", info)
	}
	err := s.Join(ctx, "c", "", 3, newFakeSender())
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("join after close: err = %v, want ErrClosed", err)
	}
	if err := s.Close(ctx, "again"); err != nil { // closing twice is harmless
		t.Fatal(err)
	}
	s.Submit(ctx, game.Action{ID: 9, ActorID: "a", Payload: []byte("NoOp")}, s.ID, 0, newFakeSender()) // a stale action from a removed member is ignored
	s.SetReady(ctx, "a", true, 10)
}

func TestCloseWithoutMatchSendsOnlyClosedState(t *testing.T) {
	s, ctx := startSession(t, &StubGame.StubGame{})
	a := newFakeSender()
	if err := s.Join(ctx, "a", "", 1, a); err != nil {
		t.Fatal(err)
	}
	a.next(t) // the join's StateUpdate
	if err := s.Close(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if state := a.next(t).GetStateUpdate(); state.GetStatus() != pb.SessionStatus_SESSION_STATUS_CLOSED {
		t.Fatalf("expected a closed StateUpdate, got %v", state)
	}
}

func TestMetricsFollowAMatch(t *testing.T) {
	metrics := admin.NewMetrics()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s := NewSession("s1", "test", &StubGame.StubGame{}, true)
	s.Metrics = metrics
	go s.Run(ctx)

	a, b := newFakeSender(), newFakeSender()
	s.Join(ctx, "a", "", 1, a)
	s.Join(ctx, "b", "", 1, b)
	s.Join(ctx, "c", "", 1, newFakeSender()) // the stub holds two players, so this join is refused
	s.SetReady(ctx, "a", true, 2)
	s.SetReady(ctx, "b", true, 2)
	s.Submit(ctx, game.Action{ID: 1, ActorID: "b", Payload: []byte("NoOp")}, s.ID, 0, newFakeSender()) // out of turn: a moves first
	for i, actor := range []string{"a", "b", "a", "b"} {
		s.Submit(ctx, game.Action{ID: uint64(10 + i), ActorID: actor, Payload: []byte("NoOp")}, s.ID, 0, newFakeSender())
	}
	if _, err := s.Info(ctx); err != nil { // Run handles messages in order, so everything above has been counted
		t.Fatal(err)
	}

	got := metrics.Snapshot()
	want := map[string]int64{
		"joins_succeeded":              2,
		"joins_rejected":               1,
		"matches_started":              1,
		"matches_completed":            1,
		"actions_accepted":             4,
		"actions_rejected_out_of_turn": 1,
		"actions_rejected_illegal":     0,
	}
	for name, n := range want {
		if got[name] != n {
			t.Errorf("%s = %d, want %d", name, got[name], n)
		}
	}
}
