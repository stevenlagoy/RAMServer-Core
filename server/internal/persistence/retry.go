package persistence

import (
	"context"
	"errors"
	"fmt"
	"time"

	"modernc.org/sqlite"
)

func isRetryableSaveError(err error) bool {
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}

	// SQLITE_BUSY = 5, SQLITE_LOCKED = 6.
	// Mask off any extended result code.
	switch sqliteErr.Code() & 0xff {
	case 5, 6:
		return true
	default:
		return false
	}
}

func SaveMatchWithRetry(
	ctx context.Context,
	store Store,
	record MatchRecord,
) (int64, error) {
	delays := []time.Duration{
		100 * time.Millisecond,
		250 * time.Millisecond,
		500 * time.Millisecond,
	}

	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return 0, err
		}

		id, err := store.SaveMatch(ctx, record)
		if err == nil {
			return id, nil
		}

		if !isRetryableSaveError(err) || attempt == len(delays) {
			return 0, fmt.Errorf(
				"persistence: save failed after %d attempt(s): %w",
				attempt+1,
				err,
			)
		}

		timer := time.NewTimer(delays[attempt])

		select {
		case <-ctx.Done():
			timer.Stop()
			return 0, ctx.Err()
		case <-timer.C:
		}
	}
}
