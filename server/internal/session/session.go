package session

import (
	"context"
	"errors"
	"log"
	"runtime/debug"

	"github.com/stevenlagoy/ramserver-core/server/game"
	"github.com/stevenlagoy/ramserver-core/server/internal/match"
	"github.com/stevenlagoy/ramserver-core/server/internal/transport"
)

// Anything a Session can deliver bytes to. Satisfied by *server.Client.
type Sender interface {
	Send(message []byte)
}

// Automatically mark clients as ready, for testing
var AutoReady bool

// Once client's participation in a Session's lobby.
type Member struct {
	ClientID    string
	DisplayName string // cosmetic label from AuthRequest
	Sender      Sender
	Role        game.Role
	Ready       bool
}

// One completed round for Session history and leaderboards
type MatchResult struct {
	Turns  int
	Result game.MatchResult
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

// Creates an empty Session; its creator joins through Join like any other member
func NewSession(id, gameName string, g game.Game, public bool) *Session {
	return &Session{
		ID:       id,
		GameName: gameName,
		Game:     g,
		Public:   public,
		inbox:    make(chan sessionMsg),
		members:  make(map[string]*Member),
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
	info       *infoMsg
	history    *historyMsg
}

type joinMsg struct {
	clientID    string
	displayName string
	seq         uint64 // request sequence, echoed on the joiner's StateUpdate
	sender      Sender
	reply       chan error
}
type infoMsg struct {
	reply chan transport.SessionInfo
}
type historyMsg struct {
	reply chan []transport.MatchResult
}
type leaveMsg struct {
	clientID string
}
type setReadyMsg struct {
	clientID string
	ready    bool
	seq      uint64 // request sequence, echoed on the requester's reply
}
type legalMovesMsg struct {
	clientID string
	reply    chan []game.ActionSpec
}

var (
	// Returned by Join when game's role limits have no room for participant
	ErrLobbyFull = transport.WrapError(transport.CodeSessionNotJoinable, errors.New("lobby is full"))
	// Returned by Join when a match is already in progress and game does not allow joining mid-match
	ErrAlreadyStarted = transport.WrapError(transport.CodeSessionNotJoinable, errors.New("match already in progress"))
)

// Adds clientID to the lobby, blocking until Run processes the request or ctx is cancelled.
// displayName is shown to other members in StateUpdate rosters; it is never used as identity.
// seq is the JoinSessionRequest's sequence; the joiner's copy of the resulting StateUpdate replies to it.
func (s *Session) Join(ctx context.Context, clientID, displayName string, seq uint64, sender Sender) error {
	reply := make(chan error, 1)
	select {
	case s.inbox <- sessionMsg{join: &joinMsg{clientID: clientID, displayName: displayName, seq: seq, sender: sender, reply: reply}}:
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

// Removes clientID from the lobby. Does not remove an empty Session; caller is reponsible for this
func (s *Session) Leave(ctx context.Context, clientID string) {
	select {
	case s.inbox <- sessionMsg{leave: &leaveMsg{clientID: clientID}}:
	case <-ctx.Done():
	}
}

// Records whether clientID considers themself ready to start. seq is the SetReadyRequest's sequence:
// the requester's copy of the resulting StateUpdate, or its ErrorResponse, replies to it.
func (s *Session) SetReady(ctx context.Context, clientID string, ready bool, seq uint64) {
	select {
	case s.inbox <- sessionMsg{setReady: &setReadyMsg{clientID: clientID, ready: ready, seq: seq}}:
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

// Returns a snapshot of the session for listings. ok is false if ctx was cancelled
func (s *Session) Info(ctx context.Context) (info transport.SessionInfo, ok bool) {
	reply := make(chan transport.SessionInfo, 1)
	select {
	case s.inbox <- sessionMsg{info: &infoMsg{reply: reply}}:
	case <-ctx.Done():
		return transport.SessionInfo{}, false
	}
	select {
	case info := <-reply:
		return info, true
	case <-ctx.Done():
		return transport.SessionInfo{}, false
	}
}

// Returns the results of every match finished in this session, with the oldest first
func (s *Session) History(ctx context.Context) []transport.MatchResult {
	reply := make(chan []transport.MatchResult, 1)
	select {
	case s.inbox <- sessionMsg{history: &historyMsg{reply: reply}}:
	case <-ctx.Done():
		return nil
	}
	select {
	case matches := <-reply:
		return matches
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
	case msg.info != nil:
		s.handleInfo(msg.info)
	case msg.history != nil:
		s.handleHistory(msg.history)
	}
}

func (s *Session) handleJoin(j *joinMsg) {
	if s.current != nil {
		// TODO: game-defined late-join policy (ie allow only spectators)
		j.reply <- ErrAlreadyStarted
		return
	}
	role := s.assignRole()
	if role == "" {
		j.reply <- ErrLobbyFull
		return
	}
	if _, exists := s.members[j.clientID]; !exists {
		s.order = append(s.order, j.clientID) // only after the join is accepted, so a rejected joiner never gets a turn
	}
	s.members[j.clientID] = &Member{ClientID: j.clientID, DisplayName: j.displayName, Sender: j.sender, Role: role, Ready: AutoReady}
	j.reply <- nil
	s.broadcastState(j.clientID, j.seq)
	if AutoReady && s.readyToStart() { // Testing only: no ready message exists yet
		s.startMatch()
	}
}

// Picks the first role the game declares that still has room. Returns "" if every role is at capacity.
func (s *Session) assignRole() game.Role {
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
	s.broadcastUnsolicitedState()
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
	s.broadcastUnsolicitedState()
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
	s.broadcastUnsolicitedState()
}

func (s *Session) handleSubmit(action game.Action) {
	if s.current == nil {
		return
	}
	member, ok := s.members[action.ActorID]
	if !ok {
		return // stale action from a member who left
	}
	// Check for out of turn action request
	if err := s.current.ApplyAction(action); err != nil {
		code := transport.CodeIllegalAction
		if errors.Is(err, match.ErrNotYourTurn) {
			code = transport.CodeOutOfTurn
		}
		perr := transport.WrapError(code, err)
		perr.SessionID = s.ID
		member.Sender.Send(transport.EncodeError(action.ID, perr))
		return
	}
	s.broadcastState(action.ActorID, action.ID)
	if result, done := s.current.Outcome(); done {
		s.endMatch(result)
	}
}

func (s *Session) endMatch(result game.MatchResult) {
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

func (s *Session) handleInfo(i *infoMsg) {
	status := transport.StatusWaiting
	if s.current != nil {
		status = transport.StatusInProgress
	}
	i.reply <- transport.SessionInfo{
		SessionID:     s.ID,
		GameID:        s.GameName,
		Status:        status,
		Players:       s.players(),
		OwnerPlayerID: s.Owner(),
		Joinable:      s.current == nil && s.assignRole() != "",
	}
}

func (s *Session) handleHistory(h *historyMsg) {
	matches := make([]transport.MatchResult, len(s.history))
	for i, r := range s.history {
		matches[i] = s.transportResult(r.Result, r.Turns)
	}
	h.reply <- matches
}

// The roster in join order, with each member's role and readiness
func (s *Session) players() []transport.SessionPlayer {
	players := make([]transport.SessionPlayer, 0, len(s.order))
	for _, id := range s.order {
		if m, ok := s.members[id]; ok {
			// TODO: track connection status on Member once disconnects keep members (reconnect grace period)
			players = append(players, transport.SessionPlayer{
				PlayerID:    id,
				DisplayName: m.DisplayName,
				Connected:   true,
				Ready:       m.Ready,
				Role:        string(m.Role),
			})
		}
	}
	return players
}

// Builds memberID's StateUpdate
func (s *Session) viewFor(memberID string) transport.StateUpdate {
	view := transport.StateUpdate{
		SessionID:     s.ID,
		GameID:        s.GameName,
		Status:        transport.StatusWaiting,
		Players:       s.players(),
		OwnerPlayerID: s.Owner(),
		// TODO: StateVersion
	}
	if s.current != nil {
		view.Status = transport.StatusInProgress
		view.EncodedView = s.current.ViewFor(memberID)
		view.PlayerOrder = s.current.RosterOrder
		if annotator, ok := s.Game.(game.Annotator); ok {
			view.Annotations = annotator.Annotations(s.current.State, memberID)
		}
		if active := s.current.Game.ActiveTurn(s.current.State); len(active) == 1 {
			view.ActivePlayerID = active[0]
		}
	}
	return view
}

// Sends each member its StateUpdate for a change no client requested, so no copy sets in_reply_to
func (s *Session) broadcastUnsolicitedState() {
	s.broadcastState("", 0)
}

// Sends each member its StateUpdate. requesterID's copy replies to seq; all others are unsolicited
func (s *Session) broadcastState(requesterID string, seq uint64) {
	for id, m := range s.members {
		var inReplyTo uint64
		if id == requesterID {
			inReplyTo = seq
		}
		m.Sender.Send(transport.EncodeStateUpdate(inReplyTo, s.viewFor(id)))
	}
}

// Convert a finished match result to wire form
func (s *Session) transportResult(result game.MatchResult, turns int) transport.MatchResult {
	// TODO: per-player summaries from the action log, and end reasons other than completion
	winners := result.GetWinnerIDs()
	return transport.MatchResult{
		SessionID:         s.ID,
		WinnerPlayerIDs:   winners,
		Draw:              len(winners) == 0,
		Reason:            transport.EndCompleted,
		TerminationReason: string(result.TerminationReason()),
		Turns:             uint32(turns),
	}
}

func (s *Session) broadcastMatchEnded(result game.MatchResult, turns int) {
	// TODO: per-player summaries from the action log, and end reasons other than completion
	payload := transport.EncodeMatchResult(s.transportResult(result, turns))
	for _, m := range s.members {
		m.Sender.Send(payload) // identical for every recipient, so the slice is shared
	}
}
