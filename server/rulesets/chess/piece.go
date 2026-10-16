package chess

import (
	"unicode"
)

type Color struct {
	name              string
	symbol            rune
	movementDirection int // 1 if moving forwards is upwards (white), -1 if moving forwards is downwards (black)
	firstToMove       bool
	backrank          uint8 // The rank where this player's rook, knight, bishop, queen, and king start
	pawnrank          uint8 // The rank where this player's pawns start
	kingfile          uint8 // The file where this player's king starts
	queenfile         uint8 // The file where this player's queen starts
	promotionRank     uint8 // The rank on which this player's pawns may promote
}

var (
	White = Color{"white", 'w', 1, true, 1, 2, 5, 4, 8}
	Black = Color{"black", 'b', -1, false, 8, 7, 5, 4, 1}
)

// Get the other player's color
func (c Color) opponent() Color {
	if c == White {
		return Black
	}
	return White
}

// Castling rights as bit flags: 0000KQkq
const (
	castleWhiteKingside  uint8 = 1 << 3
	castleWhiteQueenside uint8 = 1 << 2
	castleBlackKingside  uint8 = 1 << 1
	castleBlackQueenside uint8 = 1 << 0
	castleAll                  = castleWhiteKingside | castleWhiteQueenside | castleBlackKingside | castleBlackQueenside
)

// Get the castling right flag which lets this color castle on the given side
func (c Color) castleRight(kingside bool) uint8 {
	switch {
	case c == White && kingside:
		return castleWhiteKingside
	case c == White:
		return castleWhiteQueenside
	case c == Black && kingside:
		return castleBlackKingside
	default:
		return castleBlackQueenside
	}
}

// Whether a movement pattern may capture, must capture, or must not capture
type captureRule uint8

const (
	moveOrCapture captureRule = iota // Target square may be empty or hold an opponent piece
	moveOnly                         // Target square must be empty (pawn pushes)
	captureOnly                      // Target square must hold an opponent piece or be the en passant square (pawn captures)
)

type MovementPattern struct {
	name     string
	captures captureRule // Whether this pattern moves, captures, or does either
	// Evaluate whether a piece following this movement pattern is allowed to make the given move.
	// Only the geometry and the pattern's preconditions (like castling rights) are checked here.
	// The caller checks the squares in the path and the target square. from.occupant must be the moving piece
	evaluate func(from Square, to Square, stateBefore State) bool
	// Get the squares strictly between from and to which must be empty for this pattern to be followed.
	// Only called for squares which evaluate accepted.
	squaresInPath func(from Square, to Square) []Square
	// Get the squares which must not be attacked by the opponent for the move to be legal.
	// Only castling needs this, so may be nil.
	unattacked func(from Square, to Square) []Square
}

