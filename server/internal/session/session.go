package session

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"log"
	"log/slog"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"time"

	"github.com/stevenlagoy/ramserver-core/server/game"
	"github.com/stevenlagoy/ramserver-core/server/internal/match"
	"github.com/stevenlagoy/ramserver-core/server/internal/transport"
)

// Anything a Session can deliver bytes to. Satisfied by *server.Client.
type Sender interface {
	Send(message []byte)
}

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

	inbox   chan sessionMsg
	stopped chan struct{} // closed when Run returns, so callers stop waiting on a Session that will never answer
	onEmpty func()        // set by SessionStore before Run starts; called when the last member leaves

	// Below owned exclusively by Run goroutine

	order        []string // join order, order[0] is current owner
	members      map[string]*Member
	history      []MatchResult
	current      *match.Match
	stateVersion uint64
	emptied      bool // the last member left, so Run returns instead of handling more messages

	joinToken atomic.Pointer[string]
}

func (s *Session) setJoinToken(token string) {
	s.joinToken.Store(&token)
}

func (s *Session) joinTokenValue() string {
	token := s.joinToken.Load()
	if token == nil {
		return ""
	}
	return *token
}

// Creates an empty Session; its creator joins through Join like any other member
func NewSession(id, gameName string, g game.Game, public bool) *Session {
	return &Session{
		ID:           id,
		GameName:     gameName,
		Game:         g,
		Public:       public,
		inbox:        make(chan sessionMsg),
		stopped:      make(chan struct{}),
		members:      make(map[string]*Member),
		stateVersion: 1,
	}
}

// Returns current lobby owner
func (s *Session) owner() string {
	if len(s.order) == 0 {
		return ""
	}
	return s.order[0]
}

// Envelopes request Run processes
type sessionMsg struct {
	join              *joinMsg
	leave             *leaveMsg
	setReady          *setReadyMsg
	submit            *submitMsg
	legalMoves        *legalMovesMsg
	info              *infoMsg
	history           *historyMsg
	generateJoinToken *generateJoinTokenMsg
}

