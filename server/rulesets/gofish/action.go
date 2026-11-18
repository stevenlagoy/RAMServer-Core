package gofish

import (
	"fmt"
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

// 3 bytes: from player order, to player order, encoded rank
func (a CardRequest) Encode() []byte {
	from := uint8(a.from.order)
	to := uint8(a.to.order)
	rank := uint8(a.rank.value)
	return []byte{from, to, rank}
}

/*
Respond to a request:

	If have any cards of requested rank, send them. Otherwise, say "Go fish."
*/
type CardResponse struct {
	from  *Player
	to    *Player
	cards []*Card
}

func (a CardResponse) From() *Player {
	return a.from
}

func (a CardResponse) To() *Player {
	return a.to
}

// 3 or more bytes: from player order, to player order, number of cards, and one byte for each card. If no cards, "Go fish"
func (a CardResponse) Encode() []byte {
	from := uint8(a.from.order)
	to := uint8(a.to.order)
	numCards := uint8(len(a.cards))
	cards := make([]uint8, len(a.cards))
	for _, card := range a.cards {
		cards = append(cards, card.Encode())
	}
	return append([]byte{from, to, numCards}, cards...)
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
	cards := make([]uint8, len(a.cards))
	for _, card := range a.cards {
		cards = append(cards, card.Encode())
	}
	return append([]byte{from, to, cardDrawn, numCards}, cards...)
}

/*
Place a book:

	If have all cards of one rank, place them on the table.
	Always do this if have all cards of one rank.
	Can be repeated in one turn.
*/
type PlaceBook struct {
	from *Player
	rank Rank
}

func (a PlaceBook) From() *Player {
	return a.from
}

func (a PlaceBook) To() *Player {
	return a.from
}

// 3 bytes: player order, player order (padding), rank placed
func (a PlaceBook) Encode() []byte {
	player := uint8(a.from.order)
	rank := uint8(a.rank.value)
	return []byte{player, player, rank}
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

// 3 bytes: player order, player order (padding), card drawn
func (a DrawFromPool) Encode() []byte {
	player := uint8(a.from.order)
	card := a.card.Encode()
	return []byte{player, player, card}
}

/*
Take a card from the pool:

	If hold no cards, draw top card from the pool.
*/
type TopUpFromPool struct {
	from *Player
	card *Card
}

func (a TopUpFromPool) From() *Player {
	return a.from
}

func (a TopUpFromPool) To() *Player {
	return a.from
}

// 3 bytes: player order, player order (padding), card drawn
func (a TopUpFromPool) Encode() []byte {
	player := uint8(a.from.order)
	card := a.card.Encode()
	return []byte{player, player, card}
}

type EndTurn struct {
	from     *Player
	to       *Player // Next player in order
	newOrder int
}

func (a EndTurn) From() *Player {
	return a.from
}

func (a EndTurn) To() *Player {
	return a.to
}

// 3 bytes: from player order, to player order, new order value
func (a EndTurn) Encode() []byte {
	from := uint8(a.from.order)
	to := uint8(a.to.order)
	return []byte{from, to, uint8(a.newOrder)}
}

func DecodeAction(encoded []byte) (Action, int, error) {
	// TODO
	return DrawFromPool{nil, &Card{}}, 0, fmt.Errorf("not yet implemented")
}
