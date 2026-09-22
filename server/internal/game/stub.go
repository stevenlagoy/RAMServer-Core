package game

// StubGame is a placeholder Game until real rulesets exist
type StubGame struct{ applied int }

func (g *StubGame) Validate(Action) error        { return nil }
func (g *StubGame) Apply(Action)                 { g.applied++ }
func (g *StubGame) StateFor(memberID string) any { return g.applied }
