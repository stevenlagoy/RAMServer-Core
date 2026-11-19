package gofish

import "fmt"

type Suit struct {
	name          string
	symbolFilled  string
	symbolOutline string
	offset        int // Value used to differentiate encoded suits. The difference between any two Suit's offsets must be greater than the largest rank value (13).
}

var (
	Spades   Suit = Suit{"spades", "♠", "♤", 0}
	Hearts   Suit = Suit{"hearts", "♥", "♡", 13}
	Diamonds Suit = Suit{"diamonds", "♦", "♢", 26}
	Clubs    Suit = Suit{"clubs", "♣", "♧", 39}
)
var Suits = [...]Suit{Spades, Hearts, Diamonds, Clubs}
var SuitFromValue = map[int]Suit{
	0:  Spades,
	13: Hearts,
	26: Diamonds,
	39: Clubs,
}

type Rank struct {
	name   string
	plural string
	symbol string
	value  int // 1 to 13, also used as an offset for bitmap encoding
}

var (
	Ace   Rank = Rank{"ace", "aces", "A", 1}
	Two   Rank = Rank{"two", "twos", "2", 2}
	Three Rank = Rank{"three", "threes", "3", 3}
	Four  Rank = Rank{"four", "fours", "4", 4}
	Five  Rank = Rank{"five", "fives", "5", 5}
	Six   Rank = Rank{"six", "sixes", "6", 6}
	Seven Rank = Rank{"seven", "sevens", "7", 7}
	Eight Rank = Rank{"eight", "eights", "8", 8}
	Nine  Rank = Rank{"nine", "nines", "9", 9}
	Ten   Rank = Rank{"ten", "tens", "10", 10}
	Jack  Rank = Rank{"jack", "jacks", "J", 11}
	Queen Rank = Rank{"queen", "queens", "Q", 12}
	King  Rank = Rank{"king", "kings", "K", 13}
)
var Ranks = [...]Rank{
	Ace,
	Two,
	Three,
	Four,
	Five,
	Six,
	Seven,
	Eight,
	Nine,
	Ten,
	Jack,
	Queen,
	King,
}
var RankFromValue = map[int]Rank{
	1:  Ace,
	2:  Two,
	3:  Three,
	4:  Four,
	5:  Five,
	6:  Six,
	7:  Seven,
	8:  Eight,
	9:  Nine,
	10: Ten,
	11: Jack,
	12: Queen,
	13: King,
}

type Card struct {
	Suit   Suit
	Rank   Rank
	Holder *Player // When nil, in the pool
}

func (c Card) symbol() string {
	return c.Rank.symbol + c.Suit.symbolFilled
}

// Every card has a different encoding based on its suit's offset and its rank's value
func (c Card) Encode() uint8 {
	return uint8(c.Suit.offset + c.Rank.value)
}

func DecodeCard(encoded []byte) (Card, error) {
	if len(encoded) != 1 {
		return Card{}, fmt.Errorf("incorrect length for encoded card: %d", len(encoded))
	}
	value := encoded[0]
	if value == 0 || value > 52 {
		return Card{}, fmt.Errorf("invalid encoded card: %d", value)
	}
	rankValue := int((value-1)%13) + 1
	rank := RankFromValue[rankValue]
	suitValue := int(value-1) / 13 * 13
	suit := SuitFromValue[int(suitValue)]
	return Card{suit, rank, nil}, nil
}

func makeCards() []Card {
	cards := make([]Card, 0, len(Suits)*len(Ranks))
	for _, suit := range Suits {
		for _, rank := range Ranks {
			cards = append(cards, Card{suit, rank, nil})
		}
	}
	return cards
}
