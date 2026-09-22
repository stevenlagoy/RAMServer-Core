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
	State() any // opaque to server; protocol layer serializes
}

type Action struct {
	ID      uint64
	From    Sender
	Payload []byte // raw until protocol defines decoding
}
