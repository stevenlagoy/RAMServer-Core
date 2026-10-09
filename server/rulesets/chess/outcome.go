package chess

import "github.com/stevenlagoy/ramserver-core/server/game"

const (
	ReasonCheckmate            game.TerminationReason = "CHECKMATE"
	ReasonStalemate            game.TerminationReason = "STALEMATE"
	ReasonDrawOffer            game.TerminationReason = "MUTUAL_DRAW_OFFER"
	ReasonResignation          game.TerminationReason = "RESIGNATION"
	ReasonInsufficientMaterial game.TerminationReason = "INSUFFICIENT_MATERIAL"
	ReasonAutomaticDraw        game.TerminationReason = "AUTOMATIC_DRAW"
)

type ChessResult struct {
	game.Result
	loser      *string // playerID of losing player, or nil if draw
	finalState State   // final state of the board
}

func (r ChessResult) toSAN() string {
	if r.Result.TerminationReason() == ReasonDrawOffer {
		return "1/2-1/2"
	}
	if &r.Result.WinnerIDs[0] == &r.finalState.whitePlayer {
		return "1-0"
	} else if &r.Result.WinnerIDs[0] == &r.finalState.blackPlayer {
		return "0-1"
	}
	return "0-0" // game was not completed
}
