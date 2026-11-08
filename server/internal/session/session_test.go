package session

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/stevenlagoy/ramserver-core/server/rulesets/StubGame"

	"sync/atomic"

	"google.golang.org/protobuf/proto"

	"github.com/stevenlagoy/ramserver-core/server/game"
	pb "github.com/stevenlagoy/ramserver-core/server/gen/ramserver/v1"
	"github.com/stevenlagoy/ramserver-core/server/internal/persistence"
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
type oneSeatGame struct{ StubGame.StubGame }

type validationTestGame struct {
	StubGame.StubGame
	applied atomic.Int32
}

func (g *oneSeatGame) Roles() []game.RoleSpec {
	return []game.RoleSpec{{Name: game.RolePlayer, Min: 1, Max: 1}}
}

// TestSessionPipeline_JoinReadyLeave starts a session with a mock owner and mock sender to verify joining a session,
// tests users readying up, and a user leaving.
func TestSessionPipeline_JoinReadyLeave(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ownerSender := newFakeSender()
	clientSender := newFakeSender()

	g := &StubGame.StubGame{}

	sess := NewSession("session-1", "TestGame", g, true, nil)

	go sess.Run(ctx)

	errOwnerJoin := sess.Join(ctx, "owner-1", "Alice", 2, ownerSender)
	if errOwnerJoin != nil {
		t.Fatalf("unexpected error when owner-1 joined: %v", errOwnerJoin)
	}

	errJoin := sess.Join(ctx, "client-2", "Bob", 2, clientSender)
	if errJoin != nil {
		t.Fatalf("unexpected error when client-2 joined: %v", errJoin)
	}

	if len(sess.members) != 2 {
		t.Errorf("expected 2 members in sessions, got %d", len(sess.members))
	}

	if _, exists := sess.members["client-2"]; !exists {
		t.Error("expected client-2 to exist in member map")
	}

	_ = ownerSender.next(t)

	// Ready State Check
	sess.SetReady(ctx, "owner-1", true, 3)
	sess.SetReady(ctx, "client-2", true, 3)

	_ = ownerSender.next(t)
	_ = clientSender.next(t)

	// Client Leave Check
	// TODO: Clarify leave behavior in standup
	sess.Leave(ctx, "client-2")

	//if len(sess.members) != 1 {
	//	t.Errorf("expected 1 member left in session, got %d", len(sess.members))
	//}

	leaveMsg := ownerSender.next(t)
	if leaveMsg == nil {
		t.Error("expected owner to receive state update after client-2 left")
	}
}

// TestSessionJoin_RejectFull
func TestSessionJoin_RejectFull(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ownerSender := newFakeSender()
	clientSender := newFakeSender()
	rejectSender := newFakeSender()

	g := &StubGame.StubGame{}

	sess := NewSession("session-1", "TestGame", g, true, nil)

	go sess.Run(ctx)

	errOwnerJoin := sess.Join(ctx, "owner-1", "Alice", 1, ownerSender)
	if errOwnerJoin != nil {
		t.Fatalf("unexpected error when owner-1 joined: %v", errOwnerJoin)
	}

	errJoin := sess.Join(ctx, "client-2", "Bob", 2, clientSender)
	if errJoin != nil {
		t.Fatalf("unexpected error when client-2 joined: %v", errJoin)
	}

	if len(sess.members) != 2 {
		t.Errorf("expected 2 members in sessions, got %d", len(sess.members))
	}

	if _, exists := sess.members["client-2"]; !exists {
		t.Error("expected client-2 to exist in member map")
	}

	errReject := sess.Join(ctx, "client-3", "Bill", 3, rejectSender)
	if errReject == nil {
		t.Error("expected error when client joins full session")
	}
}

func startSession(t *testing.T, g game.Game) (*Session, context.Context) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s := NewSession("s1", "test", g, true, nil)
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

	s.SetReady(ctx, "a", true, 2) // starts the match; turn order is copied from s.order
	state := a.next(t).GetStateUpdate()
	if state.GetStatus() != pb.SessionStatus_SESSION_STATUS_IN_PROGRESS {
		t.Fatalf("expected match to start, got %v", state)
	}
	if order := state.GetPlayerOrder(); len(order) != 1 || order[0] != "a" {
		t.Fatalf("player_order = %v, want [a]", order)
	}
}

