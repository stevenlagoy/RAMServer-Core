package gofish

import (
	"fmt"
)

const (
	actionCardRequest byte = iota + 1
	actionCardResponse
	actionCardResponseAndTopUp
	actionPlaceBook
	actionDrawFromPool
	actionTopUpFromPool
	actionEndTurn
	actionDealCards
	actionMakeCatchFromPool
	actionPlaceBookAndContinue
	actionTopUpFromPoolAndContinue
)

type Action interface {
	From() *Player
	To() *Player
	Encode() []byte
}

/*
Request cards:

	Ask a specific other player for all of their cards of one rank.
	Requires having a card of the requested rank in hand.
	Can repeat as long as not told to "Go fish."
*/
type CardRequest struct {
	from *Player
	to   *Player
	rank Rank
}

func (a CardRequest) From() *Player {
	return a.from
}

func (a CardRequest) To() *Player {
	return a.to
}

// 4 bytes: type, from player order, to player order, encoded rank
func (a CardRequest) Encode() []byte {
	from := uint8(a.from.order)
	to := uint8(a.to.order)
	rank := uint8(a.rank.value)
	return []byte{actionCardRequest, from, to, rank}
}

/*
Respond to a request:

	If have any cards of requested rank, send them. Otherwise, say "Go fish."
*/
type CardResponse struct {
	from          *Player
	to            *Player
	cards         []*Card
	requestedRank Rank
}

func (a CardResponse) From() *Player {
	return a.from
}

func (a CardResponse) To() *Player {
	return a.to
}

// Empty responses append the requested rank after the card count.
func (a CardResponse) Encode() []byte {
	from := uint8(a.from.order)
	to := uint8(a.to.order)
	numCards := uint8(len(a.cards))
	cards := make([]uint8, 0, len(a.cards))
	for _, card := range a.cards {
		cards = append(cards, card.Encode())
	}
	encoded := append([]byte{actionCardResponse, from, to, numCards}, cards...)
	if len(a.cards) == 0 && a.requestedRank.value > 0 {
		encoded = append(encoded, uint8(a.requestedRank.value))
	}
	return encoded
}

type CardResponseAndTopUp struct {
	CardResponse
	TopUpFromPool
}

func (a CardResponseAndTopUp) From() *Player {
	return a.CardResponse.from
}

func (a CardResponseAndTopUp) To() *Player {
	return a.CardResponse.to
}

func (a CardResponseAndTopUp) Encode() []byte {
	from := uint8(a.CardResponse.from.order)
	to := uint8(a.CardResponse.to.order)
	cardDrawn := a.card.Encode()
	numCards := uint8(len(a.cards))
	cards := make([]uint8, 0, len(a.cards))
	for _, card := range a.cards {
		cards = append(cards, card.Encode())
	}
	return append([]byte{actionCardResponseAndTopUp, from, to, cardDrawn, numCards}, cards...)
}

/*
Place a book:

	If have all cards of one rank, place them on the table.
	Always do this if have all cards of one rank.
	Can be repeated in one turn.
*/
type PlaceBook struct {
	from         *Player
	rank         Rank
	continueTurn bool
}

func (a PlaceBook) From() *Player {
	return a.from
}

func (a PlaceBook) To() *Player {
	return a.from
}

// 3 bytes: type, player order, rank placed
func (a PlaceBook) Encode() []byte {
	player := uint8(a.from.order)
	rank := uint8(a.rank.value)
	actionType := actionPlaceBook
	if a.continueTurn {
		actionType = actionPlaceBookAndContinue
	}
	return []byte{actionType, player, rank}
}

/*
Take a card from the pool:

	If told to "Go fish" after making a request, take the top card from the pool.
*/
type DrawFromPool struct {
	from *Player
	card *Card
}

func (a DrawFromPool) From() *Player {
	return a.from
}

func (a DrawFromPool) To() *Player {
	return a.from
}

// 3 bytes: type, player order, card drawn
func (a DrawFromPool) Encode() []byte {
	player := uint8(a.from.order)
	card := a.card.Encode()
	return []byte{actionDrawFromPool, player, card}
}

// MakeCatchFromPool draws the requested rank and lets the current player ask again.
type MakeCatchFromPool struct {
	from *Player
	card *Card
}

func (a MakeCatchFromPool) From() *Player {
	return a.from
}

func (a MakeCatchFromPool) To() *Player {
	return a.from
}

func (a MakeCatchFromPool) Encode() []byte {
	return []byte{actionMakeCatchFromPool, uint8(a.from.order), a.card.Encode()}
}

/*
Take a card from the pool:

	If hold no cards, draw top card from the pool.
*/
type TopUpFromPool struct {
	from         *Player
	card         *Card
	continueTurn bool
}

func (a TopUpFromPool) From() *Player {
	return a.from
}

func (a TopUpFromPool) To() *Player {
	return a.from
}

// 3 bytes: type, player order, card drawn
func (a TopUpFromPool) Encode() []byte {
	player := uint8(a.from.order)
	card := a.card.Encode()
	actionType := actionTopUpFromPool
	if a.continueTurn {
		actionType = actionTopUpFromPoolAndContinue
	}
	return []byte{actionType, player, card}
}

