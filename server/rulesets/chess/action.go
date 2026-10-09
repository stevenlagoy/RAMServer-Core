package chess

import "unicode"

// https://en.wikipedia.org/wiki/Universal_Chess_Interface

// Payload is ASCII UCI: "e7e4", or "e7e8q" for promotion. Castling is the king's move ("e1g1"). Draw offers are transmitted separately in the same payload.
type Move struct {
	From, To     *Square
	piece        *Piece
	color        Color
	capturing    bool
	offeringDraw bool
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

// Get the SAN representation of this move. Includes moved piece symbol, clarifiers, capture marker, To UCI, promotion, check, and draw offer
// Examples: `Ra5`, `axa8=Q#`, `Rcxe1+`, `B7xe5+ (=)`, `Qh3f1`, `(=)`
func (m Move) toSAN() string {
	capturingMark := "x"
	promotingMark := "="
	checkMark := "+"
	checkmateMark := "#"
	drawOfferMark := "(=)"

	if m.From == nil && m.offeringDraw {
		return drawOfferMark
	}

	result := ""

	// Moving piece's symbol
	if m.piece.pieceType.name != pawnType.name { // Pawns are identified by their file rather than with a symbol
		result += string(unicode.ToUpper(m.piece.pieceType.symbol))
	}

	// From square disambiguators
	// TODO: Determine if from square is ambiguous in rank and/or file. Only necessary information for disambiguation should be included
	// Pawns always need their file added
	if m.piece.pieceType.name == pawnType.name {
		result += string(m.From.file)
	}
	// Determine if same type same color pieces on same rank and same file as actual moving piece

	// Determine if same type same color pieces on the same file

	// Determine if same type same color pieces on the same rank

	// Capturing mark
	if m.capturing {
		result += capturingMark
	}
	// To Square rank and file
	result += m.To.toUCI()
	// Promotion mark and symbol
	if m.Promotion != nil {
		result += promotingMark
		result += string(unicode.ToUpper(m.Promotion.symbol))
	}
	// TODO: Determine if this move places the opponent in check (result += checkMark)
	// TODO: Determine if the check is checkmate (result += checkmateMark)

	// Draw offer mark
	if m.offeringDraw {
		result += drawOfferMark
	}
	return result
}

// TODO: Decode based on protocol and message framing
func DecodeMove(b []byte) (Move, error) {
	// returns error wrapping game.ErrMalformedAction
	return Move{}, nil
}

// TODO: Decode based on protocol and message framing
func (m Move) Encode() []byte {
	return []byte{}
}