// Joins refused by the session reach clients as ERROR_CODE_SESSION_NOT_JOINABLE
func TestJoinAfterMatchStartsIsNotJoinable(t *testing.T) {
	s, ctx := startSession(t, &StubGame.StubGame{})
	a := newFakeSender()
	if err := s.Join(ctx, "a", "", 1, a); err != nil {
		t.Fatal(err)
	}
	a.next(t)
	s.SetReady(ctx, "a", true, 2)
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
	s.SetReady(ctx, "a", true, 2)
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

	s.SetReady(ctx, "a", true, 2)
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

// The stub ends after four actions with its own result type, which the core must pass on through the
// MatchResult interface rather than asserting a concrete type
func TestMatchEndBroadcastsMatchResult(t *testing.T) {
	s, ctx := startSession(t, &StubGame.StubGame{})
	a, b := newFakeSender(), newFakeSender()
	if err := s.Join(ctx, "a", "", 1, a); err != nil {
		t.Fatal(err)
	}
	if err := s.Join(ctx, "b", "", 1, b); err != nil {
		t.Fatal(err)
	}

	s.SetReady(ctx, "a", true, 2)
	s.SetReady(ctx, "b", true, 2)

	var stateVersion uint64

	for _, f := range []*fakeSender{a, b} {
		for {
			update := f.next(t).GetStateUpdate()
			if update == nil {
				t.Fatal("expected StateUpdate")
			}
			if update.GetStatus() == pb.SessionStatus_SESSION_STATUS_IN_PROGRESS {
				stateVersion = update.GetStateVersion()
				break
			}
		}
	}

	senders := map[string]*fakeSender{
		"a": a,
		"b": b,
	}

	for i, actor := range []string{"a", "b", "a", "b"} {
		seq := uint64(10 + i)

		s.Submit(ctx, game.Action{
			ID:      seq,
			ActorID: actor,
			Payload: []byte("NoOp"),
		}, s.ID, stateVersion, senders[actor])

		for _, f := range []*fakeSender{a, b} {
			got := f.next(t)
			update := got.GetStateUpdate()

			if update == nil {
				t.Fatalf("action %d: expected StateUpdate, got %v", i, got)
			}

			stateVersion = update.GetStateVersion()
		}
	}

	for name, f := range map[string]*fakeSender{"a": a, "b": b} {
		got := f.next(t)
		result := got.GetMatchResult()
		if result == nil || got.GetInReplyTo() != 0 {
			t.Fatalf("%s: expected an unsolicited MatchResult, got %v", name, got)
		}
		if !result.GetDraw() || len(result.GetWinnerPlayerIds()) != 0 {
			t.Fatalf("%s: expected a draw, got %v", name, result)
		}
	}
}

func TestLeaveOfLastUnreadyMemberStartsMatch(t *testing.T) {
	s, ctx := startSession(t, &StubGame.StubGame{}) // the stub can start with one ready player
	a := newFakeSender()
	if err := s.Join(ctx, "a", "", 1, a); err != nil {
		t.Fatal(err)
	}
	if err := s.Join(ctx, "b", "", 1, newFakeSender()); err != nil {
		t.Fatal(err)
	}
	s.SetReady(ctx, "a", true, 2)
	s.Leave(ctx, "b")

	var got *pb.ServerMessage
	for got.GetStateUpdate().GetStatus() != pb.SessionStatus_SESSION_STATUS_IN_PROGRESS {
		got = a.next(t) // fails if the match never starts
	}
	if got.GetInReplyTo() != 0 {
		t.Fatalf("expected an unsolicited start, got %v", got)
	}
	if order := got.GetStateUpdate().GetPlayerOrder(); len(order) != 1 || order[0] != "a" {
		t.Fatalf("player_order = %v, want [a]", order)
	}
}

// secretGame shows each member only their own secret, like a hand of cards
type secretGame struct{ StubGame.StubGame }

func (g *secretGame) ViewFor(_ game.GameState, memberID string) []byte {
	return []byte("secret-of-" + memberID)
}

// startMatch joins each id, readies everyone, and returns each member's sender and first in-progress StateUpdate
func startMatch(t *testing.T, s *Session, ctx context.Context, ids ...string) (map[string]*fakeSender, map[string]*pb.StateUpdate) {
	t.Helper()
	senders := make(map[string]*fakeSender, len(ids))
	for _, id := range ids {
		senders[id] = newFakeSender()
		if err := s.Join(ctx, id, "", 1, senders[id]); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range ids {
		s.SetReady(ctx, id, true, 2)
	}
	started := make(map[string]*pb.StateUpdate, len(ids))
	for id, f := range senders {
		for started[id].GetStatus() != pb.SessionStatus_SESSION_STATUS_IN_PROGRESS {
			started[id] = f.next(t).GetStateUpdate()
		}
	}
	return senders, started
}

func TestEachMemberReceivesOnlyTheirOwnView(t *testing.T) {
	s, ctx := startSession(t, &secretGame{})
	_, started := startMatch(t, s, ctx, "a", "b")
	for id, state := range started {
		if got, want := string(state.GetEncodedView()), "secret-of-"+id; got != want {
			t.Fatalf("%s's encoded_view = %q, want %q", id, got, want)
		}
	}
}

func TestRecordMatchAction(t *testing.T) {
	s := &Session{
		matchParticipants: []persistence.ParticipantRecord{
			{ClientID: "alice"},
			{ClientID: "bob"},
		},
	}

	// Record an accepted action.
	payload := []byte("e2e4")

	s.recordMatchAction(
		game.Action{
			ActorID: "alice",
			Payload: payload,
		},
		persistence.ActionAccepted,
		"",
		"",
	)

	// Record a rejected action.
	s.recordMatchAction(
		game.Action{
			ActorID: "bob",
			Payload: []byte("invalid"),
		},
		persistence.ActionRejected,
		"INVALID_ACTION",
		"Illegal move",
	)

	// Verify both actions were recorded.
	if len(s.matchActions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(s.matchActions))
	}

	// Verify ordering and outcomes.
	if s.matchActions[0].Seq != 1 ||
		s.matchActions[0].Outcome != persistence.ActionAccepted {
		t.Errorf("incorrect first action: %+v", s.matchActions[0])
	}

	if s.matchActions[1].Seq != 2 ||
		s.matchActions[1].Outcome != persistence.ActionRejected {
		t.Errorf("incorrect second action: %+v", s.matchActions[1])
	}

	// Verify participant statistics.
	if s.matchParticipants[0].ActionsAccepted != 1 {
		t.Error("Alice's accepted action was not counted")
	}

	if s.matchParticipants[1].ActionsRejected != 1 {
		t.Error("Bob's rejected action was not counted")
	}

	// Verify the payload was copied.
	payload[0] = 'x'

	if string(s.matchActions[0].Payload) != "e2e4" {
		t.Error("action payload was not copied")
	}

	// Verify the rejection information.
	if s.matchActions[1].RejectionCode != "INVALID_ACTION" {
		t.Error("rejection code was not preserved")
	}
}

func TestCompletedMatchPersists(t *testing.T) {
	ctx, cancel := context.WithTimeout(
		context.Background(), 5*time.Second,
	)
	defer cancel()

	// Create a temporary SQLite database.
	path := filepath.Join(t.TempDir(), "matches.sqlite")

	store, err := persistence.Open(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer store.Close()

	// Create a game that finishes after two actions.
	g := &StubGame.StubGame{EndAfter: 2}

	s := NewSession("persist-session", "StubGame", g, true, store)
	go s.Run(ctx)

	alice := newFakeSender()
	bob := newFakeSender()

	// Join two players.
	if err := s.Join(ctx, "alice", "Alice", 1, alice); err != nil {
		t.Fatalf("Alice join: %v", err)
	}

	if err := s.Join(ctx, "bob", "Bob", 1, bob); err != nil {
		t.Fatalf("Bob join: %v", err)
	}

	// Start the match.
	s.SetReady(ctx, "alice", true, 2)
	s.SetReady(ctx, "bob", true, 2)

	// Wait until the session processes the ready messages.
	if moves := s.LegalMoves(ctx, "alice"); len(moves) == 0 {
		t.Fatal("expected match to start")
	}

	// Submit one rejected action followed by two accepted actions.
	// Bob attempts to act when it is Alice's turn.
	s.Submit(ctx, game.Action{
		ID:      3,
		ActorID: "bob",
		Payload: []byte("NoOp"),
	}, s.ID, 0, bob)

	// Ensure the rejection has been processed.
	s.LegalMoves(ctx, "alice")

	// Submit two valid actions to finish the match.
	s.Submit(ctx, game.Action{
		ID:      4,
		ActorID: "alice",
		Payload: []byte("NoOp"),
	}, s.ID, 0, alice)

	s.Submit(ctx, game.Action{
		ID:      5,
		ActorID: "bob",
		Payload: []byte("NoOp"),
	}, s.ID, 0, bob)

	// Wait until both actions and the database save finish.
	s.LegalMoves(ctx, "alice")

	// This is the first match in a fresh database.
	saved, err := store.GetMatch(ctx, 1)
	if err != nil {
		t.Fatalf("retrieve completed match: %v", err)
	}

	// Verify match metadata.
	if saved.SessionID != "persist-session" {
		t.Errorf("unexpected session ID: %s", saved.SessionID)
	}

	if saved.MatchNumber != 1 {
		t.Errorf("expected match number 1, got %d", saved.MatchNumber)
	}

	if saved.GameID != "StubGame" {
		t.Errorf("unexpected game ID: %s", saved.GameID)
	}

	if !saved.IsDraw {
		t.Error("expected a draw")
	}

	// Verify participants.
	if len(saved.Participants) != 2 {
		t.Fatalf(
			"expected 2 participants, got %d",
			len(saved.Participants),
		)
	}

	// Verify the saved per-player action statistics.
	for _, participant := range saved.Participants {
		switch participant.ClientID {
		case "alice":
			if participant.ActionsAccepted != 1 || participant.ActionsRejected != 0 {
				t.Errorf("Alice: expected 1 accepted and 0 rejected actions, got %d and %d",
					participant.ActionsAccepted, participant.ActionsRejected)
			}
		case "bob":
			if participant.ActionsAccepted != 1 || participant.ActionsRejected != 1 {
				t.Errorf("Bob: expected 1 accepted and 1 rejected action, got %d and %d",
					participant.ActionsAccepted, participant.ActionsRejected)
			}
		default:
			t.Errorf("unexpected participant: %s", participant.ClientID)
		}
	}

	// Verify the complete action history.
	if len(saved.Actions) != 3 {
		t.Fatalf("expected 3 actions, got %d", len(saved.Actions))
	}

	expectedActors := []string{"bob", "alice", "bob"}
	expectedOutcomes := []string{
		persistence.ActionRejected,
		persistence.ActionAccepted,
		persistence.ActionAccepted,
	}

	for i, action := range saved.Actions {
		if action.Seq != int64(i+1) {
			t.Errorf("action %d: incorrect sequence", i)
		}

		if action.ClientID != expectedActors[i] {
			t.Errorf("action %d: incorrect actor", i)
		}

		if action.Outcome != expectedOutcomes[i] {
			t.Errorf("action %d: incorrect outcome", i)
		}

		if string(action.Payload) != "NoOp" {
			t.Errorf("action %d: incorrect payload", i)
		}
	}

	if saved.Actions[0].RejectionCode == "" {
		t.Error("rejected action missing rejection code")
	}

	// Stop the session and close the database.
	cancel()

	if err := store.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	// Reopen the same SQLite file.
	reopened, err := persistence.Open(path)
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}
	defer reopened.Close()

	// Retrieve the match after reopening.
	restored, err := reopened.GetMatch(
		context.Background(), saved.ID,
	)
	if err != nil {
		t.Fatalf("retrieve match after reopen: %v", err)
	}

	if !reflect.DeepEqual(saved, restored) {
		t.Errorf(
			"match changed after reopening database\nwant: %+v\ngot:  %+v",
			saved,
			restored,
		)
	}
}