var (
	// Leftwards or rightwards along one rank
	horizontal = MovementPattern{
		name:          "horizontal",
		evaluate:      func(from Square, to Square, _ State) bool { return from.rank == to.rank },
		squaresInPath: squaresBetween,
	}
	// Forwards or backwards along one file
	vertical = MovementPattern{
		name:          "vertical",
		evaluate:      func(from Square, to Square, _ State) bool { return from.file == to.file },
		squaresInPath: squaresBetween,
	}
	// Principal or minor diagonal
	diagonal = MovementPattern{
		name: "diagonal",
		evaluate: func(from Square, to Square, _ State) bool {
			return absDiff(from.file, to.file) == absDiff(from.rank, to.rank)
		},
		squaresInPath: squaresBetween,
	}
	forwardAdjacent = MovementPattern{
		name:     "forward adjacent",
		captures: moveOnly, // Pawns cannot capture straight ahead. The king ignores this: see anyAdjacent
		evaluate: func(from Square, to Square, _ State) bool {
			return from.file == to.file && int(from.rank) == int(to.rank)-from.occupant.color.movementDirection
		},
		squaresInPath: noSquares,
	}
	forwardTwiceFirstMove = MovementPattern{
		name:     "forward two squares on the first move",
		captures: moveOnly,
		evaluate: func(from Square, to Square, s State) bool {
			return (from.file == to.file && // No sidewards deviation
				int(from.rank) == int(to.rank)-(from.occupant.color.movementDirection*2) && // Correct number of squares in the correct direction
				from.rank == from.occupant.color.pawnrank) // It is this piece's first move
		},
		squaresInPath: squaresBetween, // Skipped square must be empty
	}
	sidewaysAdjacent = MovementPattern{
		name: "sideways adjacent",
		evaluate: func(from Square, to Square, _ State) bool {
			return from.rank == to.rank && absDiff(from.file, to.file) == 1
		},
		squaresInPath: noSquares,
	}
	backwardAdjacent = MovementPattern{
		name: "backward adjacent",
		evaluate: func(from Square, to Square, _ State) bool {
			return from.file == to.file && int(from.rank) == int(to.rank)+from.occupant.color.movementDirection
		},
		squaresInPath: noSquares,
	}
	diagonalAdjacent = MovementPattern{
		name: "diagonal adjacent",
		evaluate: func(from Square, to Square, _ State) bool {
			return absDiff(from.file, to.file) == 1 && absDiff(from.rank, to.rank) == 1
		},
		squaresInPath: noSquares,
	}
	anyAdjacent = MovementPattern{
		name: "any adjacent",
		evaluate: func(from Square, to Square, s State) bool {
			return (forwardAdjacent.evaluate(from, to, s) ||
				sidewaysAdjacent.evaluate(from, to, s) ||
				backwardAdjacent.evaluate(from, to, s) ||
				diagonalAdjacent.evaluate(from, to, s))
		},
		squaresInPath: noSquares,
	}
	knightsMove = MovementPattern{
		name: "knight's move",
		evaluate: func(from Square, to Square, _ State) bool {
			possibilities := [...][2]int{
				// Rank difference, File difference
				//   H   A
				// G       B
				//     *
				// F       C
				//   E   D
				{2, 1},   // A
				{1, 2},   // B
				{-1, 2},  // C
				{-2, 1},  // D
				{-2, -1}, // E
				{-1, -2}, // F
				{1, -2},  // G
				{2, -1},  // H
			}
			rankDiff := int(to.rank) - int(from.rank)
			fileDiff := int(to.file) - int(from.file)
			for _, possibility := range possibilities {
				if possibility == [2]int{rankDiff, fileDiff} {
					return true
				}
			}
			return false
		},
		squaresInPath: noSquares, // Knights jump
	}
	forwardDiagonalAdjacentToCapture = MovementPattern{
		name:     "forward diagonal adjacent to capture",
		captures: captureOnly,
		evaluate: func(from Square, to Square, stateBefore State) bool {
			return absDiff(from.file, to.file) == 1 && int(to.rank)-int(from.rank) == from.occupant.color.movementDirection
		},
		squaresInPath: noSquares,
	}
	kingsideCastle  = castlingPattern("kingside castle", true)   // King's move, not the rook's
	queensideCastle = castlingPattern("queenside castle", false) // King's move, not the rook's
)

// Build the pattern for the king's move when castling on one side. The king moves two squares towards its rook
// and the rook jumps to the other side of the king. Castling is only evaluated as the king's move
func castlingPattern(name string, kingside bool) MovementPattern {
	rookFile, step := uint8(1), -1 // Queenside: rook on the a file, king moves toward it
	if kingside {
		rookFile, step = squaresPerRank, 1
	}
	return MovementPattern{
		name: name,
		evaluate: func(from Square, to Square, stateBefore State) bool {
			color := from.occupant.color
			if from.rank != color.backrank || from.file != color.kingfile { // King is not on its starting square
				return false
			}
			if to.rank != from.rank || int(to.file) != int(from.file)+2*step {
				return false
			}
			if stateBefore.castling&color.castleRight(kingside) == 0 { // Right was lost by moving the king or rook, or never granted
				return false
			}
			rook := stateBefore.pieceAt(from.rank, rookFile)
			return rook != nil && rook.color == color && rook.pieceType.name == rookType.name
		},
		// Everything between the king and rook must be empty. The rook may pass through attacked squares, the king may not
		squaresInPath: func(from Square, to Square) []Square {
			return squaresBetween(from, Square{rank: from.rank, file: rookFile})
		},
		// King may not castle out of check, through check, or into check
		unattacked: func(from Square, to Square) []Square {
			return []Square{from, {rank: from.rank, file: uint8(int(from.file) + step)}, to}
		},
	}
}

