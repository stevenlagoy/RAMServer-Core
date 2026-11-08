package persistence

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed migrations/001_init.sql
var initialSchema string

type SQLiteStore struct {
	db *sql.DB
}

func Open(path string) (*SQLiteStore, error) {
	if path == "" {
		return nil, fmt.Errorf("persistence: database path is empty")
	}

	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, fmt.Errorf("persistence: create database directory: %w", err)
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("persistence: open database: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	store := &SQLiteStore{db: db}

	if err := store.initialize(context.Background()); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

func (s *SQLiteStore) initialize(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("persistence: connect: %w", err)
	}

	for _, pragma := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
	} {
		if _, err := s.db.ExecContext(ctx, pragma); err != nil {
			return fmt.Errorf("persistence: configure SQLite: %w", err)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("persistence: begin migration: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
        CREATE TABLE IF NOT EXISTS schema_migration (
            version INTEGER PRIMARY KEY,
            applied_at TEXT NOT NULL
        )
    `); err != nil {
		return fmt.Errorf("persistence: migration tracking: %w", err)
	}

	var version int
	err = tx.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(version), 0) FROM schema_migration",
	).Scan(&version)
	if err != nil {
		return fmt.Errorf("persistence: inspect schema version: %w", err)
	}

	switch version {
	case 0:
		for _, statement := range strings.Split(initialSchema, ";") {
			if strings.TrimSpace(statement) == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("persistence: create initial schema: %w", err)
			}
		}

		_, err := tx.ExecContext(ctx, `
            INSERT INTO schema_migration (version, applied_at)
            VALUES (1, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
        `)
		if err != nil {
			return fmt.Errorf("persistence: record schema version: %w", err)
		}

	case 1:
		// Initial schema already installed.

	default:
		return fmt.Errorf("persistence: unsupported schema version %d", version)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("persistence: commit migration: %w", err)
	}

	return nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
