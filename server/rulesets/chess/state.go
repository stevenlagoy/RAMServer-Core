package chess

import (
	"fmt"

	"github.com/stevenlagoy/ramserver-core/server/game"
)

const squaresPerRank = 8
const squaresPerFile = 8
const maxPiecesPerPlayer = 16

// If Rank and File becomes confusing, consider switching to "BoardRank" and "BoardFile"
var Files = map[uint8]rune{1: 'a', 2: 'b', 3: 'c', 4: 'd', 5: 'e', 6: 'f', 7: 'g', 8: 'h'}

// One space on the chessboard
type Square struct {
	rank uint8 // 1, 2, 3, 4, 5, 6, 7, 8
	file uint8 // a, b, c, d, e, f, g, h
	// Square previously had `resident *Piece` but this was removed. Could be readded for better performance, but requires special initialization logic.
}

// Get the shade of a square, "light" or "dark"
func (s Square) Shade() string {
	if s.rank%2+s.file%2 == 0 {
		return "light"
	} else {
		return "dark"
	}
}

// Get the square as a uint8, with the rank in the top four bits and the file in the bottom four bits
func (s Square) toUint8() uint8 {
	return (s.rank << 4) | s.file
}

// Get the square's location as a UCI string (also used for SAN)
func (s Square) toUCI() string {
	return fmt.Sprintf("%c%v", s.rank, Files[s.file]) // Sprintf formats but doesn't print. Dunno why it's got 'print' in the signature then...
}

func makeSquares() [squaresPerRank * squaresPerFile]*Square {
	squares := [squaresPerRank * squaresPerFile]*Square{}
	for i := range squaresPerRank * squaresPerFile {
		rank := uint8(i / squaresPerRank)
		file := uint8(i % squaresPerFile)
		squares[i] = &Square{rank, file}
	}
	return squares
}

// State of the chess gameboard at one moment
type State struct {
	board        [squaresPerRank * squaresPerFile]*Square // array not slice, copied by value
	pieces       [maxPiecesPerPlayer * 2]*Piece           // Captured pieces are removed, promoting pieces are removed and readded as new piece
	turn         Color                                    // whose turn is it currently?
	castling     uint8                                    // KQkq bit flags: 0000KQkq
	whiteInCheck bool                                     // is white's king in check?
	blackInCheck bool                                     // is black's king in check?
	enPassant    *Square                                  // target square or nil
	halfmove     int                                      // number of halfmoves: increments every action, reset to 0 if a pawn moves or a capture
	fullmove     int                                      // number of fullmoves: increment after every black turn
	whitePlayer  string                                   // playerID bound at NewState
	blackPlayer  string                                   // playerID bound at NewState
	stateHistory []State                                  // state history: use for threefold repetition
	moveHistory  []Move                                   // move history
}

// TODO: Check that this returns a state which gives the correct FEN string
func newState(whitePlayer string, blackPlayer string) State {
	return State{
		makeSquares(),
		makePieces(),
		White,
		0x00,
		false,
		false,
		nil,
		0,
		0,
		whitePlayer,
		blackPlayer,
		[]State{},
		[]Move{},
	}
}

func stateFromFEN(FEN string, whitePlayer string, blackPlayer string) State {
	// TODO: unpack state from FEN string. Assume this is a new board with no history
}

// TODO: Use heuristics to reduce the computation needed here. This method will be run a LOT
func (s State) isLegal(m Move) bool {

	// Make a view of the state that would exist after this action
	// This is why its important that apply does not modify in-place
	prospectiveState := s.apply(m)

	// Check that we aren't doing threefold repetation
	prospectiveStateFEN := prospectiveState.ToFEN()
	duplicates := 0
	for _, state := range prospectiveState.stateHistory {
		if state.ToFEN() == prospectiveStateFEN {
			duplicates++
		}
	}
	if duplicates > 2 { // Threefold repetition
		return false
	} // Acceptable amounts of repetation

	// Check that the moved piece is allowed to move that way
	var movementPattern *MovementPattern = nil
	for _, pattern := range m.piece.pieceType.movementPatterns {
		if pattern.evaluate(*m.From, *m.To, m.capturing) {
			movementPattern = &pattern
			break
		}
	}
	if movementPattern == nil { // Piece has no movement pattern allowing this move
		return false
	} // This piece is allowed to move this way

	// Check that no other pieces were blocking this movement
	for _, square := range movementPattern.squaresInPath(*m.From, *m.To) {
		// n^3 again. This would be a good place for square.resident
		for _, piece := range s.pieces {
			if piece.position.rank == square.rank && piece.position.file == square.file {
				if piece.position == m.piece.position { // That's ourselves, which is okay
					continue
				} else if square == *m.To && m.capturing && piece.color != m.color { // We intended to do this and are capturing the overlapped opponent piece
					continue
				} else { // We overlapped a piece which we aren't capturing
					// TODO: is it possible that another movement pattern would permit this action?
					// Leave a comment about this, even if its not possible in chess
					return false
				}
			}
		}
	} // There were no overlapping pieces which block this movement

	// Check that we are not currently in check, or if we are that this move brings us out of check
	if m.color == White {
		if s.whiteInCheck {
			// One approach: make a duplicate board, apply the change to it (skipping validation), and determine if check was broken
		}
	} else if m.color == Black {
		if s.blackInCheck {
			// One approach: make a duplicate board, apply the change to it (skipping validation), and determine if check was broken
		}
	} // There was no check, or this move breaks check

	return true
}

