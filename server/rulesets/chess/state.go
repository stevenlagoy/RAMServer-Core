package chess

type State struct {
	board [64]Piece // array not slice, copied by value
	turn Color
	castling uint8 // KQkq bit flags
	enPassant uint8 // target square or -1
	halfmove int
	fullmove int
	white string // playerID bound at NewState
	black string // playerID bound at NewState
	history []uint64 // position hashes for threefold repetition
}