package testutil

import (
	"github.com/stevenlagoy/ramserver-core/server/internal/game"
)

// Mock Game used for testing
type MockGame struct{}

func (g MockGame) NewState(rosterOrder []string, roles map[string]game.Role) game.GameState {
	//TODO implement me
	panic("implement me")
}

func (g MockGame) LegalActions(state game.GameState, memberID string) []game.ActionSpec {
	//TODO implement me
	panic("implement me")
}

func (g MockGame) Validate(state game.GameState, action game.Action) error {
	//TODO implement me
	panic("implement me")
}

func (g MockGame) Apply(state game.GameState, action game.Action) game.GameState {
	//TODO implement me
	panic("implement me")
}

func (g MockGame) ActiveTurn(state game.GameState) []string {
	//TODO implement me
	panic("implement me")
}

func (g MockGame) Outcome(state game.GameState) (result game.Result, ok bool) {
	//TODO implement me
	panic("implement me")
}

func (g MockGame) ViewFor(state game.GameState, memberID string) any {
	//TODO implement me
	panic("implement me")
}

func (MockGame) Roles() []game.RoleSpec {
	return []game.RoleSpec{
		{Name: "Player", Max: 1},
	}
}

func (MockGame) ReadyToStart(roster map[string]game.Role, ready map[string]bool) bool {
	return false
}
