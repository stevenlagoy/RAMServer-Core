package chess

type View struct {
	FEN      string
	LastMove string // UCI, empty at start
	InCheck  bool
	White    string // Player ID, clients need to know their color
	Black    string // Player ID, clients need to know their color
}
