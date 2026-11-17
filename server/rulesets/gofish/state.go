package gofish

import (
	"encoding/binary"
	"fmt"
)

type Encodable[T any] interface {
	Encode() []byte
	Decode([]byte) (T, int, error)
}

type PlayerOutsideView struct {
	id       string
	handSize int // Number of held cards
	books    []Book
	order    int // 0 is the first to play, dealer has the highest order
}

// 4 bytes: order, handSize, books * 2
func (p PlayerOutsideView) Encode() []byte {
	encodedOrder := uint8(p.order)
	encodedHandSize := uint8(p.handSize)
	encodedBooks := uint16(0)
	for _, book := range p.books {
		encodedBooks |= book.Encode()
	}
	booksBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(booksBuf, encodedBooks)
	return append([]byte{encodedOrder, encodedHandSize}, booksBuf...)
}

func (p PlayerOutsideView) Decode(encoded []byte) (PlayerOutsideView, int, error) {
	if len(encoded) < 4 {
		return PlayerOutsideView{}, 0, fmt.Errorf("incorrect length for encoded OutsidePlayerView: %d", len(encoded))
	}
	p.order = int(encoded[0])
	p.handSize = int(encoded[1])
	books := binary.BigEndian.Uint16(encoded[3:4])
	for i := 0; i < 16; i++ {
		bit := (books >> i) & 1
		if bit == 1 {
			rank := RankFromValue[i]
			p.books = append(p.books, Book{rank, p})
		}
	}
	return p, 4, nil
}

type Player struct {
	PlayerOutsideView
	hand []Card
}

// 5 or more bytes, byte 5 gives the number of bytes greater than 5
func (p Player) Encode() []byte {
	encodedHand := make([]uint8, p.handSize)
	for _, card := range p.hand {
		encodedHand = append(encodedHand, card.Encode())
	}
	return append(p.PlayerOutsideView.Encode(), encodedHand...)
}

func (p Player) Decode(encoded []byte) (Player, int, error) {
	if len(encoded) < 5 {
		return Player{}, 0, fmt.Errorf("incorrect length for encoded Player: %d", len(encoded))
	}
	_, decoded, err := p.PlayerOutsideView.Decode(encoded)
	if err != nil {
		return Player{}, 0, err
	}
	if len(encoded) != 5+p.handSize {
		return Player{}, 0, fmt.Errorf("incorrect length for encoded Player with hand size %d: %d", p.handSize, len(encoded))
	}
	for i := range p.handSize {
		card, err := DecodeCard([]byte{encoded[i+5]})
		if err != nil {
			return Player{}, 0, err
		}
		p.hand = append(p.hand, card)
		decoded++
	}
	return p, decoded, nil
}

type State struct {
	players      []Player
	turnNumber   int
	cards        []Card
	recentAction Action
}

func (s State) cardsInPool() []Card {
	pool := make([]Card, len(s.cards))
	for _, card := range s.cards {
		if card.Holder == nil {
			pool = append(pool, card)
		}
	}
	return pool
}

func (s State) CurrentTurn() string {
	order := s.turnNumber % len(s.players)
	for _, player := range s.players {
		if player.order == order {
			return player.id
		}
	}
	panic("Current turn number is invalid")
}

type View struct {
	self         Player
	players      []PlayerOutsideView
	turnNumber   int
	poolSize     int
	recentAction Action
}

func (s State) viewFor(playerID string) (View, error) {
	self := (*Player)(nil)
	playerViews := make([]PlayerOutsideView, len(s.players))
	for _, player := range s.players {
		if player.id == playerID {
			self = &player
		}
		playerViews = append(playerViews, player.PlayerOutsideView)
	}
	if self == nil {
		return View{}, fmt.Errorf("Could not find a player with ID: %s", playerID)
	}
	return View{
		*self,
		playerViews,
		s.turnNumber,
		len(s.cardsInPool()),
		s.recentAction,
	}, nil
}

/*
Structure:
Starting at byte 1: Self
1: order
2: handSize
3: completed books
4: completed books
5 to 5+handSize: One byte for each card in handSize
After self: num players
Each player:
1: order
2: handsize
3: completed books
4: completed books
After all players:
turn number
number of cards in the pool
recent action (3 or more bytes)
1: from player order
2: to player order
3: card, rank, or number of cards
4+: Encoded cards for responding to a request for cards
*/
func (v View) Encode() []byte {
	encodedSelf := v.self.Encode()
	numPlayers := uint8(len(v.players))
	encodedPlayers := make([]byte, len(v.players)*4)
	for _, player := range v.players {
		encodedPlayers = append(encodedPlayers, player.Encode()...)
	}
	turnNumber := uint8(v.turnNumber)
	poolSize := uint8(v.poolSize)
	recentAction := v.recentAction.Encode()
	encoded := []byte{}
	encoded = append(encoded, encodedSelf...)
	encoded = append(encoded, numPlayers)
	encoded = append(encoded, encodedPlayers...)
	encoded = append(encoded, turnNumber)
	encoded = append(encoded, poolSize)
	encoded = append(encoded, recentAction...)
	return encoded
}

func (v View) Decode(encoded []byte) (View, int, error) {
	self, decoded, err := v.self.Decode(encoded)
	if err != nil {
		return View{}, 0, err
	}
	v.self = self
	numPlayers := int(encoded[decoded]) // 1 byte
	decoded++
	v.players = make([]PlayerOutsideView, numPlayers)
	for range numPlayers {
		player, pDecoded, err := PlayerOutsideView{}.Decode(encoded[decoded:])
		if err != nil {
			return View{}, 0, err
		}
		v.players = append(v.players, player)
		decoded += pDecoded
	}
	v.turnNumber = int(encoded[decoded]) // 1 byte
	decoded++
	v.poolSize = int(encoded[decoded]) // 1 byte
	decoded++
	// TODO recentAction
	return v, decoded, nil
}
