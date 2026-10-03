// Package game defines the game-agnostic interface for all pluggable games
// Server core does not directly interpret game rules
// Delegate rules decisions to a Game implementation
package game

// GameState is a game-defined representation of a Match's current progress
// (board positions, players hands, scores, etc.). The server core does not
// inspect game states, only the Game implementation which produced it.
//
// Go does not enforce immutability. By convention, Apply must return a new
// GameState value instead of mutating one in-place. A game built on a struct
// should copy before modifying rather than mutate through a received pointer.
type GameState any

// Game is the immutable rulebook for one kind of game (imagine the Game as
// everything inside a board game's box, like the instructions and pieces). A
// Game must not store per-match data; all of that lives in the GameState value
// passed into the Game's methods. One Game built and registered with a name
// serves every concurrent Match of that game.
type Game interface {
	// Lists every role this game defines, and how many of each a session may hold
	Roles() []RoleSpec

	// Returns the initial GameState for a fresh Match. rosterOrder lists
	// participant IDs in a meaningful order (like join order). roles maps
	// each participant ID to their assigned Role. See StubGame for pattern
	NewState(rosterOrder []string, roles map[string]Role) GameState

	// Returns the actions memberID may currently take. Used to answer clients'
	// requests for possible moves, and to validate moves before applying them.
	LegalActions(state GameState, memberID string) []ActionSpec

	// Reports whether action is legal given state. Must not mutate state.
	Validate(state GameState, action Action) error

	// Returns the GameState after action is applied. Must not mutate in-place (see GameState).
	Apply(state GameState, action Action) GameState

	// Reports which member ID(s) may currently submit Actions. May return empty
	// for phases with no legal actors, but should not return nil.
	ActiveTurn(state GameState) []string

	// Reports whether state is a finished match, and how it ended if so. Should
	// return false as long as play is continuing.
	Outcome(state GameState) (result Result, ok bool)

	// Reports whether a lobby may begin a match, given its roster and each member's
	// ready flag. May depend on number of players present, number ready to start,
	// and whether required roles are filled.
	ReadyToStart(roster map[string]Role, ready map[string]bool) bool

	// Returns the portion of state visible to memberID, so hidden information
	// can be withheld based on the recipient. Can return the same state to
	// everyone for games with no hidden information.
	ViewFor(state GameState, memberID string) any
}

type Action struct {
	ID      uint64 // Client-assigned sequence number
	ActorID string
	Payload []byte
}

// Reports how a completed Match ended
type Result struct {
	WinnerIDs []string
	Scores    map[string]float32
	Draw      bool
}

// Describes one action a member may currently take. Payload is the same game-
// defined encoding as a client submits to validate or submit an action.
type ActionSpec struct {
	Payload     []byte
	Description string
}
