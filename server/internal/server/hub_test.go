package server

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stevenlagoy/ramserver-core/server/internal/admin"

	pb "github.com/stevenlagoy/ramserver-core/server/gen/ramserver/v1"
)

// A config whose timeouts are long enough for a test to inspect the server between messages
func hubConfig(hub *Hub) ServerConfig {
	cfg := testConfig()
	cfg.ReadTimeout = 5 * time.Second
	cfg.FirstMessageTimeout = 5 * time.Second
	cfg.Hub = hub
	return cfg
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestHubListsConnectionsAndSessions(t *testing.T) {
	hub := NewHub()
	addr, stop := startTestServer(t, hubConfig(hub))
	defer stop()
	ctx := context.Background()
	alice, aliceR, aliceID := authenticate(t, addr, "Alice")
	sessionID := createSession(t, alice, aliceR, 3)
	dialTestClient(t, addr) // connects but never speaks

	waitFor(t, "both connections to be registered", func() bool {
		conns, _ := hub.Connections(ctx)
		return len(conns) == 2
	})
	conns, _ := hub.Connections(ctx)
	var authed, silent *admin.ConnInfo
	for i := range conns {
		if conns[i].Authenticated {
			authed = &conns[i]
		} else {
			silent = &conns[i]
		}
	}
	if authed == nil || silent == nil {
		t.Fatalf("expected one authenticated and one silent connection, got %+v", conns)
	}
	if authed.PlayerID != aliceID || authed.DisplayName != "Alice" || authed.SessionID != sessionID || authed.MessagesReceived < 3 {
		t.Fatalf("unexpected authenticated connection: %+v", *authed)
	}
	if silent.PlayerID != "" || silent.SessionID != "" || silent.Remote == "" {
		t.Fatalf("unexpected silent connection: %+v", *silent)
	}

	sessions, _ := hub.Sessions(ctx)
	if len(sessions) != 1 || sessions[0].ID != sessionID || sessions[0].Game != "stub" || sessions[0].Status != "waiting" ||
		len(sessions[0].Members) != 1 || sessions[0].Members[0].PlayerID != aliceID {
		t.Fatalf("unexpected sessions: %+v", sessions)
	}
	if g := hub.Gauges(); g["connections"] != 2 || g["sessions"] != 1 {
		t.Fatalf("gauges = %v", g)
	}
	snap := hub.Metrics.Snapshot()
	if snap["connections_accepted"] != 2 || snap["auth_succeeded"] != 1 || snap["sessions_created"] != 1 ||
		snap["joins_succeeded"] != 1 || snap["messages_received"] < 3 {
		t.Fatalf("metrics = %v", snap)
	}
}

func TestConnectionIsUnregisteredWhenItCloses(t *testing.T) {
	hub := NewHub()
	addr, stop := startTestServer(t, hubConfig(hub))
	defer stop()
	conn, _, _ := authenticate(t, addr, "Alice")
	waitFor(t, "registration", func() bool { return hub.Gauges()["connections"] == 1 })
	conn.Close()
	waitFor(t, "unregistration", func() bool { return hub.Gauges()["connections"] == 0 })
}

func TestCloseConnectionDisconnectsAndFreesTheSeat(t *testing.T) {
	hub := NewHub()
	addr, stop := startTestServer(t, hubConfig(hub))
	defer stop()
	ctx := context.Background()
	alice, aliceR, aliceID := authenticate(t, addr, "Alice")
	bob, bobR, bobID := authenticate(t, addr, "Bob")
	sessionID := createSession(t, alice, aliceR, 3)
	sendMessage(t, bob, joinRequest(3, "stub", sessionID))
	expectMessage(t, bob, bobR)
	expectMessage(t, alice, aliceR)

	conns, _ := hub.Connections(ctx)
	var aliceConn string
	for _, c := range conns {
		if c.PlayerID == aliceID {
			aliceConn = c.ID
		}
	}
	if aliceConn == "" {
		t.Fatalf("Alice's connection is not listed: %+v", conns)
	}
	if err := hub.CloseConnection(ctx, aliceConn); err != nil {
		t.Fatal(err)
	}

	alice.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.Copy(io.Discard, aliceR); err != nil && errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatal("the server did not close Alice's connection")
	}
	players := expectMessage(t, bob, bobR).GetStateUpdate().GetPlayers()
	if len(players) != 1 || players[0].GetPlayerId() != bobID {
		t.Fatalf("Alice's seat was not freed, players = %v", players)
	}
	if err := hub.CloseConnection(ctx, "conn-9999"); !errors.Is(err, admin.ErrNotFound) {
		t.Fatalf("closing an unknown connection: err = %v", err)
	}
	if hub.Metrics.Snapshot()["connections_closed_by_admin"] != 1 {
		t.Fatalf("metrics = %v", hub.Metrics.Snapshot())
	}
}

func TestCloseSessionNotifiesMembersAndLetsThemStartOver(t *testing.T) {
	hub := NewHub()
	addr, stop := startTestServer(t, hubConfig(hub))
	defer stop()
	ctx := context.Background()
	alice, aliceR, _ := authenticate(t, addr, "Alice")
	bob, bobR, _ := authenticate(t, addr, "Bob")
	sessionID := createSession(t, alice, aliceR, 3)
	sendMessage(t, bob, joinRequest(3, "stub", sessionID))
	expectMessage(t, bob, bobR)
	expectMessage(t, alice, aliceR)

	if err := hub.CloseSession(ctx, sessionID, "test"); err != nil {
		t.Fatal(err)
	}
	expectState(t, alice, aliceR, 0, pb.SessionStatus_SESSION_STATUS_CLOSED)
	expectState(t, bob, bobR, 0, pb.SessionStatus_SESSION_STATUS_CLOSED)

	if sessions, _ := hub.Sessions(ctx); len(sessions) != 0 {
		t.Fatalf("the closed session is still listed: %+v", sessions)
	}
	if err := hub.CloseSession(ctx, sessionID, ""); !errors.Is(err, admin.ErrNotFound) {
		t.Fatalf("closing it twice: err = %v", err)
	}
	if _, err := hub.Session(ctx, sessionID); !errors.Is(err, admin.ErrNotFound) {
		t.Fatalf("Session after close: err = %v", err)
	}

	// Bob learns his old session is gone, and can then join or create another
	sendMessage(t, bob, joinRequest(10, "stub", sessionID))
	expectError(t, bob, bobR, 10, pb.ErrorCode_ERROR_CODE_SESSION_NOT_FOUND)
	sendMessage(t, alice, leaveRequest(11, sessionID))
	expectError(t, alice, aliceR, 11, pb.ErrorCode_ERROR_CODE_NOT_IN_SESSION)
	if again := createSession(t, alice, aliceR, 12); again == sessionID {
		t.Fatal("expected a new session id")
	}
	if hub.Metrics.Snapshot()["sessions_closed_by_admin"] != 1 {
		t.Fatalf("metrics = %v", hub.Metrics.Snapshot())
	}
}
