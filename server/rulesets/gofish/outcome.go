package gofish

import "github.com/stevenlagoy/ramserver-core/server/game"

type Outcome struct {
	books map[*Player]int // Number of books for each player (scores)
}

func (o Outcome) GetWinners() []*Player {
	maxScore := 0
	winners := []*Player{}
	for player, score := range o.books {
		if score > maxScore {
			maxScore = score
			winners = []*Player{}
		}
		if score == maxScore {
			winners = append(winners, player)
		}
	}
	return winners
}

func (o Outcome) GetWinnerIDs() []string {
	winners := o.GetWinners()
	winnerIDs := make([]string, 0, len(winners))
	for _, winner := range winners {
		winnerIDs = append(winnerIDs, winner.id)
	}
	return winnerIDs
}

func (o Outcome) TerminationReason() game.TerminationReason {
	return "No cards were left in play"
}
