package session

import (
	"context"
	"testing"
	"time"

	"github.com/stevenlagoy/ramserver-core/server/internal/testutil"
)

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

// TODO: Additional tests include: legal move checks, game start verification, owner migration
