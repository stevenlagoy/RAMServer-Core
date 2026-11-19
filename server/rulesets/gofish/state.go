package gofish

import (
	"encoding/binary"
	"fmt"
)

type Encodable[T any] interface {
	Encode() []byte
	Decode([]byte) (T, int, error)
}

func (s State) playerByID(id string) (*Player, bool) {
	for i := range s.players {
		if s.players[i].id == id {
			return &s.players[i], true
		}
	}
	return nil, false
}

func (s State) playerByOrder(order int) *Player {
	for i := range s.players {
		if s.players[i].order == order {
			return &s.players[i]
		}
	}
	return nil
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
	books := binary.BigEndian.Uint16(encoded[2:4])
	for i := 0; i < 13; i++ {
		bit := (books >> i) & 1
		if bit == 1 {
			rank := RankFromValue[i+1]
			p.books = append(p.books, Book{rank, p})
		}
	}
	return p, 4, nil
}

type Player struct {
	PlayerOutsideView
	hand []*Card
}

func (p Player) HeldRanks() []Rank {
	ranks := make(map[Rank]struct{}) // Idiomatic way to make a set in Go
	for _, card := range p.hand {
		ranks[card.Rank] = struct{}{}
	}
	held := make([]Rank, 0, len(ranks))
	for rank := range ranks {
		held = append(held, rank)
	}
	return held
}

func (p Player) ValidHeldBooks() []Rank {
	// For each rank, count the number of held cards of that rank
	heldsOfRanks := make(map[Rank]int, len(Ranks))
	for _, rank := range Ranks {
		heldsOfRanks[rank] = 0
		for _, held := range p.hand {
			if held.Rank == rank {
				heldsOfRanks[rank]++
			}
		}
	}
	// Find any ranks of which four cards are held
	books := []Rank{}
	for rank, held := range heldsOfRanks {
		if held >= 4 {
			books = append(books, rank)
		}
	}
	return books
}

// 4 or more bytes: 4-byte public view followed by one byte per held card.
func (p Player) Encode() []byte {
	encodedHand := make([]uint8, 0, p.handSize)
	for _, card := range p.hand {
		encodedHand = append(encodedHand, card.Encode())
	}
	return append(p.PlayerOutsideView.Encode(), encodedHand...)
}

func (p Player) Decode(encoded []byte) (Player, int, error) {
	if len(encoded) < 4 {
		return Player{}, 0, fmt.Errorf("incorrect length for encoded Player: %d", len(encoded))
	}
	outside, decoded, err := p.PlayerOutsideView.Decode(encoded)
	if err != nil {
		return Player{}, 0, err
	}
	p.PlayerOutsideView = outside
	if len(encoded)-decoded < p.handSize {
		return Player{}, 0, fmt.Errorf("incorrect length for encoded Player with hand size %d: %d", p.handSize, len(encoded)-decoded)
	}
	for i := range p.handSize {
		card, err := DecodeCard([]byte{encoded[i+4]})
		if err != nil {
			return Player{}, 0, err
		}
		p.hand = append(p.hand, &card)
		decoded++
	}
	return p, decoded, nil
}

type State struct {
	players      []Player // All players of the game, in order of their turn order
	turnNumber   int      // Total number of turns which have passed
	cards        []Card   // Cards in the game, not modified during play
	recentAction Action   // Most recent action completed before this state
}

func (s State) cardsInPool() []*Card {
	pool := make([]*Card, 0, len(s.cards))
	for _, card := range s.cards {
		if card.Holder == nil {
			pool = append(pool, &card)
		}
	}
	return pool
}

func (s State) topOfPool() *Card {
	if len(s.cardsInPool()) > 0 {
		return s.cardsInPool()[0]
	}
	return nil
}

func (s State) CurrentTurn() *Player {
	order := s.turnNumber % len(s.players)
	for _, player := range s.players {
		if player.order == order {
			return &player
		}
	}
	panic("Current turn number is invalid")
}

// Return the player whose turn follows the current one. Requires that players are ordered by their
// play order. Skips any players who hold no cards.
func (s State) NextTurn() *Player {
	if len(s.players) == 0 {
		return nil
	}
	// Start after the current turn and wrap around play order. Skipping
	// empty-handed players keeps turn advancement valid at the end of the table.
	for distance := 1; distance <= len(s.players); distance++ {
		order := (s.turnNumber + distance) % len(s.players)
		for i := range s.players {
			if s.players[i].order == order && len(s.players[i].hand) > 0 {
				return &s.players[i]
			}
		}
	}
	return nil
}

func (s State) OpponentPlayersOf(player *Player) []*Player {
	opponents := make([]*Player, 0, len(s.players)-1)
	for _, opponent := range s.players {
		if opponent.id != player.id {
			opponents = append(opponents, &opponent)
		}
	}
	return opponents
}

func (s State) AllValidRequestsBy(player *Player) []CardRequest {
	validRequests := make([]CardRequest, 0, len(s.players)*player.handSize)
	for _, opponent := range s.OpponentPlayersOf(player) {
		if opponent.handSize <= 0 { // They don't have any cards to request
			continue
		}
		for _, rank := range player.HeldRanks() {
			validRequests = append(validRequests, CardRequest{from: player, to: opponent, rank: rank})
		}
	}
	return validRequests
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
	playerViews := make([]PlayerOutsideView, 0, len(s.players))
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
	encodedPlayers := make([]byte, 0, len(v.players)*4)
	for _, player := range v.players {
		encodedPlayers = append(encodedPlayers, player.Encode()...)
	}
	turnNumber := uint8(v.turnNumber)
	poolSize := uint8(v.poolSize)
	var recentAction []byte
	if v.recentAction != nil {
		recentAction = v.recentAction.Encode()
	}
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
	if decoded >= len(encoded) {
		return View{}, 0, fmt.Errorf("missing player count in encoded view")
	}
	numPlayers := int(encoded[decoded]) // 1 byte
	decoded++
	v.players = make([]PlayerOutsideView, 0, numPlayers)
	for range numPlayers {
		if decoded >= len(encoded) {
			return View{}, 0, fmt.Errorf("missing player data in encoded view")
		}
		player, pDecoded, err := PlayerOutsideView{}.Decode(encoded[decoded:])
		if err != nil {
			return View{}, 0, err
		}
		v.players = append(v.players, player)
		decoded += pDecoded
	}
	if decoded+2 > len(encoded) {
		return View{}, 0, fmt.Errorf("missing turn and pool data in encoded view")
	}
	v.turnNumber = int(encoded[decoded]) // 1 byte
	decoded++
	v.poolSize = int(encoded[decoded]) // 1 byte
	decoded++
	if decoded < len(encoded) {
		action, consumed, err := DecodeAction(encoded[decoded:])
		if err != nil {
			return View{}, 0, err
		}
		v.recentAction = action
		decoded += consumed
	}
	return v, decoded, nil
}
