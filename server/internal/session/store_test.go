package session

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stevenlagoy/ramserver-core/server/rulesets/StubGame"
)

// waitStopped fails unless s's Run goroutine exits within a second
func waitStopped(t *testing.T, s *Session) {
	t.Helper()
	select {
	case <-s.stopped:
	case <-time.After(time.Second):
		t.Fatal("the session's goroutine did not exit")
	}
}

func TestLastLeaveRemovesAndStopsSession(t *testing.T) {
	st := NewSessionStore(t.Context())
	s, err := st.Create("sess-1", "stub", &StubGame.StubGame{}, true)
	if err != nil {
		t.Fatal(err)
	}
	a := newFakeSender()
	if err := s.Join(t.Context(), "a", "Alice", 1, a); err != nil {
		t.Fatal(err)
	}
	a.next(t)

	s.Leave(t.Context(), "a")
	waitStopped(t, s)
	if _, ok := st.Get("sess-1"); ok {
		t.Fatal("the empty session is still in the store")
	}

	// A caller still holding the session, such as matchmaking, gets an error instead of waiting forever
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := s.Join(ctx, "b", "Bob", 2, newFakeSender()); !errors.Is(err, ErrSessionClosed) {
		t.Fatalf("join on a stopped session: err = %v, want ErrSessionClosed", err)
	}
}

func TestRemoveStopsSession(t *testing.T) {
	st := NewSessionStore(t.Context())
	s, err := st.Create("sess-1", "stub", &StubGame.StubGame{}, true)
	if err != nil {
		t.Fatal(err)
	}
	st.Remove("sess-1")
	waitStopped(t, s)
}

func TestNewStore(t *testing.T) {
	st := NewSessionStore(t.Context())
	if st == nil {
		t.Fatalf("expected non-nil SessionStore")
	}

	sessions := st.List("", false)
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions in a new store, got %d", len(sessions))
	}
}

func TestCreateAndGet(t *testing.T) {
	st := NewSessionStore(t.Context())
	if st == nil {
		t.Fatalf("expected non-nil SessionStore")
	}

	g := &StubGame.StubGame{}
	s, err := st.Create("sess-1", "chess", g, true)
	if err != nil {
		t.Fatalf("err creating session: %v", err)
	}
	if s == nil {
		t.Fatal("expected session to exist")
	}

	fetched, ok := st.Get("sess-1") // expect session to exist

	if !ok || fetched != s {
		t.Errorf("expected to find session, got status: %v", ok)
	}

	_, ok = st.Get("non-existent")
	if ok {
		t.Error("expected to not find session")
	}
}

func TestCreateDuplicateID(t *testing.T) {
	st := NewSessionStore(t.Context())
	if st == nil {
		t.Fatalf("expected non-nil SessionStore")
	}

	g := &StubGame.StubGame{}
	_, err := st.Create("sess-1", "chess", g, true)
	if err != nil {
		t.Fatalf("err creating session: %v", err)
	}

	_, err = st.Create("sess-1", "checkers", g, true)
	if err == nil {
		t.Fatal("expected error when creating duplicate session")
	}
}

func TestList(t *testing.T) {
	st := NewSessionStore(t.Context())
	if st == nil {
		t.Fatalf("expected non-nil SessionStore")
	}

	g := &StubGame.StubGame{}
	st.Create("sess-1", "chess", g, true)
	st.Create("sess-2", "chess", g, false)
	st.Create("sess-3", "checkers", g, true)

	all := st.List("", false)
	if len(all) != 3 {
		t.Errorf("expected 3 sessions, got: %d", len(all))
	}

	chessAll := st.List("chess", false)
	if len(chessAll) != 2 {
		t.Errorf("expected 2 chess sessions, got: %d", len(chessAll))
	}

	chessPublic := st.List("chess", true)
	if len(chessPublic) != 1 {
		t.Errorf("expected 1 chess sessions, got: %d", len(chessPublic))
	}

	allPublic := st.List("", true)
	if len(allPublic) != 2 {
		t.Errorf("expected 2 sessions, got: %d", len(allPublic))
	}
}

func TestRemove(t *testing.T) {
	st := NewSessionStore(t.Context())
	if st == nil {
		t.Fatalf("expected non-nil SessionStore")
	}

	g := &StubGame.StubGame{}
	st.Create("sess-1", "chess", g, true)

	_, ok := st.Get("sess-1")
	if !ok {
		t.Fatal("expected session to exist")
	}

	st.Remove("sess-1")

	_, ok = st.Get("sess-1")
	if ok {
		t.Fatal("expected session to be removed")
	}

	if len(st.List("", false)) != 0 {
		t.Fatal("expected 0 sessions to exist")
	}
}

