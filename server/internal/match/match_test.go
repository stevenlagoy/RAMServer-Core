package match

import (
	"errors"
	"github.com/stevenlagoy/ramserver-core/server/game"
	"github.com/stevenlagoy/ramserver-core/server/internal/testgame"
	"testing"
)

func TestMatchFlow(t *testing.T) {
	g := &testgame.StubGame{EndAfter: 2}
	roster := []string{"p1", "p2"}
	roles := map[string]game.Role{
		"p1": game.RolePlayer,
		"p2": game.RolePlayer,
	}

	m := NewMatch(g, roster, roles)

	if m.Turns != 0 {
		t.Errorf("expected 0 turns, got %d", m.Turns)
	}

	actionP1 := game.Action{ActorID: "p1", Payload: []byte("NoOp")}
	if err := m.ApplyAction(actionP1); err != nil {
		t.Fatalf("error applying action: %v", err)
	}
	if m.Turns != 1 {
		t.Errorf("expected 1 turns, got %d", m.Turns)
	}

	if err := m.ApplyAction(actionP1); !errors.Is(err, ErrNotYourTurn) {
		t.Errorf("expected ErrNotYourTurn, got: %v", err)
	}

	actionP2 := game.Action{ActorID: "p2", Payload: []byte("NoOp")}
	if err := m.ApplyAction(actionP2); err != nil {
		t.Fatalf("error applying action: %v", err)
	}
	if m.Turns != 2 {
		t.Errorf("expected 2 turns, got %d", m.Turns)
	}

	_, ended := m.Outcome()
	if !ended {
		t.Error("expected match to end")
	}

	// Update to expected result
	//if !res {
	//	t.Errorf("expected draw result, got: %v", res)
	//}
}

func TestLegalActions(t *testing.T) {
	g := &testgame.StubGame{EndAfter: 4}
	roster := []string{"p1"}
	roles := map[string]game.Role{
		"p1": game.RolePlayer,
	}

	m := NewMatch(g, roster, roles)

	actions := m.LegalActions("p1")
	if len(actions) == 0 {
		t.Errorf("expected legal actions for p1, got 0")
	}

	inactiveActions := m.LegalActions("p2")
	if len(inactiveActions) != 0 {
		t.Errorf("expected no legal actions for nonexistent player, got %v: ", inactiveActions)
	}

	view := m.ViewFor("p1")
	if view != 0 {
		t.Errorf("expected view state to be 0, got %v", view)
	}

	if err := m.ApplyAction(game.Action{ActorID: "p1"}); err != nil {
		t.Fatalf("unexpected error applying action: %v", err)
	}

	viewAfter := m.ViewFor("p1")
	if viewAfter != 1 {
		t.Errorf("expected view state to be 1, got %v", viewAfter)
	}
}
