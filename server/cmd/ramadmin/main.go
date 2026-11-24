package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
	"unicode"

	"github.com/stevenlagoy/ramserver-core/server/internal/admin"
)

const usage = `usage: ramadmin [-url URL] [-token-file FILE] [-json] <command> [arguments]

commands:
  token <name>                  create your token and print the line an administrator adds to the server's token file
  whoami                        check your token and show the name it belongs to
  sessions                      list sessions
  session <id>                  show one session and its members
  close-session <id> [reason]   end a session and notify its members
  connections                   list connections
  close-conn <id>               disconnect a connection and free its seat
  logs [-n N] [-grep TEXT]      show recent server log lines
  metrics                       show counters and current levels

The token is read from $RAMSERVER_ADMIN_TOKEN, or else from the token file
(default: ramserver/admin-token in your user config directory).
The URL defaults to $RAMSERVER_ADMIN_URL, or else http://127.0.0.1:8081.`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, os.Getenv))
}

func run(args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	fs := flag.NewFlagSet("ramadmin", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprintln(stderr, usage) }
	baseURL := fs.String("url", firstNonEmpty(getenv("RAMSERVER_ADMIN_URL"), "http://127.0.0.1:8081"), "admin API address")
	tokenFile := fs.String("token-file", "", "file holding your token")
	asJSON := fs.Bool("json", false, "print the server's JSON response unchanged")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fs.Usage()
		return 2
	}
	c := &cli{out: stdout, asJSON: *asJSON}
	if rest[0] == "token" {
		return c.report(stderr, c.createToken(rest[1:], *tokenFile, getenv))
	}
	if err := c.connect(*baseURL, *tokenFile, getenv); err != nil {
		fmt.Fprintln(stderr, "ramadmin:", err)
		return 1
	}
	return c.report(stderr, c.dispatch(rest))
}

func (c *cli) report(stderr io.Writer, err error) int {
	if err == nil {
		return 0
	}
	var u usageError
	if errors.As(err, &u) {
		fmt.Fprintln(stderr, "ramadmin:", err)
		return 2
	}
	fmt.Fprintln(stderr, "ramadmin:", err)
	return 1
}

type usageError string

func (u usageError) Error() string { return string(u) }

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

type cli struct {
	out    io.Writer
	asJSON bool
	base   *url.URL
	token  string
	http   *http.Client
}

func defaultTokenFile() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ramserver", "admin-token"), nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Validates the address and loads the token
func (c *cli) connect(baseURL, tokenFile string, getenv func(string) string) error {
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return fmt.Errorf("invalid URL %q", baseURL)
	}
	if u.Scheme == "http" && !isLoopback(u.Hostname()) {
		return fmt.Errorf("refusing to send a token over plain http to %s; use the SSH tunnel and a loopback address", u.Host)
	}
	token := strings.TrimSpace(getenv("RAMSERVER_ADMIN_TOKEN"))
	if token == "" {
		if tokenFile == "" {
			resolved, err := defaultTokenFile()
			if err != nil {
				return err
			}
			tokenFile = resolved
		}
		b, err := os.ReadFile(tokenFile)
		if err != nil {
			return fmt.Errorf("no token: set RAMSERVER_ADMIN_TOKEN or create %s with \"ramadmin token <name>\" (%v)", tokenFile, err)
		}
		token = strings.TrimSpace(string(b))
	}
	if token == "" {
		return errors.New("the token is empty")
	}
	c.base, c.token = u, token
	c.http = &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse // don't forward the token to a redirect target
		},
	}
	return nil
}

