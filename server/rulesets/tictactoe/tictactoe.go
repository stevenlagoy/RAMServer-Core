// Package tictactoe is the RAMServer ruleset for tic-tac-toe.
//
// The encodings clients use (encoded_action, encoded_view, annotations) are specified in README.md in
// this directory. Keep that file and this code in step: it is the only description client authors get.
package tictactoe

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/stevenlagoy/ramserver-core/server/game"
)

// Name is the game_id clients send in JoinSessionRequest.
const Name = "tictactoe"

// Termination reasons reported in MatchResult.termination_reason.
const (
	ReasonThreeInARow      game.TerminationReason = "THREE_IN_A_ROW"
	ReasonDraw             game.TerminationReason = "DRAW"
	ReasonResignation      game.TerminationReason = "RESIGNATION"
	ReasonNotEnoughPlayers game.TerminationReason = "NOT_ENOUGH_PLAYERS"
)

// ResignPayload is the encoded_action for resigning.
const ResignPayload = "resign"

// Annotation keys. See README.md.
const (
	AnnotationXPlayer     = "x_player"
	AnnotationOPlayer     = "o_player"
	AnnotationMoves       = "moves"
	AnnotationLastMove    = "last_move"
	AnnotationWinningLine = "winning_line"
	AnnotationResigned    = "resigned"
)

// ViewLen is the length in bytes of every encoded_view this game produces.
const ViewLen = 11

// Bytes used in the encoded view.
const (
	cellEmpty = '.'
	markX     = 'X'
	markO     = 'O'

	statusXWon = 'x'
	statusOWon = 'o'
	statusDraw = 'D'
	statusOver = '-' // ended without a winner or a draw (NOT_ENOUGH_PLAYERS)

	youSpectator = 'S'
)

// Seats. X is seat 0 and always moves first.
const (
	seatX    = 0
	seatO    = 1
	noSeat   = -1
	noCell   = -1
	numCells = 9
)

// lines lists every three-in-a-row, in the order they are checked: rows, columns, then diagonals.
var lines = [8][3]int{
	{0, 1, 2}, {3, 4, 5}, {6, 7, 8},
	{0, 3, 6}, {1, 4, 7}, {2, 5, 8},
	{0, 4, 8}, {2, 4, 6},
}

// Game implements game.Game for tic-tac-toe. It holds no data: every match lives in its State.
type Game struct{}

var (
	_ game.Game             = (*Game)(nil)
	_ game.OutOfTurnActions = (*Game)(nil)
	_ game.Annotator        = (*Game)(nil)
)

// State is one position of a match. It contains only arrays and scalars, so copying the struct copies
// everything: Apply works on a copy and can never change the state it was given.
type State struct {
	Players  [2]string      // member IDs: Players[seatX] plays X, Players[seatO] plays O
	Board    [numCells]byte // cellEmpty, markX or markO, row-major from the top-left
	Turn     int            // seat to move while the game is running
	Moves    int            // number of marks placed
	LastMove int            // cell of the last mark, or noCell
	Over     bool
	Winner   int    // seat that won, or noSeat (draw, or not enough players)
	Line     [3]int // the winning line, when Reason is ReasonThreeInARow
	Resigned int    // seat that resigned, or noSeat
	Reason   game.TerminationReason
}

// Roles: exactly two players, then any number of spectators. Joiners take the first role with room, so
// the first two members to join play and everyone after them watches.
func (*Game) Roles() []game.RoleSpec {
	return []game.RoleSpec{
		{Name: game.RolePlayer, Min: 2, Max: 2},
		{Name: game.RoleSpectator, Min: 0, Max: 0}, // Max 0: unlimited
	}
}

// ReadyToStart is true when exactly two players are present and both are ready. Spectators' ready flags
// are ignored.
func (*Game) ReadyToStart(roster map[string]game.Role, ready map[string]bool) bool {
	players := 0
	for id, role := range roster {
		if role != game.RolePlayer {
			continue
		}
		if !ready[id] {
			return false
		}
		players++
	}
	return players == 2
}

