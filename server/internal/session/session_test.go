package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stevenlagoy/ramserver-core/server/internal/testutil"
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

func (g *oneSeatGame) Roles() []game.RoleSpec {
	return []game.RoleSpec{{Name: game.RolePlayer, Min: 1, Max: 1}}
}

// TestSessionPipeline_JoinReadyLeave starts a session with a mock owner and mock sender to verify joining a session,
// tests users readying up, and a user leaving.
func TestSessionPipeline_JoinReadyLeave(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ownerSender := &testutil.MockSender{}
	clientSender := &testutil.MockSender{}

	g := testutil.MockGame{}

	sess := NewSession("session-1", "TestGame", g, true, "owner-1", ownerSender)

	go sess.Run(ctx)

	errJoin := sess.Join(ctx, "client-2", clientSender)
	if errJoin != nil {
		t.Fatalf("unexpected error when client-2 joined: %v", errJoin)
	}

	if len(sess.members) != 2 {
		t.Errorf("expected 2 members in sessions, got %d", len(sess.members))
	}

	if _, exists := sess.members["client-2"]; !exists {
		t.Error("expected client-2 to exist in member map")
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if len(ownerSender.Messages) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	messCount := len(ownerSender.Messages)
	if messCount == 0 {
		t.Error("expected lobby state broadcast upon client-2 join")
	}

	// Ready State Check
	sess.SetReady(ctx, "owner-1", true)
	sess.SetReady(ctx, "client-2", true)

	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if len(ownerSender.Messages) >= 2 && len(clientSender.Messages) >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if len(ownerSender.Messages) == messCount {
		t.Error("expected owner to receive ready state updates")
	}
	messCount = len(ownerSender.Messages)

	// Client Leave Check
	// TODO: Clarify leave behavior in standup
	sess.Leave(ctx, "client-2")

	if len(sess.members) != 1 {
		t.Errorf("expected 1 member left in session, got %d", len(sess.members))
	}

	if _, exists := sess.members["client-2"]; exists {
		t.Error("expected client-2 to be removed from member map")
	}

	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if len(ownerSender.Messages) >= 2 && len(clientSender.Messages) >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if len(ownerSender.Messages) == messCount {
		t.Error("expected owner to receive leave state updates")
	}
}

// TestSessionJoin_RejectFull
func TestSessionJoin_RejectFull(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ownerSender := &testutil.MockSender{}
	clientSender := &testutil.MockSender{}
	rejectSender := &testutil.MockSender{}

	g := testutil.MockGame{}

	sess := NewSession("session-1", "TestGame", g, true, "owner-1", ownerSender)

	go sess.Run(ctx)

	errJoin := sess.Join(ctx, "client-2", clientSender)
	if errJoin != nil {
		t.Fatalf("unexpected error when client-2 joined: %v", errJoin)
	}

	if len(sess.members) != 2 {
		t.Errorf("expected 2 members in sessions, got %d", len(sess.members))
	}

	if _, exists := sess.members["client-2"]; !exists {
		t.Error("expected client-2 to exist in member map")
	}

	errReject := sess.Join(ctx, "client-3", rejectSender)
	if errReject == nil {
		t.Error("expected error when client joins full session") // TODO: Raise question for role count assignment
	}

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
