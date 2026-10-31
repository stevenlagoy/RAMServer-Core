package session

import (
	"context"
	"fmt"
	"github.com/stevenlagoy/ramserver-core/server/rulesets/StubGame"
	"sync"
	"testing"
)

func TestNewStore(t *testing.T) {
	st := NewSessionStore()
	if st == nil {
		t.Fatalf("expected non-nil SessionStore")
	}

	sessions := st.List("", false)
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions in a new store, got %d", len(sessions))
	}
}

func TestCreateAndGet(t *testing.T) {
	st := NewSessionStore()
	if st == nil {
		t.Fatalf("expected non-nil SessionStore")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	g := &StubGame.StubGame{}
	s, err := st.Create(ctx, "sess-1", "chess", g, true)
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
	st := NewSessionStore()
	if st == nil {
		t.Fatalf("expected non-nil SessionStore")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	g := &StubGame.StubGame{}
	_, err := st.Create(ctx, "sess-1", "chess", g, true)
	if err != nil {
		t.Fatalf("err creating session: %v", err)
	}

	_, err = st.Create(ctx, "sess-1", "checkers", g, true)
	if err == nil {
		t.Fatal("expected error when creating duplicate session")
	}
}

func TestList(t *testing.T) {
	st := NewSessionStore()
	if st == nil {
		t.Fatalf("expected non-nil SessionStore")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	g := &StubGame.StubGame{}
	st.Create(ctx, "sess-1", "chess", g, true)
	st.Create(ctx, "sess-2", "chess", g, false)
	st.Create(ctx, "sess-3", "checkers", g, true)

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
	st := NewSessionStore()
	if st == nil {
		t.Fatalf("expected non-nil SessionStore")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	g := &StubGame.StubGame{}
	st.Create(ctx, "sess-1", "chess", g, true)

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
	st := NewSessionStore()
	if st == nil {
		t.Fatalf("expected non-nil SessionStore")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

			_, _ = st.Create(ctx, fmt.Sprintf("s-%d", id), gameName, &StubGame.StubGame{}, id%3 == 0)
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
