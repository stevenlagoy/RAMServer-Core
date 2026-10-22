package session

import (
	"testing"

	pb "github.com/stevenlagoy/ramserver-core/server/gen/ramserver/v1"
	"github.com/stevenlagoy/ramserver-core/server/internal/game"
	"github.com/stevenlagoy/ramserver-core/server/internal/transport"
)

// A session must answer requests even when the legacy match-based game
// implementation has not started a match. Rejection must not advance the version.
func TestWaitingPhaseActionReturnsError(t *testing.T) {
	s, ctx := startSession(t, &validationTestGame{})
	a := newFakeSender()
	if err := s.Join(ctx, "a", "Alice", 1, a); err != nil {
		t.Fatal(err)
	}
	joined := a.next(t).GetStateUpdate()
	if joined == nil {
		t.Fatal("expected join state update")
	}
	version := joined.GetStateVersion()

	s.Submit(ctx, game.Action{ID: 2, ActorID: "a", Payload: []byte("legal")}, s.ID, version, a)
	rejected := a.next(t)
	if rejected.GetInReplyTo() != 2 || rejected.GetErrorResponse().GetCode() != transport.CodeIllegalAction {
		t.Fatalf("expected waiting-phase ILLEGAL_ACTION response, got %v", rejected)
	}

	s.SetReady(ctx, "a", true)
	started := a.next(t).GetStateUpdate()
	if started == nil || started.GetStatus() != pb.SessionStatus_SESSION_STATUS_IN_PROGRESS || started.GetStateVersion() != version+1 {
		t.Fatalf("rejection mutated session state: got %v", started)
	}
}

func TestWrongTurnRejectionHasNoBroadcast(t *testing.T) {
	s, ctx := startSession(t, &validationTestGame{})
	a, b := newFakeSender(), newFakeSender()
	if err := s.Join(ctx, "a", "Alice", 1, a); err != nil {
		t.Fatal(err)
	}
	a.next(t)
	if err := s.Join(ctx, "b", "Bob", 2, b); err != nil {
		t.Fatal(err)
	}
	a.next(t)
	b.next(t)
	s.SetReady(ctx, "a", true)
	a.next(t)
	b.next(t)
	s.SetReady(ctx, "b", true)
	startedA, startedB := a.next(t).GetStateUpdate(), b.next(t).GetStateUpdate()
	if startedA == nil || startedB == nil {
		t.Fatal("expected start updates")
	}
	version := startedA.GetStateVersion()

	// Player a has the first turn; b must not act or cause any broadcast.
	s.Submit(ctx, game.Action{ID: 3, ActorID: "b", Payload: []byte("legal")}, s.ID, version, b)
	rejection := b.next(t)
	if rejection.GetInReplyTo() != 3 || rejection.GetErrorResponse().GetCode() != transport.CodeOutOfTurn {
		t.Fatalf("expected OUT_OF_TURN, got %v", rejection)
	}

	// An accepted action with the original version proves the rejection did not advance it.
	s.Submit(ctx, game.Action{ID: 4, ActorID: "a", Payload: []byte("legal")}, s.ID, version, a)
	accepted := a.next(t)
	updatedB := b.next(t)
	if accepted.GetInReplyTo() != 4 || accepted.GetStateUpdate().GetStateVersion() != version+1 {
		t.Fatalf("expected correlated accepted action at version %d, got %v", version+1, accepted)
	}
	// A broadcast caused by b's rejection would be the first frame here instead.
	if updatedB.GetInReplyTo() != 0 || updatedB.GetStateUpdate().GetStateVersion() != version+1 {
		t.Fatalf("rejected action apparently broadcast or incorrect update: %v", updatedB)
	}
}

func TestMalformedActionCode(t *testing.T) {
	s, ctx := startSession(t, &validationTestGame{})
	a := newFakeSender()
	if err := s.Join(ctx, "a", "Alice", 1, a); err != nil {
		t.Fatal(err)
	}
	a.next(t)
	s.SetReady(ctx, "a", true)
	started := a.next(t).GetStateUpdate()
	if started == nil {
		t.Fatal("expected match to start")
	}
	version := started.GetStateVersion()

	s.Submit(ctx, game.Action{ID: 2, ActorID: "a", Payload: []byte("malformed")}, s.ID, version, a)
	rejection := a.next(t)
	if rejection.GetInReplyTo() != 2 || rejection.GetErrorResponse().GetCode() != transport.CodeMalformedMessage {
		t.Fatalf("expected MALFORMED_MESSAGE, got %v", rejection)
	}
	s.Submit(ctx, game.Action{ID: 3, ActorID: "a", Payload: []byte("legal")}, s.ID, version, a)
	accepted := a.next(t)
	if accepted.GetInReplyTo() != 3 || accepted.GetStateUpdate().GetStateVersion() != version+1 {
		t.Fatalf("malformed action changed state: %v", accepted)
	}
}

func TestWrongSessionAndNonmemberRejected(t *testing.T) {
	s, ctx := startSession(t, &validationTestGame{})
	a := newFakeSender()
	if err := s.Join(ctx, "a", "Alice", 1, a); err != nil {
		t.Fatal(err)
	}
	a.next(t)
	s.SetReady(ctx, "a", true)
	started := a.next(t).GetStateUpdate()
	if started == nil {
		t.Fatal("expected match to start")
	}
	version := started.GetStateVersion()

	s.Submit(ctx, game.Action{ID: 2, ActorID: "a", Payload: []byte("legal")}, "different-session", version, a)
	response := a.next(t)
	if response.GetInReplyTo() != 2 || response.GetErrorResponse().GetCode() != transport.CodeNotInSession {
		t.Fatalf("expected wrong-session NOT_IN_SESSION, got %v", response)
	}

	stranger := newFakeSender()
	s.Submit(ctx, game.Action{ID: 3, ActorID: "stranger", Payload: []byte("legal")}, s.ID, version, stranger)
	response = stranger.next(t)
	if response.GetInReplyTo() != 3 || response.GetErrorResponse().GetCode() != transport.CodeNotInSession {
		t.Fatalf("expected nonmember NOT_IN_SESSION, got %v", response)
	}

	s.Submit(ctx, game.Action{ID: 4, ActorID: "a", Payload: []byte("legal")}, s.ID, version, a)
	accepted := a.next(t)
	if accepted.GetInReplyTo() != 4 || accepted.GetStateUpdate().GetStateVersion() != version+1 {
		t.Fatalf("rejected requests changed state or broadcast: %v", accepted)
	}
}
