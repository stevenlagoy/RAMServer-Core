package session

import (
	"context"
	"errors"
	"fmt"
	"github.com/stevenlagoy/ramserver-core/server/rulesets/StubGame"
	"sync"
	"testing"
	"time"
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
