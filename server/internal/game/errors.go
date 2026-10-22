package game

import "errors"

// ErrMalformedAction indicates that a game's action payload
// could not be decoded.
var ErrMalformedAction = errors.New("malformed action payload")