type EndTurn struct {
	from     *Player
	to       *Player // Next player in order
	newOrder int
}

type DealCards struct {
	dealer *Player
	first  *Player
}

func (a DealCards) From() *Player { return a.dealer }
func (a DealCards) To() *Player   { return a.first }

func (a DealCards) Encode() []byte {
	return []byte{actionDealCards, uint8(a.dealer.order), uint8(a.first.order)}
}

func (a EndTurn) From() *Player {
	return a.from
}

func (a EndTurn) To() *Player {
	return a.to
}

// 4 bytes: type, from player order, to player order, new order value
func (a EndTurn) Encode() []byte {
	from := uint8(a.from.order)
	to := uint8(a.to.order)
	return []byte{actionEndTurn, from, to, uint8(a.newOrder)}
}

func DecodeAction(encoded []byte) (Action, int, error) {
	if len(encoded) == 0 {
		return nil, 0, fmt.Errorf("empty action")
	}
	player := func(order byte) *Player {
		return &Player{PlayerOutsideView: PlayerOutsideView{order: int(order)}}
	}
	card := func(value byte) (*Card, error) {
		decoded, err := DecodeCard([]byte{value})
		if err != nil {
			return nil, err
		}
		return &decoded, nil
	}
	switch encoded[0] {
	case actionCardRequest:
		if len(encoded) != 4 || encoded[3] < 1 || encoded[3] > 13 {
			return nil, 0, fmt.Errorf("invalid card request")
		}
		return CardRequest{player(encoded[1]), player(encoded[2]), RankFromValue[int(encoded[3])]}, 4, nil
	case actionCardResponse:
		if len(encoded) < 4 {
			return nil, 0, fmt.Errorf("invalid card response")
		}
		numCards := int(encoded[3])
		hasRequestedRank := numCards == 0 && len(encoded) == 5
		if len(encoded) != 4+numCards && !hasRequestedRank {
			return nil, 0, fmt.Errorf("invalid card response")
		}
		var requestedRank Rank
		if hasRequestedRank {
			if encoded[4] < 1 || encoded[4] > 13 {
				return nil, 0, fmt.Errorf("invalid requested rank")
			}
			requestedRank = RankFromValue[int(encoded[4])]
		}
		cards := make([]*Card, 0, numCards)
		for _, value := range encoded[4 : 4+numCards] {
			decoded, err := card(value)
			if err != nil {
				return nil, 0, err
			}
			cards = append(cards, decoded)
		}
		return CardResponse{
			from:          player(encoded[1]),
			to:            player(encoded[2]),
			cards:         cards,
			requestedRank: requestedRank,
		}, len(encoded), nil
	case actionCardResponseAndTopUp:
		if len(encoded) < 5 || int(encoded[4]) != len(encoded)-5 {
			return nil, 0, fmt.Errorf("invalid card response and top-up")
		}
		drawn, err := card(encoded[3])
		if err != nil {
			return nil, 0, err
		}
		cards := make([]*Card, 0, encoded[4])
		for _, value := range encoded[5:] {
			decoded, err := card(value)
			if err != nil {
				return nil, 0, err
			}
			cards = append(cards, decoded)
		}
		return CardResponseAndTopUp{
			CardResponse:  CardResponse{from: player(encoded[1]), to: player(encoded[2]), cards: cards},
			TopUpFromPool: TopUpFromPool{from: player(encoded[1]), card: drawn},
		}, len(encoded), nil
	case actionPlaceBook, actionPlaceBookAndContinue:
		if len(encoded) != 3 || encoded[2] < 1 || encoded[2] > 13 {
			return nil, 0, fmt.Errorf("invalid place-book action")
		}
		return PlaceBook{
			from:         player(encoded[1]),
			rank:         RankFromValue[int(encoded[2])],
			continueTurn: encoded[0] == actionPlaceBookAndContinue,
		}, 3, nil
	case actionDrawFromPool, actionMakeCatchFromPool, actionTopUpFromPool, actionTopUpFromPoolAndContinue:
		if len(encoded) != 3 {
			return nil, 0, fmt.Errorf("invalid draw action")
		}
		drawn, err := card(encoded[2])
		if err != nil {
			return nil, 0, err
		}
		if encoded[0] == actionDrawFromPool {
			return DrawFromPool{player(encoded[1]), drawn}, 3, nil
		}
		if encoded[0] == actionMakeCatchFromPool {
			return MakeCatchFromPool{player(encoded[1]), drawn}, 3, nil
		}
		return TopUpFromPool{
			from:         player(encoded[1]),
			card:         drawn,
			continueTurn: encoded[0] == actionTopUpFromPoolAndContinue,
		}, 3, nil
	case actionEndTurn:
		if len(encoded) != 4 {
			return nil, 0, fmt.Errorf("invalid end-turn action")
		}
		return EndTurn{player(encoded[1]), player(encoded[2]), int(encoded[3])}, 4, nil
	case actionDealCards:
		if len(encoded) != 3 {
			return nil, 0, fmt.Errorf("invalid deal action")
		}
		return DealCards{player(encoded[1]), player(encoded[2])}, 3, nil
	default:
		return nil, 0, fmt.Errorf("unknown action type: %d", encoded[0])
	}
}
