package session

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/stevenlagoy/ramserver-core/server/game"
)

// Owns the lifetime of every active Session.
type SessionStore struct {
	ctx      context.Context // the server's lifetime
	mu       sync.Mutex
	sessions map[string]storedSession
}

type storedSession struct {
	session *Session
	cancel  context.CancelFunc // stops the Session's Run goroutine
}

func NewSessionStore(ctx context.Context) *SessionStore {
	return &SessionStore{ctx: ctx, sessions: make(map[string]storedSession)}
}

// Starts a new, empty Session for g under gameName. It runs until its last member leaves, it is
// removed, or the store's context is cancelled. The creator must Join it. Caller must ensure id is unique.
func (st *SessionStore) Create(id, gameName string, g game.Game, public bool) (*Session, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if _, exists := st.sessions[id]; exists {
		return nil, fmt.Errorf("session: id %q already in use", id)
	}
	ctx, cancel := context.WithCancel(st.ctx)
	s := NewSession(id, gameName, g, public)
	token, err := generateJoinToken(id)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("generate initial join token: %w", err)
	}
	s.setJoinToken(token)
	s.onEmpty = func() { st.Remove(id) }
	st.sessions[id] = storedSession{session: s, cancel: cancel}
	go s.Run(ctx)
	return s, nil
}

// Returns session for id, if it exists.
func (st *SessionStore) Get(id string) (*Session, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	stored, ok := st.sessions[id]
	// Find by Join Token
	if !ok {
		for _, s := range st.sessions {
			if strings.EqualFold(s.session.joinTokenValue(), id) {
				stored, ok = s, true
			}
		}
	}
	return stored.session, ok
}

// Returns sessions for gameName (or all games if gameName is empty). When publicOnly
// is true, private lobbies are excluded.
func (st *SessionStore) List(gameName string, publicOnly bool) []*Session {
	st.mu.Lock()
	defer st.mu.Unlock()
	var result []*Session
	for _, stored := range st.sessions {
		s := stored.session
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

// Deletes id from the store and stops its Session's Run goroutine
func (st *SessionStore) Remove(id string) {
	st.mu.Lock()
	stored, ok := st.sessions[id]
	delete(st.sessions, id)
	st.mu.Unlock()
	if ok {
		stored.cancel()
	}
}
