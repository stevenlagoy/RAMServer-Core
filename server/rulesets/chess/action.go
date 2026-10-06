package chess

// https://en.wikipedia.org/wiki/Universal_Chess_Interface

// Payload is ASCII UCI: "e7e4", or "e7e8q" for promotion. Castling is the king's move ("e1g1")
type Move struct {
	From, To Square
	Promo    Piece // 0 when not a promotion
}

func DecodeMove(b []byte) (Move, error) {
	// returns error wrapping game.ErrMalformedAction
	return Move{}, nil
}

func (m Move) Encode() []byte {
	return []byte{}
}

type Square struct {
	rank int8 // 1, 2, 3, 4, 5, 6, 7, 8
	file rune // a, b, c, d, e, f, g, h
}
