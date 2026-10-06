package testgame

import "github.com/stevenlagoy/ramserver-core/server/game"

// Placeholder game to exercise the server
type StubGame struct {
	EndAfter int // How many actions end the match; defaults to 4
}

// StubGame's state
type stubState struct {
	order    []string
	turn     int
	applied  int
	endAfter int
}

func (g *StubGame) Roles() []game.RoleSpec {
	return []game.RoleSpec{{Name: game.RolePlayer, Min: 1, Max: 0}}
}

func (g *StubGame) NewState(rosterOrder []string, roles map[string]game.Role) game.GameState {
	var order []string
	for _, id := range rosterOrder {
		if roles[id] == game.RolePlayer {
			order = append(order, id)
		}
	}
	endAfter := g.EndAfter
	if endAfter == 0 {
		endAfter = 4
	}
	return stubState{order: order, endAfter: endAfter}
}

func (g *StubGame) LegalActions(state game.GameState, memberID string) []game.ActionSpec {
	for _, id := range g.ActiveTurn(state) {
		if id == memberID {
			return []game.ActionSpec{{Payload: []byte("NoOp"), Description: "advance the counter"}}
		}
	}
	return nil
}

func (g *StubGame) Validate(state game.GameState, action game.Action) error {
	return nil // Accept anything
}

func (g *StubGame) Apply(state game.GameState, action game.Action) game.GameState {
	s := state.(stubState)
	s.applied++
	if len(s.order) > 0 {
		s.turn = (s.turn + 1) % len(s.order)
	}
	return s
}

func (g *StubGame) ActiveTurn(state game.GameState) []string {
	s := state.(stubState)
	if len(s.order) == 0 {
		return nil
	}
	return []string{s.order[s.turn%len(s.order)]} // This is kinda gross sorry but the linter wants it this way
}

func (g *StubGame) Outcome(state game.GameState) (game.Result, bool) {
	s := state.(stubState)
	if s.applied >= s.endAfter {
		return game.Result{Draw: true}, true
	}
	return game.Result{}, false
}

func (g *StubGame) ReadyToStart(roster map[string]game.Role, ready map[string]bool) bool {
	for id := range roster {
		if !ready[id] {
			return false
		}
	}
	return len(roster) > 0
}

func (g *StubGame) ViewFor(state game.GameState, memberID string) any {
	return state.(stubState).applied
}
