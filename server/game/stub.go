package game

// Placeholder game to exercise the server
type StubGame struct{}

// StubGame's state
type stubState struct{}

func (g *StubGame) Roles() []RoleSpec {
	return nil
}

func (g *StubGame) NewState(rosterOrder []string, roles map[string]Role) GameState {
	return stubState{}
}

func (g *StubGame) LegalActions(state GameState, memberID string) []ActionSpec {
	return nil
}

func (g *StubGame) Validate(state GameState, action Action) error {
	return nil
}

func (g *StubGame) Apply(state GameState, action Action) GameState {
	return state
}

func (g *StubGame) ActiveTurn(state GameState) []string {
	return nil
}

func (g *StubGame) Outcome(state GameState) (MatchResult, bool) {
	return Result{}, false
}

func (g *StubGame) ReadyToStart(roster map[string]Role, ready map[string]bool) bool {
	return true
}

func (g *StubGame) ViewFor(state GameState, memberID string) []byte {
	return nil
}