type PieceType struct {
	name             string            // Name of the piece. This should be used for equality checking
	movementPatterns []MovementPattern // Movement patterns this piece can follow
	symbol           rune              // Caller must always determine case: see Piece.Symbol() for black / white casing
	canPromote       bool              // is this piece type able to promote? (pawns)
}

var (
	kingType      = PieceType{"king", []MovementPattern{anyAdjacent, kingsideCastle, queensideCastle}, 'K', false}
	queenType     = PieceType{"queen", []MovementPattern{horizontal, vertical, diagonal}, 'Q', false}
	bishopType    = PieceType{"bishop", []MovementPattern{diagonal}, 'B', false}
	knightType    = PieceType{"knight", []MovementPattern{knightsMove}, 'N', false}
	rookType      = PieceType{"rook", []MovementPattern{horizontal, vertical}, 'R', false}
	pawnType      = PieceType{"pawn", []MovementPattern{forwardAdjacent, forwardDiagonalAdjacentToCapture, forwardTwiceFirstMove}, 'P', true} // Pawns use P/p in FEN, but have no symbol in SAN or UCI
	allPieceTypes = []PieceType{kingType, queenType, bishopType, knightType, rookType, pawnType}
	// Piece types a pawn may promote to (king and pawn not allowed)
	promotionTypes = []PieceType{queenType, rookType, bishopType, knightType}
)

type Piece struct {
	pieceType PieceType
	color     Color
	position  *Square
}

// Get the piece's symbol, uppercase if the piece is white and lowercase if the piece is black
func (p Piece) Symbol() rune {
	switch p.color {
	case White:
		return unicode.ToUpper(p.pieceType.symbol)
	case Black:
		return unicode.ToLower(p.pieceType.symbol)
	default:
		return p.pieceType.symbol // Unreachable
	}
}

// Find the piece type and color a FEN symbol stands for. Uppercase symbols are white and lowercase are black
func pieceFromSymbol(symbol rune) (PieceType, Color, bool) {
	color := Black
	if unicode.IsUpper(symbol) {
		color = White
	}
	for _, pieceType := range allPieceTypes {
		if pieceType.symbol == unicode.ToUpper(symbol) {
			return pieceType, color, true
		}
	}
	return PieceType{}, Color{}, false
}

func makePieces() []*Piece {
	pieces := make([]*Piece, 0, maxPiecesPerPlayer*2)
	for _, color := range []Color{White, Black} {
		// Backrank
		pieces = append(pieces, &Piece{rookType, color, &Square{color.backrank, 1, nil}})
		pieces = append(pieces, &Piece{knightType, color, &Square{color.backrank, 2, nil}})
		pieces = append(pieces, &Piece{bishopType, color, &Square{color.backrank, 3, nil}})
		pieces = append(pieces, &Piece{kingType, color, &Square{color.backrank, color.kingfile, nil}})
		pieces = append(pieces, &Piece{queenType, color, &Square{color.backrank, color.queenfile, nil}})
		pieces = append(pieces, &Piece{bishopType, color, &Square{color.backrank, 6, nil}})
		pieces = append(pieces, &Piece{knightType, color, &Square{color.backrank, 7, nil}})
		pieces = append(pieces, &Piece{rookType, color, &Square{color.backrank, 8, nil}})
		// Pawns
		for file := uint8(1); file <= squaresPerRank; file++ {
			pieces = append(pieces, &Piece{pawnType, color, &Square{color.pawnrank, file, nil}})
		}
	}
	return pieces
}
