package gofish

import (
	"fmt"

	"github.com/stevenlagoy/ramserver-core/server/game"
)

const Name = "go fish"

type Game struct{}

var _ game.Game = (*Game)(nil) // Compile-time check that Go Fish implements Game

const RoleDealer = "Dealer"

func (g *Game) Roles() []game.RoleSpec {
	return []game.RoleSpec{
		{Name: game.RolePlayer, Min: 2, Max: 0},    // Unlimited
		{Name: RoleDealer, Min: 1, Max: 1},         // Dealers act like players most of the time
		{Name: game.RoleSpectator, Min: 0, Max: 0}, // Optional, Unlimited
	}
}

func (g *Game) NewState(order []string, roles map[string]game.Role) game.GameState {
	players := make([]Player, len(order))
	for order, id := range order {
		if roles[id] != game.RoleSpectator {
			players = append(players, Player{PlayerOutsideView{id, 0, []Book{}, order}, []Card{}})
		}
	}
	if len(players) < 3 {
		panic(fmt.Sprintf("gofish: NewState needs 3 players, got %d", len(players)))
	}
	return State{
		players,
		0,
		makeCards(),
		nil,
	}
}

func (g *Game) Validate(s game.GameState, a game.Action) error {
	action, _, err := DecodeAction(a.Payload)
	if err != nil {
		return err
	}
}

func (g *Game) Apply(s game.GameState, a game.Action) game.GameState {
	action, _, err := DecodeAction(a.Payload)
	if err != nil {
		return err
	}
}

func (g *Game) AllowsOutOfTurn(_ game.GameState, a game.Action) bool {
	return false // No actions are allowed out-of-turn
}

func (g *Game) ActiveTurn(s game.GameState) []string {
	// Depends on recent action
}

func (g *Game) ReadyToStart(roster map[string]game.Role, ready map[string]bool) bool {
	players := 0
	for id, role := range roster {
		if role == game.RoleSpectator {
			continue // spectators do not block starting the game
		} // Role is Player or Dealer
		if !ready[id] {
			return false
		}
		players++
	}
	return players == 2
}

func (g *Game) Outcome(s game.GameState) (game.MatchResult, bool) {

}

func (g *Game) ViewFor(s game.GameState, player string) []byte {

}

func (g *Game) LegalActions(s game.GameState, player string) []game.ActionSpec {

}
