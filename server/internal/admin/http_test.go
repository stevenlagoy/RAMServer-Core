package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeControl struct {
	mu         sync.Mutex
	calls      []string
	sessionErr error
	closedWith string
}

func (f *fakeControl) record(s string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, s)
}

func (f *fakeControl) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeControl) Sessions(context.Context) ([]SessionInfo, error) {
	f.record("sessions")
	return []SessionInfo{{ID: "session-1", Game: "chess", Status: "waiting", Members: []MemberInfo{{PlayerID: "p1"}}}}, nil
}
func (f *fakeControl) Session(_ context.Context, id string) (SessionInfo, error) {
	f.record("session " + id)
	if id != "session-1" {
		return SessionInfo{}, ErrNotFound
	}
	return SessionInfo{ID: id}, f.sessionErr
}
func (f *fakeControl) CloseSession(_ context.Context, id, reason string) error {
	f.record("close-session " + id)
	f.closedWith = reason
	if id != "session-1" {
		return ErrNotFound
	}
	return nil
}
func (f *fakeControl) Connections(context.Context) ([]ConnInfo, error) {
	f.record("connections")
	return nil, nil
}
func (f *fakeControl) CloseConnection(_ context.Context, id string) error {
	f.record("close-connection " + id)
	return nil
}
func (f *fakeControl) Gauges() map[string]int64 { return map[string]int64{"connections": 2} }

type fixture struct {
	srv     *httptest.Server
	control *fakeControl
	token   string
	logs    *LogRing
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	token, hash, _ := GenerateToken()
	ts, err := NewTokenStore(writeTokenFile(t, "alice "+hash+"\n"))
	if err != nil {
		t.Fatal(err)
	}
	logs := NewLogRing(200)
	log.SetOutput(logs)
	t.Cleanup(func() { log.SetOutput(io.Discard) })
	fc := &fakeControl{}
	srv := httptest.NewServer(NewHandler(Options{Tokens: ts, Control: fc, Logs: logs, Metrics: NewMetrics()}))
	t.Cleanup(srv.Close)
	return &fixture{srv: srv, control: fc, token: token, logs: logs}
}

func (f *fixture) do(t *testing.T, method, path, token string, body io.Reader) (*http.Response, []byte) {
	t.Helper()
	req, _ := http.NewRequest(method, f.srv.URL+path, body)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, b
}

var allRoutes = []struct{ method, path string }{
	{"GET", "/v1/whoami"},
	{"GET", "/v1/sessions"},
	{"GET", "/v1/sessions/session-1"},
	{"POST", "/v1/sessions/session-1/close"},
	{"GET", "/v1/connections"},
	{"POST", "/v1/connections/conn-1/close"},
	{"GET", "/v1/logs"},
	{"GET", "/v1/metrics"},
}

