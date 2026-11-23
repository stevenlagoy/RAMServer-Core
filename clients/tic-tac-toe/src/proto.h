/* Building every ClientMessage this client sends, and naming protocol enums for display. Each builder
 * returns the serialized ClientMessage (not yet framed) in a malloc'd buffer the caller frees with
 * free(); .data is NULL if allocation failed. NULL strings are sent as empty. */
#ifndef TTT_PROTO_H
#define TTT_PROTO_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#include "ramserver/v1/ramserver.pb-c.h"

#define TTT_PROTOCOL_VERSION "1.2"
#define TTT_GAME_ID "tictactoe"

typedef struct {
    uint8_t *data;
    size_t len;
} ttt_bytes;

ttt_bytes proto_connect(uint64_t seq, const char *protocol_version, const char *client_name);
ttt_bytes proto_auth(uint64_t seq, const char *display_name, const char *player_id, const char *reconnect_token);
ttt_bytes proto_join(uint64_t seq, const char *game_id, const char *session_id, bool create_new, bool private_session);
ttt_bytes proto_leave(uint64_t seq, const char *session_id);
ttt_bytes proto_ready(uint64_t seq, const char *session_id, bool ready);
ttt_bytes proto_action(uint64_t seq, const char *session_id, const uint8_t *action, size_t action_len,
                       uint64_t expected_state_version);
ttt_bytes proto_list_sessions(uint64_t seq, const char *game_id);
ttt_bytes proto_match_history(uint64_t seq, const char *session_id);

/* "OUT_OF_TURN" for ERROR_CODE_OUT_OF_TURN, etc. Unknown values give "UNKNOWN". */
const char *proto_error_name(int code);
const char *proto_status_name(int status);
const char *proto_end_reason_name(int reason);

#endif
