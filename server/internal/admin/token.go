package admin

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

// Tokens are 256 random bits with a recognizable prefix, so secret scanners can find a leaked one.
// The server stores only SHA-256 hashes. A hash of 256 random bits cannot be reversed or
// brute-forced, so a leaked token file does not let anyone authenticate.
const tokenPrefix = "ramadm_"

// GenerateToken returns a new random token and the hash the server stores for it.
func GenerateToken() (token, hash string, err error) {
	var b [32]byte
	if _, err = rand.Read(b[:]); err != nil {
		return "", "", err
	}
	token = tokenPrefix + base64.RawURLEncoding.EncodeToString(b[:])
	return token, HashToken(token), nil
}

// HashToken returns the lowercase hex SHA-256 of token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// ValidName reports whether name may identify an administrator: 1 to 32 characters of a-z, 0-9, '.', '_' or '-'.
// The name is written to the audit log, so it must not contain anything that could forge a log line.
func ValidName(name string) bool {
	if len(name) < 1 || len(name) > 32 {
		return false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

type tokenEntry struct {
	name string
	hash [sha256.Size]byte
}

// TokenStore authenticates bearer tokens against a file of "<name> <sha256-hex>" lines.
// '#' starts a comment. The file is re-read whenever its modification time or size changes, so
// adding or revoking an administrator needs no restart. A file which becomes unreadable or
// malformed locks every administrator out until it is fixed, because failing open would be worse.
type TokenStore struct {
	path string

	mu      sync.Mutex
	entries []tokenEntry
	modTime time.Time
	size    int64
}

// NewTokenStore loads path. It fails if the file is unreadable, malformed, or lists no administrators.
func NewTokenStore(path string) (*TokenStore, error) {
	ts := &TokenStore{path: path}
	entries, info, err := ts.read()
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("%s lists no administrators", path)
	}
	ts.entries, ts.modTime, ts.size = entries, info.ModTime(), info.Size()
	return ts, nil
}

func (ts *TokenStore) read() ([]tokenEntry, os.FileInfo, error) {
	f, err := os.Open(ts.path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}
	entries, err := parseTokens(io.LimitReader(f, 1<<20))
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", ts.path, err)
	}
	return entries, info, nil
}

func parseTokens(r io.Reader) ([]tokenEntry, error) {
	var entries []tokenEntry
	seen := map[string]bool{}
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 {
			return nil, fmt.Errorf("line %d: want \"<name> <sha256-hex>\"", n)
		}
		if !ValidName(fields[0]) {
			return nil, fmt.Errorf("line %d: invalid administrator name", n)
		}
		if seen[fields[0]] {
			return nil, fmt.Errorf("line %d: duplicate administrator %q", n, fields[0])
		}
		raw, err := hex.DecodeString(fields[1])
		if err != nil || len(raw) != sha256.Size {
			return nil, fmt.Errorf("line %d: the hash must be %d hex characters", n, 2*sha256.Size)
		}
		seen[fields[0]] = true
		e := tokenEntry{name: fields[0]}
		copy(e.hash[:], raw)
		entries = append(entries, e)
	}
	return entries, sc.Err()
}

// refresh reloads the file if it changed since the last load.
func (ts *TokenStore) refresh() {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	info, err := os.Stat(ts.path)
	if err == nil && info.ModTime().Equal(ts.modTime) && info.Size() == ts.size {
		return
	}
	entries, info, err := ts.read()
	if err != nil {
		if len(ts.entries) > 0 || !ts.modTime.IsZero() {
			log.Printf("admin: token file unusable, rejecting all administrators: %v", err)
		}
		ts.entries, ts.modTime, ts.size = nil, time.Time{}, 0
		return
	}
	ts.entries, ts.modTime, ts.size = entries, info.ModTime(), info.Size()
	log.Printf("admin: loaded %d administrator(s) from the token file", len(entries))
}

// Authenticate returns the administrator name which owns token.
func (ts *TokenStore) Authenticate(token string) (string, bool) {
	if token == "" || len(token) > 200 {
		return "", false
	}
	ts.refresh()
	sum := sha256.Sum256([]byte(token))
	ts.mu.Lock()
	entries := ts.entries
	ts.mu.Unlock()
	name, match := "", 0
	for _, e := range entries { // visit every entry so the time taken does not depend on which one matched
		if subtle.ConstantTimeCompare(sum[:], e.hash[:]) == 1 {
			name, match = e.name, 1
		}
	}
	return name, match == 1
}
