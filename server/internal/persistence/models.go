package persistence

import (
	"context"
	"time"
)

type Store interface {
	SaveMatch(ctx context.Context, record MatchRecord) (int64, error)
	GetMatch(ctx context.Context, id int64) (MatchRecord, error)
	Close() error
}

type MatchRecord struct {
	ID                int64
	SessionID         string
	SessionCreatedAt  time.Time
	MatchNumber       int
	GameID            string
	Seed              []byte
	StartedAt         time.Time
	EndedAt           time.Time
	TerminationReason string
	IsDraw            bool
	Participants      []ParticipantRecord
	Actions           []ActionRecord
}

type ParticipantRecord struct {
	ClientID        string
	DisplayName     string
	Role            string
	Seat            int
	IsWinner        bool
	Score           *float64
	ActionsAccepted int
	ActionsRejected int
}

const (
	ActionAccepted = "ACCEPTED"
	ActionRejected = "REJECTED"
)

type ActionRecord struct {
	Seq             int64
	ClientID        string
	ReceivedAt      time.Time
	Payload         []byte
	Outcome         string
	RejectionCode   string
	RejectionReason string
	StateVersion    *int64
}
