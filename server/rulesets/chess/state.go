package chess

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/stevenlagoy/ramserver-core/server/game"
)

const squaresPerRank = 8
const squaresPerFile = 8
const maxPiecesPerPlayer = 16

// The letter naming each file, indexed by file number - 1
const fileLetters = "abcdefgh"

// One space on the chessboard
type Square struct {
	rank     uint8 // 1, 2, 3, 4, 5, 6, 7, 8
	file     uint8 // a, b, c, d, e, f, g, h
	occupant *Piece
}

// Get the shade of a square, "light" or "dark". The a1 square is dark
func (s Square) Shade() string {
	if (s.rank+s.file)%2 == 0 {
		return "dark"
	}
	return "light"
}

// Get the square's location as a UCI string (also used for SAN)
func (s Square) toUCI() string {
	return string(fileLetters[s.file-1]) + strconv.Itoa(int(s.rank))
}

// Parse a square from its UCI string, like "e4". Result has no occupant
func squareFromUCI(uci string) (Square, error) {
	if len(uci) != 2 {
		return Square{}, fmt.Errorf("square %q is not a file followed by a rank", uci)
	}
	file := strings.IndexByte(fileLetters, uci[0])
	rank := int(uci[1]) - '0'
	if file < 0 || rank < 1 || rank > squaresPerFile {
		return Square{}, fmt.Errorf("square %q is not on the board", uci)
	}
	return Square{rank: uint8(rank), file: uint8(file + 1)}, nil
}

// Get the index of a square in State.board
func squareIndex(rank uint8, file uint8) int {
	return int(rank-1)*squaresPerRank + int(file-1)
}

