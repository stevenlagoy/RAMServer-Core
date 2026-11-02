package game

import "errors"

var (
	ErrIllegalAction   = errors.New("illegal action")   // ERROR_CODE_ILLEGAL_ACTION
	ErrMalformedAction = errors.New("malformed action") // ERROR_CODE_MALFORMED_MESSAGE
)