// No route may do anything, or reveal anything, without a valid token.
func TestEveryRouteRequiresAuthentication(t *testing.T) {
	f := newFixture(t)
	headers := map[string]string{
		"no header":         "",
		"wrong scheme":      "Basic " + f.token,
		"token only":        f.token,
		"empty bearer":      "Bearer ",
		"wrong token":       "Bearer ramadm_" + strings.Repeat("A", 43),
		"hash of the token": "Bearer " + HashToken(f.token),
	}
	for _, rt := range allRoutes {
		for name, h := range headers {
			req, _ := http.NewRequest(rt.method, f.srv.URL+rt.path, nil)
			if h != "" {
				req.Header.Set("Authorization", h)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusTooManyRequests {
				t.Fatalf("%s %s with %s: status %d", rt.method, rt.path, name, resp.StatusCode)
			}
			if strings.Contains(string(body), "session-1") {
				t.Fatalf("%s %s leaked data to an unauthenticated caller: %s", rt.method, rt.path, body)
			}
		}
	}
	if n := f.control.callCount(); n != 0 {
		t.Fatalf("controller was called %d times without authentication: %v", n, f.control.calls)
	}
}

func TestTokenInQueryStringIsIgnored(t *testing.T) {
	f := newFixture(t)
	resp, _ := f.do(t, "GET", "/v1/sessions?token="+f.token+"&access_token="+f.token, "", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestAuthenticatedRoutes(t *testing.T) {
	f := newFixture(t)

	resp, body := f.do(t, "GET", "/v1/whoami", f.token, nil)
	if resp.StatusCode != 200 || !strings.Contains(string(body), `"alice"`) {
		t.Fatalf("whoami: %d %s", resp.StatusCode, body)
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Error("responses must not be cacheable")
	}

	resp, body = f.do(t, "GET", "/v1/sessions", f.token, nil)
	var sessions []SessionInfo
	if resp.StatusCode != 200 || json.Unmarshal(body, &sessions) != nil || len(sessions) != 1 || sessions[0].ID != "session-1" {
		t.Fatalf("sessions: %d %s", resp.StatusCode, body)
	}

	resp, body = f.do(t, "GET", "/v1/connections", f.token, nil)
	if resp.StatusCode != 200 || strings.TrimSpace(string(body)) != "[]" {
		t.Fatalf("an empty connection list must be [], got %d %s", resp.StatusCode, body)
	}

	resp, _ = f.do(t, "GET", "/v1/sessions/missing", f.token, nil)
	if resp.StatusCode != 404 {
		t.Fatalf("missing session: %d", resp.StatusCode)
	}

	resp, body = f.do(t, "GET", "/v1/metrics", f.token, nil)
	var m struct {
		Counters map[string]int64 `json:"counters"`
		Gauges   map[string]int64 `json:"gauges"`
	}
	if resp.StatusCode != 200 || json.Unmarshal(body, &m) != nil || m.Gauges["connections"] != 2 || m.Counters["admin_requests"] < 1 {
		t.Fatalf("metrics: %d %s", resp.StatusCode, body)
	}
}

func TestMutationsNeedPOST(t *testing.T) {
	f := newFixture(t)
	for _, path := range []string{"/v1/sessions/session-1/close", "/v1/connections/conn-1/close"} {
		for _, method := range []string{"GET", "PUT", "DELETE"} {
			resp, _ := f.do(t, method, path, f.token, nil)
			if resp.StatusCode != http.StatusMethodNotAllowed {
				t.Errorf("%s %s = %d, want 405", method, path, resp.StatusCode)
			}
		}
	}
	if n := f.control.callCount(); n != 0 {
		t.Fatalf("a wrong method reached the controller: %v", f.control.calls)
	}
}

func TestCloseSessionRecordsSanitizedReason(t *testing.T) {
	f := newFixture(t)
	reason := "stuck\nadmin: mallory closed session \"x\"" + strings.Repeat("!", 500)
	payload, _ := json.Marshal(map[string]string{"reason": reason})
	resp, body := f.do(t, "POST", "/v1/sessions/session-1/close", f.token, bytes.NewReader(payload))
	if resp.StatusCode != 200 {
		t.Fatalf("close: %d %s", resp.StatusCode, body)
	}
	if strings.ContainsAny(f.control.closedWith, "\n\r") || len(f.control.closedWith) > 200 {
		t.Fatalf("reason was not sanitized: %q", f.control.closedWith)
	}
	for _, line := range f.logs.Tail(50, "") {
		if strings.HasPrefix(line, "admin: mallory") {
			t.Fatalf("a reason forged a log line: %q", line)
		}
	}
	resp, _ = f.do(t, "POST", "/v1/sessions/missing/close", f.token, nil)
	if resp.StatusCode != 404 {
		t.Fatalf("closing a missing session: %d", resp.StatusCode)
	}
	resp, _ = f.do(t, "POST", "/v1/sessions/session-1/close", f.token, strings.NewReader(`{"reason":"x","extra":1}`))
	if resp.StatusCode != 400 {
		t.Fatalf("unknown body fields: %d", resp.StatusCode)
	}
	resp, _ = f.do(t, "POST", "/v1/sessions/session-1/close", f.token, strings.NewReader(strings.Repeat("a", 10000)))
	if resp.StatusCode != 400 {
		t.Fatalf("an oversized body: %d", resp.StatusCode)
	}
}

func TestCloseConnection(t *testing.T) {
	f := newFixture(t)
	resp, _ := f.do(t, "POST", "/v1/connections/conn-7/close", f.token, nil)
	if resp.StatusCode != 200 || f.control.calls[len(f.control.calls)-1] != "close-connection conn-7" {
		t.Fatalf("close connection: %d %v", resp.StatusCode, f.control.calls)
	}
}

func TestAuditLogNamesAdminAndNeverLogsTheToken(t *testing.T) {
	f := newFixture(t)
	f.do(t, "GET", "/v1/sessions?x=1", f.token, nil)
	f.do(t, "GET", "/v1/sessions", "ramadm_wrongwrongwrongwrongwrongwrongwrongwrong", nil)
	f.do(t, "POST", "/v1/connections/conn-1/close", f.token, nil)
	all := strings.Join(f.logs.Tail(100, ""), "\n")
	if !strings.Contains(all, `admin: alice GET "/v1/sessions" -> 200`) || !strings.Contains(all, "closed connection") {
		t.Fatalf("audit lines missing:\n%s", all)
	}
	if !strings.Contains(all, "authentication failed") {
		t.Fatalf("failed authentication was not logged:\n%s", all)
	}
	if strings.Contains(all, f.token) || strings.Contains(all, HashToken(f.token)) || strings.Contains(all, "wrongwrong") {
		t.Fatalf("a credential reached the log:\n%s", all)
	}
	if strings.Contains(all, "x=1") {
		t.Fatalf("the query string must not be logged:\n%s", all)
	}
}

func TestRepeatedFailuresLockOutTheCaller(t *testing.T) {
	f := newFixture(t)
	for i := 0; i < 5; i++ {
		resp, _ := f.do(t, "GET", "/v1/sessions", "ramadm_bad", nil)
		if resp.StatusCode != 401 {
			t.Fatalf("attempt %d: %d", i, resp.StatusCode)
		}
	}
	// Even the right token is refused while the lockout lasts
	resp, _ := f.do(t, "GET", "/v1/sessions", f.token, nil)
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", resp.StatusCode)
	}
}

func TestThrottleWindowAndExpiry(t *testing.T) {
	now := time.Now()
	th := newThrottle(3, time.Minute, 5*time.Minute)
	th.now = func() time.Time { return now }
	th.fail("a")
	th.fail("a")
	now = now.Add(2 * time.Minute) // the window passed, so the count restarts
	th.fail("a")
	if th.blocked("a") {
		t.Fatal("failures outside the window must not accumulate")
	}
	th.fail("a")
	th.fail("a")
	if !th.blocked("a") || th.blocked("b") {
		t.Fatal("only the offending key should be blocked")
	}
	now = now.Add(6 * time.Minute)
	if th.blocked("a") {
		t.Fatal("the lockout should expire")
	}
}

func TestLogsEndpointBounds(t *testing.T) {
	f := newFixture(t)
	for i := 0; i < 5; i++ {
		log.Printf("player line %d", i)
	}
	resp, body := f.do(t, "GET", "/v1/logs?n=2&contains=player", f.token, nil)
	var out struct{ Lines []string }
	if resp.StatusCode != 200 || json.Unmarshal(body, &out) != nil || len(out.Lines) != 2 || !strings.HasSuffix(out.Lines[1], "player line 4") {
		t.Fatalf("logs: %d %s", resp.StatusCode, body)
	}
	for _, q := range []string{"n=0", "n=abc", "n=-3", "contains=" + strings.Repeat("a", 101)} {
		if resp, _ := f.do(t, "GET", "/v1/logs?"+q, f.token, nil); resp.StatusCode != 400 {
			t.Errorf("?%s: status %d, want 400", q[:min(len(q), 12)], resp.StatusCode)
		}
	}
}

func TestControllerFailuresDoNotLeakDetails(t *testing.T) {
	f := newFixture(t)
	f.control.sessionErr = errors.New("secret internal path /var/lib/x")
	resp, body := f.do(t, "GET", "/v1/sessions/session-1", f.token, nil)
	if resp.StatusCode != 500 || strings.Contains(string(body), "secret") {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
}

func TestListenRefusesNonLoopbackByDefault(t *testing.T) {
	for _, addr := range []string{":0", "0.0.0.0:0", "10.1.2.3:0", "example.com:0", "noport"} {
		if ln, err := Listen(addr, false); err == nil {
			ln.Close()
			t.Errorf("Listen(%q) succeeded", addr)
		}
	}
	for _, addr := range []string{"127.0.0.1:0", "localhost:0", "[::1]:0"} {
		ln, err := Listen(addr, false)
		if err != nil {
			t.Logf("skipping %s: %v", addr, err) // IPv6 may be unavailable on the host
			continue
		}
		ln.Close()
	}
	ln, err := Listen("0.0.0.0:0", true)
	if err != nil {
		t.Fatalf("explicit opt-in should allow it: %v", err)
	}
	ln.Close()
}

func TestServeStopsOnContextCancel(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, ln, http.NotFoundHandler()) }()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after cancellation")
	}
}
