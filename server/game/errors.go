package game

import "errors"

// ErrMalformedAction indicates that a game's action payload
// could not be decoded.
var (
	ErrIllegalAction   = errors.New("illegal action")
	ErrMalformedAction = errors.New("malformed action payload")
)