// NewState seats the first two players in join order: the first plays X and moves first.
func (*Game) NewState(rosterOrder []string, roles map[string]game.Role) game.GameState {
	s := State{Turn: seatX, LastMove: noCell, Winner: noSeat, Resigned: noSeat}
	for i := range s.Board {
		s.Board[i] = cellEmpty
	}
	seated := 0
	for _, id := range rosterOrder {
		if roles[id] == game.RolePlayer && seated < 2 {
			s.Players[seated] = id
			seated++
		}
	}
	if seated < 2 {
		// ReadyToStart never allows this, but don't start a match nobody can finish.
		s.Over = true
		s.Reason = ReasonNotEnoughPlayers
	}
	return s
}

// stateOf extracts our State. ok is false for a value this game did not produce.
func stateOf(st game.GameState) (State, bool) {
	switch s := st.(type) {
	case State:
		return s, true
	case *State:
		if s != nil {
			return *s, true
		}
	}
	return State{}, false
}

// seatOf returns the seat memberID plays, or noSeat.
func (s State) seatOf(memberID string) int {
	switch {
	case memberID == "":
		return noSeat
	case s.Players[seatX] == memberID:
		return seatX
	case s.Players[seatO] == memberID:
		return seatO
	}
	return noSeat
}

func markOf(seat int) byte {
	if seat == seatO {
		return markO
	}
	return markX
}

// ActiveTurn is the player to move, or empty once the game is over.
func (*Game) ActiveTurn(st game.GameState) []string {
	s, ok := stateOf(st)
	if !ok || s.Over {
		return []string{}
	}
	return []string{s.Players[s.Turn]}
}

// LegalActions offers the player to move every empty cell, and both players "resign" while the game runs.
// Spectators and finished games get nothing.
func (*Game) LegalActions(st game.GameState, memberID string) []game.ActionSpec {
	s, ok := stateOf(st)
	if !ok || s.Over {
		return []game.ActionSpec{}
	}
	seat := s.seatOf(memberID)
	if seat == noSeat {
		return []game.ActionSpec{}
	}
	var specs []game.ActionSpec
	if seat == s.Turn {
		for cell, c := range s.Board {
			if c == cellEmpty {
				specs = append(specs, game.ActionSpec{
					Payload:     []byte{byte('0' + cell)},
					Description: cellNames[cell],
				})
			}
		}
	}
	specs = append(specs, game.ActionSpec{Payload: []byte(ResignPayload), Description: "resign"})
	return specs
}

var cellNames = [numCells]string{
	"top-left", "top-center", "top-right",
	"middle-left", "center", "middle-right",
	"bottom-left", "bottom-center", "bottom-right",
}

// Error messages players can see in ErrorResponse.message. Each is prefixed by the wrapped error's text
// ("malformed action: " or "illegal action: ").
const (
	msgMalformed    = `send one digit "0" to "8" for a cell, or "resign"`
	msgGameOver     = "the game is over"
	msgNotAPlayer   = "spectators cannot play"
	msgNotYourTurn  = "it is not your turn"
	msgCellTaken    = "cell %d is already taken"
	msgUnknownState = "no game in progress"
)

// parsed is a decoded action: a cell, or resign.
type parsed struct {
	cell   int
	resign bool
}

// parse decodes an encoded_action. Exactly one ASCII digit '0'..'8', or exactly "resign"; nothing else.
func parse(payload []byte) (parsed, error) {
	if len(payload) == 1 && payload[0] >= '0' && payload[0] <= '8' {
		return parsed{cell: int(payload[0] - '0')}, nil
	}
	if string(payload) == ResignPayload {
		return parsed{cell: noCell, resign: true}, nil
	}
	return parsed{}, fmt.Errorf("%w: %s", game.ErrMalformedAction, msgMalformed)
}

// Validate accepts a move on an empty cell from the player to move, and a resignation from either player,
// while the game runs.
func (*Game) Validate(st game.GameState, action game.Action) error {
	a, err := parse(action.Payload)
	if err != nil {
		return err
	}
	s, ok := stateOf(st)
	if !ok {
		return fmt.Errorf("%w: %s", game.ErrIllegalAction, msgUnknownState)
	}
	if s.Over {
		return fmt.Errorf("%w: %s", game.ErrIllegalAction, msgGameOver)
	}
	seat := s.seatOf(action.ActorID)
	if seat == noSeat {
		return fmt.Errorf("%w: %s", game.ErrIllegalAction, msgNotAPlayer)
	}
	if a.resign {
		return nil
	}
	if seat != s.Turn {
		return fmt.Errorf("%w: %s", game.ErrIllegalAction, msgNotYourTurn)
	}
	if s.Board[a.cell] != cellEmpty {
		return fmt.Errorf("%w: "+msgCellTaken, game.ErrIllegalAction, a.cell)
	}
	return nil
}

