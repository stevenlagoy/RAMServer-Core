package transport

import (
	"fmt"

	"github.com/stevenlagoy/ramserver-core/server/internal/game"
)

func DecodeAction(from game.Sender, frame []byte) (game.Action, error) {
	return game.Action{From: from, Payload: frame}, nil // ID 0 until protocol defined
}

// DecodeHello parses a client's first frame as a handshake/auth message. PLACEHOLDER
func DecodeHello(frame []byte) (id string, err error) {
	return string(frame), nil
}

// EncodeWelcome builds the server's handshake acceptance response. PLACEHOLDER
func EncodeWelcome(id string) []byte {
	return fmt.Appendf(nil, "welcome %s", id)
}

func EncodeReject(id uint64, err error) []byte {
	return fmt.Appendf(nil, "reject %d: %v", id, err)
}

func EncodeState(state any) []byte {
	return fmt.Appendf(nil, "state %v", state)
}
