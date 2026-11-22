package chess

import (
	"slices"
	"strconv"
	"strings"

	"github.com/stevenlagoy/ramserver-core/server/game"
)

// Keys of annotations sent along with the FEN view. All values are strings
const (
	AnnotationPlies      = "plies"       // Number of moves played, always present
	AnnotationLastMove   = "last_move"   // Last move in UCI without any draw offer mark
	AnnotationCheck      = "check"       // Color of the king in check
	AnnotationCheckers   = "checkers"    // Squares of pieces giving check
	AnnotationKingUnsafe = "king_unsafe" // Squares next to the king to move being attacked by opponent
	AnnotationDrawOffer  = "draw_offer"  // ID of the player whose draw offer is pending
)

var (
	_ game.Annotator        = (*Game)(nil)
	_ game.OutOfTurnActions = (*Game)(nil)
)

// Chess has no hidden information, everyone gets the same annotations
func (g *Game) Annotations(s game.GameState, _ string) map[string]string {
	return s.(State).annotations()
}

func (s State) annotations() map[string]string {
	result := map[string]string{AnnotationPlies: strconv.Itoa(len(s.moveHistory))}
	if len(s.moveHistory) > 0 {
		result[AnnotationLastMove] = s.moveHistory[len(s.moveHistory)-1].squaresUCI()
	}
	if s.activeDrawOffer {
		result[AnnotationDrawOffer] = s.playerID(s.turn.opponent()) // Player to move is receiving the offer
	}
	if s.kingOf(s.turn) == nil {
		return result
	}
	if checkers := s.checkers(s.turn); len(checkers) > 0 {
		result[AnnotationCheck] = s.turn.name
		result[AnnotationCheckers] = strings.Join(checkers, ",")
	}
	if unsafe := s.unsafeKingSquares(); len(unsafe) > 0 {
		result[AnnotationKingUnsafe] = strings.Join(unsafe, ",")
	}
	return result
}

// Squares of pieces attacking king of the given color, alphabetically
func (s State) checkers(color Color) []string {
	king := s.kingOf(color)
	if king == nil {
		return nil
	}
	var squares []string
	for _, piece := range s.attackers(*king.position, color.opponent()) {
		squares = append(squares, piece.position.toUCI())
	}
	slices.Sort(squares)
	return squares
}

// Squares next to the king of the player to move which the opponent is attacking.
// Squares of own pieces are not listed. Accounts for spaces behind the king
func (s State) unsafeKingSquares() []string {
	king := s.kingOf(s.turn)
	if king == nil {
		return nil
	}
	bare := s.deepCopy()
	bare.removePieceAt(king.position.rank, king.position.file)
	bare.update()

	var squares []string
	for rankStep := -1; rankStep <= 1; rankStep++ {
		for fileStep := -1; fileStep <= 1; fileStep++ {
			rank, file := int(king.position.rank)+rankStep, int(king.position.file)+fileStep
			if (rankStep == 0 && fileStep == 0) || rank < 1 || rank > squaresPerFile || file < 1 || file > squaresPerRank {
				continue // Either the king is currently there or the square is out of bounds
			}
			square := bare.board[squareIndex(uint8(rank), uint8(file))]
			if square.occupant != nil && square.occupant.color == s.turn {
				continue // Own piece is there
			}
			if bare.isAttacked(*square, s.turn.opponent()) {
				squares = append(squares, square.toUCI())
			}
		}
	}
	slices.Sort(squares)
	return squares
}
