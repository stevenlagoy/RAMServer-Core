package match

import (
	"errors"

	"github.com/stevenlagoy/ramserver-core/server/internal/game"
)

// Holds the live state of one round of play, including Game rules, roster and
// role assignments, current GameState, and how many actions have been applied.
// Owned by a Session which also manages channels and serialization
type Match struct {
	Game        game.Game
	RosterOrder []string
	Roles       map[string]game.Role
	State       game.GameState
	Turns       int
}

// Builds a Match and asks g for the starting GameState
func NewMatch(g game.Game, rosterOrder []string, roles map[string]game.Role) *Match {
	return &Match{
		Game:        g,
		RosterOrder: rosterOrder,
		Roles:       roles,
		State:       g.NewState(rosterOrder, roles),
	}
}

// Returned by ApplyAction when the action's ActorID is not currently active
var ErrNotYourTurn = errors.New("not your turn")

// Checks turn legality and asks Game to validate and apply action. Updates
// m.State and m.Turns when successful. Returns an error to the caller on failure.
func (m *Match) ApplyAction(action game.Action) error {
	if !isActive(m.Game.ActiveTurn(m.State), action.ActorID) {
		return ErrNotYourTurn
	}
	if err := m.Game.Validate(m.State, action); err != nil {
		return err
	}
	m.State = m.Game.Apply(m.State, action)
	m.Turns++
	return nil
}

func isActive(activeIDs []string, id string) bool {
	for _, a := range activeIDs {
		if a == id {
			return true
		}
	}
	return false
}

// Reports whether the match has ended
func (m *Match) Outcome() (game.Result, bool) {
	return m.Game.Outcome(m.State)
}

// Returns memberID's view of the current state
func (m *Match) ViewFor(memberID string) any {
	return m.Game.ViewFor(m.State, memberID)
}

// Returns the actions memberID may currently take
func (m *Match) LegalActions(memberID string) []game.ActionSpec {
	return m.Game.LegalActions(m.State, memberID)
}
