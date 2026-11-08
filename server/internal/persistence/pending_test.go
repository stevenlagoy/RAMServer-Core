package persistence

import (
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestPendingMatchRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pending")
	now := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)
	version := int64(7)
	record := MatchRecord{
		SessionID:         "session-1",
		SessionCreatedAt:  now.Add(-time.Hour),
		MatchNumber:       1,
		GameID:            "chess",
		StartedAt:         now,
		EndedAt:           now.Add(5 * time.Minute),
		TerminationReason: "CHECKMATE",
		Seed:              []byte{0, 1, 2, 255},
		Participants: []ParticipantRecord{{
			ClientID: "alice", DisplayName: "Alice", Role: "player", Seat: 0,
		}},
		Actions: []ActionRecord{{
			Seq: 1, ClientID: "alice", ReceivedAt: now,
			Payload: []byte{0, 0xff, 'a'}, Outcome: ActionAccepted,
			StateVersion: &version,
		}},
	}

	path, err := WritePendingMatch(dir, record)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("pending file not written: %v", err)
	}

	matches, err := LoadPendingMatches(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || !reflect.DeepEqual(matches[0].Record, record) {
		t.Fatalf("pending roundtrip mismatch: %+v", matches)
	}

	if err := RemovePendingMatch(path); err != nil {
		t.Fatal(err)
	}
	matches, err = LoadPendingMatches(dir)
	if err != nil || len(matches) != 0 {
		t.Fatalf("expected empty pending queue, got %+v, %v", matches, err)
	}
}

// Each writer must publish a separate complete journal file, even when
// multiple callers are saving the very same match concurrently.
func TestConcurrentPendingWrites(t *testing.T) {
	const writers = 32
	dir := filepath.Join(t.TempDir(), "pending")
	record := MatchRecord{
		SessionID:         "shared-session",
		MatchNumber:       1,
		GameID:            "chess",
		StartedAt:         time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC),
		EndedAt:           time.Date(2026, 11, 8, 0, 1, 0, 0, time.UTC),
		TerminationReason: "CHECKMATE",
	}

	paths := make(chan string, writers)
	errs := make(chan error, writers)
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			path, err := WritePendingMatch(dir, record)
			if err != nil {
				errs <- err
				return
			}
			paths <- path
		}()
	}
	wg.Wait()
	close(paths)
	close(errs)
	for err := range errs {
		t.Errorf("concurrent pending write failed: %v", err)
	}

	seen := make(map[string]bool)
	for path := range paths {
		if seen[path] {
			t.Errorf("two concurrent writes used the same recovery path: %s", path)
		}
		seen[path] = true
	}
	if len(seen) != writers {
		t.Fatalf("expected %d distinct files, got %d", writers, len(seen))
	}

	pending, err := LoadPendingMatches(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != writers {
		t.Fatalf("expected %d intact records, got %d", writers, len(pending))
	}
	for _, item := range pending {
		if !seen[item.Path] || !reflect.DeepEqual(item.Record, record) {
			t.Fatalf("unexpected pending record: %+v", item)
		}
	}
}

func TestPendingMatchInterruptedWrite(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pending")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}

	// An interrupted write leaves a .tmp file. The recovery loader must
	// ignore it instead of misinterpreting it as a saved match.
	partial := filepath.Join(dir, ".match-interrupted.tmp")
	if err := os.WriteFile(partial, []byte(`{"SessionID":"session-1",`), 0600); err != nil {
		t.Fatal(err)
	}
	pending, err := LoadPendingMatches(dir)
	if err != nil || len(pending) != 0 {
		t.Fatalf("temporary file was loaded: %+v, err=%v", pending, err)
	}

	// A damaged *published* recovery file must be reported, never silently
	// skipped or deleted.
	corrupt := filepath.Join(dir, ".match-corrupt.json")
	if err := os.WriteFile(corrupt, []byte(`{"SessionID":"session-1",`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPendingMatches(dir); err == nil {
		t.Fatal("expected a decoding error for corrupt published recovery file")
	}
	if _, err := os.Stat(corrupt); err != nil {
		t.Fatalf("corrupt recovery file was lost: %v", err)
	}
}