// Get the absolute difference of two coordinates
func absDiff(a uint8, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

// Get the direction (-1, 0, or 1) which leads from a toward b
func stepToward(a uint8, b uint8) int {
	switch {
	case a < b:
		return 1
	case a > b:
		return -1
	default:
		return 0
	}
}

// Get the squares strictly between two squares on the same rank, file, or diagonal
func squaresBetween(from Square, to Square) []Square {
	rankStep, fileStep := stepToward(from.rank, to.rank), stepToward(from.file, to.file)
	squares := []Square{}
	rank, file := int(from.rank)+rankStep, int(from.file)+fileStep
	for rank != int(to.rank) || file != int(to.file) {
		squares = append(squares, Square{rank: uint8(rank), file: uint8(file)})
		rank += rankStep
		file += fileStep
	}
	return squares
}

// For pieces which have nothing between their start and end squares
func noSquares(Square, Square) []Square { return nil }

// All the squares of an empty board
func makeSquares() [squaresPerRank * squaresPerFile]*Square {
	backing := new([squaresPerRank * squaresPerFile]Square) // Use backing to avoid multiple allocations
	squares := [squaresPerRank * squaresPerFile]*Square{}
	for i := range squaresPerRank * squaresPerFile {
		backing[i] = Square{uint8(i/squaresPerRank + 1), uint8(i%squaresPerFile + 1), nil}
		squares[i] = &backing[i]
	}
	return squares
}

// State of the chess gameboard at one moment. A state is never modified after it is built: apply returns a new State
type State struct {
	board           [squaresPerRank * squaresPerFile]*Square // Squares are owned by this State only. Index with squareIndex
	pieces          []*Piece                                 // Captured pieces are removed, promoting pieces change type in place
	turn            Color                                    // whose turn is it currently?
	castling        uint8                                    // KQkq bit flags: 0000KQkq
	enPassant       *Square                                  // target square or nil
	halfmove        int                                      // number of halfmoves: increments every action, reset to 0 if a pawn moves or a capture
	fullmove        int                                      // number of fullmoves: increment after every black turn
	whitePlayer     string                                   // playerID bound at NewState
	blackPlayer     string                                   // playerID bound at NewState
	stateHistory    []string                                 // position key of every state reached, including this one: use for repetition
	moveHistory     []Move                                   // move history
	activeDrawOffer bool                                     // Whether a draw offer is active from any player
	drawAccepted    bool                                     // Whether a draw offer was accepted
}

func newState(whitePlayer string, blackPlayer string) State {
	state := State{
		board:       makeSquares(),
		pieces:      makePieces(),
		turn:        White,
		castling:    castleAll,
		fullmove:    1,
		whitePlayer: whitePlayer,
		blackPlayer: blackPlayer,
		moveHistory: []Move{},
	}
	state.update()
	state.stateHistory = []string{state.positionKey()}
	return state
}

func StateFromFEN(FEN string, whitePlayer string, blackPlayer string) (*State, error) {
	// rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 1
	// 7k/8/8/8/8/8/8/K7 w - - 74 149
	fields := strings.Fields(FEN)
	if len(fields) != 6 {
		return nil, fmt.Errorf("FEN string sequence was invalid")
	}
	placementField := fields[0]
	activeColor := fields[1]
	castlingRights := fields[2]
	enPassantTarget := fields[3]
	halfmoveClock, err := strconv.Atoi(fields[4])
	if err != nil || halfmoveClock < 0 {
		return nil, fmt.Errorf("invalid value for half-move clock in the FEN string")
	}
	fullmoveNumber, err := strconv.Atoi(fields[5])
	if err != nil || fullmoveNumber < 1 {
		return nil, fmt.Errorf("invalid value for full-move number in the FEN string")
	}

	// Unpack piece placements. FEN lists rank 8 first and rank 1 last
	pieces := []*Piece{}
	kings := map[Color]int{}
	ranks := strings.Split(placementField, "/")
	if len(ranks) != squaresPerFile {
		return nil, fmt.Errorf("FEN string placement field had an incorrect number of ranks")
	}
	for i, rankString := range ranks {
		rank := uint8(squaresPerFile - i)
		file := 0 // Files filled so far in this rank
		for _, symbol := range rankString {
			if symbol >= '1' && symbol <= '8' { // A run of empty squares
				file += int(symbol - '0')
				continue
			}
			pieceType, color, ok := pieceFromSymbol(symbol)
			if !ok {
				return nil, fmt.Errorf("invalid rune encountered when placing pieces: %q", symbol)
			}
			file++
			if file > squaresPerRank {
				return nil, fmt.Errorf("too many files in rank %d of the FEN string placement field", rank)
			}
			if pieceType.name == kingType.name {
				kings[color]++
			}
			pieces = append(pieces, &Piece{pieceType, color, &Square{rank, uint8(file), nil}})
		}
		if file != squaresPerRank {
			return nil, fmt.Errorf("rank %d of the FEN string placement fields does not have %d files", rank, squaresPerRank)
		}
	}
	if kings[White] != 1 || kings[Black] != 1 {
		return nil, fmt.Errorf("FEN string must place exactly one king of each color")
	}

	// Determine next color to move
	var turn Color
	switch activeColor {
	case string(White.symbol):
		turn = White
	case string(Black.symbol):
		turn = Black
	default:
		return nil, fmt.Errorf("FEN active color field had an invalid value")
	}

	// Determine castling rights
	var castling uint8
	if castlingRights != "-" {
		for _, right := range castlingRights {
			var flag uint8
			switch right {
			case 'K':
				flag = castleWhiteKingside
			case 'Q':
				flag = castleWhiteQueenside
			case 'k':
				flag = castleBlackKingside
			case 'q':
				flag = castleBlackQueenside
			}
			if flag == 0 || castling&flag != 0 { // Unrecognized or repeated
				return nil, fmt.Errorf("castling rights contained an unrecognized character in the FEN string")
			}
			castling |= flag
		}
	}

	// Determine en passant target square
	var enPassant *Square
	if enPassantTarget != "-" {
		target, err := squareFromUCI(enPassantTarget)
		if err != nil || (target.rank != 3 && target.rank != 6) {
			return nil, fmt.Errorf("en passant target was invalid in the FEN string")
		}
		enPassant = &target
	}

	result := &State{
		board:       makeSquares(),
		pieces:      pieces,
		turn:        turn,
		castling:    castling,
		enPassant:   enPassant,
		halfmove:    halfmoveClock,
		fullmove:    fullmoveNumber,
		whitePlayer: whitePlayer,
		blackPlayer: blackPlayer,
		moveHistory: []Move{},
	}
	result.update()
	result.stateHistory = []string{result.positionKey()}
	return result, nil
}

// Copt the board and pieces so that the copy can be changed without changing s.
// History slices are shared, so they must only be extended through append(slices.Clip(...), ...)
func (s State) deepCopy() State {
	state := s
	state.board = makeSquares()
	pieces := make([]Piece, len(s.pieces)) // One allocation for all pieces
	state.pieces = make([]*Piece, len(s.pieces))
	for i, piece := range s.pieces {
		pieces[i] = *piece
		pieces[i].position = state.board[squareIndex(piece.position.rank, piece.position.file)]
		state.pieces[i] = &pieces[i]
	}
	state.update()
	return state
}

// Update each square's occupant to its current occupant based on piece positions.
// Only call this on a State which is still being built
func (s State) update() {
	for _, square := range s.board {
		square.occupant = nil
	}
	for _, piece := range s.pieces {
		square := s.board[squareIndex(piece.position.rank, piece.position.file)]
		square.occupant = piece
		piece.position = square
	}
}

// Get the piece standing on a square, or nil if it is empty
func (s State) pieceAt(rank uint8, file uint8) *Piece {
	return s.board[squareIndex(rank, file)].occupant
}

// Get a player's ling, or nil if they have none
func (s State) kingOf(color Color) *Piece {
	for _, piece := range s.pieces {
		if piece.color == color && piece.pieceType.name == kingType.name {
			return piece
		}
	}
	return nil
}

func (s State) piecesOf(color Color) []*Piece {
	result := make([]*Piece, 0, len(s.pieces))
	for _, piece := range s.pieces {
		if piece.color == color {
			result = append(result, piece)
		}
	}
	return result
}

// Determine whether every square is empty
func (s State) pathClear(squares []Square) bool {
	for _, square := range squares {
		if s.pieceAt(square.rank, square.file) != nil {
			return false
		}
	}
	return true
}

// Whether any piece of the given color could capture on the target square if an opponent stood there.
// This does not generate legal movs, so it is safe to call while deciding legality
func (s State) isAttacked(target Square, by Color) bool {
	for _, piece := range s.pieces {
		if piece.color != by {
			continue
		}
		for _, pattern := range piece.pieceType.movementPatterns {
			if pattern.captures == moveOnly || pattern.unattacked != nil { // Pawn pushes and capturing never capture
				continue
			}
			if pattern.evaluate(*piece.position, target, s) && s.pathClear(pattern.squaresInPath(*piece.position, target)) {
				return true
			}
		}
	}
	return false
}

// Whether any of the squares are attacked by the given color
func (s State) anyAttacked(squares []Square, by Color) bool {
	for _, square := range squares {
		if s.isAttacked(square, by) {
			return true
		}
	}
	return false
}

// Whether the give player's king is being attacked
func (s State) inCheck(color Color) bool {
	king := s.kingOf(color)
	return king != nil && s.isAttacked(*king.position, color.opponent())
}

// Determine which players' kings are in check
func (s State) PlayersInCheck() (white bool, black bool) {
	return s.inCheck(White), s.inCheck(Black)
}

// Get the ID of the player controlling a color
func (s State) playerID(color Color) string {
	if color == White {
		return s.whitePlayer
	}
	return s.blackPlayer
}

// Whether this move is a pawn capturing on the en passant square
func (s State) isEnPassant(m Move) bool {
	return m.piece != nil && m.piece.pieceType.name == pawnType.name &&
		s.enPassant != nil && m.To.rank == s.enPassant.rank && m.To.file == s.enPassant.file &&
		m.From.file != m.To.file && s.pieceAt(m.To.rank, m.To.file) == nil
}

// Fill in the parts of a decoded Move which depend on the board: moving piece, its color, whether it captures.
// Fails with ErrIllegalAction if there is no piece of the player to move on the From square
func (s State) resolve(m Move) (Move, error) {
	if m.isDrawResponse() { // Draw accept needs no square lookups
		m.color = s.turn
		return m, nil
	}
	if m.From == nil || m.To == nil {
		return m, fmt.Errorf("%w: the move has no origin or target square", game.ErrIllegalAction)
	}
	from := s.board[squareIndex(m.From.rank, m.From.file)]
	to := s.board[squareIndex(m.To.rank, m.To.file)]
	if from.occupant == nil {
		return m, fmt.Errorf("%w: there is no piece on %s", game.ErrIllegalAction, from.toUCI())
	}
	if from.occupant.color != s.turn {
		return m, fmt.Errorf("%w: the piece on %s does not belong to the player to move", game.ErrIllegalAction, from.toUCI())
	}
	m.From, m.To, m.piece, m.color = from, to, from.occupant, from.occupant.color
	m.capturing = to.occupant != nil || s.isEnPassant(m)
	return m, nil
}

// Whether a resolved move (see resolve) may be played in this state: the piece moves in a way it is allowed to, nothing blocks it,
// it promotes to a valid type exactly when it reaches the last rank, and it does not leave its own king attacked.
func (s State) isLegal(m Move) bool {
	// (=) alone accepts opponent's draw offer
	if m.isDrawResponse() {
		return s.activeDrawOffer
	}

	if m.From == nil || m.To == nil || m.piece == nil { // Not a piece move, or not resolved
		return false
	}
	if m.color != s.turn || m.piece.color != s.turn || m.From == m.To {
		return false
	}
	if m.To.occupant != nil && m.To.occupant.color == m.color { // Cannot capture own piece
		return false
	}
	promoting := m.piece.pieceType.canPromote && m.To.rank == m.color.promotionRank
	if promoting != (m.Promotion != nil) { // A pawn must promote on the last rank, nothing else may promote
		return false
	}

	// Check that the moved piece is allowed to move that way: some pattern must accept the move and be unobstructed.
	// Every pattern is tried because a piece could have more than one pattern which reaches teh same square.
	followed := false
	for _, pattern := range m.piece.pieceType.movementPatterns {
		if !pattern.evaluate(*m.From, *m.To, s) {
			continue
		}
		if (pattern.captures == moveOnly && m.capturing) || (pattern.captures == captureOnly && !m.capturing) {
			continue
		}
		if !s.pathClear(pattern.squaresInPath(*m.From, *m.To)) { // Another piece is in the way
			continue
		}
		if pattern.unattacked != nil && s.anyAttacked(pattern.unattacked(*m.From, *m.To), m.color.opponent()) {
			continue
		}
		followed = true
		break
	}
	if !followed {
		return false
	}
	// The mover's king may not be attacked once the move is made. This covers moving into check, ignoring check, and pins
	return !s.makeMove(m).inCheck(m.color)
}

// Remove the piece on a square, if any
func (s *State) removePieceAt(rank uint8, file uint8) {
	s.pieces = slices.DeleteFunc(s.pieces, func(piece *Piece) bool {
		return piece.position.rank == rank && piece.position.file == file
	})
}

// Returns the State after the move is played, without recording history. Does not validate: see isLegal.
// Never modifies s, so it is safe to use for testing whether a move would be legal
func (s State) makeMove(m Move) State {
	next := s.deepCopy()
	piece := next.pieceAt(m.From.rank, m.From.file)

	// Capture. An en passant capture removes the pawn beside the mover instead of the piece on the target square
	capturedRank := m.To.rank
	if s.isEnPassant(m) {
		capturedRank = m.From.rank
	}
	next.removePieceAt(capturedRank, m.To.file)

	if piece.pieceType.name == kingType.name && absDiff(m.From.file, m.To.file) == 2 { // Castling
		rookFrom, rookTo := uint8(1), m.To.file+1 // Queenside a -> d
		if m.To.file > m.From.file {
			rookFrom, rookTo = squaresPerRank, m.To.file-1 // Kingside h -> f
		}
		if rook := next.pieceAt(m.From.rank, rookFrom); rook != nil {
			rook.position = &Square{m.From.rank, rookTo, nil}
		}
	}

	// Move the piece, promoting it if needed
	piece.position = &Square{m.To.rank, m.To.file, nil}
	if m.Promotion != nil {
		piece.pieceType = *m.Promotion
	}

	// Castling rights are lost when the king moves, or when a rook moves from or is captured on its corner
	if piece.pieceType.name == kingType.name {
		next.castling &^= m.color.castleRight(true) | m.color.castleRight(false) // &^= is the bitwise AND NOT assignment operator
	}
	for _, square := range []*Square{m.From, m.To} {
		switch {
		case square.rank == White.backrank && square.file == 1:
			next.castling &^= castleWhiteQueenside
		case square.rank == White.backrank && square.file == squaresPerRank:
			next.castling &^= castleWhiteKingside
		case square.rank == Black.backrank && square.file == 1:
			next.castling &^= castleBlackQueenside
		case square.rank == Black.backrank && square.file == squaresPerRank:
			next.castling &^= castleBlackKingside
		}
	}

	// A pawn moving two squares leaves the square it skipped open to en passant for one turn
	next.enPassant = nil
	if piece.pieceType.name == pawnType.name && absDiff(m.From.rank, m.To.rank) == 2 { // en passant
		next.enPassant = &Square{(m.From.rank + m.To.rank) / 2, m.From.file, nil}
	}

	// Adjust counters
	next.halfmove++
	if piece.pieceType.name == pawnType.name || m.capturing {
		next.halfmove = 0
	}
	// Increment fullmove after black's turn
	if m.color == Black {
		next.fullmove++
	}
	next.turn = m.color.opponent()

	next.update()
	next.activeDrawOffer = false // Always reset draw offers when not accepted
	return next
}

// Returns the GameState after action is applied. Must not mutate in-place: see GameState. Should not do its own validation: see isLegal
func (s State) apply(m Move) State {
	if m.isDrawResponse() { // Accepts the offer after validation, no change to board
		next := s.deepCopy()
		next.drawAccepted = s.activeDrawOffer // Validate rejects invalid answers
		return next
	}
	next := s.makeMove(m)
	next.activeDrawOffer = m.offeringDraw
	// Clip so that append copies. States which share a history slice would otherwise overwrite each other's entries
	next.stateHistory = append(slices.Clip(s.stateHistory), next.positionKey())
	next.moveHistory = append(slices.Clip(s.moveHistory), m)
	return next
}

// Get the color a player controls
func (s State) playerColor(id string) (Color, bool) {
	switch id {
	case s.whitePlayer:
		return White, true
	case s.blackPlayer:
		return Black, true
	default:
		return Color{}, false
	}
}

// Get the ID of the player whose turn it is
func (s State) activePlayer() string {
	return s.playerID(s.turn)
}

// Whether no checkmate is possible: king against king, king and one minor piece against king, or only bishops which stand on same color squares
func (s State) insufficientMaterial() bool {
	knights, bishops := 0, 0
	shades := map[string]bool{}
	for _, piece := range s.pieces {
		switch piece.pieceType.name {
		case kingType.name:
		case knightType.name:
			knights++
		case bishopType.name:
			bishops++
			shades[piece.position.Shade()] = true
		default: // All other types (pawn, rook, queen, etc) can always eventually checkmate
			return false
		}
	}
	return knights+bishops <= 1 || (knights == 0 && len(shades) == 1)
}

// Result of a draw, which has no winner or loser
func (s State) drawResult(reason game.TerminationReason) (game.MatchResult, bool) {
	return ChessResult{game.Result{Reason: reason}, nil, s}, true
}

// Check for win conditions. The expensive part, generating the moves of the player to move, is done exactly once
// TODO: Check whether a player resigned.
func (s State) outcome() (game.MatchResult, bool) {
	// A player with no legal moves has been checkmated, stalemated, or has no pieces remaining
	// This has priority over automatic draws
	if len(s.legalActionSpecs(s.activePlayer())) == 0 {
		if s.inCheck(s.turn) || len(s.piecesOf(s.turn)) == 0 { // Checkmate or no more pieces
			loser := s.activePlayer()
			winner := s.playerID(s.turn.opponent())
			return ChessResult{game.Result{WinnerIDs: []string{winner}, Reason: ReasonCheckmate}, &loser, s}, true
		}
		return s.drawResult(ReasonStalemate)
	}

	// Check whether a draw was accepted
	if s.drawAccepted {
		return s.drawResult(ReasonDrawOffer)
	}

	// Check whether players lack sufficient material
	if s.insufficientMaterial() {
		return s.drawResult(ReasonInsufficientMaterial)
	}

	// Check whether we reached a move limit for draw enforcement
	if s.halfmove >= 150 { // 75-move limit (150 halfmoves) forces a draw
		return s.drawResult(ReasonAutomaticDraw)
	} else if s.halfmove >= 100 { // 50-move limit (100 halfmoves) mean draw offers are automatically accepted
		if s.activeDrawOffer {
			return s.drawResult(ReasonDrawOffer)
		}
	}

	// Check whether we reached a repetition limit for draw enforcement
	repetitions := 0
	for _, key := range s.stateHistory {
		if key == s.stateHistory[len(s.stateHistory)-1] {
			repetitions++
		}
	}
	if repetitions >= 5 { // Arbiter forces a draw for fivefold repetition
		return s.drawResult(ReasonAutomaticDraw)
	} else if repetitions >= 3 { // Draw offer is automatically accepted for threefold repetition
		if s.activeDrawOffer {
			return s.drawResult(ReasonDrawOffer)
		}
	}

	// Game is not complete
	return ChessResult{}, false
}

// Return an array of all the current legal actions for a player with the given id in UCI. Only the player to move has any
func (s State) legalActionSpecs(id string) []game.ActionSpec {
	result := []game.ActionSpec{}
	color, ok := s.playerColor(id)
	if !ok || color != s.turn {
		return result
	}
	for _, piece := range s.pieces {
		if piece.color != color {
			continue // Skip moves for the opponent's pieces
		}
		// Evaludate whether each targetSquare on the board is possible
		for _, targetSquare := range s.board {
			if piece.position == targetSquare {
				continue
			}
			// Pawns which reach the last rank get one move for each piece they may promote to
			promotions := []*PieceType{nil}
			if piece.pieceType.canPromote && targetSquare.rank == color.promotionRank {
				promotions = promotions[:0]
				for i := range promotionTypes {
					promotions = append(promotions, &promotionTypes[i])
				}
			}
			for _, promotion := range promotions {
				// Create the possible move and validate it
				move, err := s.resolve(Move{From: piece.position, To: targetSquare, Promotion: promotion})
				if err != nil || !s.isLegal(move) {
					continue // The move is not allowed
				}
				description := fmt.Sprintf("%s %s moves from %s to %s", s.turn.name, piece.pieceType.name, move.From.toUCI(), move.To.toUCI())
				if move.capturing {
					description = fmt.Sprintf("%s %s moves to capture from %s to %s", s.turn.name, piece.pieceType.name, move.From.toUCI(), move.To.toUCI())
				}
				if promotion != nil {
					description += " and promotes to " + promotion.name
				}
				result = append(result, game.ActionSpec{Payload: move.Encode(), Description: description})
			}
		}
	}
	// Any action can have a draw offer appended to it, but this is not returned as part of ActionSpecs.
	return result
}

// First four fields of the FEN string: piece placement, active color, castling rights, and en passant target.
// For repetition, two states with the same key are the same position
func (s State) positionKey() string {
	separator := " "     // What goes between the parts of the FEN string
	rankSeparator := "/" // What goes between each rank in the piece placement field (but not after the last rank)
	noneSpecifier := "-" // What's used when no value is present for a field

	var result strings.Builder
	// Piece placement. FEN lists rank 8 first and rank 1 last
	for rank := squaresPerFile; rank >= 1; rank-- {
		first := (rank - 1) * squaresPerRank
		result.WriteString(rankFEN([squaresPerRank]*Square(s.board[first : first+squaresPerRank])))
		if rank > 1 {
			result.WriteString(rankSeparator)
		}
	}
	result.WriteString(separator)
	result.WriteRune(s.turn.symbol)
	result.WriteString(separator)
	result.WriteString(s.CastlingRights())
	result.WriteString(separator)
	if s.enPassant != nil {
		result.WriteString(s.enPassant.toUCI())
	} else {
		result.WriteString(noneSpecifier)
	}
	return result.String()
}

// FEN representation of the board state which can be passed to clients
func (s State) ToFEN() string {
	return s.positionKey() + " " + strconv.Itoa(s.halfmove) + " " + strconv.Itoa(s.fullmove)
}

// FEN representation of one rank, from the a file to the h file
func rankFEN(rank [squaresPerRank]*Square) string {
	var result []rune
	contiguousNulls := 0
	for _, square := range rank {
		if square.occupant != nil {
			if contiguousNulls > 0 {
				result = append(result, rune('0'+contiguousNulls)) // runes are int32
				contiguousNulls = 0
			}
			result = append(result, square.occupant.Symbol())
		} else {
			contiguousNulls++
		}
	}
	// Add number for null entries at the end
	if contiguousNulls > 0 {
		result = append(result, rune('0'+contiguousNulls))
	}
	// Should be like `1k5R` instead of `_k_____R`
	return string(result) // Automatic conversion of rune slices to strings <3
}

// FEN string representation of castling rights, as "KQkq"
func (s State) CastlingRights() string {
	result := ""
	// KQkq bit flags: 0000KQkq
	if s.castling&castleWhiteKingside != 0 { // 00001___
		result += "K"
	}
	if s.castling&castleWhiteQueenside != 0 { // 0000_1__
		result += "Q"
	}
	if s.castling&castleBlackKingside != 0 { // 0000__1_
		result += "k"
	}
	if s.castling&castleBlackQueenside != 0 { // 0000___1
		result += "q"
	}
	if result == "" { // 00000000
		result = "-"
	}
	return result
}
