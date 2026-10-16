package chess

import (
	"fmt"
	"strconv"
	"unicode"

	"github.com/stevenlagoy/ramserver-core/server/game"
)

// https://en.wikipedia.org/wiki/Universal_Chess_Interface

// Payload is ASCII UCIL "e7e4", or "e7e8q" for promotion. Castling is the king's move ("e1g1").
// A decoded Move only knows its squares and promotion. State.resolve fills in the piece, color, and capture.
// TODO: Draw offers are not transmitted yet. See offeringDraw
type Move struct {
	From, To     *Square
	piece        *Piece
	color        Color
	capturing    bool
	offeringDraw bool // TODO: nothing sets this yet. Decide how a draw offer is sent, then decode it in DecodeMove
	Promotion    *PieceType
}

// Get the UCI representation of this move. Includes From UCI, To UCI, and promotion type symbol. Example: `a7a8q`
func (m Move) toUCI() string {
	result := m.From.toUCI() + m.To.toUCI()
	if m.Promotion != nil {
		result += string(unicode.ToLower(m.Promotion.symbol))
	}
	return result
}

const (
	capturingMark   = "x"
	promotingMark   = "="
	checkMark       = "+"
	checkmateMark   = "#"
	drawOfferMark   = "(=)"
	castleKingside  = "0-0"   // SAN uses 0-0, PGN uses O-O
	castleQueenside = "0-0-0" // SAN uses 0-0-0, PGN uses O-O-O
)

// Get the SAN representation of this move. Includes moved piece symbol, clarifiers, capture marker, To UCI, promotion, check, and draw offer
// Examples: `Ra5`, `axa8=Q#`, `Rcxe1+`, `B7xe5+ (=)`, `Qh3f1`, `(=)`
// m must be resolved and s must be the state before the move.
func (m Move) toSAN(s State) (string, error) {
	if m.From == nil && m.offeringDraw {
		return drawOfferMark, nil
	}

	if m.From == nil || m.To == nil || m.piece == nil {
		return "", fmt.Errorf("the move has not been resolved against a state")
	}

	result := ""

	switch {
	case m.piece.pieceType.name == kingType.name && absDiff(m.From.file, m.To.file) == 2:
		if m.To.file > m.From.file {
			result = castleKingside
		} else {
			result = castleQueenside
		}
	default:
		if m.piece.pieceType.name != pawnType.name { // Pawns are identified by their file rather than with a symbol
			result += string(unicode.ToUpper(m.piece.pieceType.symbol))
			result += s.disambiguation(m)
		} else if m.capturing {
			result += string(fileLetters[m.From.file-1])
		}
		if m.capturing {
			result += capturingMark
		}
		result += m.To.toUCI()
		if m.Promotion != nil {
			result += promotingMark + string(unicode.ToUpper(m.Promotion.symbol))
		}
	}
	// Determine if this move places the opponent in check, and whether the check is a regular check or is checkmate
	after := s.makeMove(m)
	if after.inCheck(after.turn) {
		if len(after.legalActionSpecs(after.activePlayer())) == 0 { // Checkmate
			result += checkmateMark
		} else { // Check
			result += checkMark
		}
	}

	// Draw offer mark
	if m.offeringDraw {
		result += " " + drawOfferMark
	}
	return result, nil
}

// The part of a SAN move which tells apart pieces of the same type that can reach the same square.
// Prefers the file, then the rank if another such piece is on the same file, and uses both only if nether alone is enough
func (s State) disambiguation(m Move) string {
	ambiguous, sameFile, sameRank := false, false, false
	for _, other := range s.pieces {
		if other == m.piece || other.color != m.color || other.pieceType.name != m.piece.pieceType.name {
			continue
		}
		candidate, err := s.resolve(Move{From: other.position, To: m.To, Promotion: m.Promotion})
		if err != nil || !s.isLegal(candidate) { // The other piece cannot make this move, so there's no confusion
			continue
		}
		ambiguous = true
		sameFile = sameFile || other.position.file == m.From.file
		sameRank = sameRank || other.position.rank == m.From.rank
	}
	switch {
	case !ambiguous:
		return ""
	case !sameFile:
		return string(fileLetters[m.From.file-1])
	case !sameRank:
		return strconv.Itoa(int(m.From.rank))
	default:
		return m.From.toUCI()
	}
}

// Decode an ASCII UCI payload: four characters ("e2e4") or five with a promotion piece ("e7e8q").
// This only reads the payload: whether the move makes sense on a board is determined by State.resolve and State.isLegal.
func DecodeMove(b []byte) (Move, error) {
	if len(b) != 4 && len(b) != 5 {
		return Move{}, fmt.Errorf("%w: the message had an invalid size", game.ErrMalformedAction)
	}
	from, err := squareFromUCI(string(b[0:2]))
	if err != nil {
		return Move{}, fmt.Errorf("%w: %v", game.ErrMalformedAction, err)
	}
	to, err := squareFromUCI(string(b[2:4]))
	if err != nil {
		return Move{}, fmt.Errorf("%w: %v", game.ErrMalformedAction, err)
	}

	// Determine type of promotion
	var promotionType *PieceType
	if len(b) == 5 {
		for i := range promotionTypes {
			if byte(unicode.ToLower(promotionTypes[i].symbol)) == b[4] {
				promotionType = &promotionTypes[i]
			}
		}
		if promotionType == nil {
			return Move{}, fmt.Errorf("%w: a promotion was specified but the promotion type was invalid", game.ErrMalformedAction)
		}
	}
	return Move{From: &from, To: &to, Promotion: promotionType}, nil
}

func (s State) moveFromPayload(payload []byte) (Move, error) {
	m, err := DecodeMove(payload)
	if err != nil {
		return Move{}, err
	}
	return s.resolve(m)
}

// Get the ASCII UCI payload for this move, the inverse fo DecodeMove
func (m Move) Encode() []byte {
	return []byte(m.toUCI())
}
