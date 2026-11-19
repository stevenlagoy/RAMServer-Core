package integration

import (
	"context"
	"testing"

	"github.com/stevenlagoy/ramserver-core/server/game"
	v1 "github.com/stevenlagoy/ramserver-core/server/gen/ramserver/v1"
	"github.com/stevenlagoy/ramserver-core/server/internal/session"
	"github.com/stevenlagoy/ramserver-core/server/rulesets/gofish"
)

type goFishIntegrationGame struct{ gofish.Game }

func (g *goFishIntegrationGame) Roles() []game.RoleSpec {
	return []game.RoleSpec{{Name: game.RolePlayer, Min: 3, Max: 3}}
}

func (g *goFishIntegrationGame) ReadyToStart(roster map[string]game.Role, ready map[string]bool) bool {
	if len(roster) != 3 {
		return false
	}
	for id := range roster {
		if !ready[id] {
			return false
		}
	}
	return true
}

func startGoFishMatch(t *testing.T) (*session.Session, context.Context, map[string]*recorder, []*v1.StateUpdate) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s := session.NewSession("s1", gofish.Name, &goFishIntegrationGame{}, true)
	go s.Run(ctx)

	ids := []string{"alice", "bob", "dealer"}
	recorders := make(map[string]*recorder, len(ids))
	for i, id := range ids {
		recorders[id] = newRecorder()
		if err := s.Join(ctx, id, "", uint64(i+1), recorders[id]); err != nil {
			t.Fatal(err)
		}
	}
	for i, id := range ids {
		s.SetReady(ctx, id, true, uint64(i+10))
	}

	started := make([]*v1.StateUpdate, 0, len(ids))
	for _, id := range ids {
		var state *v1.StateUpdate
		for state == nil || state.GetStatus() != v1.SessionStatus_SESSION_STATUS_IN_PROGRESS {
			state = recorders[id].next(t).GetStateUpdate()
		}
		started = append(started, state)
	}
	return s, ctx, recorders, started
}

func TestGoFishStartsWithDealtHandsAndAcceptsARequest(t *testing.T) {
	s, ctx, recorders, started := startGoFishMatch(t)
	for _, state := range started {
		if state.GetStatus() != v1.SessionStatus_SESSION_STATUS_IN_PROGRESS {
			t.Fatalf("match status = %v, want in progress", state.GetStatus())
		}
		if len(state.GetEncodedView()) == 0 {
			t.Fatal("starting Go Fish view is empty")
		}
	}

	deal := s.LegalMoves(ctx, "dealer")
	if len(deal) != 1 {
		t.Fatalf("dealer has %d legal actions before dealing, want one", len(deal))
	}
	s.Submit(ctx, game.Action{
		ID:      20,
		ActorID: "dealer",
		Payload: deal[0].Payload,
	}, s.ID, started[0].GetStateVersion(), recorders["dealer"])
	var dealt *v1.StateUpdate
	for _, id := range []string{"alice", "bob", "dealer"} {
		dealt = recorders[id].next(t).GetStateUpdate()
		if dealt == nil {
			t.Fatalf("%s did not receive the deal StateUpdate", id)
		}
	}

	legal := s.LegalMoves(ctx, "alice")
	if len(legal) == 0 {
		t.Fatal("alice has no legal request after the match started")
	}
	stateVersion := dealt.GetStateVersion()
	s.Submit(ctx, game.Action{
		ID:      21,
		ActorID: "alice",
		Payload: legal[0].Payload,
	}, s.ID, stateVersion, recorders["alice"])

	var updates []*v1.StateUpdate
	for _, id := range []string{"alice", "bob", "dealer"} {
		msg := recorders[id].next(t)
		update := msg.GetStateUpdate()
		if update == nil {
			t.Fatalf("%s received %v instead of a StateUpdate", id, msg)
		}
		updates = append(updates, update)
	}
	for _, update := range updates {
		if update.GetStateVersion() <= stateVersion {
			t.Errorf("state version = %d, want greater than %d", update.GetStateVersion(), stateVersion)
		}
	}
	if len(s.LegalMoves(ctx, "alice")) != 0 {
		t.Error("requesting player still has legal actions after yielding to the requestee")
	}
	requestee := map[byte]string{0: "alice", 1: "bob", 2: "dealer"}[legal[0].Payload[2]]
	if requestee == "" {
		t.Fatalf("request payload has unknown requestee order: %d", legal[0].Payload[2])
	}
	if len(s.LegalMoves(ctx, requestee)) == 0 {
		t.Errorf("requestee %s has no legal response", requestee)
	}
}
