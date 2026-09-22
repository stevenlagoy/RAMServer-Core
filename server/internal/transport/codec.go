package transport

import (
	"fmt"

	"github.com/stevenlagoy/ramserver-core/server/internal/game"
)

func DecodeAction(from game.Sender, frame []byte) (game.Action, error) {
	return game.Action{From: from, Payload: frame}, nil // ID 0 until protocol defined
}

func EncodeReject(id uint64, err error) []byte {
	return fmt.Appendf(nil, "reject %d: %v", id, err)
}

func EncodeState(state any) []byte {
	return fmt.Appendf(nil, "state %v", state)
}