func TestSessionStoreConcurrency(t *testing.T) {
	st := NewSessionStore(t.Context())
	if st == nil {
		t.Fatalf("expected non-nil SessionStore")
	}

	var wg sync.WaitGroup
	workers := 50

	for i := range workers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			gameName := "chess"
			if id%2 == 0 {
				gameName = "checkers"
			}

			_, _ = st.Create(fmt.Sprintf("s-%d", id), gameName, &StubGame.StubGame{}, id%3 == 0)
		}(i)
	}
	wg.Wait()

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			if id%2 == 0 {
				st.Remove(fmt.Sprintf("s-%d", id))
			}
		}(i)
	}
	wg.Wait()
}

func TestJoinWithCaseInsensitiveToken(t *testing.T) {
	tests := []struct {
		name      string
		transform func(string) string
	}{
		{
			name:      "exact case",
			transform: func(token string) string { return token },
		},
		{
			name:      "lowercase",
			transform: strings.ToLower,
		},
		{
			name:      "uppercase",
			transform: strings.ToUpper,
		},
		{
			name: "mixed case",
			transform: func(token string) string {
				var result strings.Builder
				for i, r := range token {
					if i%2 == 0 {
						result.WriteString(strings.ToLower(string(r)))
					} else {
						result.WriteString(strings.ToUpper(string(r)))
					}
				}
				return result.String()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := NewSessionStore(t.Context())

			s, err := st.Create(
				"sess-1",
				"stub",
				&StubGame.StubGame{},
				true,
			)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				st.Remove("sess-1")
				waitStopped(t, s)
			})

			token := s.joinTokenValue()
			if token == "" {
				t.Fatal("expected session to have a join token")
			}

			suppliedToken := tt.transform(token)

			fetched, ok := st.Get(suppliedToken)
			if !ok {
				t.Fatalf("failed to find session using token %q", suppliedToken)
			}
			if fetched != s {
				t.Fatal("token lookup returned the wrong session")
			}

			sender := newFakeSender()
			if err := fetched.Join(
				t.Context(),
				"client-1",
				"Alice",
				1,
				sender,
			); err != nil {
				t.Fatalf("failed to join session using token %q: %v", suppliedToken, err)
			}

			if update := sender.next(t).GetStateUpdate(); update == nil {
				t.Fatal("expected a state update after joining")
			}
		})
	}
}

func TestJoinWithInvalidToken(t *testing.T) {
	st := NewSessionStore(t.Context())

	s, err := st.Create("sess-1", "stub", &StubGame.StubGame{}, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		st.Remove("sess-1")
		waitStopped(t, s)
	})

	token := s.joinTokenValue()

	tests := []struct {
		name  string
		token string
	}{
		{name: "incorrect token", token: token + "X"},
		{name: "empty token", token: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, ok := st.Get(tt.token); ok {
				t.Fatalf("unexpectedly found session using invalid token %q", tt.token)
			}
		})
	}
}

func TestOldJoinTokenBecomesInvalidAfterRegeneration(t *testing.T) {
	st := NewSessionStore(t.Context())

	s, err := st.Create("sess-1", "stub", &StubGame.StubGame{}, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		st.Remove("sess-1")
		waitStopped(t, s)
	})

	oldToken := s.joinTokenValue()
	if oldToken == "" {
		t.Fatal("expected session to have an initial join token")
	}

	// The session creator must join before generating a replacement token.
	sender := newFakeSender()
	if err := s.Join(t.Context(), "owner", "Alice", 1, sender); err != nil {
		t.Fatal(err)
	}
	sender.next(t)

	if err := s.GenerateJoinToken(t.Context(), "owner", 2); err != nil {
		t.Fatalf("failed to generate replacement token: %v", err)
	}

	// Consume the resulting state update.
	sender.next(t)

	newToken := s.joinTokenValue()
	if newToken == "" {
		t.Fatal("expected a replacement join token")
	}
	if newToken == oldToken {
		t.Fatal("expected replacement token to differ from old token")
	}

	if _, ok := st.Get(oldToken); ok {
		t.Fatal("old join token should no longer resolve to the session")
	}

	fetched, ok := st.Get(newToken)
	if !ok {
		t.Fatal("new join token should resolve to the session")
	}
	if fetched != s {
		t.Fatal("new join token resolved to the wrong session")
	}
}
