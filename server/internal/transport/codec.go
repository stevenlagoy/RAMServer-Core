package transport

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/stevenlagoy/ramserver-core/server/game"

	"google.golang.org/protobuf/proto"

	pb "github.com/stevenlagoy/ramserver-core/server/gen/ramserver/v1"
)

// Schema version this server implements, as "<major>.<minor>"
const ProtocolVersion = "1.2"

// Reports whether a client built against clientVersion may connect
func CompatibleVersion(clientVersion string) bool {
	major, _, ok := strings.Cut(clientVersion, ".")
	if !ok || major == "" {
		return false
	}
	serverMajor, _, _ := strings.Cut(ProtocolVersion, ".")
	return major == serverMajor
}

// --- Errors ---

type ErrorCode = pb.ErrorCode

const (
	CodeInvalidProtocolVersion = pb.ErrorCode_ERROR_CODE_INVALID_PROTOCOL_VERSION
	CodeAuthFailed             = pb.ErrorCode_ERROR_CODE_AUTH_FAILED
	CodeUnknownGame            = pb.ErrorCode_ERROR_CODE_UNKNOWN_GAME
	CodeSessionNotFound        = pb.ErrorCode_ERROR_CODE_SESSION_NOT_FOUND
	CodeSessionNotJoinable     = pb.ErrorCode_ERROR_CODE_SESSION_NOT_JOINABLE
	CodeOutOfTurn              = pb.ErrorCode_ERROR_CODE_OUT_OF_TURN
	CodeIllegalAction          = pb.ErrorCode_ERROR_CODE_ILLEGAL_ACTION
	CodeMalformedMessage       = pb.ErrorCode_ERROR_CODE_MALFORMED_MESSAGE
	CodeInvalidState           = pb.ErrorCode_ERROR_CODE_INVALID_STATE
	CodeNotInSession           = pb.ErrorCode_ERROR_CODE_NOT_IN_SESSION
	CodeAlreadyInSession       = pb.ErrorCode_ERROR_CODE_ALREADY_IN_SESSION
	CodeInvalidArgument        = pb.ErrorCode_ERROR_CODE_INVALID_ARGUMENT
	CodeStaleState             = pb.ErrorCode_ERROR_CODE_STALE_STATE
	CodeInternal               = pb.ErrorCode_ERROR_CODE_INTERNAL
)

// Protocol-level failure, reported to the client as an ErrorResponse
type ProtocolError struct {
	Code      ErrorCode
	Message   string
	SessionID string
	Err       error
}

