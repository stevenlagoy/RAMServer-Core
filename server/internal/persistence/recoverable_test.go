package persistence

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// failingSaveStore simulates a database that cannot accept writes.
// All other Store methods delegate to the real SQLite store.
type failingSaveStore struct {
	Store
}

func (s failingSaveStore) SaveMatch(context.Context, MatchRecord) (int64, error) {
	return 0, errors.New("simulated database failure")
}

func TestRecoverPendingAfterFailedSave(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	pendingDir := filepath.Join(dir, "pending")

	sqliteStore, err := Open(filepath.Join(dir, "matches.sqlite"))
	if err != nil {
		t.Fatalf("open SQLite: %v", err)
	}
	defer sqliteStore.Close()

	now := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)
	version := int64(2)
	record := MatchRecord{
		SessionID:         "recovery-session",
		SessionCreatedAt:  now.Add(-time.Hour),
		MatchNumber:       1,
		GameID:            "chess",
		StartedAt:         now,
		EndedAt:           now.Add(time.Minute),
		TerminationReason: "CHECKMATE",
		Participants: []ParticipantRecord{
			{ClientID: "alice", DisplayName: "Alice", Role: "player", Seat: 0, ActionsAccepted: 1},
			{ClientID: "bob", DisplayName: "Bob", Role: "player", Seat: 1, IsWinner: true},
		},
		Actions: []ActionRecord{
			{Seq: 1, ClientID: "alice", ReceivedAt: now.Add(time.Second),
				Payload: []byte("f2f3"), Outcome: ActionAccepted, StateVersion: &version},
		},
	}

	// The save fails, but the pending record must remain on disk.
	failedStore := NewRecoverableStore(failingSaveStore{Store: sqliteStore}, pendingDir)
	if _, err := failedStore.SaveMatch(ctx, record); err == nil {
		t.Fatal("expected simulated save failure")
	}

	pending, err := LoadPendingMatches(pendingDir)
	if err != nil {
		t.Fatalf("load pending matches: %v", err)
	}
	if len(pending) != 1 || !reflect.DeepEqual(pending[0].Record, record) {
		t.Fatalf("expected one intact pending record, got %+v", pending)
	}

	var count int
	if err := sqliteStore.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM "match"`).Scan(&count); err != nil {
		t.Fatalf("count matches before recovery: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no saved matches before recovery, got %d", count)
	}

	// Simulate the next startup with a working database.
	recoveredStore := NewRecoverableStore(sqliteStore, pendingDir)
	if err := recoveredStore.RecoverPending(ctx); err != nil {
		t.Fatalf("recover pending: %v", err)
	}

	pending, err = LoadPendingMatches(pendingDir)
	if err != nil {
		t.Fatalf("reload pending matches: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected pending records to be removed, got %d", len(pending))
	}

	var matchID int64
	if err := sqliteStore.db.QueryRowContext(ctx,
		`SELECT match_id FROM "match" WHERE session_id = ? AND match_number = ?`,
		record.SessionID, record.MatchNumber,
	).Scan(&matchID); err != nil {
		t.Fatalf("find recovered match: %v", err)
	}

	saved, err := sqliteStore.GetMatch(ctx, matchID)
	if err != nil {
		t.Fatalf("get recovered match: %v", err)
	}
	record.ID = matchID
	if !reflect.DeepEqual(saved, record) {
		t.Errorf("recovered match differs from original\nwant: %+v\ngot:  %+v", record, saved)
	}

	if err := sqliteStore.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM "match"`).Scan(&count); err != nil {
		t.Fatalf("count recovered matches: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly one recovered match, got %d", count)
	}
}