// Returns the GameState after action is applied. Must not mutate in-place
// (see GameState). Should not do its own validation.
// TODO: Create a NEW!!!! state.
func (s State) apply(m Move) State {
	// Determine whether we did a real action or just offered a draw
	if m.To != nil { // Did a real action

		// TODO: Deep copy s
		newState := nil

		// Apply the move

		// Add history
		// TODO: Check that this actually works once Deep Copy is implemented
		newState.stateHistory = s.stateHistory + s
		newState.moveHistory = s.moveHistory + m

		// Adjust counters
		// Adjust halfmove
		newState.halfmove++
		if m.piece.pieceType.name == pawnType.name || m.capturing {
			newState.halfmove = 0
		}
		// Increment fullmove after black's turn
		if m.color == Black {
			newState.fullmove++
		}
	} else { // Draw offer with no piece move
		// TODO: Deep copy s
	}
}

func (s State) activePlayer() string {
	return s.turn.name
}

// TODO: Check for win conditions. Use heuristics to reduce expected computation as much as possible. This will be run every turn
func (s State) outcome() (game.MatchResult, bool) {
	// Check whether we passed the 75-halfmove limit for automatic arbiter draw enforcement
	if s.halfmove > 75 { // 150-ply: automatic draw
		return ChessResult{game.Result{WinnerIDs: nil, Scores: nil, Reason: ReasonAutomaticDraw}, nil, s}, true
	}

	// TODO: if s.halfmove > 100 (50-ply) and one of the players offers a draw, the other player automatically accepts it

	// TODO: Check whether a player resigned

	// TODO: Check whether a draw was accepted

	// Check if either player is checkmated
	if s.whiteInCheck && len(s.legalActionSpecs(s.whitePlayer)) == 0 { // White is checkmated
		return ChessResult{game.Result{WinnerIDs: []string{s.blackPlayer}, Scores: nil, Reason: ReasonCheckmate}, &s.whitePlayer, s}, true
	}
	if s.blackInCheck { // Black is checkmated
		return ChessResult{game.Result{WinnerIDs: []string{s.whitePlayer}, Scores: nil, Reason: ReasonCheckmate}, &s.blackPlayer, s}, true
	}
	// Check for stalemate
	if s.turn == White {
		if !s.whiteInCheck && len(s.legalActionSpecs(s.whitePlayer)) == 0 { // No legal moves: Stalemate!
			return ChessResult{game.Result{WinnerIDs: nil, Scores: nil, Reason: ReasonStalemate}, nil, s}, true
		}
	}
	if s.turn == Black {
		if !s.blackInCheck && len(s.legalActionSpecs(s.blackPlayer)) == 0 { // No legal moves: Stalemate!
			return ChessResult{game.Result{WinnerIDs: nil, Scores: nil, Reason: ReasonStalemate}, nil, s}, true
		}
	}

	// Check whether players lack sufficient material
	if len(s.pieces) == 2 { // Each player only has king
		// Insufficient material!
		return ChessResult{game.Result{WinnerIDs: nil, Scores: nil, Reason: ReasonInsufficientMaterial}, nil, s}, true
	}
	if len(s.pieces) < 5 { // possible lack of material
		// Sort the pieces by owner
		whitePieces := make([]Piece, len(s.pieces))
		blackPieces := make([]Piece, len(s.pieces))
		for _, piece := range s.pieces {
			if piece.color == White {
				whitePieces = append(whitePieces, *piece)
			} else if piece.color == Black {
				blackPieces = append(blackPieces, *piece)
			}
		}
		// Check for lack of material cases
		if len(whitePieces) > 1 && len(blackPieces) > 1 { // Yikes the never-nesters would kill me. Can we do this whole block any better?
			for _, piece := range whitePieces {
				if piece.pieceType.name == bishopType.name { // White has a king and bishop
					for _, opponentPiece := range blackPieces {
						if opponentPiece.pieceType.name == bishopType.name { // Black has a king and bishop
							if piece.position.Shade() == opponentPiece.position.Shade() { // The bishops are on the same color squares
								// Insufficient material!
								return ChessResult{game.Result{WinnerIDs: nil, Scores: nil, Reason: ReasonInsufficientMaterial}, nil, s}, true
							}
						}
					}
				}
			}
		} else if len(whitePieces) == 1 { // White has only king left
			for _, piece := range blackPieces {
				if piece.pieceType.name == bishopType.name { // Black has a king and bishop
					// Insufficient material!
					return ChessResult{game.Result{WinnerIDs: nil, Scores: nil, Reason: ReasonInsufficientMaterial}, nil, s}, true
				}
				if piece.pieceType.name == knightType.name { // Black has a king and knight
					// Insufficient material!
					return ChessResult{game.Result{WinnerIDs: nil, Scores: nil, Reason: ReasonInsufficientMaterial}, nil, s}, true
				}
			}
		} else if len(blackPieces) == 1 { // Black has only king left
			for _, piece := range whitePieces {
				if piece.pieceType.name == bishopType.name { // White has a king and bishop
					// Insufficient material!
					return ChessResult{game.Result{WinnerIDs: nil, Scores: nil, Reason: ReasonInsufficientMaterial}, nil, s}, true
				}
				if piece.pieceType.name == knightType.name { // White has a king and knight
					// Insufficient material!
					return ChessResult{game.Result{WinnerIDs: nil, Scores: nil, Reason: ReasonInsufficientMaterial}, nil, s}, true
				}
			}
		}
	}

	return ChessResult{}, false
}

