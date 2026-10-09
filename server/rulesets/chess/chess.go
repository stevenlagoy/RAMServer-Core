package chess

import "github.com/stevenlagoy/ramserver-core/server/game"

const Name = "chess"

type Game struct{}

var _ game.Game = (*Game)(nil) // compile-time check that we know what a game is

func (g *Game) Roles() []game.RoleSpec {
	return []game.RoleSpec{
		{Name: game.RolePlayer, Min: 2, Max: 2},
		{Name: game.RoleSpectator, Min: 0, Max: 0}, // unlimited
	}
}

func (g *Game) NewState(order []string, roles map[string]game.Role) game.GameState {
	var players []string
	for _, id := range order {
		if roles[id] == game.RolePlayer {
			players = append(players, id)
		}
	}
	return newState(players[0], players[1]) // First player to join will play white
}

func (g *Game) Validate(s game.GameState, a game.Action) error {
	m, err := DecodeMove(a.Payload)
	if err != nil {
		return err // game.ErrMalformedAction wrapper
	}
	if !s.(State).isLegal(m) {
		return game.ErrIllegalAction
	}
	return nil
}

func (g *Game) Apply(s game.GameState, a game.Action) game.GameState {
	m, _ := DecodeMove(a.Payload) // already validated
	return s.(State).apply(m)
}

func (g *Game) ActiveTurn(s game.GameState) []string {
	return []string{s.(State).activePlayer()}
}

func (g *Game) ReadyToStart(roster map[string]game.Role, ready map[string]bool) bool {
	players := 0
	for id, role := range roster {
		if role != game.RolePlayer {
			continue // spectators do not block starting the game
		}
		if !ready[id] {
			return false
		}
		players++
	}
	return players == 2
}

func (g *Game) Outcome(s game.GameState) (game.MatchResult, bool) {
	return s.(State).outcome()
}

func (g *Game) ViewFor(s game.GameState, _ string) any {
	return s.(State).view()
}

func (g *Game) LegalActions(s game.GameState, id string) []game.ActionSpec {
	return s.(State).legalActionSpecs(id)
}
