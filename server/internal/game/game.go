// Package game defines the game-agnostic interface for all pluggable games
// Server core does not directly interpret game rules
// Delegate rules decisions to a Game implementation
package game

// Sender is anything an Action can be replied to. *server.Client satisfies this
type Sender interface {
	Send(message []byte)
}

type Game interface {
	Validate(Action) error
	Apply(Action)
	// StateFor returns the game state as a memberID should see it. Games with hidden information use memberID to withhold what that members shouldn't see. Games with no hidden information can ignore this and return the same value for everyone.
	StateFor(memberID string) any
}

type Action struct {
	ID      uint64
	From    Sender
	Payload []byte // raw until protocol defines decoding
}
