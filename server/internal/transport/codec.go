package transport

import (
	"fmt"

	"github.com/stevenlagoy/ramserver-core/server/internal/game"
)

// TODO: These are placeholders and need to be completed based on the protocol

// Decode one message from a sender containing action information
func DecodeAction(frame []byte) (game.Action, error) {
	return game.Action{Payload: frame}, nil // ID and ActorID set by caller; see client.go
}

// Parse a client's first frame as a handshake/auth message
func DecodeHello(frame []byte) (id string, err error) {
	return string(frame), nil
}

// Builds the server's handshake acceptance response
func EncodeWelcome(id string) []byte {
	return fmt.Appendf(nil, "welcome %s", id)
}

// Encode a rejection message from server to client based on an error
func EncodeReject(id uint64, err error) []byte {
	return fmt.Appendf(nil, "reject %d: %v", id, err)
}

// Encode the current state of an object
func EncodeState(state any) []byte {
	return fmt.Appendf(nil, "state %v", state)
}

// Encode the state of a lobby
func EncodeLobbyState(lobby any) []byte {
	return fmt.Appendf(nil, "lobby %v", lobby)
}

// Encode a match-conclusion message
func EncodeMatchEnded(result any) []byte {
	return fmt.Appendf(nil, "match ended %v", result)
}

// Encode a legal-moves message
func EncodeLegalMoves(moves []game.ActionSpec) []byte {
	return fmt.Appendf(nil, "legal moves %v", moves)
}

// Distinguishes post-handshake message kinds needed by lobby/session flows
type ClientMessageKind int

const (
	KindUnknown ClientMessageKind = iota
	KindCreateLobby
	KindJoinLobby
	KindSetReady
	KindSubmitAction
	KindRequestLegalMoves
	KindLeaveLobby
)

// Decoded post-handshake frame
// TODO: These are just guesses at what CreateLobby/JoinLobby/etc will need. Replace these based on protocol
type ClientMessage struct {
	Kind      ClientMessageKind
	GameName  string      // CreateLobby
	Public    bool        // CreateLobby
	SessionID string      // JoinLobby
	Ready     bool        // SetReady
	Action    game.Action // SubmitAction
}

// TODO: distinguish message based on protocol schema
func DecodeClientMessage(frame []byte) (ClientMessage, error) {
	return ClientMessage{}, fmt.Errorf("transport: DecodeClientMessage not implemented")
}
