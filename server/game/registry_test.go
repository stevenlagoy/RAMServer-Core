package game

import (
	"testing"
)

func TestRegistry_RegisterNew(t *testing.T) {
	reg := NewRegistry()

	fac := func() Game {
		return &StubGame{}
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
		return &StubGame{}
	}

	reg.Register("Game-1", fac)

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected issue on duplicate game registration. got nil")
		}
	}()

	reg.Register("Game-1", fac)
}