// Generates a token on the new administrator's machine in a private file
func (c *cli) createToken(args []string, tokenFile string, getenv func(string) string) error {
	if len(args) != 1 || !admin.ValidName(args[0]) {
		return usageError("usage: ramadmin token <name>   (name: 1-32 characters of a-z, 0-9, '.', '_', '-')")
	}
	if tokenFile == "" {
		var err error
		if tokenFile, err = defaultTokenFile(); err != nil {
			return err
		}
	}
	token, hash, err := admin.GenerateToken()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(tokenFile), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(tokenFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // don't overwrite existing tokens
	if err != nil {
		return fmt.Errorf("%v (delete the file first if you mean to replace your token)", err)
	}
	if _, err := f.WriteString(token + "\n"); err != nil {
		f.Close()
		os.Remove(tokenFile)
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	fmt.Fprintf(c.out, "Your token was saved to %s (keep it private; it is not shown here).\n", tokenFile)
	fmt.Fprintln(c.out, "Send this line alone to an administrator who maintains the server's token file:")
	fmt.Fprintf(c.out, "\n%s %s\n", args[0], hash)
	return nil
}

func (c *cli) dispatch(args []string) error {
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "whoami":
		return c.simple("GET", "/v1/whoami", nil, func(v map[string]any) string {
			return fmt.Sprintf("authenticated as %s", clean(fmt.Sprint(v["admin"]), 64))
		})
	case "sessions":
		return c.sessions()
	case "session":
		if len(rest) != 1 {
			return usageError("usage: ramadmin session <id>")
		}
		return c.session(rest[0])
	case "close-session":
		if len(rest) < 1 {
			return usageError("usage: ramadmin close-session <id> [reason]")
		}
		body, _ := json.Marshal(map[string]string{"reason": strings.Join(rest[1:], " ")})
		return c.simple("POST", "/v1/sessions/"+url.PathEscape(rest[0])+"/close", body, func(map[string]any) string {
			return "closed session " + clean(rest[0], 80)
		})
	case "connections":
		if len(rest) != 0 {
			return usageError("usage: ramadmin connections")
		}
		return c.connections()
	case "close-conn":
		if len(rest) != 1 {
			return usageError("usage: ramadmin close-conn <id>")
		}
		return c.simple("POST", "/v1/connections/"+url.PathEscape(rest[0])+"/close", nil, func(map[string]any) string {
			return "closed connection " + clean(rest[0], 80)
		})
	case "logs":
		return c.logs(rest)
	case "metrics":
		return c.metrics()
	default:
		return usageError(fmt.Sprintf("unknown command %q; run ramadmin with no arguments for the list", cmd))
	}
}

// Send one request and return the body
func (c *cli) call(method, path string, body []byte) ([]byte, error) {
	u := *c.base
	requestPath, rawQuery, _ := strings.Cut(path, "?")
	u.Path = strings.TrimRight(c.base.Path, "/") + requestPath
	u.RawQuery = rawQuery
	req, err := http.NewRequest(method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach the admin API (is the tunnel open?): %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		var e struct{ Error string }
		json.Unmarshal(data, &e)
		return nil, fmt.Errorf("%s (HTTP %d)", clean(firstNonEmpty(e.Error, http.StatusText(resp.StatusCode)), 120), resp.StatusCode)
	}
	return data, nil
}

func (c *cli) raw(data []byte) { c.out.Write(data) }

func (c *cli) simple(method, path string, body []byte, msg func(map[string]any) string) error {
	data, err := c.call(method, path, body)
	if err != nil {
		return err
	}
	if c.asJSON {
		c.raw(data)
		return nil
	}
	var v map[string]any
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	fmt.Fprintln(c.out, msg(v))
	return nil
}

// Outputtable to terminal, handling control characters and escape sequences
func clean(s string, max int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
			return '?'
		}
		return r
	}, s)
	if r := []rune(s); len(r) > max {
		return string(r[:max-1]) + "…"
	}
	return s
}

func age(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return time.Since(t).Round(time.Second).String()
}

