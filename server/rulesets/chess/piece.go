package chess

import "unicode"

type Color struct {
	name        string
	symbol      rune
	firstToMove bool
}

var (
	White = Color{"white", 'w', true}
	Black = Color{"black", 'b', false}
)

type MovementPattern struct {
	name string
	// Evaluate whether a piece following this movement pattern is allowed to make the given move
	evaluate func(Square, Square, bool) bool
	// Get an array of squares along the path between squares while following this movement pattern. Should include the From and To squares.
	squaresInPath func(Square, Square) []Square
}

// TODO: finish movement patterns' evaluate logic. This is tricky because there's unique context for some movesets
// TODO: IE knights are able to jump over pieces, and kings may castle only if neither they nor their rooks have moved (the board state already tracks castleRights)
var (
	// Leftwards or rightwards along one rank
	horizontal = MovementPattern{"horizontal", func(from Square, to Square, _ bool) bool { return from.rank == to.rank }}
	// Forwards or backwards along one file
	vertical = MovementPattern{"vertical", func(from Square, to Square, _ bool) bool { return from.file == to.file }}
	// Principal or minor diagonal
	diagonal = MovementPattern{"diagonal", func(from Square, to Square, _ bool) bool {
		return from.file-to.file == from.rank-to.rank || to.file-from.file == from.rank-to.rank
	}}
	forwardAdjacent                  = MovementPattern{"forward adjacent", func(from Square, to Square, _ bool) bool { return from.file == to.file && from.rank == to.rank-1 }}
	sidewaysAdjacent                 = MovementPattern{"sideways adjacent", func(from Square, to Square, _ bool) bool { return false }}
	backwardAdjacent                 = MovementPattern{"backward adjacent", func(from Square, to Square, _ bool) bool { return from.file == to.file && from.rank == to.rank+1 }}
	diagonalAdjacent                 = MovementPattern{"diagonal adjacent", func(from Square, to Square, _ bool) bool { return false }}
	knightsMove                      = MovementPattern{"knight's move", func(from Square, to Square, _ bool) bool { return false }}
	forwardDiagonalAdjacentToCapture = MovementPattern{"forward diagonal adjacent to capture", func(from Square, to Square, capturing bool) bool { return false }}
	kingsideCastle                   = MovementPattern{"kingside castle", func(from Square, to Square, _ bool) bool { return false }} // This needs more context for rooks and whether the rooks have moved
)

type PieceType struct {
	name             string            // Name of the piece. This should be used for equality checking
	movementPatterns []MovementPattern // Movement patterns this piece can follow
	symbol           rune              // Caller must always determine case: see Piece.Symbol() for black / white casing
	canPromote       bool              // is this piece type able to promote? (pawns)
}

var (
	kingType   = PieceType{"king", []MovementPattern{forwardAdjacent, sidewaysAdjacent, backwardAdjacent, diagonalAdjacent}, 'K', false}
	queenType  = PieceType{"queen", []MovementPattern{horizontal, vertical, diagonal}, 'Q', false}
	bishopType = PieceType{"bishop", []MovementPattern{diagonal}, 'B', false}
	knightType = PieceType{"knight", []MovementPattern{knightsMove}, 'N', false}
	rookType   = PieceType{"rook", []MovementPattern{horizontal, vertical}, 'R', false}
	pawnType   = PieceType{"pawn", []MovementPattern{forwardAdjacent, forwardDiagonalAdjacentToCapture}, 'P', false} // Pawns use P/p in FEN, but have no symbol in SAN or UCI
)

type Piece struct {
	pieceType PieceType
	color     Color
	position  Square
}

// Get the piece's symbol, uppercase if the piece is white and lowercase if the piece is black
func (p Piece) Symbol() rune {
	if p.color == White {
		return unicode.ToUpper(p.pieceType.symbol)
	} else {
		return unicode.ToLower(p.pieceType.symbol)
	}
}

func makePieces() [32]*Piece {
	var pieces = [32]*Piece{}
	var slice = pieces[:] // Slice of pieces to allow append
	var backrank = map[Color]uint8{White: 1, Black: 8}
	var pawnrank = map[Color]uint8{White: 2, Black: 7}
	var kingfile = map[Color]uint8{White: 5, Black: 4}
	var queenfile = map[Color]uint8{White: 4, Black: 5}
	for _, color := range []Color{White, Black} {
		// Backrank
		slice = append(slice, &Piece{rookType, color, Square{backrank[color], 1}})
		slice = append(slice, &Piece{knightType, color, Square{backrank[color], 2}})
		slice = append(slice, &Piece{bishopType, color, Square{backrank[color], 3}})
		slice = append(slice, &Piece{kingType, color, Square{backrank[color], kingfile[color]}})
		slice = append(slice, &Piece{queenType, color, Square{backrank[color], queenfile[color]}})
		slice = append(slice, &Piece{bishopType, color, Square{backrank[color], 6}})
		slice = append(slice, &Piece{knightType, color, Square{backrank[color], 7}})
		slice = append(slice, &Piece{rookType, color, Square{backrank[color], 8}})
		// Pawns
		for i := range 8 {
			slice = append(slice, &Piece{pawnType, color, Square{pawnrank[color], uint8(i)}})
		}
	}
	return pieces
}
