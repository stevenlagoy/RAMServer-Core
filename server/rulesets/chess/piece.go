package chess

import (
	"math"
	"unicode"
)

type Color struct {
	name              string
	symbol            rune
	movementDirection int // 1 if moving forwards is upwards (white), -1 if moving forwards is downwards (black)
	firstToMove       bool
}

var (
	White = Color{"white", 'w', 1, true}
	Black = Color{"black", 'b', -1, false}
)

type MovementPattern struct {
	name string
	// Evaluate whether a piece following this movement pattern is allowed to make the given move
	evaluate func(from Square, to Square, stateBefore State) bool
	// Get an array of squares along the path between squares while following this movement pattern.
	// Should include the From and To squares. Order does not matter.
	squaresInPath func(from Square, to Square) []Square
}

// TODO: finish movement patterns' evaluate logic. This is tricky because there's unique context for some movesets
// TODO: IE knights are able to jump over pieces, and kings may castle only if neither they nor their rooks have moved (the board state already tracks castleRights)
var (
	// Leftwards or rightwards along one rank
	horizontal = MovementPattern{
		name:     "horizontal",
		evaluate: func(from Square, to Square, _ State) bool { return from.rank == to.rank },
		squaresInPath: func(from Square, to Square) []Square {
			var step int
			if from.file < to.file { // Rightwards (positive file)
				step = 1
			} else { // Leftwards (negative file)
				step = -1
			}
			squares := []Square{}
			// Get the squares in the correct direction
			for i := range int(math.Abs(float64(from.file - to.file))) {
				squares = append(squares, Square{from.rank, uint8(int(from.file) + i*step), nil})
			}
			return squares
		},
	}
	// Forwards or backwards along one file
	vertical = MovementPattern{
		name:     "vertical",
		evaluate: func(from Square, to Square, _ State) bool { return from.file == to.file },
		squaresInPath: func(from Square, to Square) []Square {
			var step int
			if from.rank < to.rank { // Upwards (positive rank)
				step = 1
			} else { // Downwards (negative rank)
				step = -1
			}
			squares := []Square{}
			// Get the squares in the correct direction
			for i := range int(math.Abs(float64(from.rank - to.rank))) {
				squares = append(squares, Square{uint8(int(from.rank) + i*step), from.file, nil})
			}
			return squares
		},
	}
	// Principal or minor diagonal
	diagonal = MovementPattern{
		name: "diagonal",
		evaluate: func(from Square, to Square, _ State) bool {
			return math.Abs(float64(from.file-to.file)) == math.Abs(float64(from.rank-to.rank))
		},
		squaresInPath: func(from Square, to Square) []Square {
			// 2-array of positive 1 or negative 1: {rank step, file step}
			var step [2]int
			if from.rank < to.rank { // Upwards (positive rank)
				step[0] = 1
			} else { // Downwards (negative rank)
				step[0] = -1
			}
			if from.file < to.file { // Rightwards (positive file)
				// Up-Right direction (principal diagonal forwards)
				step[1] = 1
			} else { // Leftwards (negative file)
				// Up-Left direction (minor diagonal normal)
				step[1] = -1
			}
			squares := []Square{}
			// Get the squares in the correct direction
			for i := range int(math.Abs(float64(from.rank - to.rank))) { // Truncation: safe downcast
				squares = append(squares, Square{uint8(int(from.rank) + i*step[0]), uint8(int(from.file) + i*step[1]), nil})
			}
			return squares
		},
	}
	forwardAdjacent = MovementPattern{
		name: "forward adjacent",
		evaluate: func(from Square, to Square, _ State) bool {
			return from.file == to.file && int(from.rank) == int(to.rank)-from.occupant.color.movementDirection
		},
		squaresInPath: func(from Square, to Square) []Square { return []Square{from, to} },
	}
	sidewaysAdjacent = MovementPattern{
		name:          "sideways adjacent",
		evaluate:      func(from Square, to Square, _ State) bool { return false },
		squaresInPath: func(from Square, to Square) []Square { return []Square{from, to} },
	}
	backwardAdjacent = MovementPattern{
		name: "backward adjacent",
		evaluate: func(from Square, to Square, _ State) bool {
			return from.file == to.file && int(from.rank) == int(to.rank)+from.occupant.color.movementDirection
		},
		squaresInPath: func(from Square, to Square) []Square { return []Square{from, to} },
	}
	diagonalAdjacent = MovementPattern{
		name: "diagonal adjacent",
		evaluate: func(from Square, to Square, _ State) bool {
			return math.Abs(float64(from.file-to.file)) == math.Abs(float64(from.rank-to.rank)) && math.Abs(float64(from.file-to.file)) == 1
		},
		squaresInPath: func(from Square, to Square) []Square { return []Square{from, to} },
	}
	anyAdjacent = MovementPattern{
		name: "any adjacent",
		evaluate: func(from Square, to Square, s State) bool {
			return (forwardAdjacent.evaluate(from, to, s) ||
				sidewaysAdjacent.evaluate(from, to, s) ||
				backwardAdjacent.evaluate(from, to, s) ||
				diagonalAdjacent.evaluate(from, to, s))
		},
		squaresInPath: func(from Square, to Square) []Square { return []Square{from, to} },
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
				[2]int{2, 1},   // A
				[2]int{1, 2},   // B
				[2]int{-1, 2},  // C
				[2]int{-2, 1},  // D
				[2]int{-2, -1}, // E
				[2]int{-1, -2}, // F
				[2]int{1, -2},  // G
				[2]int{2, -1},  // H
			}
			rankDiff := int(to.rank - from.rank) // Upcasting
			fileDiff := int(to.file - from.file) // Upcasting
			for _, possibility := range possibilities {
				if possibility == [2]int{rankDiff, fileDiff} {
					return true
				}
			}
			return false
		},
		squaresInPath: func(from Square, to Square) []Square { return []Square{from, to} },
	}
	forwardDiagonalAdjacentToCapture = MovementPattern{
		name: "forward diagonal adjacent to capture",
		evaluate: func(from Square, to Square, stateBefore State) bool {
			// Determine whether the 'to' square contains an opponent piece
			if to.occupant != nil && from.occupant.color != to.occupant.color { // We are capturing
				// Check that 'to' square is forward diagonal of 'from' square
				if to.rank != from.rank+uint8(from.occupant.color.movementDirection) { // Not going the right way
					return false
				}
				if math.Abs(float64(to.file-from.file)) != 1 { // Wrong horizontal difference
					return false
				}
				// Correct direction and horizontal distance, and we are capturing
				return true
			} else { // We are not capturing
				return false
			}
		},
		squaresInPath: func(from Square, to Square) []Square { return []Square{from, to} },
	}
	kingsideCastle = MovementPattern{ // King's move, not the rook's
		name: "kingside castle",
		evaluate: func(from Square, to Square, stateBefore State) bool {
			// Check that we have castling privledges
			switch from.occupant.color {
			case White:
				return stateBefore.castling>>3&1 == 1
			case Black:
				return stateBefore.castling>>1&1 == 1
			default:
				return false // Unreachable
			}
		},
		squaresInPath: func(from Square, to Square) []Square {
			switch from.occupant.color { // This could have been done with branchless algebra, but it's probably not worth the drop in readability
			case White:
				// _ _ _ _ K _ _ R -> _ _ _ _ _ R K _
				return []Square{
					Square{from.rank, 4, nil},
					Square{from.rank, 5, nil},
					Square{from.rank, 6, nil},
					Square{from.rank, 7, nil},
				}
			case Black:
				// R _ _ K _ _ _ _ -> _ K R _ _ _ _ _
				return []Square{
					Square{from.rank, 0, nil},
					Square{from.rank, 1, nil},
					Square{from.rank, 2, nil},
					Square{from.rank, 3, nil},
				}
			default:
				return []Square{} // Unreachable
			}
		},
	}
	queensideCastle = MovementPattern{ // King's move, not the rook's
		name: "queenside castle",
		evaluate: func(from Square, to Square, stateBefore State) bool {
			// Check that we have castling privledges
			switch from.occupant.color {
			case White:
				return stateBefore.castling>>2&1 == 1
			case Black:
				return stateBefore.castling>>0&1 == 1
			default:
				return false // Unreachable
			}
		},
		squaresInPath: func(from Square, to Square) []Square {
			switch from.occupant.color { // This could have been done with branchless algebra, but it's probably not worth the drop in readability
			case White:
				// R _ _ _ K _ _ _ -> _ _ K R _ _ _ _
				return []Square{
					Square{from.rank, 0, nil},
					Square{from.rank, 1, nil},
					Square{from.rank, 2, nil},
					Square{from.rank, 3, nil},
					Square{from.rank, 4, nil},
				}
			case Black:
				// _ _ _ K _ _ _ R -> _ _ _ _ R K _ _
				return []Square{
					Square{from.rank, 3, nil},
					Square{from.rank, 4, nil},
					Square{from.rank, 5, nil},
					Square{from.rank, 6, nil},
					Square{from.rank, 7, nil},
				}
			default:
				return []Square{} // Unreachable
			}
		},
	}
)

