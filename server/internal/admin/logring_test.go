package admin

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestLogRingKeepsNewestLinesInOrder(t *testing.T) {
	r := NewLogRing(3)
	for i := 1; i <= 5; i++ {
		fmt.Fprintf(r, "line %d\n", i)
	}
	got := strings.Join(r.Tail(10, ""), "|")
	if got != "line 3|line 4|line 5" {
		t.Fatalf("Tail = %q", got)
	}
	if got := strings.Join(r.Tail(2, ""), "|"); got != "line 4|line 5" {
		t.Fatalf("Tail(2) = %q", got)
	}
}

func TestLogRingFilterAndPartialLines(t *testing.T) {
	r := NewLogRing(10)
	r.Write([]byte("alpha one\nbeta"))
	if got := r.Tail(10, ""); len(got) != 1 {
		t.Fatalf("a partial line must wait for its newline: %v", got)
	}
	r.Write([]byte(" two\r\nalpha three\n"))
	if got := strings.Join(r.Tail(10, "alpha"), "|"); got != "alpha one|alpha three" {
		t.Fatalf("filter = %q", got)
	}
	if got := r.Tail(10, "beta two"); len(got) != 1 {
		t.Fatalf("CRLF line = %v", got)
	}
}

func TestLogRingBoundsLongLines(t *testing.T) {
	r := NewLogRing(2)
	r.Write([]byte(strings.Repeat("x", 5000) + "\n"))
	if got := r.Tail(1, ""); len(got[0]) > maxLogLine+20 {
		t.Fatalf("line was %d bytes", len(got[0]))
	}
	r.Write([]byte(strings.Repeat("y", 100000))) // never ends its line
	if len(r.partial) > 2*maxLogLine {
		t.Fatalf("partial buffer grew to %d bytes", len(r.partial))
	}
}

func TestLogRingConcurrentWrites(t *testing.T) {
	r := NewLogRing(50)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				fmt.Fprintf(r, "w %d\n", j)
				r.Tail(5, "w")
			}
		}()
	}
	wg.Wait()
	if len(r.Tail(1000, "")) != 50 {
		t.Fatal("ring should be full")
	}
}

func TestMetricsAreNilSafeAndCount(t *testing.T) {
	var nilMetrics *Metrics
	nilMetrics.Inc(AuthFailed) // must not panic
	if nilMetrics.Snapshot()["auth_failed"] != 0 || nilMetrics.Uptime() != 0 {
		t.Fatal("nil metrics must report zeros")
	}
	m := NewMetrics()
	m.Inc(ActionsAccepted)
	m.Inc(ActionsAccepted)
	m.Inc(Counter(-1))
	m.Inc(numCounters)
	snap := m.Snapshot()
	if snap["actions_accepted"] != 2 || len(snap) != int(numCounters) {
		t.Fatalf("snapshot = %v", snap)
	}
	for c := Counter(0); c < numCounters; c++ {
		if counterNames[c] == "" {
			t.Errorf("counter %d has no name", c)
		}
	}
}
