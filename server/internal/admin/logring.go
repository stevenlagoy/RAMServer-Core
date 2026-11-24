package admin

import (
	"bytes"
	"strings"
	"sync"
)

const maxLogLine = 1024

// LogRing keeps the most recent log lines in memory so administrators can read
// them through the API. It implements io.Writer, so it can sit behind
// log.SetOutput next to the real destination.
type LogRing struct {
	mu      sync.Mutex
	lines   []string
	next    int // index of the oldest line once the ring is full
	max     int
	partial []byte
}

func NewLogRing(capacity int) *LogRing {
	if capacity < 1 {
		capacity = 1
	}
	return &LogRing{max: capacity}
}

// Write never fails: a logger must not be able to stall or break on the ring.
func (l *LogRing) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.partial = append(l.partial, p...)
	for {
		i := bytes.IndexByte(l.partial, '\n')
		if i < 0 {
			break
		}
		l.push(string(bytes.TrimRight(l.partial[:i], "\r")))
		l.partial = l.partial[i+1:]
	}
	if len(l.partial) > 2*maxLogLine { // a writer that never ends its line must not grow the buffer without bound
		l.push(string(l.partial[:maxLogLine]))
		l.partial = nil
	}
	if len(l.partial) == 0 {
		l.partial = nil
	}
	return len(p), nil
}

func (l *LogRing) push(line string) {
	if len(line) > maxLogLine {
		line = line[:maxLogLine] + "...[truncated]"
	}
	if len(l.lines) < l.max {
		l.lines = append(l.lines, line)
		return
	}
	l.lines[l.next] = line
	l.next = (l.next + 1) % l.max
}

// Tail returns up to n of the newest lines which contain the substring, oldest first.
func (l *LogRing) Tail(n int, contains string) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	ordered := make([]string, 0, len(l.lines))
	ordered = append(ordered, l.lines[l.next:]...)
	ordered = append(ordered, l.lines[:l.next]...)
	var out []string
	for i := len(ordered) - 1; i >= 0 && len(out) < n; i-- {
		if contains == "" || strings.Contains(ordered[i], contains) {
			out = append(out, ordered[i])
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}