type PieceType struct {
	name             string            // Name of the piece. This should be used for equality checking
	movementPatterns []MovementPattern // Movement patterns this piece can follow
	symbol           rune              // Caller must always determine case: see Piece.Symbol() for black / white casing
	canPromote       bool              // is this piece type able to promote? (pawns)
}

var (
	kingType   = PieceType{"king", []MovementPattern{anyAdjacent}, 'K', false}
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
	switch p.color {
	case White:
		return unicode.ToUpper(p.pieceType.symbol)
	case Black:
		return unicode.ToLower(p.pieceType.symbol)
	default:
		return p.pieceType.symbol // Unreachable
	}
}

func makePieces() [32]*Piece {
	pieces := [32]*Piece{}
	slice := pieces[:] // Slice of pieces to allow append
	backrank := map[Color]uint8{White: 1, Black: 8}
	pawnrank := map[Color]uint8{White: 2, Black: 7}
	kingfile := map[Color]uint8{White: 5, Black: 4}
	queenfile := map[Color]uint8{White: 4, Black: 5}
	for _, color := range []Color{White, Black} {
		// Backrank
		slice = append(slice, &Piece{rookType, color, Square{backrank[color], 1, nil}})
		slice = append(slice, &Piece{knightType, color, Square{backrank[color], 2, nil}})
		slice = append(slice, &Piece{bishopType, color, Square{backrank[color], 3, nil}})
		slice = append(slice, &Piece{kingType, color, Square{backrank[color], kingfile[color], nil}})
		slice = append(slice, &Piece{queenType, color, Square{backrank[color], queenfile[color], nil}})
		slice = append(slice, &Piece{bishopType, color, Square{backrank[color], 6, nil}})
		slice = append(slice, &Piece{knightType, color, Square{backrank[color], 7, nil}})
		slice = append(slice, &Piece{rookType, color, Square{backrank[color], 8, nil}})
		// Pawns
		for i := range 8 {
			slice = append(slice, &Piece{pawnType, color, Square{pawnrank[color], uint8(i), nil}})
		}
	}
	return pieces
}
