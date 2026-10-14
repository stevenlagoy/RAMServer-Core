package game

import (
	"testing"
)

type dummyGame struct{}

func (d dummyGame) Roles() []RoleSpec {
	panic("implement me")
}

func (d dummyGame) NewState(rosterOrder []string, roles map[string]Role) GameState {
	panic("implement me")
}

func (d dummyGame) LegalActions(state GameState, memberID string) []ActionSpec {
	panic("implement me")
}

func (d dummyGame) Validate(state GameState, action Action) error {
	panic("implement me")
}

func (d dummyGame) Apply(state GameState, action Action) GameState {
	panic("implement me")
}

func (d dummyGame) ActiveTurn(state GameState) []string {
	panic("implement me")
}

func (d dummyGame) Outcome(state GameState) (result Result, ok bool) {
	panic("implement me")
}

func (d dummyGame) ReadyToStart(roster map[string]Role, ready map[string]bool) bool {
	panic("implement me")
}

func (d dummyGame) ViewFor(state GameState, memberID string) any {
	panic("implement me")
}

func TestRegistry_RegisterNew(t *testing.T) {
	reg := NewRegistry()

	fac := func() Game {
		return &dummyGame{}
	}

	reg.Register("Game-1", fac)

	names := reg.Names()
	if len(names) != 1 || names[0] != "Game-1" {
		t.Errorf("expected names to consist of ['Game-1'], recived: %v", names)
	}

	gameInst, err := reg.New("Game-1")
	if err != nil {
		t.Fatalf("unexpected error creating game: %v", err)
	}
	if gameInst == nil {
		t.Fatal("expected valid game instance. got nil")
	}

	_, err = reg.New("nonexistent")
	if err == nil {
		t.Error("expected error for unknown game. got nil")
	}
}

func TestRegistry_Duplicate(t *testing.T) {
	reg := NewRegistry()

	fac := func() Game {
		return &dummyGame{}
	}

	reg.Register("Game-1", fac)

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected issue on duplicate game registration. got nil")
		}
	}()

	reg.Register("Game-1", fac)
}
