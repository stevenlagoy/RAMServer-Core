package session

import (
	"context"
	"errors"
	"log"
	"runtime/debug"

	"github.com/stevenlagoy/ramserver-core/server/internal/game"
	"github.com/stevenlagoy/ramserver-core/server/internal/match"
	"github.com/stevenlagoy/ramserver-core/server/internal/transport"
)

// Anything a Session can deliver bytes to. Satisfied by *server.Client.
type Sender interface {
	Send(message []byte)
}

// Once client's participation in a Session's lobby.
type Member struct {
	ClientID string
	Sender   Sender
	Role     game.Role
	Ready    bool
}

// One completed round for Session history and leaderboards
type MatchResult struct {
	Turns  int
	Result game.Result
}

// Sole actor for everything that happens within: Run goroutine owns members,
// history, and current Match. Other methods communicate over a channel.
type Session struct {
	ID       string
	GameName string
	Game     game.Game
	Public   bool

	inbox chan sessionMsg

	// Below owned exclusively by Run goroutine

	order   []string // join order, order[0] is current owner
	members map[string]*Member
	history []MatchResult
	current *match.Match
}

// Creates a Session with owner as its first member
func NewSession(id, gameName string, g game.Game, public bool, ownerID string, owner Sender) *Session {
	return &Session{
		ID:       id,
		GameName: gameName,
		Game:     g,
		Public:   public,
		inbox:    make(chan sessionMsg),
		order:    []string{ownerID},
		members:  map[string]*Member{ownerID: {ClientID: ownerID, Sender: owner}},
	}
}

// Returns current lobby owner
func (s *Session) Owner() string {
	if len(s.order) == 0 {
		return ""
	}
	return s.order[0]
}

// Envelopes request Run processes
type sessionMsg struct {
	join       *joinMsg
	leave      *leaveMsg
	setReady   *setReadyMsg
	submit     *game.Action
	legalMoves *legalMovesMsg
}

type joinMsg struct {
	clientID string
	sender   Sender
	reply    chan error
}
type leaveMsg struct {
	clientID string
}
type setReadyMsg struct {
	clientID string
	ready    bool
}
type legalMovesMsg struct {
	clientID string
	reply    chan []game.ActionSpec
}

var (
	// Returned by Join when game's role limits have no room for participant
	ErrLobbyFull = errors.New("lobby is full")
	// Returned by Join when a match is already in progress and game does not allow joining mid-match
	ErrAlreadyStarted = errors.New("match already in progress")
)

// Adds clientID to the lobby, blocking until Run processes the request or ctx is cancelled
func (s *Session) Join(ctx context.Context, clientID string, sender Sender) error {
	reply := make(chan error, 1)
	select {
	case s.inbox <- sessionMsg{join: &joinMsg{clientID: clientID, sender: sender, reply: reply}}:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-reply:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Removes clientID from the lobby. DOes not remove an empties Session; caller is reponsible for this
func (s *Session) Leave(ctx context.Context, clientID string) {
	select {
	case s.inbox <- sessionMsg{leave: &leaveMsg{clientID: clientID}}:
	case <-ctx.Done():
	}
}

// Records whether clientID considers themself ready to start
func (s *Session) SetReady(ctx context.Context, clientID string, ready bool) {
	select {
	case s.inbox <- sessionMsg{setReady: &setReadyMsg{clientID: clientID, ready: ready}}:
	case <-ctx.Done():
	}
}

// Enqueues action for processing. action.ActorID must be set to authenticated client's ID by caller
func (s *Session) Submit(ctx context.Context, action game.Action) {
	select {
	case s.inbox <- sessionMsg{submit: &action}:
	case <-ctx.Done():
	}
}

// Returns the actions clientID may currently take.
func (s *Session) LegalMoves(ctx context.Context, clientID string) []game.ActionSpec {
	reply := make(chan []game.ActionSpec, 1)
	select {
	case s.inbox <- sessionMsg{legalMoves: &legalMovesMsg{clientID: clientID, reply: reply}}:
	case <-ctx.Done():
		return nil
	}
	select {
	case moves := <-reply:
		return moves
	case <-ctx.Done():
		return nil
	}
}

// Session's actor loop. Owns exclusively every mutable field on Session
func (s *Session) Run(ctx context.Context) {
	for {
		select {
		case msg := <-s.inbox:
			s.handle(msg)
		case <-ctx.Done():
			return
		}
	}
}

func (s *Session) handle(msg sessionMsg) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("session %s: panic handling message: %v\n%s", s.ID, r, debug.Stack())
		}
	}()
	switch {
	case msg.join != nil:
		s.handleJoin(msg.join)
	case msg.leave != nil:
		s.handleLeave(msg.leave)
	case msg.setReady != nil:
		s.handleSetReady(msg.setReady)
	case msg.submit != nil:
		s.handleSubmit(*msg.submit)
	case msg.legalMoves != nil:
		s.handleLegalMoves(msg.legalMoves)
	}
}

