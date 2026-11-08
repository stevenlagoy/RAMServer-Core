package persistence

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestSaveMatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.sqlite")

	store, err := Open(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 11, 7, 17, 0, 0, 0, time.UTC)
	version := int64(1)

	record := MatchRecord{
		SessionID:         "session-123",
		SessionCreatedAt:  now.Add(-time.Hour),
		MatchNumber:       1,
		GameID:            "chess",
		StartedAt:         now,
		EndedAt:           now.Add(10 * time.Minute),
		TerminationReason: "completed",
		IsDraw:            false,

		Participants: []ParticipantRecord{
			{
				ClientID:    "alice-123",
				DisplayName: "Alice",
				Role:        "white",
				Seat:        0,
				IsWinner:    true,
			},
			{
				ClientID:    "bob-456",
				DisplayName: "Bob",
				Role:        "black",
				Seat:        1,
				IsWinner:    false,
			},
		},

		Actions: []ActionRecord{
			{
				Seq:          1,
				ClientID:     "alice-123",
				ReceivedAt:   now.Add(time.Minute),
				Payload:      []byte("e2e4"),
				Outcome:      ActionAccepted,
				StateVersion: &version,
			},
			{
				Seq:             2,
				ClientID:        "bob-456",
				ReceivedAt:      now.Add(2 * time.Minute),
				Payload:         []byte("invalid-move"),
				Outcome:         ActionRejected,
				RejectionCode:   "INVALID_ACTION",
				RejectionReason: "Illegal move",
				StateVersion:    &version,
			},
		},
	}

	matchID, err := store.SaveMatch(context.Background(), record)
	if err != nil {
		t.Fatalf("save match: %v", err)
	}

	if matchID <= 0 {
		t.Fatalf("expected positive match ID, got %d", matchID)
	}

	checks := []struct {
		name  string
		query string
		want  int
	}{
		{"sessions", "SELECT COUNT(*) FROM session", 1},
		{"clients", "SELECT COUNT(*) FROM client", 2},
		{"matches", `SELECT COUNT(*) FROM "match"`, 1},
		{"participants", "SELECT COUNT(*) FROM match_participant", 2},
		{"actions", "SELECT COUNT(*) FROM action_log", 2},
	}

	for _, check := range checks {
		var got int

		err := store.db.QueryRow(check.query).Scan(&got)
		if err != nil {
			t.Fatalf("count %s: %v", check.name, err)
		}

		if got != check.want {
			t.Errorf(
				"%s: expected %d records, got %d",
				check.name, check.want, got,
			)
		}
	}

	var outcome string
	var payload []byte
	var stateVersion int64

	err = store.db.QueryRow(`
        SELECT outcome, payload, state_version
        FROM action_log
        WHERE match_id = ? AND seq = 2
    `, matchID).Scan(&outcome, &payload, &stateVersion)

	if err != nil {
		t.Fatalf("retrieve action: %v", err)
	}

	if outcome != ActionRejected {
		t.Errorf("expected rejected action, got %s", outcome)
	}

	if string(payload) != "invalid-move" {
		t.Errorf("unexpected action payload: %s", payload)
	}

	if stateVersion != 1 {
		t.Errorf("expected state version 1, got %d", stateVersion)
	}

	retrieved, err := store.GetMatch(context.Background(), matchID)
	if err != nil {
		t.Fatalf("retrieve match: %v", err)
	}

	record.ID = matchID

	if !reflect.DeepEqual(record, retrieved) {
		t.Errorf(
			"retrieved match does not match original\nwant: %+v\ngot:  %+v",
			record,
			retrieved,
		)
	}

	// Saving the same match again must be idempotent.
	retryID, err := store.SaveMatch(context.Background(), record)
	if err != nil {
		t.Fatalf("retrying the same match failed: %v", err)
	}

	if retryID != matchID {
		t.Errorf(
			"retry returned match ID %d, expected %d",
			retryID,
			matchID,
		)
	}

	var matchCount int
	err = store.db.QueryRow(
		`SELECT COUNT(*) FROM "match"`,
	).Scan(&matchCount)
	if err != nil {
		t.Fatalf("count matches: %v", err)
	}

	if matchCount != 1 {
		t.Errorf("expected 1 stored match, got %d", matchCount)
	}

	// The same match identity must not silently accept different data.
	conflicting := record
	conflicting.TerminationReason = "ABORTED"

	if _, err := store.SaveMatch(context.Background(), conflicting); err == nil {
		t.Error("expected conflicting match data to be rejected")
	}

}

