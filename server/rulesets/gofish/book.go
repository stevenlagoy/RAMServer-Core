package gofish

type Book struct {
	rank   Rank
	player PlayerOutsideView
}

func (b Book) Encode() uint16 {
	return 1 << (b.rank.value - 1)
}
