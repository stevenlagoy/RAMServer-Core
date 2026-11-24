package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stevenlagoy/ramserver-core/server/internal/admin"
)

type recorded struct {
	method, path, auth, body string
}

// A stand-in for the admin API which records what the client sent
func fakeAPI(t *testing.T, routes map[string]string) (*httptest.Server, *[]recorded) {
	t.Helper()
	var seen []recorded
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := new(bytes.Buffer)
		buf.ReadFrom(r.Body)
		seen = append(seen, recorded{r.Method, r.URL.RequestURI(), r.Header.Get("Authorization"), buf.String()})
		resp, ok := routes[r.Method+" "+r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"not found"}`))
			return
		}
		w.Write([]byte(resp))
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestSessionsTableCleansTerminalEscapes(t *testing.T) {
	sessions, _ := json.Marshal([]admin.SessionInfo{{ID: "session-1", Game: "chess", Status: "waiting", Members: []admin.MemberInfo{{PlayerID: "p1"}}}})
	members, _ := json.Marshal(admin.SessionInfo{ID: "session-1", Members: []admin.MemberInfo{{PlayerID: "p1", DisplayName: "\x1b]0;pwned\x07evil\nname"}}})
	srv, _ := fakeAPI(t, map[string]string{"GET /v1/sessions": string(sessions), "GET /v1/sessions/session-1": string(members)})
	e := env(map[string]string{"RAMSERVER_ADMIN_TOKEN": "ramadm_test", "RAMSERVER_ADMIN_URL": srv.URL})

	var out, errOut bytes.Buffer
	if code := run([]string{"sessions"}, &out, &errOut, e); code != 0 || !strings.Contains(out.String(), "session-1") || !strings.Contains(out.String(), "chess") {
		t.Fatalf("code %d, out %q, err %q", code, out.String(), errOut.String())
	}
	out.Reset()
	if code := run([]string{"session", "session-1"}, &out, &errOut, e); code != 0 {
		t.Fatalf("code %d, err %q", code, errOut.String())
	}
	if strings.ContainsAny(out.String(), "\x1b\x07") || strings.Contains(out.String(), "evil\nname") {
		t.Fatalf("control characters from a player's name reached the terminal: %q", out.String())
	}
}

func TestRequestsCarryTheBearerTokenAndBody(t *testing.T) {
	srv, seen := fakeAPI(t, map[string]string{"POST /v1/sessions/s%2F1/close": `{"ok":true}`, "POST /v1/sessions/s1/close": `{"ok":true}`, "POST /v1/connections/conn-3/close": `{"ok":true}`})
	e := env(map[string]string{"RAMSERVER_ADMIN_TOKEN": "ramadm_test", "RAMSERVER_ADMIN_URL": srv.URL})
	var out, errOut bytes.Buffer
	if code := run([]string{"close-session", "s1", "client", "was", "spamming"}, &out, &errOut, e); code != 0 {
		t.Fatalf("code %d, err %q", code, errOut.String())
	}
	if code := run([]string{"close-conn", "conn-3"}, &out, &errOut, e); code != 0 {
		t.Fatalf("code %d, err %q", code, errOut.String())
	}
	got := *seen
	if len(got) != 2 || got[0].auth != "Bearer ramadm_test" || got[0].method != "POST" || !strings.Contains(got[0].body, "client was spamming") || got[1].path != "/v1/connections/conn-3/close" {
		t.Fatalf("unexpected requests: %+v", got)
	}
}

func TestServerErrorsAreReported(t *testing.T) {
	srv, _ := fakeAPI(t, nil)
	e := env(map[string]string{"RAMSERVER_ADMIN_TOKEN": "ramadm_test", "RAMSERVER_ADMIN_URL": srv.URL})
	var out, errOut bytes.Buffer
	if code := run([]string{"session", "nope"}, &out, &errOut, e); code != 1 || !strings.Contains(errOut.String(), "not found") {
		t.Fatalf("code %d, err %q", code, errOut.String())
	}
}

func TestRefusesToSendTheTokenInClearTextOffLoopback(t *testing.T) {
	srv, seen := fakeAPI(t, map[string]string{"GET /v1/whoami": `{"admin":"alice"}`})
	for _, u := range []string{"http://10.1.2.3:8081", "http://example.com:8081", "ftp://127.0.0.1:8081", "not a url"} {
		var out, errOut bytes.Buffer
		if code := run([]string{"whoami"}, &out, &errOut, env(map[string]string{"RAMSERVER_ADMIN_TOKEN": "ramadm_test", "RAMSERVER_ADMIN_URL": u})); code == 0 {
			t.Errorf("%s was accepted", u)
		}
	}
	if len(*seen) != 0 {
		t.Fatal("a request was sent")
	}
	var out, errOut bytes.Buffer
	if code := run([]string{"whoami"}, &out, &errOut, env(map[string]string{"RAMSERVER_ADMIN_TOKEN": "ramadm_test", "RAMSERVER_ADMIN_URL": srv.URL})); code != 0 || !strings.Contains(out.String(), "alice") {
		t.Fatalf("loopback was refused: %d %q %q", code, out.String(), errOut.String())
	}
}

func TestTokenIsNeverFollowedToARedirect(t *testing.T) {
	var leaked bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked = r.Header.Get("Authorization") != ""
	}))
	defer target.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirector.Close()
	var out, errOut bytes.Buffer
	run([]string{"whoami"}, &out, &errOut, env(map[string]string{"RAMSERVER_ADMIN_TOKEN": "ramadm_test", "RAMSERVER_ADMIN_URL": redirector.URL}))
	if leaked {
		t.Fatal("the token was sent to a redirect target")
	}
}

func TestNoTokenIsAnError(t *testing.T) {
	var out, errOut bytes.Buffer
	missing := filepath.Join(t.TempDir(), "none")
	if code := run([]string{"-token-file", missing, "whoami"}, &out, &errOut, env(nil)); code != 1 || !strings.Contains(errOut.String(), "no token") {
		t.Fatalf("code %d, err %q", code, errOut.String())
	}
}

func TestTokenCommandKeepsTheSecretOffTheScreen(t *testing.T) {
	file := filepath.Join(t.TempDir(), "sub", "admin-token")
	var out, errOut bytes.Buffer
	if code := run([]string{"-token-file", file, "token", "alice"}, &out, &errOut, env(nil)); code != 0 {
		t.Fatalf("code %d, err %q", code, errOut.String())
	}
	secret, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	token := strings.TrimSpace(string(secret))
	if !strings.HasPrefix(token, "ramadm_") {
		t.Fatalf("unexpected token file: %q", secret)
	}
	if strings.Contains(out.String(), token) || strings.Contains(errOut.String(), token) {
		t.Fatal("the token was printed")
	}
	if !strings.Contains(out.String(), "alice "+admin.HashToken(token)) {
		t.Fatalf("the hash line is missing: %q", out.String())
	}
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(file); info.Mode().Perm() != 0o600 {
			t.Fatalf("token file mode = %v, want 0600", info.Mode().Perm())
		}
	}
	// A second run must not replace the existing token
	if code := run([]string{"-token-file", file, "token", "alice"}, &out, &errOut, env(nil)); code == 0 {
		t.Fatal("an existing token file was overwritten")
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(after, secret) {
		t.Fatal("the token file changed")
	}
	for _, bad := range [][]string{{"token"}, {"token", "Alice"}, {"token", "a b"}, {"token", "a", "b"}} {
		if code := run(append([]string{"-token-file", filepath.Join(t.TempDir(), "x")}, bad...), &out, &errOut, env(nil)); code != 2 {
			t.Errorf("%v: code %d, want 2", bad, code)
		}
	}
}

func TestUsageErrors(t *testing.T) {
	srv, _ := fakeAPI(t, nil)
	e := env(map[string]string{"RAMSERVER_ADMIN_TOKEN": "ramadm_test", "RAMSERVER_ADMIN_URL": srv.URL})
	for _, args := range [][]string{{}, {"bogus"}, {"session"}, {"close-session"}, {"close-conn"}, {"logs", "extra"}} {
		var out, errOut bytes.Buffer
		if code := run(args, &out, &errOut, e); code != 2 {
			t.Errorf("%v: code %d, want 2", args, code)
		}
	}
}

func TestCleanTruncatesAndReplaces(t *testing.T) {
	if got := clean("a\x1b[31mb", 20); strings.ContainsRune(got, 0x1b) {
		t.Fatalf("clean = %q", got)
	}
	if got := clean(strings.Repeat("é", 50), 10); len([]rune(got)) != 10 {
		t.Fatalf("clean truncated to %d runes", len([]rune(got)))
	}
}