func TestSaveMatchRollback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.sqlite")

	store, err := Open(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 11, 7, 17, 0, 0, 0, time.UTC)

	record := MatchRecord{
		SessionID:         "rollback-session",
		SessionCreatedAt:  now,
		MatchNumber:       1,
		GameID:            "chess",
		StartedAt:         now,
		EndedAt:           now.Add(10 * time.Minute),
		TerminationReason: "completed",

		Participants: []ParticipantRecord{
			{
				ClientID:    "alice-123",
				DisplayName: "Alice",
				Role:        "white",
				Seat:        0,
			},
			{
				ClientID:    "bob-456",
				DisplayName: "Bob",
				Role:        "black",
				Seat:        0, // Deliberate duplicate!
			},
		},
	}

	matchID, err := store.SaveMatch(context.Background(), record)

	if err == nil {
		t.Fatal("expected SaveMatch to fail")
	}

	if matchID != 0 {
		t.Errorf("expected match ID 0, got %d", matchID)
	}
}

func TestGetMatchNotFound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.sqlite")

	store, err := Open(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer store.Close()

	_, err = store.GetMatch(context.Background(), 999)

	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestSaveMatchSessionConflict(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Date(2026, 11, 7, 17, 0, 0, 0, time.UTC)

	record := MatchRecord{
		SessionID:         "shared-session",
		SessionCreatedAt:  now,
		MatchNumber:       1,
		GameID:            "chess",
		StartedAt:         now,
		EndedAt:           now.Add(10 * time.Minute),
		TerminationReason: "completed",

		Participants: []ParticipantRecord{
			{
				ClientID:    "alice",
				DisplayName: "Alice",
				Role:        "white",
				Seat:        0,
			},
			{
				ClientID:    "bob",
				DisplayName: "Bob",
				Role:        "black",
				Seat:        1,
			},
		},
	}

	// Save the original chess match.
	originalID, err := store.SaveMatch(ctx, record)
	if err != nil {
		t.Fatalf("save original match: %v", err)
	}

	// Attempt to reuse the session for a different game.
	conflicting := record
	conflicting.MatchNumber = 2
	conflicting.GameID = "checkers"

	_, err = store.SaveMatch(ctx, conflicting)
	if err == nil {
		t.Fatal("expected conflicting game ID to be rejected")
	}

	// Verify that only the original match exists.
	var matchCount int

	err = store.db.QueryRow(`
        SELECT COUNT(*) FROM "match"
    `).Scan(&matchCount)

	if err != nil {
		t.Fatalf("count matches: %v", err)
	}

	if matchCount != 1 {
		t.Errorf("expected 1 match, got %d", matchCount)
	}

	// Verify that the original match remains unchanged.
	saved, err := store.GetMatch(ctx, originalID)
	if err != nil {
		t.Fatalf("retrieve original match: %v", err)
	}

	if saved.GameID != "chess" {
		t.Errorf("expected chess, got %s", saved.GameID)
	}
}

func TestSaveMultipleMatchesSameSession(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Date(2026, 11, 7, 17, 0, 0, 0, time.UTC)

	first := MatchRecord{
		SessionID:         "shared-session",
		SessionCreatedAt:  now.Add(-time.Hour),
		MatchNumber:       1,
		GameID:            "chess",
		StartedAt:         now,
		EndedAt:           now.Add(10 * time.Minute),
		TerminationReason: "completed",

		Participants: []ParticipantRecord{
			{
				ClientID:    "alice",
				DisplayName: "Alice",
				Role:        "white",
				Seat:        0,
				IsWinner:    true,
			},
			{
				ClientID:    "bob",
				DisplayName: "Bob",
				Role:        "black",
				Seat:        1,
			},
		},

		Actions: []ActionRecord{
			{
				Seq:        1,
				ClientID:   "alice",
				ReceivedAt: now.Add(time.Minute),
				Payload:    []byte("e2e4"),
				Outcome:    ActionAccepted,
			},
		},
	}

	firstID, err := store.SaveMatch(ctx, first)
	if err != nil {
		t.Fatalf("save first match: %v", err)
	}

	second := first
	second.MatchNumber = 2
	second.StartedAt = now.Add(20 * time.Minute)
	second.EndedAt = now.Add(30 * time.Minute)
	second.Actions = []ActionRecord{
		{
			Seq:        1,
			ClientID:   "bob",
			ReceivedAt: now.Add(21 * time.Minute),
			Payload:    []byte("d7d5"),
			Outcome:    ActionAccepted,
		},
	}

	secondID, err := store.SaveMatch(ctx, second)
	if err != nil {
		t.Fatalf("save second match: %v", err)
	}

	if firstID <= 0 || secondID <= 0 || firstID == secondID {
		t.Fatalf("expected distinct positive match IDs, got %d and %d",
			firstID, secondID)
	}

	checks := []struct {
		name  string
		query string
		want  int
	}{
		{"sessions", "SELECT COUNT(*) FROM session", 1},
		{"clients", "SELECT COUNT(*) FROM client", 2},
		{"matches", `SELECT COUNT(*) FROM "match"`, 2},
		{"participants", "SELECT COUNT(*) FROM match_participant", 4},
		{"actions", "SELECT COUNT(*) FROM action_log", 2},
	}

	for _, check := range checks {
		var got int

		if err := store.db.QueryRow(check.query).Scan(&got); err != nil {
			t.Fatalf("count %s: %v", check.name, err)
		}

		if got != check.want {
			t.Errorf("%s: expected %d, got %d",
				check.name, check.want, got)
		}
	}

	saved, err := store.GetMatch(ctx, secondID)
	if err != nil {
		t.Fatalf("retrieve second match: %v", err)
	}

	if saved.MatchNumber != 2 || saved.SessionID != first.SessionID {
		t.Errorf("incorrect second match metadata: %+v", saved)
	}

	if len(saved.Actions) != 1 ||
		string(saved.Actions[0].Payload) != "d7d5" {
		t.Errorf("incorrect second match action history: %+v",
			saved.Actions)
	}
}

func TestSaveMatchInvalidActionSequence(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 11, 7, 17, 0, 0, 0, time.UTC)

	record := MatchRecord{
		SessionID:         "sequence-test",
		SessionCreatedAt:  now,
		MatchNumber:       1,
		GameID:            "chess",
		StartedAt:         now,
		EndedAt:           now.Add(10 * time.Minute),
		TerminationReason: "completed",

		Participants: []ParticipantRecord{
			{
				ClientID:    "alice",
				DisplayName: "Alice",
				Role:        "white",
				Seat:        0,
			},
		},

		Actions: []ActionRecord{
			{
				Seq:        1,
				ClientID:   "alice",
				ReceivedAt: now.Add(time.Minute),
				Payload:    []byte("e2e4"),
				Outcome:    ActionAccepted,
			},
			{
				Seq:        3, // Deliberately skips sequence 2.
				ClientID:   "alice",
				ReceivedAt: now.Add(2 * time.Minute),
				Payload:    []byte("g1f3"),
				Outcome:    ActionAccepted,
			},
		},
	}

	_, err = store.SaveMatch(context.Background(), record)
	if err == nil {
		t.Fatal("expected invalid action sequence to be rejected")
	}

	var count int
	err = store.db.QueryRow(`
        SELECT COUNT(*) FROM "match"
    `).Scan(&count)

	if err != nil {
		t.Fatalf("count matches: %v", err)
	}

	if count != 0 {
		t.Errorf("expected 0 matches after rollback, got %d", count)
	}
}

