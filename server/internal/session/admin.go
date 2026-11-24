package session

import (
	"context"
	"errors"
	"log"

	"github.com/stevenlagoy/ramserver-core/server/internal/admin"
	"github.com/stevenlagoy/ramserver-core/server/internal/transport"
)

// Returned by Join once an administrator has closed the session.
var ErrClosed = transport.WrapError(transport.CodeSessionNotJoinable, errors.New("session is closed"))

type infoMsg struct {
	reply chan admin.SessionInfo
}

type closeMsg struct {
	reason string
	reply  chan struct{}
}

// Reports whether Close has been called. Safe to call from any goroutine.
func (s *Session) IsClosed() bool { return s.closed.Load() }

// Returns a snapshot of the session, taken by the Run goroutine so it never races with it.
// Blocks until Run answers or ctx is done, so give ctx a timeout: a wedged session will not answer.
func (s *Session) Info(ctx context.Context) (admin.SessionInfo, error) {
	reply := make(chan admin.SessionInfo, 1)
	select {
	case s.inbox <- sessionMsg{info: &infoMsg{reply: reply}}:
	case <-ctx.Done():
		return admin.SessionInfo{}, ctx.Err()
	}
	select {
	case info := <-reply:
		return info, nil
	case <-ctx.Done():
		return admin.SessionInfo{}, ctx.Err()
	}
}

func (s *Session) handleInfo(m *infoMsg) {
	info := admin.SessionInfo{
		ID:            s.ID,
		Game:          s.GameName,
		Public:        s.Public,
		Status:        "waiting",
		Created:       s.created,
		LastActivity:  s.lastActive,
		MatchesPlayed: len(s.history),
		Members:       make([]admin.MemberInfo, 0, len(s.order)),
	}
	switch {
	case s.closed.Load():
		info.Status = "closed"
	case s.current != nil:
		info.Status = "in_progress"
		info.Turns = s.current.Turns
	}
	for _, id := range s.order {
		if m, ok := s.members[id]; ok {
			info.Members = append(info.Members, admin.MemberInfo{
				PlayerID:        id,
				DisplayName:     m.DisplayName,
				Role:            string(m.Role),
				Ready:           m.Ready,
				ActionsAccepted: m.Accepted,
				ActionsRejected: m.Rejected,
			})
		}
	}
	m.reply <- info
}

// Ends the session on an administrator's request. An in-progress match is aborted. Every member
// receives a StateUpdate with SESSION_STATUS_CLOSED and is removed; later joins are refused.
// The caller removes the session from its store. Blocks until Run has finished or ctx is done.
func (s *Session) Close(ctx context.Context, reason string) error {
	reply := make(chan struct{}, 1)
	select {
	case s.inbox <- sessionMsg{closeReq: &closeMsg{reason: reason, reply: reply}}:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-reply:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Session) handleClose(c *closeMsg) {
	defer func() { c.reply <- struct{}{} }()
	if s.closed.Load() {
		return
	}
	s.closed.Store(true) // first, so a concurrent Client sees it as soon as it is told
	log.Printf("session %s: closed by an administrator (reason: %q)", s.ID, c.reason)
	if s.current != nil {
		aborted := transport.EncodeMatchResult(transport.MatchResult{SessionID: s.ID, Reason: transport.EndAborted})
		for _, m := range s.members {
			m.Sender.Send(aborted)
		}
	}
	for id, m := range s.members {
		view := s.viewFor(id)
		view.Status = transport.StatusClosed
		view.EncodedView = nil
		view.ActivePlayerID = ""
		m.Sender.Send(transport.EncodeStateUpdate(0, view))
	}
	s.current = nil
	s.members = make(map[string]*Member)
	s.order = nil
}