func (c *cli) sessions() error {
	data, err := c.call("GET", "/v1/sessions", nil)
	if err != nil {
		return err
	}
	if c.asJSON {
		c.raw(data)
		return nil
	}
	var list []admin.SessionInfo
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	tw := tabwriter.NewWriter(c.out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tGAME\tSTATUS\tPLAYERS\tMATCHES\tAGE\tIDLE")
	for _, s := range list {
		fmt.Fprintf(tw,
			"%s\t%s\t%s\t%d\t%d\t%s\t%s\n",
			clean(s.ID, 40), clean(s.Game, 20), clean(s.Status, 16), len(s.Members), s.MatchesPlayed, age(s.Created), age(s.LastActivity),
		)
	}
	return tw.Flush()
}

func (c *cli) session(id string) error {
	data, err := c.call("GET", "/v1/sessions/"+url.PathEscape(id), nil)
	if err != nil {
		return err
	}
	if c.asJSON {
		c.raw(data)
		return nil
	}
	var s admin.SessionInfo
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	fmt.Fprintf(c.out,
		"session %s\n  game: %s   status: %s   public: %v\n  created: %s ago   idle: %s   matches played: %d   turns this match: %d\n",
		clean(s.ID, 60), clean(s.Game, 20), clean(s.Status, 16), s.Public, age(s.Created), age(s.LastActivity), s.MatchesPlayed, s.Turns,
	)
	tw := tabwriter.NewWriter(c.out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "PLAYER\tNAME\tROLE\tREADY\tACCEPTED\tREJECTED")
	for _, m := range s.Members {
		fmt.Fprintf(tw,
			"%s\t%s\t%s\t%v\t%d\t%d\n",
			clean(m.PlayerID, 40), clean(m.DisplayName, 30), clean(m.Role, 16), m.Ready, m.ActionsAccepted, m.ActionsRejected,
		)
	}
	return tw.Flush()
}

func (c *cli) connections() error {
	data, err := c.call("GET", "/v1/connections", nil)
	if err != nil {
		return err
	}
	if c.asJSON {
		c.raw(data)
		return nil
	}
	var list []admin.ConnInfo
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	tw := tabwriter.NewWriter(c.out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tREMOTE\tNAME\tPLAYER\tSESSION\tMSGS\tAGE")
	for _, n := range list {
		fmt.Fprintf(tw,
			"%s\t%s\t%s\t%s\t%s\t%d\t%s\n", clean(n.ID, 20), clean(n.Remote, 45), clean(firstNonEmpty(n.DisplayName, "-"), 30),
			clean(firstNonEmpty(n.PlayerID, "(unauthenticated)"), 40), clean(firstNonEmpty(n.SessionID, "-"), 40), n.MessagesReceived, age(n.Connected),
		)
	}
	return tw.Flush()
}

func (c *cli) logs(args []string) error {
	fs := flag.NewFlagSet("logs", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	n := fs.Int("n", 100, "number of lines")
	grep := fs.String("grep", "", "only lines containing this text")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return usageError("usage: ramadmin logs [-n N] [-grep TEXT]")
	}
	q := url.Values{"n": {fmt.Sprint(*n)}}
	if *grep != "" {
		q.Set("contains", *grep)
	}
	data, err := c.call("GET", "/v1/logs?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	if c.asJSON {
		c.raw(data)
		return nil
	}
	var v struct{ Lines []string }
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	for _, line := range v.Lines {
		fmt.Fprintln(c.out, clean(line, 1100))
	}
	return nil
}

func (c *cli) metrics() error {
	data, err := c.call("GET", "/v1/metrics", nil)
	if err != nil {
		return err
	}
	if c.asJSON {
		c.raw(data)
		return nil
	}
	var v struct {
		Uptime   int64            `json:"uptime_seconds"`
		Counters map[string]int64 `json:"counters"`
		Gauges   map[string]int64 `json:"gauges"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	fmt.Fprintf(c.out, "uptime: %s\n\ncurrent:\n", (time.Duration(v.Uptime) * time.Second).String())
	printMap(c.out, v.Gauges)
	fmt.Fprintln(c.out, "\ntotals since start:")
	printMap(c.out, v.Counters)
	return nil
}

func printMap(w io.Writer, m map[string]int64) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	for _, k := range keys {
		fmt.Fprintf(tw, "  %s\t%d\n", clean(k, 50), m[k])
	}
	tw.Flush()
}
