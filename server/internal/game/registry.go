package game

import "fmt"

// Factory constructs a fresh Game instance. Games are stateless, so this just
// returns the same instance most of the time. This can be useful for Games
// which have to load a lot of data for setup.
type Factory func() Game

// The server's library of available Games. Each server owns its own Registry
// so tests can register different sets of games per server instance.
type Registry struct {
	games map[string]Factory
}

func NewRegistry() *Registry {
	return &Registry{games: make(map[string]Factory)}
}

// Make a game available under name. Panics on duplicate names
func (r *Registry) Register(name string, factory Factory) {
	if _, exists := r.games[name]; exists {
		panic(fmt.Sprint("game: duplicate registration for %q", name))
	}
	r.games[name] = factory
}

func (r *Registry) New(name string) (Game, error) {
	factory, ok := r.games[name]
	if !ok {
		return nil, fmt.Errorf("game: unknown game %q", name)
	}
	return factory(), nil
}

// List every registered game
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.games))
	for name := range r.games {
		names = append(names, name)
	}
	return names
}
