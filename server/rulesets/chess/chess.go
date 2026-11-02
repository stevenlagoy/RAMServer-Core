package chess

import (
	"fmt"

	"github.com/stevenlagoy/ramserver-core/server/game"
)

const Name = "chess"

type Game struct{}

var _ game.Game = (*Game)(nil) // compile-time check that we know what a game is

func (g *Game) Roles() []game.RoleSpec {
	return []game.RoleSpec{
		{Name: game.RolePlayer, Min: 2, Max: 2},
		{Name: game.RoleSpectator, Min: 0, Max: 0}, // unlimited
	}
}

// Panics unless two players are present: ReadyToStart must be satisfied before a match is created
func (g *Game) NewState(order []string, roles map[string]game.Role) game.GameState {
	var players []string
	for _, id := range order {
		if roles[id] == game.RolePlayer {
			players = append(players, id)
		}
	}
	if len(players) < 2 {
		panic(fmt.Sprintf("chess: NewState needs two players, got %d", len(players)))
	}
	return newState(players[0], players[1]) // First player to join will play white
}

func (g *Game) Validate(s game.GameState, a game.Action) error {
	state := s.(State)
	if a.ActorID != state.activePlayer() {
		return fmt.Errorf("%w: it is not %q's turn", game.ErrIllegalAction, a.ActorID)
	}
	m, err := state.moveFromPayload(a.Payload)
	if err != nil {
		return err // game.ErrMalformedAction or game.ErrIllegalAction wrapper
	}
	if !state.isLegal(m) {
		return fmt.Errorf("%w: %s", game.ErrIllegalAction, m.toUCI())
	}
	return nil
}

// Assumes Validate accepted the action. A payload which cannot be decoded leaves the state unchanged
func (g *Game) Apply(s game.GameState, a game.Action) game.GameState {
	state := s.(State)
	m, err := state.moveFromPayload(a.Payload)
	if err != nil {
		return state
	}
	return state.apply(m)
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

// View is what a recipient sees. Chess has no hidden information, so every recipient gets the same view
type View struct {
	FEN string
	// TODO: move history in UCI and SAN
}

func (s State) view() View { return View{FEN: s.ToFEN()} }

// Encode is the chess wire format for encoded_view. Today it is the FEN as ASCII
func (v View) Encode() []byte { return []byte(v.FEN) }

func (g *Game) ViewFor(s game.GameState, _ string) []byte {
	return s.(State).view().Encode()
}

func (g *Game) LegalActions(s game.GameState, id string) []game.ActionSpec {
	state := s.(State)
	specs := state.legalActionSpecs(id)
	// Player to move may answera  draw offer
	// Not a part of legalActionSpecs because that list being empty is used to detect checkmate and stalemate
	if state.activeDrawOffer && id == state.activePlayer() {
		specs = append(specs, game.ActionSpec{
			Payload:     []byte(drawOfferMark),
			Description: fmt.Sprintf("%s accepts the draw offer", state.turn.name),
		})
	}
	return specs
}
