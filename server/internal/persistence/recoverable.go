package persistence

import (
	"context"
	"errors"
	"fmt"
	"log"
)

// RecoverableStore adds disk-backed recovery and retries
// to an existing persistence store.
type RecoverableStore struct {
	Store
	pendingDir string
}

func NewRecoverableStore(store Store, pendingDir string) *RecoverableStore {
	return &RecoverableStore{
		Store:      store,
		pendingDir: pendingDir,
	}
}

func (s *RecoverableStore) SaveMatch(
	ctx context.Context,
	record MatchRecord,
) (int64, error) {
	// Preserve the record before attempting SQLite.
	path, err := WritePendingMatch(s.pendingDir, record)
	if err != nil {
		return 0, fmt.Errorf("cannot create match recovery record: %w", err)
	}

	id, err := SaveMatchWithRetry(ctx, s.Store, record)
	if err != nil {
		// Leave the recovery file in place.
		return 0, fmt.Errorf("match remains pending at %s: %w", path, err)
	}

	// SQLite confirmed the save. The recovery file is no longer needed.
	if err := RemovePendingMatch(path); err != nil {
		log.Printf("match %d saved, but recovery cleanup failed: %v", id, err)
	}

	return id, nil
}

// RecoverPending replays records left by earlier failed or interrupted saves.
func (s *RecoverableStore) RecoverPending(ctx context.Context) error {
	pending, err := LoadPendingMatches(s.pendingDir)
	if err != nil {
		return err
	}

	var failures []error

	for _, item := range pending {
		id, err := SaveMatchWithRetry(ctx, s.Store, item.Record)
		if err != nil {
			failures = append(failures,
				fmt.Errorf("recover %s: %w", item.Path, err))
			continue
		}

		if err := RemovePendingMatch(item.Path); err != nil {
			failures = append(failures,
				fmt.Errorf("cleanup %s: %w", item.Path, err))
			continue
		}

		log.Printf("recovered match %d from %s", id, item.Path)
	}

	return errors.Join(failures...)
}
