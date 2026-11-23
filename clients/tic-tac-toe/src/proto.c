#include "proto.h"

#include <stdlib.h>

/* protobuf-c doesn't take const strings; it never writes through them when packing */
static char *str(const char *s) { return (char *)(s ? s : ""); }

static ttt_bytes pack(Ramserver__V1__ClientMessage *msg) {
    ttt_bytes out = {NULL, 0};
    size_t n = ramserver__v1__client_message__get_packed_size(msg);
    out.data = malloc(n > 0 ? n : 1);
    if (out.data == NULL) return out;
    out.len = ramserver__v1__client_message__pack(msg, out.data);
    return out;
}

ttt_bytes proto_connect(uint64_t seq, const char *protocol_version, const char *client_name) {
    Ramserver__V1__ConnectRequest req = RAMSERVER__V1__CONNECT_REQUEST__INIT;
    req.protocol_version = str(protocol_version);
    req.client_name = str(client_name);
    Ramserver__V1__ClientMessage msg = RAMSERVER__V1__CLIENT_MESSAGE__INIT;
    msg.sequence = seq;
    msg.payload_case = RAMSERVER__V1__CLIENT_MESSAGE__PAYLOAD_CONNECT_REQUEST;
    msg.connect_request = &req;
    return pack(&msg);
}

ttt_bytes proto_auth(uint64_t seq, const char *display_name, const char *player_id, const char *reconnect_token) {
    Ramserver__V1__AuthRequest req = RAMSERVER__V1__AUTH_REQUEST__INIT;
    req.display_name = str(display_name);
    req.player_id = str(player_id);
    req.reconnect_token = str(reconnect_token);
    Ramserver__V1__ClientMessage msg = RAMSERVER__V1__CLIENT_MESSAGE__INIT;
    msg.sequence = seq;
    msg.payload_case = RAMSERVER__V1__CLIENT_MESSAGE__PAYLOAD_AUTH_REQUEST;
    msg.auth_request = &req;
    return pack(&msg);
}

ttt_bytes proto_join(uint64_t seq, const char *game_id, const char *session_id, bool create_new, bool private_session) {
    Ramserver__V1__JoinSessionRequest req = RAMSERVER__V1__JOIN_SESSION_REQUEST__INIT;
    req.game_id = str(game_id);
    req.session_id = str(session_id);
    req.create_new = create_new;
    req.private_session = private_session;
    Ramserver__V1__ClientMessage msg = RAMSERVER__V1__CLIENT_MESSAGE__INIT;
    msg.sequence = seq;
    msg.payload_case = RAMSERVER__V1__CLIENT_MESSAGE__PAYLOAD_JOIN_SESSION_REQUEST;
    msg.join_session_request = &req;
    return pack(&msg);
}

ttt_bytes proto_leave(uint64_t seq, const char *session_id) {
    Ramserver__V1__LeaveSessionRequest req = RAMSERVER__V1__LEAVE_SESSION_REQUEST__INIT;
    req.session_id = str(session_id);
    Ramserver__V1__ClientMessage msg = RAMSERVER__V1__CLIENT_MESSAGE__INIT;
    msg.sequence = seq;
    msg.payload_case = RAMSERVER__V1__CLIENT_MESSAGE__PAYLOAD_LEAVE_SESSION_REQUEST;
    msg.leave_session_request = &req;
    return pack(&msg);
}

ttt_bytes proto_ready(uint64_t seq, const char *session_id, bool ready) {
    Ramserver__V1__SetReadyRequest req = RAMSERVER__V1__SET_READY_REQUEST__INIT;
    req.session_id = str(session_id);
    req.ready = ready;
    Ramserver__V1__ClientMessage msg = RAMSERVER__V1__CLIENT_MESSAGE__INIT;
    msg.sequence = seq;
    msg.payload_case = RAMSERVER__V1__CLIENT_MESSAGE__PAYLOAD_SET_READY_REQUEST;
    msg.set_ready_request = &req;
    return pack(&msg);
}

ttt_bytes proto_action(uint64_t seq, const char *session_id, const uint8_t *action, size_t action_len,
                       uint64_t expected_state_version) {
    Ramserver__V1__ActionRequest req = RAMSERVER__V1__ACTION_REQUEST__INIT;
    req.session_id = str(session_id);
    req.encoded_action.data = (uint8_t *)action;
    req.encoded_action.len = action_len;
    req.expected_state_version = expected_state_version;
    Ramserver__V1__ClientMessage msg = RAMSERVER__V1__CLIENT_MESSAGE__INIT;
    msg.sequence = seq;
    msg.payload_case = RAMSERVER__V1__CLIENT_MESSAGE__PAYLOAD_ACTION_REQUEST;
    msg.action_request = &req;
    return pack(&msg);
}

ttt_bytes proto_list_sessions(uint64_t seq, const char *game_id) {
    Ramserver__V1__ListSessionsRequest req = RAMSERVER__V1__LIST_SESSIONS_REQUEST__INIT;
    req.game_id = str(game_id);
    Ramserver__V1__ClientMessage msg = RAMSERVER__V1__CLIENT_MESSAGE__INIT;
    msg.sequence = seq;
    msg.payload_case = RAMSERVER__V1__CLIENT_MESSAGE__PAYLOAD_LIST_SESSIONS_REQUEST;
    msg.list_sessions_request = &req;
    return pack(&msg);
}

ttt_bytes proto_match_history(uint64_t seq, const char *session_id) {
    Ramserver__V1__MatchHistoryRequest req = RAMSERVER__V1__MATCH_HISTORY_REQUEST__INIT;
    req.session_id = str(session_id);
    Ramserver__V1__ClientMessage msg = RAMSERVER__V1__CLIENT_MESSAGE__INIT;
    msg.sequence = seq;
    msg.payload_case = RAMSERVER__V1__CLIENT_MESSAGE__PAYLOAD_MATCH_HISTORY_REQUEST;
    msg.match_history_request = &req;
    return pack(&msg);
}

const char *proto_error_name(int code) {
    static const char *names[] = {
        "UNSPECIFIED",     "INVALID_PROTOCOL_VERSION", "AUTH_FAILED",       "UNKNOWN_GAME",
        "SESSION_NOT_FOUND", "SESSION_NOT_JOINABLE",   "OUT_OF_TURN",       "ILLEGAL_ACTION",
        "MALFORMED_MESSAGE", "INVALID_STATE",          "NOT_IN_SESSION",    "ALREADY_IN_SESSION",
        "INVALID_ARGUMENT",  "STALE_STATE",            "INTERNAL",
    };
    if (code < 0 || code >= (int)(sizeof names / sizeof names[0])) return "UNKNOWN";
    return names[code];
}

const char *proto_status_name(int status) {
    static const char *names[] = {"UNSPECIFIED", "WAITING", "READY", "STARTING", "IN_PROGRESS", "CLOSED"};
    if (status < 0 || status >= (int)(sizeof names / sizeof names[0])) return "UNKNOWN";
    return names[status];
}

const char *proto_end_reason_name(int reason) {
    static const char *names[] = {"UNSPECIFIED", "COMPLETED", "FORFEIT", "DISCONNECT", "ABORTED"};
    if (reason < 0 || reason >= (int)(sizeof names / sizeof names[0])) return "UNKNOWN";
    return names[reason];
}