func NewError(code ErrorCode, format string, args ...any) *ProtocolError {
	return &ProtocolError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Wraps err with a protocol error code, keeping err's text as the message
func WrapError(code ErrorCode, err error) *ProtocolError {
	return &ProtocolError{Code: code, Message: err.Error(), Err: err}
}

func (e *ProtocolError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *ProtocolError) Unwrap() error { return e.Err }

// --- Decoding ---

// Identifies which payload a ClientMessage envelope carried
type ClientMessageKind int

const (
	KindUnknown ClientMessageKind = iota
	KindConnect
	KindAuth
	KindJoinSession
	KindLeaveSession
	KindAction
	KindSetReady
	KindLegalActions
	KindListSessions
	KindMatchHistory
)

func (k ClientMessageKind) String() string {
	switch k {
	case KindConnect:
		return "ConnectRequest"
	case KindAuth:
		return "AuthRequest"
	case KindJoinSession:
		return "JoinSessionRequest"
	case KindLeaveSession:
		return "LeaveSessionRequest"
	case KindAction:
		return "ActionRequest"
	case KindSetReady:
		return "SetReadyRequest"
	case KindLegalActions:
		return "LegalActionsRequest"
	case KindListSessions:
		return "ListSessionsRequest"
	case KindMatchHistory:
		return "MatchHistoryRequest"
	default:
		return "Unknown"
	}
}

// Decoded ClientMessage envelope. Only Sequence and the fields for Kind are set.
type ClientMessage struct {
	Kind     ClientMessageKind
	Sequence uint64

	// KindConnect
	ProtocolVersion string
	ClientName      string

	// KindAuth
	DisplayName    string
	PlayerID       string
	ReconnectToken string

	// KindJoinSession (GameID, SessionID, CreateNew, PrivateSession); KindListSessions (GameID, an optional filter);
	// KindLeaveSession, KindAction, KindSetReady, KindLegalActions and KindMatchHistory (SessionID)
	GameID         string
	SessionID      string
	CreateNew      bool
	PrivateSession bool

	// KindSetReady
	Ready bool

	// KindAction. Action.ID is Sequence; ActorID is left for the caller to set
	Action               game.Action
	ExpectedStateVersion uint64
}

// Parse one frame payload as a ClientMessage envelope
func DecodeClientMessage(frame []byte) (ClientMessage, error) {
	var env pb.ClientMessage
	if err := proto.Unmarshal(frame, &env); err != nil {
		return ClientMessage{}, WrapError(CodeMalformedMessage, err)
	}
	msg := ClientMessage{Sequence: env.GetSequence()}
	switch p := env.GetPayload().(type) {
	case *pb.ClientMessage_ConnectRequest:
		msg.Kind = KindConnect
		msg.ProtocolVersion = p.ConnectRequest.GetProtocolVersion()
		msg.ClientName = p.ConnectRequest.GetClientName()
	case *pb.ClientMessage_AuthRequest:
		msg.Kind = KindAuth
		msg.DisplayName = p.AuthRequest.GetDisplayName()
		msg.PlayerID = p.AuthRequest.GetPlayerId()
		msg.ReconnectToken = p.AuthRequest.GetReconnectToken()
	case *pb.ClientMessage_JoinSessionRequest:
		msg.Kind = KindJoinSession
		msg.GameID = p.JoinSessionRequest.GetGameId()
		msg.SessionID = p.JoinSessionRequest.GetSessionId()
		msg.CreateNew = p.JoinSessionRequest.GetCreateNew()
		msg.PrivateSession = p.JoinSessionRequest.GetPrivateSession()
	case *pb.ClientMessage_LeaveSessionRequest:
		msg.Kind = KindLeaveSession
		msg.SessionID = p.LeaveSessionRequest.GetSessionId()
	case *pb.ClientMessage_ActionRequest:
		msg.Kind = KindAction
		msg.SessionID = p.ActionRequest.GetSessionId()
		msg.ExpectedStateVersion = p.ActionRequest.GetExpectedStateVersion()
		msg.Action = game.Action{ID: msg.Sequence, Payload: p.ActionRequest.GetEncodedAction()}
	case *pb.ClientMessage_SetReadyRequest:
		msg.Kind = KindSetReady
		msg.SessionID = p.SetReadyRequest.GetSessionId()
		msg.Ready = p.SetReadyRequest.GetReady()
	case *pb.ClientMessage_LegalActionsRequest:
		msg.Kind = KindLegalActions
		msg.SessionID = p.LegalActionsRequest.GetSessionId()
	case *pb.ClientMessage_ListSessionsRequest:
		msg.Kind = KindListSessions
		msg.GameID = p.ListSessionsRequest.GetGameId()
	case *pb.ClientMessage_MatchHistoryRequest:
		msg.Kind = KindMatchHistory
		msg.SessionID = p.MatchHistoryRequest.GetSessionId()
	default:
		return msg, NewError(CodeMalformedMessage, "client message has no known payload")
	}
	return msg, nil
}

// --- Encoding ---

type SessionStatus = pb.SessionStatus

const (
	StatusWaiting    = pb.SessionStatus_SESSION_STATUS_WAITING
	StatusReady      = pb.SessionStatus_SESSION_STATUS_READY
	StatusStarting   = pb.SessionStatus_SESSION_STATUS_STARTING
	StatusInProgress = pb.SessionStatus_SESSION_STATUS_IN_PROGRESS
	StatusClosed     = pb.SessionStatus_SESSION_STATUS_CLOSED
)

type MatchEndReason = pb.MatchEndReason

const (
	EndCompleted  = pb.MatchEndReason_MATCH_END_REASON_COMPLETED
	EndForfeit    = pb.MatchEndReason_MATCH_END_REASON_FORFEIT
	EndDisconnect = pb.MatchEndReason_MATCH_END_REASON_DISCONNECT
	EndAborted    = pb.MatchEndReason_MATCH_END_REASON_ABORTED
)

// One recipient's snapshot of a session
type StateUpdate struct {
	SessionID      string
	GameID         string
	Status         SessionStatus
	Players        []SessionPlayer
	StateVersion   uint64
	EncodedView    []byte
	ActivePlayerID string
	PlayerOrder    []string
	OwnerPlayerID  string
	Annotations    map[string]string
}

type SessionPlayer struct {
	PlayerID    string
	DisplayName string
	Connected   bool
	Ready       bool
	Role        string
}

type MatchResult struct {
	SessionID         string
	WinnerPlayerIDs   []string
	Draw              bool
	Summaries         []PlayerActionSummary
	Reason            MatchEndReason
	TerminationReason string // Game-defined, like "CHECKMATE", may be empty
	Turns             uint32
}

type PlayerActionSummary struct {
	PlayerID        string
	ActionsAccepted int32
	ActionsRejected int32
}

type SessionInfo struct {
	SessionID     string
	GameID        string
	Status        SessionStatus
	Players       []SessionPlayer
	OwnerPlayerID string
	Joinable      bool
}

// Reply to a successful ConnectRequest
func EncodeConnectResponse(inReplyTo uint64, connectionID string) []byte {
	return marshal(&pb.ServerMessage{
		InReplyTo: inReplyTo,
		Payload: &pb.ServerMessage_ConnectResponse{ConnectResponse: &pb.ConnectResponse{
			ConnectionId:          connectionID,
			ServerProtocolVersion: ProtocolVersion,
		}},
	})
}

// Reply to a successful AuthRequest. Send only to the token's owner, never broadcast
func EncodeAuthResponse(inReplyTo uint64, playerID, reconnectToken, sessionID string) []byte {
	return marshal(&pb.ServerMessage{
		InReplyTo: inReplyTo,
		Payload: &pb.ServerMessage_AuthResponse{AuthResponse: &pb.AuthResponse{
			PlayerId:       playerID,
			ReconnectToken: reconnectToken,
			SessionId:      sessionID,
		}},
	})
}

// Reply to a successful LeaveSessionRequest
func EncodeLeaveSessionResponse(inReplyTo uint64, sessionID string) []byte {
	return marshal(&pb.ServerMessage{
		InReplyTo: inReplyTo,
		Payload: &pb.ServerMessage_LeaveSessionResponse{LeaveSessionResponse: &pb.LeaveSessionResponse{
			SessionId: sessionID,
		}},
	})
}

// Reply to a LegalActionsRequest: the actions playerID may take right now. Send only to that player
func EncodeLegalActionsResponse(inReplyTo uint64, playerID string, actions []game.ActionSpec) []byte {
	specs := make([]*pb.LegalActionSpec, len(actions))
	for i, a := range actions {
		specs[i] = &pb.LegalActionSpec{Payload: a.Payload, Description: a.Description}
	}
	return marshal(&pb.ServerMessage{
		InReplyTo: inReplyTo,
		Payload: &pb.ServerMessage_LegalActionsResponse{LegalActionsResponse: &pb.LegalActionsResponse{
			PlayerId:     playerID,
			LegalActions: specs,
		}},
	})
}

func playersToPB(players []SessionPlayer) []*pb.SessionPlayer {
	out := make([]*pb.SessionPlayer, len(players))
	for i, p := range players {
		out[i] = &pb.SessionPlayer{
			PlayerId:    p.PlayerID,
			DisplayName: p.DisplayName,
			Connected:   p.Connected,
			Ready:       p.Ready,
			Role:        p.Role,
		}
	}
	return out
}

// One recipient's copy of a session broadcast
func EncodeStateUpdate(inReplyTo uint64, s StateUpdate) []byte {
	return marshal(&pb.ServerMessage{
		InReplyTo: inReplyTo,
		Payload: &pb.ServerMessage_StateUpdate{StateUpdate: &pb.StateUpdate{
			SessionId:      s.SessionID,
			GameId:         s.GameID,
			Status:         s.Status,
			Players:        playersToPB(s.Players),
			StateVersion:   s.StateVersion,
			EncodedView:    s.EncodedView,
			ActivePlayerId: s.ActivePlayerID,
			PlayerOrder:    s.PlayerOrder,
			OwnerPlayerId:  s.OwnerPlayerID,
			Annotations:    s.Annotations,
		}},
	})
}

func matchResultToPB(m MatchResult) *pb.MatchResult {
	summaries := make([]*pb.PlayerActionSummary, len(m.Summaries))
	for i, s := range m.Summaries {
		summaries[i] = &pb.PlayerActionSummary{
			PlayerId:        s.PlayerID,
			ActionsAccepted: s.ActionsAccepted,
			ActionsRejected: s.ActionsRejected,
		}
	}
	return &pb.MatchResult{
		SessionId:         m.SessionID,
		WinnerPlayerIds:   m.WinnerPlayerIDs,
		Draw:              m.Draw,
		Summaries:         summaries,
		Reason:            m.Reason,
		TerminationReason: m.TerminationReason,
		Turns:             m.Turns,
	}
}

// Match-conclusion broadcast; always unsolicited, so in_reply_to is 0
func EncodeMatchResult(m MatchResult) []byte {
	return marshal(&pb.ServerMessage{
		Payload: &pb.ServerMessage_MatchResult{MatchResult: matchResultToPB(m)},
	})
}

func EncodeListSessionsResponse(inReplyTo uint64, sessions []SessionInfo) []byte {
	infos := make([]*pb.SessionInfo, len(sessions))
	for i, s := range sessions {
		infos[i] = &pb.SessionInfo{
			SessionId:     s.SessionID,
			GameId:        s.GameID,
			Status:        s.Status,
			Players:       playersToPB(s.Players),
			OwnerPlayerId: s.OwnerPlayerID,
			Joinable:      s.Joinable,
		}
	}
	return marshal(&pb.ServerMessage{
		InReplyTo: inReplyTo,
		Payload: &pb.ServerMessage_ListSessionsResponse{ListSessionsResponse: &pb.ListSessionsResponse{
			Sessions: infos,
		}},
	})
}

func EncodeMatchHistoryResponse(inReplyTo uint64, sessionID string, matches []MatchResult) []byte {
	results := make([]*pb.MatchResult, len(matches))
	for i, m := range matches {
		results[i] = matchResultToPB(m)
	}
	return marshal(&pb.ServerMessage{
		InReplyTo: inReplyTo,
		Payload: &pb.ServerMessage_MatchHistoryResponse{MatchHistoryResponse: &pb.MatchHistoryResponse{
			SessionId: sessionID,
			Matches:   results,
		}},
	})
}

func EncodeError(inReplyTo uint64, err error) []byte {
	resp := &pb.ErrorResponse{Code: CodeInternal, Message: err.Error()}
	var perr *ProtocolError
	if errors.As(err, &perr) {
		resp.Code = perr.Code
		resp.Message = perr.Message
		resp.SessionId = perr.SessionID
	}
	return marshal(&pb.ServerMessage{
		InReplyTo: inReplyTo,
		Payload:   &pb.ServerMessage_ErrorResponse{ErrorResponse: resp},
	})
}

// Serialize msg. Marshalling only fails on invalid UTF-8 in a string field;
// rather than drop the reply, fall back to a fixed ErrorResponse so the
// request still gets exactly one answer. The result is never empty, since a
// set oneof field always encodes, so it cannot be mistaken for a heartbeat.
func marshal(msg *pb.ServerMessage) []byte {
	b, err := proto.Marshal(msg)
	if err == nil {
		return b
	}
	log.Printf("transport: encode %T: %v", msg.GetPayload(), err)
	b, err = proto.Marshal(&pb.ServerMessage{
		InReplyTo: msg.GetInReplyTo(),
		Payload: &pb.ServerMessage_ErrorResponse{ErrorResponse: &pb.ErrorResponse{
			Code:    CodeInternal,
			Message: "server failed to encode response",
		}},
	})
	if err != nil {
		panic(fmt.Sprintf("transport: encode fallback error response: %v", err))
	}
	return b
}
