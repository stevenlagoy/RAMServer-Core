package admin

import (
	"sync/atomic"
	"time"
)

// Counter identifies one monotonically increasing metric.
type Counter int

const (
	ConnectionsAccepted Counter = iota
	ConnectionsRejectedFull
	ConnectionsRejectedPerIP
	ConnectionsKicked
	AuthSucceeded
	AuthFailed
	MessagesReceived
	MessagesMalformed
	SessionsCreated
	SessionsClosedByAdmin
	JoinsSucceeded
	JoinsRejected
	MatchesStarted
	MatchesCompleted
	ActionsAccepted
	ActionsRejectedIllegal
	ActionsRejectedOutOfTurn
	ActionsRejectedMalformed
	AdminRequests
	AdminAuthFailures
	numCounters
)

var counterNames = [numCounters]string{
	ConnectionsAccepted:      "connections_accepted",
	ConnectionsRejectedFull:  "connections_rejected_server_full",
	ConnectionsRejectedPerIP: "connections_rejected_per_ip_limit",
	ConnectionsKicked:        "connections_closed_by_admin",
	AuthSucceeded:            "auth_succeeded",
	AuthFailed:               "auth_failed",
	MessagesReceived:         "messages_received",
	MessagesMalformed:        "messages_malformed",
	SessionsCreated:          "sessions_created",
	SessionsClosedByAdmin:    "sessions_closed_by_admin",
	JoinsSucceeded:           "joins_succeeded",
	JoinsRejected:            "joins_rejected",
	MatchesStarted:           "matches_started",
	MatchesCompleted:         "matches_completed",
	ActionsAccepted:          "actions_accepted",
	ActionsRejectedIllegal:   "actions_rejected_illegal",
	ActionsRejectedOutOfTurn: "actions_rejected_out_of_turn",
	ActionsRejectedMalformed: "actions_rejected_malformed",
	AdminRequests:            "admin_requests",
	AdminAuthFailures:        "admin_auth_failures",
}

// Metrics is a set of lock-free counters. Every method accepts a nil receiver
// and does nothing, so code under test may leave metrics unset.
type Metrics struct {
	start    time.Time
	counters [numCounters]atomic.Int64
}

func NewMetrics() *Metrics { return &Metrics{start: time.Now()} }

func (m *Metrics) Inc(c Counter) {
	if m == nil || c < 0 || c >= numCounters {
		return
	}
	m.counters[c].Add(1)
}

// Snapshot returns every counter by name.
func (m *Metrics) Snapshot() map[string]int64 {
	out := make(map[string]int64, numCounters)
	for i := Counter(0); i < numCounters; i++ {
		if m == nil {
			out[counterNames[i]] = 0
			continue
		}
		out[counterNames[i]] = m.counters[i].Load()
	}
	return out
}

func (m *Metrics) Uptime() time.Duration {
	if m == nil {
		return 0
	}
	return time.Since(m.start)
}
