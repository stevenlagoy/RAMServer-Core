package chess

import "github.com/stevenlagoy/ramserver-core/server/game"

const (
	ReasonCheckmate            game.TerminationReason = "CHECKMATE"
	ReasonStalemate            game.TerminationReason = "STALEMATE"
	ReasonDrawOffer            game.TerminationReason = "MUTUAL_DRAW_OFFER"
	ReasonResignation          game.TerminationReason = "RESIGNATION"
	ReasonInsufficientMaterial game.TerminationReason = "INSUFFICIENT_MATERIAL"
	ReasonAutomaticDraw        game.TerminationReason = "AUTOMATIC_DRAW" // 75-ply rule or 5-fold repetition
)

type ChessResult struct {
	game.Result
	loser      *string // playerID of losing player, or nil if draw
	finalState State   // final state of the board
}

// Get the result in the symbols specified by the README
// "1-0" for white win, "0-1" for black win, "1/2-1/2" for a draw
func (r ChessResult) toSAN() string {
	if r.Result.IsDraw() {
		return "1/2-1/2"
	}
	switch r.Result.WinnerIDs[0] {
	case r.finalState.whitePlayer:
		return "1-0"
	case r.finalState.blackPlayer:
		return "0-1"
	}
	return "*" // Game was not completed
}