func TestForeignKeyEnforcement(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer store.Close()

	// Verify foreign-key enforcement is enabled.
	var enabled int

	err = store.db.QueryRow(`
        PRAGMA foreign_keys
    `).Scan(&enabled)

	if err != nil {
		t.Fatalf("check foreign keys: %v", err)
	}

	if enabled != 1 {
		t.Fatal("foreign-key enforcement is disabled")
	}

	// Attempt to insert a match referencing a nonexistent session.
	_, err = store.db.Exec(`
        INSERT INTO "match" (
            session_id,
            match_number,
            game_id,
            started_at,
            ended_at,
            termination_reason,
            is_draw,
            action_count
        )
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)
    `,
		"nonexistent-session",
		1,
		"chess",
		"2026-11-07T17:00:00Z",
		"2026-11-07T17:10:00Z",
		"completed",
		0,
		0,
	)

	if err == nil {
		t.Fatal("expected foreign-key violation")
	}

	// Verify the invalid match was not inserted.
	var count int

	err = store.db.QueryRow(`
        SELECT COUNT(*) FROM "match"
    `).Scan(&count)

	if err != nil {
		t.Fatalf("count matches: %v", err)
	}

	if count != 0 {
		t.Errorf("expected 0 matches, got %d", count)
	}
}
