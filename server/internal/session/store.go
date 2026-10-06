package session

import (
	"context"
	"fmt"
	"sync"

	"github.com/stevenlagoy/ramserver-core/server/game"
)

// Owns the lifetime of every active Session.
type SessionStore struct {
	mu       sync.Mutex
	sessions map[string]*Session
}

func NewSessionStore() *SessionStore {
	return &SessionStore{sessions: make(map[string]*Session)}
}

// Starts a new Session for g under gameName, owned by ownerID, and runs it until
// ctx is canelled or the store's Remove is called. Caller must ensure id is unique.
func (st *SessionStore) Create(
	ctx context.Context,
	id,
	gameName string,
	g game.Game,
	public bool,
	ownerID string,
	owner Sender,
) (*Session, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if _, exists := st.sessions[id]; exists {
		return nil, fmt.Errorf("session: id %q already in use", id)
	}
	s := NewSession(id, gameName, g, public, ownerID, owner)
	st.sessions[id] = s
	go s.Run(ctx)
	return s, nil
}

// Returns session for id, if it exists.
func (st *SessionStore) Get(id string) (*Session, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	s, ok := st.sessions[id]
	return s, ok
}

// Returns sessions for gameName (or all games if gameName is empty). When publicOnly
// is true, private lobbies are exclided.
func (st *SessionStore) List(gameName string, publicOnly bool) []*Session {
	st.mu.Lock()
	defer st.mu.Unlock()
	var result []*Session
	for _, s := range st.sessions {
		if gameName != "" && s.GameName != gameName {
			continue
		}
		if publicOnly && !s.Public {
			continue
		}
		result = append(result, s)
	}
	return result
}

// Deletes id from the store
func (st *SessionStore) Remove(id string) {
	// TODO: Session's own Run goroutine is not stopped here. Empty lobbies should stop own Run loops, but this can't be done currently because they share ctx
	st.mu.Lock()
	defer st.mu.Unlock()
	delete(st.sessions, id)
}