type submitMsg struct {
	action           game.Action
	sessionID        string
	expectedStateVer uint64
	sender           Sender
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
type generateJoinTokenMsg struct {
	clientID string
	seq      uint64
	reply    chan error
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
	ErrAlreadyStarted = transport.WrapError(
		transport.CodeSessionNotJoinable,
		errors.New("match already in progress"),
	)

	ErrNotInSession = errors.New(
		"player is not in the requested session",
	)

	ErrStaleState = errors.New(
		"action was submitted against an outdated state",
	)

	// Returned by Join when the Session has stopped, such as after its last member left
	ErrSessionClosed = transport.WrapError(transport.CodeSessionNotJoinable, errors.New("session has closed"))
)

// Hands msg to Run. Returns false if ctx is cancelled or the Session stops first
func (s *Session) send(ctx context.Context, msg sessionMsg) bool {
	select {
	case s.inbox <- msg:
		return true
	case <-ctx.Done():
	case <-s.stopped:
	}
	return false
}

// Waits for Run's reply to a request it accepted. Returns false if ctx is cancelled or the Session stops first
func await[T any](ctx context.Context, s *Session, reply chan T) (T, bool) {
	select {
	case v := <-reply:
		return v, true
	case <-ctx.Done():
	case <-s.stopped:
		select {
		case v := <-reply: // Run may have replied just before stopping
			return v, true
		default:
		}
	}
	var zero T
	return zero, false
}

// The error for a request that got no reply: ctx's error if it was cancelled, otherwise ErrSessionClosed
func (s *Session) noReplyErr(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return ErrSessionClosed
}

// Adds clientID to the lobby, blocking until Run processes the request, ctx is cancelled, or the Session stops.
// displayName is shown to other members in StateUpdate rosters; it is never used as identity.
// seq is the JoinSessionRequest's sequence; the joiner's copy of the resulting StateUpdate replies to it.
func (s *Session) Join(ctx context.Context, clientID, displayName string, seq uint64, sender Sender) error {
	reply := make(chan error, 1)
	if !s.send(ctx, sessionMsg{join: &joinMsg{clientID: clientID, displayName: displayName, seq: seq, sender: sender, reply: reply}}) {
		return s.noReplyErr(ctx)
	}
	err, ok := await(ctx, s, reply)
	if !ok {
		return s.noReplyErr(ctx)
	}
	return err
}

// Removes clientID from the lobby. When the last member leaves, the Session removes itself from its store and stops
func (s *Session) Leave(ctx context.Context, clientID string) {
	s.send(ctx, sessionMsg{leave: &leaveMsg{clientID: clientID}})
}

// Records whether clientID considers themself ready to start. seq is the SetReadyRequest's sequence:
// the requester's copy of the resulting StateUpdate, or its ErrorResponse, replies to it.
func (s *Session) SetReady(ctx context.Context, clientID string, ready bool, seq uint64) {
	s.send(ctx, sessionMsg{setReady: &setReadyMsg{clientID: clientID, ready: ready, seq: seq}})
}

// Enqueues action for processing. action.ActorID must be set to authenticated client's ID by caller
func (s *Session) Submit(
	ctx context.Context,
	action game.Action,
	sessionID string,
	expectedStateVer uint64,
	sender Sender,
) {
	request := &submitMsg{
		action:           action,
		sessionID:        sessionID,
		expectedStateVer: expectedStateVer,
		sender:           sender,
	}
	select {
	case s.inbox <- sessionMsg{submit: request}:
	case <-ctx.Done():
	}
}

// Returns the actions clientID may currently take. Empty before the match starts.
func (s *Session) LegalMoves(ctx context.Context, clientID string) []game.ActionSpec {
	reply := make(chan []game.ActionSpec, 1)
	if !s.send(ctx, sessionMsg{legalMoves: &legalMovesMsg{clientID: clientID, reply: reply}}) {
		return nil
	}
	moves, _ := await(ctx, s, reply)
	return moves
}

// Returns a snapshot of the session for listings. ok is false if ctx was cancelled or the Session has stopped
func (s *Session) Info(ctx context.Context) (info transport.SessionInfo, ok bool) {
	reply := make(chan transport.SessionInfo, 1)
	if !s.send(ctx, sessionMsg{info: &infoMsg{reply: reply}}) {
		return transport.SessionInfo{}, false
	}
	return await(ctx, s, reply)
}

// Returns the results of every match finished in this session, with the oldest first
func (s *Session) History(ctx context.Context) []transport.MatchResult {
	reply := make(chan []transport.MatchResult, 1)
	if !s.send(ctx, sessionMsg{history: &historyMsg{reply: reply}}) {
		return nil
	}
	matches, _ := await(ctx, s, reply)
	return matches
}

func (s *Session) GenerateJoinToken(ctx context.Context, clientID string, seq uint64) error {
	reply := make(chan error, 1)

	if !s.send(ctx, sessionMsg{
		generateJoinToken: &generateJoinTokenMsg{
			clientID: clientID,
			seq:      seq,
			reply:    reply,
		},
	}) {
		return s.noReplyErr(ctx)
	}

	err, ok := await(ctx, s, reply)
	if !ok {
		return s.noReplyErr(ctx)
	}
	return err
}

func (s *Session) handleGenerateJoinToken(req *generateJoinTokenMsg) {
	if _, ok := s.members[req.clientID]; !ok {
		req.reply <- ErrNotInSession
		return
	}

	if s.owner() != req.clientID {
		req.reply <- transport.NewError(
			transport.CodeIllegalAction,
			"only the session owner may generate a join token",
		)
		return
	}

	token, err := generateJoinToken(s.ID)
	if err != nil {
		req.reply <- err
		return
	}

	s.setJoinToken(token)
	s.stateVersion++
	s.broadcastState(req.clientID, req.seq)

	req.reply <- nil
}

func generateJoinToken(sessionID string) (string, error) {
	cryptoBuf := make([]byte, 8)
	if _, err := rand.Read(cryptoBuf); err != nil {
		return "", err
	}

	timeBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(timeBuf, uint64(time.Now().UnixNano()))

	hasher := sha256.New()
	hasher.Write([]byte(sessionID))
	hasher.Write(timeBuf)
	hasher.Write(cryptoBuf)
	hashBytes := hasher.Sum(nil)

	tokenRunes := []rune{
		'A', 'B', 'C', 'D', 'E', 'F', 'G',
		'H', 'J', 'K', 'M', 'N', 'P', 'R',
		'S', 'T', 'U', 'V', 'W', 'X', 'Y',
		'3', '4', '5', '6', '7', '8', '9',
	}
	possibilities := uint64(len(tokenRunes))

	var result strings.Builder
	for i := range 6 {
		// Read a unique 4-byte window from the hash for each character position
		chunk := binary.BigEndian.Uint32(hashBytes[i*4 : (i+1)*4])
		idx := uint64(chunk) % possibilities
		result.WriteRune(tokenRunes[idx])
	}
	return result.String(), nil
}

// Session's actor loop. Owns exclusively every mutable field on Session. Returns when ctx is
// cancelled or the last member leaves; either way, later calls into the Session stop waiting
func (s *Session) Run(ctx context.Context) {
	defer close(s.stopped)
	for {
		select {
		case msg := <-s.inbox:
			s.handle(msg)
			if s.emptied {
				return // handle nothing else, so no one joins a Session that has been removed
			}
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
		s.handleSubmit(msg.submit)
	case msg.legalMoves != nil:
		s.handleLegalMoves(msg.legalMoves)
	case msg.info != nil:
		s.handleInfo(msg.info)
	case msg.history != nil:
		s.handleHistory(msg.history)
	case msg.generateJoinToken != nil:
		s.handleGenerateJoinToken(msg.generateJoinToken)
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
	s.members[j.clientID] = &Member{ClientID: j.clientID, DisplayName: j.displayName, Sender: j.sender, Role: role}
	s.stateVersion++
	j.reply <- nil
	s.broadcastState(j.clientID, j.seq)
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
	s.stateVersion++
	if len(s.members) == 0 {
		s.emptied = true
		if s.onEmpty != nil {
			s.onEmpty() // the store removes this Session and cancels its context
		}
		return
	}
	if s.current == nil && s.readyToStart() {
		s.startMatch("", 0) // the leaver was the last member not ready
		return
	}
	s.broadcastUnsolicitedState()
}

func (s *Session) handleSetReady(r *setReadyMsg) {
	member, ok := s.members[r.clientID]
	if !ok {
		return
	}

	if s.current != nil {
		perr := transport.NewError(
			transport.CodeInvalidState,
			"the match has already started",
		)
		perr.SessionID = s.ID
		member.Sender.Send(transport.EncodeError(r.seq, perr))
		return
	}

	if member.Ready != r.ready {
		member.Ready = r.ready
		s.stateVersion++
	}

	if s.readyToStart() {
		s.startMatch(r.clientID, r.seq)
		return
	}

	s.broadcastState(r.clientID, r.seq)
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

// Starts the match and broadcasts the first in-progress state. requesterID's copy replies to seq,
// for the SetReadyRequest that completed readiness; pass "" and 0 when no request caused the start
func (s *Session) startMatch(requesterID string, seq uint64) {
	roles := make(map[string]game.Role, len(s.members))
	for id, m := range s.members {
		roles[id] = m.Role
	}
	s.current = match.NewMatch(s.Game, append([]string(nil), s.order...), roles) // copy order
	s.broadcastState(requesterID, seq)
}

func (s *Session) handleSubmit(req *submitMsg) {
	action := req.action

	if req.sessionID != s.ID {
		s.rejectAction(req, transport.CodeNotInSession, ErrNotInSession)
		return
	}

	if _, ok := s.members[action.ActorID]; !ok {
		s.rejectAction(req, transport.CodeNotInSession, ErrNotInSession)
		return
	}

	if req.expectedStateVer != 0 &&
		req.expectedStateVer != s.stateVersion {
		s.rejectAction(req, transport.CodeStaleState, ErrStaleState)
		return
	}

	if s.current == nil {
		s.rejectAction(
			req,
			transport.CodeIllegalAction,
			errors.New("action not allowed while the session is waiting"),
		)
		return
	}

	if err := s.current.ApplyAction(action); err != nil {
		code := transport.CodeIllegalAction

		switch {
		case errors.Is(err, match.ErrNotYourTurn):
			code = transport.CodeOutOfTurn
		case errors.Is(err, game.ErrMalformedAction):
			code = transport.CodeMalformedMessage
		}

		s.rejectAction(req, code, err)
		return
	}

	s.stateVersion++

	slog.Info("action accepted",
		"session_id", s.ID,
		"player_id", action.ActorID,
		"sequence", action.ID,
		"state_version", s.stateVersion,
		"match_turn", s.current.Turns,
	)

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
	s.stateVersion++
	s.broadcastUnsolicitedState()
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
		OwnerPlayerID: s.owner(),
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
		StateVersion:  s.stateVersion,
		OwnerPlayerID: s.owner(),
		JoinToken:     s.joinTokenValue(),
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

func (s *Session) rejectAction(
	req *submitMsg,
	code transport.ErrorCode,
	err error,
) {
	action := req.action

	slog.Warn("action rejected",
		"session_id", req.sessionID,
		"player_id", action.ActorID,
		"sequence", action.ID,
		"state_version", s.stateVersion,
		"error_code", code.String(),
		"reason", err.Error(),
		"match_active", s.current != nil,
	)

	perr := transport.WrapError(code, err)
	perr.SessionID = req.sessionID
	req.sender.Send(transport.EncodeError(action.ID, perr))
}