func (s *Session) handleJoin(j *joinMsg) {
	if s.current != nil {
		// TODO: game-defined late-join policy (ie allow only spectators)
		j.reply <- ErrAlreadyStarted
		return
	}
	if _, exists := s.members[j.clientID]; !exists {
		s.order = append(s.order, j.clientID)
	}
	role := s.assignRole()
	if role == "" {
		j.reply <- ErrLobbyFull
		return
	}
	s.members[j.clientID] = &Member{ClientID: j.clientID, Sender: j.sender, Role: role}
	j.reply <- nil
	s.broadcastLobbyState()
}

// Picks a role for a newly joining member. Returns "" if every role is at capacity.
func (s *Session) assignRole() game.Role {
	// TODO: allow clients to explicitly request roles
	counts := make(map[game.Role]int)
	for _, m := range s.members {
		counts[m.Role]++
	}
	for _, spec := range s.Game.Roles() {
		if spec.Max == 0 || counts[spec.Name] < spec.Max {
			return spec.Name
		}
	}
	return ""
}

func (s *Session) handleLeave(l *leaveMsg) {
	if _, exists := s.members[l.clientID]; !exists {
		return
	}
	delete(s.members, l.clientID)
	for i, id := range s.order {
		if id == l.clientID {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
	if len(s.members) == 0 {
		return // caller removes Session from store
	}
	s.broadcastLobbyState()
}

func (s *Session) handleSetReady(r *setReadyMsg) {
	member, ok := s.members[r.clientID]
	if !ok {
		return
	}
	member.Ready = r.ready
	if s.current == nil && s.readyToStart() {
		s.startMatch()
		return
	}
	s.broadcastLobbyState()
}

func (s *Session) readyToStart() bool {
	roster := make(map[string]game.Role, len(s.members))
	ready := make(map[string]bool, len(s.members))
	for id, m := range s.members {
		roster[id] = m.Role
		ready[id] = m.Ready
	}
	return s.Game.ReadyToStart(roster, ready)
}

func (s *Session) startMatch() {
	roles := make(map[string]game.Role, len(s.members))
	for id, m := range s.members {
		roles[id] = m.Role
	}
	s.current = match.NewMatch(s.Game, append([]string(nil), s.order...), roles) // copy order
	s.broadcastState()
}

func (s *Session) handleSubmit(action game.Action) {
	if s.current == nil {
		return
	}
	member, ok := s.members[action.ActorID]
	if !ok {
		return // stale action from a member who left
	}
	if err := s.current.ApplyAction(action); err != nil {
		member.Sender.Send(transport.EncodeReject(action.ID, err))
		return
	}
	s.broadcastState()
	if result, done := s.current.Outcome(); done {
		s.endMatch(result)
	}
}

func (s *Session) endMatch(result game.Result) {
	s.history = append(s.history, MatchResult{Turns: s.current.Turns, Result: result})
	s.broadcastMatchEnded(result, s.current.Turns)
	s.current = nil
	for _, m := range s.members {
		m.Ready = false // require re-ready before next round
	}
}

func (s *Session) handleLegalMoves(l *legalMovesMsg) {
	if s.current == nil {
		l.reply <- nil
		return
	}
	l.reply <- s.current.LegalActions(l.clientID)
}

// Presentation-ready snapshot passed to transport.EncodeLobbyState
type LobbyView struct {
	SessionID string
	Owner     string
	Members   []MemberView
	History   []MatchResult
}

// Presentation-ready snapshot passed to transport.EncodeLobbyState
type MemberView struct {
	ClientID string
	Role     game.Role
	Ready    bool
}

func (s *Session) viewFor(memberID string) LobbyView {
	members := make([]MemberView, 0, len(s.members))
	for _, m := range s.members {
		members = append(members, MemberView{ClientID: m.ClientID, Role: m.Role, Ready: m.Ready})
	}
	return LobbyView{SessionID: s.ID, Owner: s.Owner(), Members: members, History: s.history}
}

func (s *Session) broadcastLobbyState() {
	for id, m := range s.members {
		m.Sender.Send(transport.EncodeLobbyState(s.viewFor(id)))
	}
}

func (s *Session) broadcastState() {
	for id, m := range s.members {
		m.Sender.Send(transport.EncodeLobbyState(s.viewFor(id)))
	}
}

// Plain snapshot passed to transport.EncodeMatchEnded
type MatchEndedView struct {
	Result game.Result
	Turns  int
}

func (s *Session) broadcastMatchEnded(result game.Result, turns int) {
	for _, m := range s.members {
		m.Sender.Send(transport.EncodeMatchEnded(MatchEndedView{Result: result, Turns: turns}))
	}
}
