package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Options wires the API to the running server.
type Options struct {
	Tokens  *TokenStore
	Control Controller
	Logs    *LogRing
	Metrics *Metrics
}

type api struct {
	opts     Options
	throttle *throttle
}

type authedHandler func(w http.ResponseWriter, r *http.Request, admin string)

// NewHandler returns the admin API. Routes require a valid bearer token.
func NewHandler(o Options) http.Handler {
	a := &api{opts: o, throttle: newThrottle(5, time.Minute, 5*time.Minute)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/whoami", a.auth(a.whoami))
	mux.HandleFunc("GET /v1/sessions", a.auth(a.listSessions))
	mux.HandleFunc("GET /v1/sessions/{id}", a.auth(a.getSession))
	mux.HandleFunc("POST /v1/sessions/{id}/close", a.auth(a.closeSession))
	mux.HandleFunc("GET /v1/connections", a.auth(a.listConnections))
	mux.HandleFunc("POST /v1/connections/{id}/close", a.auth(a.closeConnection))
	mux.HandleFunc("GET /v1/logs", a.auth(a.logs))
	mux.HandleFunc("GET /v1/metrics", a.auth(a.metrics))
	return mux
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// auth authenticates the request, throttles repeated failures, and writes audit lines for each request.
func (a *api) auth(h authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		ip := remoteIP(r)
		if a.throttle.blocked(ip) {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many failed attempts; try again later"})
			return
		}
		name, ok := a.authenticate(r)
		if !ok {
			a.throttle.fail(ip)
			a.opts.Metrics.Inc(AdminAuthFailures)
			log.Printf("admin: authentication failed from %s", ip)
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		a.throttle.success(ip)
		a.opts.Metrics.Inc(AdminRequests)
		r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		h(rec, r, name)
		log.Printf("admin: %s %s %q -> %d", name, r.Method, r.URL.Path, rec.status)
	}
}

// The token is accepted only in the Authorization header
func (a *api) authenticate(r *http.Request) (string, bool) {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) <= len(prefix) || len(h) > 256 || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	return a.opts.Tokens.Authenticate(strings.TrimSpace(h[len(prefix):]))
}

// remoteIP is the peer's address. X-Forwarded-For is deliberately ignored and is controlled by a client.
func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// fail maps a Controller error to a response. Internal details go to the log, not to the caller.
func fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		writeJSON(w, http.StatusGatewayTimeout, map[string]string{"error": "timed out"})
	default:
		log.Printf("admin: internal error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
}

func (a *api) whoami(w http.ResponseWriter, _ *http.Request, admin string) {
	writeJSON(w, http.StatusOK, map[string]string{"admin": admin})
}

func (a *api) listSessions(w http.ResponseWriter, r *http.Request, _ string) {
	v, err := a.opts.Control.Sessions(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	if v == nil {
		v = []SessionInfo{}
	}
	writeJSON(w, http.StatusOK, v)
}

func (a *api) getSession(w http.ResponseWriter, r *http.Request, _ string) {
	v, err := a.opts.Control.Session(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// cleanReason drops control characters, which could forge log lines, and bounds the length.
func cleanReason(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

func (a *api) closeSession(w http.ResponseWriter, r *http.Request, admin string) {
	var body struct {
		Reason string `json:"reason"`
	}
	if r.ContentLength != 0 {
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
	}
	id, reason := r.PathValue("id"), cleanReason(body.Reason)
	if err := a.opts.Control.CloseSession(r.Context(), id, reason); err != nil {
		fail(w, err)
		return
	}
	log.Printf("admin: %s closed session %q (reason: %q)", admin, id, reason)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *api) listConnections(w http.ResponseWriter, r *http.Request, _ string) {
	v, err := a.opts.Control.Connections(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	if v == nil {
		v = []ConnInfo{}
	}
	writeJSON(w, http.StatusOK, v)
}

func (a *api) closeConnection(w http.ResponseWriter, r *http.Request, admin string) {
	id := r.PathValue("id")
	if err := a.opts.Control.CloseConnection(r.Context(), id); err != nil {
		fail(w, err)
		return
	}
	log.Printf("admin: %s closed connection %q", admin, id)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *api) logs(w http.ResponseWriter, r *http.Request, _ string) {
	n := 100
	if s := r.URL.Query().Get("n"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v < 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "n must be a positive integer"})
			return
		}
		n = min(v, 1000)
	}
	contains := r.URL.Query().Get("contains")
	if len(contains) > 100 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "contains is too long"})
		return
	}
	lines := a.opts.Logs.Tail(n, contains)
	if lines == nil {
		lines = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"lines": lines})
}

func (a *api) metrics(w http.ResponseWriter, _ *http.Request, _ string) {
	writeJSON(w, http.StatusOK, map[string]any{
		"uptime_seconds": int64(a.opts.Metrics.Uptime().Seconds()),
		"counters":       a.opts.Metrics.Snapshot(),
		"gauges":         a.opts.Control.Gauges(),
	})
}

// Listen opens the admin listener. A loopback address is always allowed. Anything else is refused
// unless allowNonLoopback is set, which is for containers: inside Docker the process must listen
// on all interfaces and the host's port mapping (127.0.0.1:8081:8081) maintains privacy.
func Listen(addr string, allowNonLoopback bool) (net.Listener, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("admin address %q: %w", addr, err)
	}
	ip := net.ParseIP(host)
	loopback := host == "localhost" || (ip != nil && ip.IsLoopback())
	if !loopback && !allowNonLoopback {
		return nil, fmt.Errorf("admin address %q is not a loopback address; set RAMSERVER_ADMIN_ALLOW_NONLOOPBACK=1 only if the port is published to loopback by the host", addr)
	}
	if !loopback {
		log.Printf("admin: WARNING listening on non-loopback address %s; make sure it is not reachable from the network", addr)
	}
	return net.Listen("tcp", addr)
}

// Serve runs the API on ln until ctx is cancelled.
func Serve(ctx context.Context, ln net.Listener, h http.Handler) error {
	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    8 << 10,
	}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		srv.Shutdown(sctx)
	}()
	log.Printf("admin: API listening on %s", ln.Addr())
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
