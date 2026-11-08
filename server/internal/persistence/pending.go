package persistence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// PendingMatch is a match awaiting successful persistence to SQLite.
// Path identifies the recovery file to remove after a confirmed save.
type PendingMatch struct {
	Path   string
	Record MatchRecord
}

// WritePendingMatch writes a complete recovery record before attempting SQLite.
// The temporary file is synced and renamed only after its contents are written.
// The caller must not delete the returned file until SaveMatch succeeds.
func WritePendingMatch(dir string, record MatchRecord) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("persistence: pending directory is empty")
	}
	if record.SessionID == "" || record.MatchNumber <= 0 {
		return "", fmt.Errorf("persistence: pending match missing identity")
	}

	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return "", fmt.Errorf("persistence: encode pending match: %w", err)
	}
	data = append(data, '\n')

	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("persistence: create pending directory: %w", err)
	}
	// If the pending directory was just created, persist its entry in
	// the parent directory where directory fsync is supported.
	if err := syncPendingDirectory(filepath.Dir(dir)); err != nil {
		return "", fmt.Errorf("persistence: sync pending directory parent: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".match-*.tmp")
	if err != nil {
		return "", fmt.Errorf("persistence: create pending file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "", fmt.Errorf("persistence: write pending match: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return "", fmt.Errorf("persistence: sync pending match: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("persistence: close pending match: %w", err)
	}

	path := strings.TrimSuffix(tmpPath, ".tmp") + ".json"
	if err := os.Rename(tmpPath, path); err != nil {
		return "", fmt.Errorf("persistence: publish pending match: %w", err)
	}
	// Syncing the directory makes the rename durable on filesystems that
	// support it. If this fails, the complete recovery file is retained.
	if err := syncPendingDirectory(dir); err != nil {
		return "", fmt.Errorf("persistence: sync published pending match: %w", err)
	}

	return path, nil
}

// LoadPendingMatches loads the complete recovery queue from disk.
// Corrupt records are reported, not silently ignored or deleted.
func LoadPendingMatches(dir string) ([]PendingMatch, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("persistence: list pending matches: %w", err)
	}

	var pending []PendingMatch
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), ".match-") || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("persistence: read pending match %s: %w", path, err)
		}

		var record MatchRecord
		if err := json.Unmarshal(data, &record); err != nil {
			return nil, fmt.Errorf("persistence: decode pending match %s: %w", path, err)
		}
		if record.SessionID == "" || record.MatchNumber <= 0 {
			return nil, fmt.Errorf("persistence: invalid pending match %s", path)
		}
		pending = append(pending, PendingMatch{Path: path, Record: record})
	}

	// os.ReadDir already returns sorted entries, but retain explicit ordering.
	sort.Slice(pending, func(i, j int) bool { return pending[i].Path < pending[j].Path })
	return pending, nil
}

// RemovePendingMatch is called only after SQLite confirms a successful save.
func RemovePendingMatch(path string) error {
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("persistence: remove pending match %s: %w", path, err)
	}
	if err := syncPendingDirectory(filepath.Dir(path)); err != nil {
		return fmt.Errorf("persistence: sync removed pending match %s: %w", path, err)
	}
	return nil
}

// syncPendingDirectory flushes directory-entry changes to storage where Go
// supports it. Windows does not expose directory fsync through os.File.Sync,
// so we rely there on file.Sync and the same-directory atomic rename.
func syncPendingDirectory(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
