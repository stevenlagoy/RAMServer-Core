package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	pb "github.com/stevenlagoy/ramserver-core/server/gen/ramserver/v1"
	"github.com/stevenlagoy/ramserver-core/server/internal/game"
)

// fakeSender records every frame a Session sends to one member
type fakeSender struct{ frames chan []byte }

func newFakeSender() *fakeSender { return &fakeSender{frames: make(chan []byte, 16)} }

func (f *fakeSender) Send(b []byte) { f.frames <- b }

func (f *fakeSender) next(t *testing.T) *pb.ServerMessage {
	t.Helper()
	select {
	case b := <-f.frames:
		var msg pb.ServerMessage
		if err := proto.Unmarshal(b, &msg); err != nil {
			t.Fatal(err)
		}
		return &msg
	case <-time.After(time.Second):
		t.Fatal("expected a message from the session")
		return nil
	}
}

// twoSeatGame allows two players and any number of spectators
type twoSeatGame struct{ game.StubGame }

func (g *twoSeatGame) Roles() []game.RoleSpec {
	return []game.RoleSpec{
		{Name: game.RolePlayer, Min: 2, Max: 2},
		{Name: game.RoleSpectator, Max: 0},
	}
}

// oneSeatGame allows a single player and nothing else
type oneSeatGame struct{ game.StubGame }

func (g *oneSeatGame) Roles() []game.RoleSpec {
	return []game.RoleSpec{{Name: game.RolePlayer, Min: 1, Max: 1}}
}

func startSession(t *testing.T, g game.Game) (*Session, context.Context) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s := NewSession("s1", "test", g, true)
	go s.Run(ctx)
	return s, ctx
}

func rolesIn(msg *pb.ServerMessage) map[string]string {
	roles := make(map[string]string)
	for _, p := range msg.GetStateUpdate().GetPlayers() {
		roles[p.GetPlayerId()] = p.GetRole()
	}
	return roles
}

func TestJoinAssignsFirstRoleWithRoom(t *testing.T) {
	s, ctx := startSession(t, &twoSeatGame{})
	senders := map[string]*fakeSender{"a": newFakeSender(), "b": newFakeSender(), "c": newFakeSender()}
	for i, id := range []string{"a", "b", "c"} {
		if err := s.Join(ctx, id, uint64(i+1), "", senders[id]); err != nil {
			t.Fatalf("join %s: %v", id, err)
		}
	}
	var last *pb.ServerMessage
	for range 3 { // a sees all three broadcasts
		last = senders["a"].next(t)
	}
	want := map[string]string{"a": "player", "b": "player", "c": "spectator"}
	if got := rolesIn(last); len(got) != 3 || got["a"] != want["a"] || got["b"] != want["b"] || got["c"] != want["c"] {
		t.Fatalf("roles = %v, want %v", got, want)
	}
}

func TestJoinWithRequestedRole(t *testing.T) {
	s, ctx := startSession(t, &twoSeatGame{})
	a := newFakeSender()
	if err := s.Join(ctx, "a", 1, game.RoleSpectator, a); err != nil {
		t.Fatal(err)
	}
	if got := rolesIn(a.next(t))["a"]; got != "spectator" {
		t.Fatalf("role = %q, want spectator", got)
	}
}

func TestJoinRejectsUnknownRole(t *testing.T) {
	s, ctx := startSession(t, &twoSeatGame{})
	if err := s.Join(ctx, "a", 1, "dealer", newFakeSender()); !errors.Is(err, ErrUnknownRole) {
		t.Fatalf("err = %v, want ErrUnknownRole", err)
	}
}

func TestJoinRejectsFullRole(t *testing.T) {
	s, ctx := startSession(t, &twoSeatGame{})
	for _, id := range []string{"a", "b"} {
		if err := s.Join(ctx, id, 1, game.RolePlayer, newFakeSender()); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Join(ctx, "c", 1, game.RolePlayer, newFakeSender()); !errors.Is(err, ErrRoleFull) {
		t.Fatalf("err = %v, want ErrRoleFull", err)
	}
}

func TestRejectedJoinDoesNotEnterTurnOrder(t *testing.T) {
	s, ctx := startSession(t, &oneSeatGame{})
	a := newFakeSender()
	if err := s.Join(ctx, "a", 1, "", a); err != nil {
		t.Fatal(err)
	}
	a.next(t)
	if err := s.Join(ctx, "b", 2, "", newFakeSender()); !errors.Is(err, ErrLobbyFull) {
		t.Fatalf("err = %v, want ErrLobbyFull", err)
	}

	s.SetReady(ctx, "a", true) // starts the match; turn order is copied from s.order
	state := a.next(t).GetStateUpdate()
	if state.GetStatus() != pb.SessionStatus_SESSION_STATUS_IN_PROGRESS {
		t.Fatalf("expected match to start, got %v", state)
	}
	if order := state.GetPlayerOrder(); len(order) != 1 || order[0] != "a" {
		t.Fatalf("player_order = %v, want [a]", order)
	}
}