func (s State) view() State {
	return s // No hidden state, can see the full board state and history
}

// TODO: Return an array of all the current legal actions for a player with the given id
func (s State) legalActionSpecs(id string) []game.ActionSpec {
	return nil
}

// FEN representation of the board state to be passed to clients
func (s State) ToFEN() string {
	separator := " " // What goes between the parts of the FEN string
	result := ""
	// Piece placement
	// Collect all the pieces separated by rank
	var ranksPieces [squaresPerFile][]*Piece
	for _, piece := range s.pieces {
		ranksPieces[piece.position.rank] = append(ranksPieces[piece.position.rank], piece)
	}
	// Turn each rank into its FEN string
	for rank := range len(s.board) / squaresPerFile {
		squaresInRank := s.board[rank*squaresPerFile : (rank+1)*squaresPerFile]
		piecesInRank := ranksPieces[rank]
		rankFEN := rankFEN([8]*Square(squaresInRank), piecesInRank)
		result += rankFEN
	}
	result += separator
	result += string(s.turn.symbol)
	result += separator
	result += s.CastlingRights()
	result += separator
	result += s.enPassant.toUCI()
	result += separator
	result += string(s.halfmove)
	result += separator
	result += string(s.fullmove)
	return result
}

// FEN representation of one rank
func rankFEN(rank [squaresPerRank]*Square, pieces []*Piece) string {
	symbols := [squaresPerRank]rune{}
	// Hmm n^3. n^2 if passed pieces are all on this rank, so the caller should filter them first
	for i, square := range rank {
		for _, piece := range pieces {
			if piece.position.rank == square.rank && piece.position.file == square.file {
				symbols[i] = piece.Symbol()
			}
		}
	}
	// Loop through result and add counts for null entries
	var result []rune
	contiguousNulls := 0
	for _, r := range symbols {
		if r != 0 { // Not a null character
			if contiguousNulls > 0 {
				result = append(result, rune('0'+contiguousNulls)) // runes are int32
			}
			result = append(result, r)
		} else {
			contiguousNulls++
		}
	}
	// Should be like `1k5R` instead of `_k_____R`
	return string(result[:]) // Automatic conversion of rune slices to strings <3
}

// FEN string representation of castling rights, as "KQkq"
func (s State) CastlingRights() string {
	result := ""
	// KQkq bit flags: 0000KQkq
	if s.castling>>3&1 != 0 { // 00001___
		result += "K"
	}
	if s.castling>>2&1 != 0 { // 0000_1__
		result += "Q"
	}
	if s.castling>>1&1 != 0 { // 0000__1_
		result += "k"
	}
	if s.castling>>0&1 != 0 { // 0000___1
		result += "q"
	}
	if result == "" { // 00000000
		result = "-"
	}
	return result
}
