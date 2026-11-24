package admin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeTokenFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "admin-tokens")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestGenerateTokenAuthenticates(t *testing.T) {
	token, hash, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(token, tokenPrefix) || len(hash) != 64 || strings.Contains(hash, token) {
		t.Fatalf("unexpected token or hash: %q %q", token, hash)
	}
	other, _, _ := GenerateToken()
	if token == other {
		t.Fatal("two generated tokens were identical")
	}
	ts, err := NewTokenStore(writeTokenFile(t, "alice "+hash+" # laptop\n"))
	if err != nil {
		t.Fatal(err)
	}
	if name, ok := ts.Authenticate(token); !ok || name != "alice" {
		t.Fatalf("Authenticate(valid) = %q, %v", name, ok)
	}
}

func TestAuthenticateRejects(t *testing.T) {
	token, hash, _ := GenerateToken()
	ts, err := NewTokenStore(writeTokenFile(t, "alice "+hash+"\n"))
	if err != nil {
		t.Fatal(err)
	}
	for name, bad := range map[string]string{
		"empty":           "",
		"wrong":           token + "x",
		"the hash":        hash, // knowing the stored hash must not be enough
		"upper cased":     strings.ToUpper(token),
		"very long":       strings.Repeat("a", 5000),
		"other generated": func() string { o, _, _ := GenerateToken(); return o }(),
	} {
		if who, ok := ts.Authenticate(bad); ok {
			t.Errorf("%s: authenticated as %q", name, who)
		}
	}
}

func TestTokenFileReloadsAndRevokes(t *testing.T) {
	aliceTok, aliceHash, _ := GenerateToken()
	bobTok, bobHash, _ := GenerateToken()
	path := writeTokenFile(t, "alice "+aliceHash+"\n")
	ts, err := NewTokenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ts.Authenticate(bobTok); ok {
		t.Fatal("bob authenticated before being added")
	}
	update := func(content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		future := time.Now().Add(time.Duration(len(content)) * time.Second) // force a visible modification time change
		os.Chtimes(path, future, future)
	}
	update("alice " + aliceHash + "\nbob " + bobHash + "\n")
	if name, ok := ts.Authenticate(bobTok); !ok || name != "bob" {
		t.Fatalf("added admin rejected: %q %v", name, ok)
	}
	update("bob " + bobHash + "\n") // revoke alice without restarting
	if _, ok := ts.Authenticate(aliceTok); ok {
		t.Fatal("revoked admin still authenticates")
	}
	if _, ok := ts.Authenticate(bobTok); !ok {
		t.Fatal("remaining admin was locked out by a revocation")
	}
}

func TestTokenFileFailsClosed(t *testing.T) {
	tok, hash, _ := GenerateToken()
	path := writeTokenFile(t, "alice "+hash+"\n")
	ts, err := NewTokenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, []byte("alice not-a-hash\n"), 0o600)
	future := time.Now().Add(time.Hour)
	os.Chtimes(path, future, future)
	if _, ok := ts.Authenticate(tok); ok {
		t.Fatal("a malformed token file must reject everyone")
	}
	os.Remove(path)
	if _, ok := ts.Authenticate(tok); ok {
		t.Fatal("a deleted token file must reject everyone")
	}
}

func TestNewTokenStoreValidation(t *testing.T) {
	_, hash, _ := GenerateToken()
	bad := map[string]string{
		"missing file":     "",
		"no admins":        "# nothing here\n\n",
		"bad hash length":  "alice abcd\n",
		"bad name":         "Alice " + hash + "\n",
		"name with spaces": "al ice " + hash + "\n",
		"duplicate name":   "alice " + hash + "\nalice " + hash + "\n",
		"extra fields":     "alice " + hash + " extra\n",
	}
	for name, content := range bad {
		path := filepath.Join(t.TempDir(), "tokens")
		if name != "missing file" {
			os.WriteFile(path, []byte(content), 0o600)
		}
		if _, err := NewTokenStore(path); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestValidName(t *testing.T) {
	for name, want := range map[string]bool{"alice": true, "a.b_c-1": true, "": false, "Alice": false, "a b": false, "a\nb": false, strings.Repeat("a", 33): false} {
		if got := ValidName(name); got != want {
			t.Errorf("ValidName(%q) = %v, want %v", name, got, want)
		}
	}
}
