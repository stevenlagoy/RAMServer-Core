package match

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stevenlagoy/ramserver-core/server/game"
	"github.com/stevenlagoy/ramserver-core/server/rulesets/StubGame"
)

func TestMatchFlow(t *testing.T) {
	g := &StubGame.StubGame{EndAfter: 2}
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
	g := &StubGame.StubGame{EndAfter: 4}
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

	expect1 := []byte{0, 0, 0, 0, 0, 0, 0, 0}
	expect2 := []byte{0, 0, 0, 0, 0, 0, 0, 1}

	view := m.ViewFor("p1")
	if !bytes.Equal(view, expect1) {
		t.Errorf("expected view state to be [0 0 0 0 0 0 0 0], got %v", view)
	}

	if err := m.ApplyAction(game.Action{ActorID: "p1"}); err != nil {
		t.Fatalf("unexpected error applying action: %v", err)
	}

	viewAfter := m.ViewFor("p1")

	if !bytes.Equal(viewAfter, expect2) {
		t.Errorf("expected view state to be [0 0 0 0 0 0 0 1], got %v", viewAfter)
	}
}

// interruptGame lets any member submit "Interrupt" even when it is not their turn
type interruptGame struct{ StubGame.StubGame }

func (g *interruptGame) AllowsOutOfTurn(_ game.GameState, a game.Action) bool {
	return string(a.Payload) == "Interrupt"
}

func TestOutOfTurnActions(t *testing.T) {
	roles := map[string]game.Role{"p1": game.RolePlayer, "p2": game.RolePlayer}
	m := NewMatch(&interruptGame{}, []string{"p1", "p2"}, roles) // p1 is active

	err := m.ApplyAction(game.Action{ActorID: "p2", Payload: []byte("NoOp")})
	if !errors.Is(err, ErrNotYourTurn) {
		t.Fatalf("ordinary action out of turn: error = %v, want ErrNotYourTurn", err)
	}
	if m.Turns != 0 {
		t.Fatalf("a refused action counted as a turn")
	}
	if err := m.ApplyAction(game.Action{ActorID: "p2", Payload: []byte("Interrupt")}); err != nil {
		t.Fatalf("an action the game allows out of turn was refused: %v", err)
	}
	if m.Turns != 1 {
		t.Errorf("expected 1 turn, got %d", m.Turns)
	}
}
