package game

import "fmt"

// Factory constructs a fresh Game instance for one match.
type Factory func() Game

var registry = make(map[string]Factory)

// Register makes a game available by name. Call it from an init() in the package that implements the game.
// EX: func init() { game.Register("stub", func() game.Game { return &StubGame{} }) }
func Register(name string, factory Factory) {
	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("game: duplicate registration for %q", name))
	}
	registry[name] = factory
}

// New constructs a fresh Game by name
func New(name string) (Game, error) {
	factory, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("game: unknown game %q", name)
	}
	return factory(), nil
}

func init() {
	Register("stub", func() Game { return &StubGame{} })
}
