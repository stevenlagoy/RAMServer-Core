package persistence

import (
	"path/filepath"
	"testing"
)

func TestOpenInitializesSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.sqlite")

	store, err := Open(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer store.Close()

	tables := []string{
		"client",
		"session",
		"match",
		"match_participant",
		"action_log",
		"schema_migration",
	}

	for _, table := range tables {
		var count int

		err := store.db.QueryRow(`
            SELECT COUNT(*)
            FROM sqlite_master
            WHERE type = 'table' AND name = ?
        `, table).Scan(&count)

		if err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}

		if count != 1 {
			t.Errorf("expected table %s to exist", table)
		}
	}

	_, err = store.db.Exec(`
        INSERT INTO client (client_id, display_name, created_at)
        VALUES (?, ?, ?)
    `, "test-client", "Alice", "2026-11-07T12:00:00Z")

	if err != nil {
		t.Fatalf("insert client: %v", err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}
	defer reopened.Close()

	var name string
	err = reopened.db.QueryRow(`
        SELECT display_name
        FROM client
        WHERE client_id = ?
    `, "test-client").Scan(&name)

	if err != nil {
		t.Fatalf("retrieve client: %v", err)
	}

	if name != "Alice" {
		t.Errorf("expected Alice, got %q", name)
	}

	var migrations int

	err = reopened.db.QueryRow(`
        SELECT COUNT(*) FROM schema_migration
    `).Scan(&migrations)

	if err != nil {
		t.Fatalf("count migrations: %v", err)
	}

	if migrations != 1 {
		t.Errorf("expected 1 migration, got %d", migrations)
	}

}
