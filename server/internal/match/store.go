package match

import (
	"context"
	"fmt"
	"sync"

	"github.com/stevenlagoy/ramserver-core/server/internal/game"
)

// Store owns the lifetime of every running Match.
type Store struct {
	mu      sync.Mutex
	matches map[string]*Match
	wg      sync.WaitGroup
}

func NewStore() *Store {
	return &Store{matches: make(map[string]*Match)}
}

// Create starts a new match under id and runs it until ctx is canelled or the match ends on its own. id must not already be in use.
func (s *Store) Create(ctx context.Context, id string, g game.Game) (*Match, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.matches[id]; exists {
		return nil, fmt.Errorf("match: id %q already in use", id)
	}
	m := NewMatch(g)
	s.matches[id] = m
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		m.Run(ctx)
		s.mu.Lock()
		delete(s.matches, id)
		s.mu.Unlock()
	}()
	return m, nil
}

// Get returns the match for the id, if it exists and is still running.
func (s *Store) Get(id string) (*Match, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.matches[id]
	return m, ok
}

// Wait blocks until every match this Store started has finished running.
func (s *Store) Wait() {
	s.wg.Wait()
}
