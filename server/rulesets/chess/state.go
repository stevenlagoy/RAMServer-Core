package chess

import "github.com/stevenlagoy/ramserver-core/server/game"

type Color string

const (
	white Color = "white"
	black Color = "black"
)

type Piece string

const (
	pawn   Piece = "pawn"
	rook   Piece = "rook"
	knight Piece = "knight"
	bishop Piece = "bishop"
	queen  Piece = "queen"
	king   Piece = "king"
)

type State struct {
	board     [64]Piece // array not slice, copied by value
	turn      Color
	castling  uint8 // KQkq bit flags
	enPassant uint8 // target square or -1
	halfmove  int
	fullmove  int
	white     string   // playerID bound at NewState
	black     string   // playerID bound at NewState
	history   []uint64 // position hashes for threefold repetition
}

func newState(whitePlayer string, blackPlayer string) State {
	return State{}
}

func (s State) isLegal(m Move) bool {
	return true
}

func (s State) apply(m Move) error {
	return nil
}

func (s State) activePlayer() string {
	return s.white
}

func (s State) outcome() (game.Result, bool) {
	return game.Result{}, false
}

func (s State) view() any {
	return nil
}

func (s State) legalActionSpecs(id string) []game.ActionSpec {
	return nil
}
