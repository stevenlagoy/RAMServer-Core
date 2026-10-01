package game

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

func (g *StubGame) Roles() []RoleSpec {
	return []RoleSpec{{Name: RolePlayer, Min: 1, Max: 0}}
}

func (g *StubGame) NewState(rosterOrder []string, roles map[string]Role) GameState {
	var order []string
	for _, id := range rosterOrder {
		if roles[id] == RolePlayer {
			order = append(order, id)
		}
	}
	endAfter := g.EndAfter
	if endAfter == 0 {
		endAfter = 4
	}
	return stubState{order: order, endAfter: endAfter}
}

func (g *StubGame) LegalActions(state GameState, memberID string) []ActionSpec {
	for _, id := range g.ActiveTurn(state) {
		if id == memberID {
			return []ActionSpec{{Payload: []byte("NoOp"), Description: "advance the counter"}}
		}
	}
	return nil
}

func (g *StubGame) Validate(state GameState, action Action) error {
	return nil // Accept anything
}

func (g *StubGame) Apply(state GameState, action Action) GameState {
	s := state.(stubState)
	s.applied++
	if len(s.order) > 0 {
		s.turn = (s.turn + 1) % len(s.order)
	}
	return s
}

func (g *StubGame) ActiveTurn(state GameState) []string {
	s := state.(stubState)
	if len(s.order) == 0 {
		return nil
	}
	return []string{s.order[s.turn%len(s.order)]} // This is kinda gross sorry but the linter wants it this way
}

func (g *StubGame) Outcome(state GameState) (Result, bool) {
	s := state.(stubState)
	if s.applied >= s.endAfter {
		return Result{Draw: true}, true
	}
	return Result{}, false
}

func (g *StubGame) ReadyToStart(roster map[string]Role, ready map[string]bool) bool {
	for id := range roster {
		if !ready[id] {
			return false
		}
	}
	return len(roster) > 0
}

func (g *StubGame) ViewFor(state GameState, memberID string) any {
	return state.(stubState).applied
}
