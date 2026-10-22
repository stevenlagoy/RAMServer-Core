package session

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"sync/atomic"

	"google.golang.org/protobuf/proto"

	pb "github.com/stevenlagoy/ramserver-core/server/gen/ramserver/v1"
	"github.com/stevenlagoy/ramserver-core/server/internal/game"
	"github.com/stevenlagoy/ramserver-core/server/internal/transport"
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

// oneSeatGame allows a single player and nothing else
type oneSeatGame struct{ game.StubGame }
type validationTestGame struct {
	game.StubGame
	applied atomic.Int32
}

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

func TestRejectedJoinDoesNotEnterTurnOrder(t *testing.T) {
	s, ctx := startSession(t, &oneSeatGame{})
	a := newFakeSender()
	if err := s.Join(ctx, "a", "", 1, a); err != nil {
		t.Fatal(err)
	}
	a.next(t)
	if err := s.Join(ctx, "b", "", 2, newFakeSender()); !errors.Is(err, ErrLobbyFull) {
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

// Joins refused by the session reach clients as ERROR_CODE_SESSION_NOT_JOINABLE. Tested here
// because a match cannot yet be started over the protocol.
func TestJoinAfterMatchStartsIsNotJoinable(t *testing.T) {
	s, ctx := startSession(t, &game.StubGame{})
	a := newFakeSender()
	if err := s.Join(ctx, "a", "", 1, a); err != nil {
		t.Fatal(err)
	}
	a.next(t)
	s.SetReady(ctx, "a", true)
	if state := a.next(t).GetStateUpdate(); state.GetStatus() != pb.SessionStatus_SESSION_STATUS_IN_PROGRESS {
		t.Fatalf("expected match to start, got %v", state)
	}

	err := s.Join(ctx, "b", "", 2, newFakeSender())
	var perr *transport.ProtocolError
	if !errors.Is(err, ErrAlreadyStarted) || !errors.As(err, &perr) || perr.Code != transport.CodeSessionNotJoinable {
		t.Fatalf("err = %v, want ErrAlreadyStarted with CodeSessionNotJoinable", err)
	}
}

func TestFullLobbyIsNotJoinable(t *testing.T) {
	var perr *transport.ProtocolError
	if !errors.As(ErrLobbyFull, &perr) || perr.Code != transport.CodeSessionNotJoinable {
		t.Fatalf("ErrLobbyFull has code %v, want CodeSessionNotJoinable", perr)
	}
}

func (g *validationTestGame) Validate(
	_ game.GameState,
	action game.Action,
) error {
	switch string(action.Payload) {
	case "illegal":
		return errors.New("illegal test action")
	case "malformed":
		return fmt.Errorf("%w: invalid test encoding", game.ErrMalformedAction)
	}

	return nil
}

func (g *validationTestGame) Apply(
	state game.GameState,
	action game.Action,
) game.GameState {
	g.applied.Add(1)
	return g.StubGame.Apply(state, action)
}

func TestActionStateVersion(t *testing.T) {
	g := &validationTestGame{}
	s, ctx := startSession(t, g)
	sender := newFakeSender()

	// Join the session.
	if err := s.Join(ctx, "a", "Alice", 1, sender); err != nil {
		t.Fatal(err)
	}
	sender.next(t) // Consume the join StateUpdate.

	// Start the match.
	s.SetReady(ctx, "a", true)
	started := sender.next(t).GetStateUpdate()
	if started == nil {
		t.Fatal("expected StateUpdate when match starts")
	}

	initialVersion := started.GetStateVersion()

	// Submit an illegal action.
	s.Submit(ctx, game.Action{
		ID:      2,
		ActorID: "a",
		Payload: []byte("illegal"),
	}, s.ID, initialVersion, sender)

	rejected := sender.next(t)
	if rejected.GetInReplyTo() != 2 ||
		rejected.GetErrorResponse().GetCode() != transport.CodeIllegalAction {
		t.Fatalf("expected ILLEGAL_ACTION response, got %v", rejected)
	}

	if g.applied.Load() != 0 {
		t.Fatal("illegal action was applied")
	}

	// Submit a legal action using the original version.
	s.Submit(ctx, game.Action{
		ID:      3,
		ActorID: "a",
		Payload: []byte("legal"),
	}, s.ID, initialVersion, sender)

	accepted := sender.next(t)
	update := accepted.GetStateUpdate()
	if update == nil {
		t.Fatalf("expected StateUpdate, got %v", accepted)
	}

	if update.GetStateVersion() != initialVersion+1 {
		t.Fatalf(
			"stateVersion = %d, want %d",
			update.GetStateVersion(),
			initialVersion+1,
		)
	}

	if g.applied.Load() != 1 {
		t.Fatalf("applied %d actions, want 1", g.applied.Load())
	}
}

func TestStaleStateRejected(t *testing.T) {
	g := &validationTestGame{}
	s, ctx := startSession(t, g)
	sender := newFakeSender()

	// Join and start the match.
	if err := s.Join(ctx, "a", "Alice", 1, sender); err != nil {
		t.Fatal(err)
	}
	sender.next(t)

	s.SetReady(ctx, "a", true)
	started := sender.next(t).GetStateUpdate()
	if started == nil {
		t.Fatal("expected match to start")
	}

	version := started.GetStateVersion()
	if version < 2 {
		t.Fatalf("unexpected initial version: %d", version)
	}

	// Submit a legal action using an outdated state version.
	s.Submit(ctx, game.Action{
		ID:      2,
		ActorID: "a",
		Payload: []byte("legal"),
	}, s.ID, version-1, sender)

	rejected := sender.next(t)
	resp := rejected.GetErrorResponse()

	if resp == nil ||
		resp.GetCode() != transport.CodeStaleState {
		t.Fatalf("expected STALE_STATE, got %v", rejected)
	}

	if rejected.GetInReplyTo() != 2 {
		t.Fatalf("incorrect response sequence: %v", rejected)
	}

	if g.applied.Load() != 0 {
		t.Fatal("stale action was applied")
	}

	// Retry with the correct version.
	s.Submit(ctx, game.Action{
		ID:      3,
		ActorID: "a",
		Payload: []byte("legal"),
	}, s.ID, version, sender)

	accepted := sender.next(t)
	update := accepted.GetStateUpdate()

	if update == nil {
		t.Fatalf("expected StateUpdate, got %v", accepted)
	}

	if update.GetStateVersion() != version+1 {
		t.Fatalf(
			"version = %d, want %d",
			update.GetStateVersion(),
			version+1,
		)
	}

	if g.applied.Load() != 1 {
		t.Fatalf(
			"applied %d actions, want 1",
			g.applied.Load(),
		)
	}
}
