package match

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/stevenlagoy/ramserver-core/server/game"
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
	Seed        int64
	Actions     []game.Action
}

// Builds a Match using a server-generated seed.
func NewMatch(g game.Game, rosterOrder []string, roles map[string]game.Role) (*Match, error) {
	var seedBytes [8]byte
	if _, err := rand.Read(seedBytes[:]); err != nil {
		return nil, fmt.Errorf("generate match seed: %w", err)
	}
	return NewMatchWithSeed(g, rosterOrder, roles, int64(binary.BigEndian.Uint64(seedBytes[:]))), nil
}

// Builds a Match with a known seed, for deterministic replay.
func NewMatchWithSeed(g game.Game, rosterOrder []string, roles map[string]game.Role, seed int64) *Match {
	order := append([]string(nil), rosterOrder...)
	roleCopy := make(map[string]game.Role, len(roles))
	for id, role := range roles {
		roleCopy[id] = role
	}
	state := g.NewStateWithSeed(order, roleCopy, seed)
	return &Match{
		Game:        g,
		RosterOrder: order,
		Roles:       roleCopy,
		State:       state,
		Seed:        seed,
	}
}

// Replays actions from a match using its recorded seed.
func ReplayMatch(
	g game.Game,
	rosterOrder []string,
	roles map[string]game.Role,
	seed int64,
	actions []game.Action,
) (*Match, error) {
	replay := NewMatchWithSeed(g, rosterOrder, roles, seed)
	for i, action := range actions {
		if err := replay.ApplyAction(action); err != nil {
			return nil, fmt.Errorf("replay action %d: %w", i+1, err)
		}
	}
	return replay, nil
}

// Returned by ApplyAction when the action's ActorID is not currently active
var ErrNotYourTurn = errors.New("not your turn")

// Checks turn legality and asks Game to validate and apply action. Updates
// m.State and m.Turns when successful. Returns an error to the caller on failure.
// A member who is not active is refused unless the Game allows the action out of turn.
func (m *Match) ApplyAction(action game.Action) error {
	if !isActive(m.Game.ActiveTurn(m.State), action.ActorID) && !m.allowedOutOfTurn(action) {
		return ErrNotYourTurn
	}
	if err := m.Game.Validate(m.State, action); err != nil {
		return err
	}
	recorded := copyAction(action)
	m.State = m.Game.Apply(m.State, action)
	m.Turns++
	m.Actions = append(m.Actions, recorded)
	return nil
}

func copyAction(action game.Action) game.Action {
	action.Payload = append([]byte(nil), action.Payload...)
	return action
}

// Determine whether the Game allows an out-of-turn actor to submit this action
func (m *Match) allowedOutOfTurn(action game.Action) bool {
	g, ok := m.Game.(game.OutOfTurnActions)
	return ok && g.AllowsOutOfTurn(m.State, action)
}

func isActive(activeIDs []string, id string) bool {
	for _, a := range activeIDs {
		if a == id {
			return true
		}
	}
	return false
}

// Reports whether the match has ended. The result is whatever MatchResult the game returns, so it
// must only be used through the interface's methods: games may return their own result types
func (m *Match) Outcome() (game.MatchResult, bool) {
	return m.Game.Outcome(m.State)
}

// Returns memberID's view of the current state, encoded by the game
func (m *Match) ViewFor(memberID string) []byte {
	return m.Game.ViewFor(m.State, memberID)
}

// Returns the actions memberID may currently take
func (m *Match) LegalActions(memberID string) []game.ActionSpec {
	return m.Game.LegalActions(m.State, memberID)
}
