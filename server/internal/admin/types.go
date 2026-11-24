package admin

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned by a Controller when the named session or connection does not exist.
var ErrNotFound = errors.New("not found")

// MemberInfo describes one participant of a session.
type MemberInfo struct {
	PlayerID        string `json:"player_id"`
	DisplayName     string `json:"display_name"`
	Role            string `json:"role"`
	Ready           bool   `json:"ready"`
	ActionsAccepted int64  `json:"actions_accepted"`
	ActionsRejected int64  `json:"actions_rejected"`
}

// SessionInfo is a point-in-time description of a session.
type SessionInfo struct {
	ID            string       `json:"id"`
	Game          string       `json:"game"`
	Public        bool         `json:"public"`
	Status        string       `json:"status"` // waiting, in_progress, closed, or unresponsive
	Created       time.Time    `json:"created"`
	LastActivity  time.Time    `json:"last_activity"`
	MatchesPlayed int          `json:"matches_played"`
	Turns         int          `json:"turns"` // actions applied in the current match
	Members       []MemberInfo `json:"members"`
}

// ConnInfo describes one TCP connection to the game port.
type ConnInfo struct {
	ID               string    `json:"id"`
	Remote           string    `json:"remote"`
	PlayerID         string    `json:"player_id,omitempty"`
	DisplayName      string    `json:"display_name,omitempty"`
	Authenticated    bool      `json:"authenticated"`
	Connected        time.Time `json:"connected"`
	MessagesReceived int64     `json:"messages_received"`
	SessionID        string    `json:"session_id,omitempty"`
}

// Controller is what the API needs from the running server.
type Controller interface {
	Sessions(ctx context.Context) ([]SessionInfo, error)
	Session(ctx context.Context, id string) (SessionInfo, error)
	// CloseSession ends a session and notifies its members. reason is only recorded in the log.
	CloseSession(ctx context.Context, id, reason string) error
	Connections(ctx context.Context) ([]ConnInfo, error)
	CloseConnection(ctx context.Context, id string) error
	// Gauges reports current levels, such as open connections and live sessions.
	Gauges() map[string]int64
}