// AllowsOutOfTurn lets either player resign at any time, so a match can always be ended, even when the
// player to move has left or stopped responding. Validate still refuses spectators.
func (*Game) AllowsOutOfTurn(_ game.GameState, action game.Action) bool {
	return string(action.Payload) == ResignPayload
}

// Apply returns the state after a validated action. s is a copy (State holds only arrays and scalars),
// so the caller's state is never changed.
func (*Game) Apply(st game.GameState, action game.Action) game.GameState {
	s, ok := stateOf(st)
	if !ok {
		return st
	}
	a, err := parse(action.Payload)
	if err != nil || s.Over {
		return s
	}
	seat := s.seatOf(action.ActorID)
	if seat == noSeat {
		return s
	}
	if a.resign {
		s.Over = true
		s.Resigned = seat
		s.Winner = 1 - seat
		s.Reason = ReasonResignation
		return s
	}
	s.Board[a.cell] = markOf(seat)
	s.Moves++
	s.LastMove = a.cell
	if line, won := winningLine(s.Board); won {
		s.Over = true
		s.Winner = seat
		s.Line = line
		s.Reason = ReasonThreeInARow
	} else if s.Moves == numCells {
		s.Over = true
		s.Reason = ReasonDraw
	} else {
		s.Turn = 1 - seat
	}
	return s
}

// winningLine returns the first complete line on board, in the order of lines.
func winningLine(board [numCells]byte) ([3]int, bool) {
	for _, l := range lines {
		if c := board[l[0]]; c != cellEmpty && board[l[1]] == c && board[l[2]] == c {
			return l, true
		}
	}
	return [3]int{}, false
}

// Outcome reports the result once the game is over.
func (*Game) Outcome(st game.GameState) (game.MatchResult, bool) {
	s, ok := stateOf(st)
	if !ok || !s.Over {
		return nil, false
	}
	var winners []string
	if s.Winner != noSeat {
		winners = []string{s.Players[s.Winner]}
	}
	return game.Result{WinnerIDs: winners, Reason: s.Reason}, true
}

// ViewFor encodes the state as ViewLen ASCII bytes: 9 board cells, a status byte, and the recipient's
// own mark. The format is specified in README.md.
func (*Game) ViewFor(st game.GameState, memberID string) []byte {
	s, ok := stateOf(st)
	if !ok {
		return []byte{}
	}
	view := make([]byte, 0, ViewLen)
	view = append(view, s.Board[:]...)
	view = append(view, s.statusByte())
	if seat := s.seatOf(memberID); seat != noSeat {
		view = append(view, markOf(seat))
	} else {
		view = append(view, youSpectator)
	}
	return view
}

func (s State) statusByte() byte {
	switch {
	case !s.Over:
		return markOf(s.Turn)
	case s.Winner == seatX:
		return statusXWon
	case s.Winner == seatO:
		return statusOWon
	case s.Reason == ReasonDraw:
		return statusDraw
	}
	return statusOver
}

// Annotations gives clients the seats, move count, and how the game ended without decoding the view.
// They are the same for every member.
func (*Game) Annotations(st game.GameState, _ string) map[string]string {
	s, ok := stateOf(st)
	if !ok {
		return map[string]string{}
	}
	a := map[string]string{
		AnnotationXPlayer: s.Players[seatX],
		AnnotationOPlayer: s.Players[seatO],
		AnnotationMoves:   strconv.Itoa(s.Moves),
	}
	if s.LastMove != noCell {
		a[AnnotationLastMove] = strconv.Itoa(s.LastMove)
	}
	if s.Reason == ReasonThreeInARow {
		a[AnnotationWinningLine] = fmt.Sprintf("%d,%d,%d", s.Line[0], s.Line[1], s.Line[2])
	}
	if s.Resigned != noSeat {
		a[AnnotationResigned] = string(markOf(s.Resigned))
	}
	return a
}

// String renders the board for test failure messages.
func (s State) String() string {
	var b strings.Builder
	for r := 0; r < 3; r++ {
		b.Write(s.Board[r*3 : r*3+3])
		if r < 2 {
			b.WriteByte('/')
		}
	}
	return b.String()
}
