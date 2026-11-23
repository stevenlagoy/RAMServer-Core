/* Test helpers: build ServerMessages the way the server would send them, and decode what the client
 * queued to send. */
#ifndef TTT_TEST_HELPERS_H
#define TTT_TEST_HELPERS_H

#include <setjmp.h> /* cmocka needs these first */
#include <stdarg.h>
#include <stddef.h>
#include <stdint.h>

#include <cmocka.h>

#include "client.h"

/* A packed ServerMessage, freed with free() */
typedef ttt_bytes sm_bytes;

sm_bytes sm_connect_response(uint64_t in_reply_to);
sm_bytes sm_auth_response(uint64_t in_reply_to, const char *player_id, const char *token, const char *session_id);
sm_bytes sm_error(uint64_t in_reply_to, int code, const char *message);
sm_bytes sm_leave_response(uint64_t in_reply_to, const char *session_id);
sm_bytes sm_match_result(const char *session_id, const char *winner, const char *termination, int end_reason, unsigned turns);

/* A StateUpdate. players is "id:name:role:ready:connected;..." (ready/connected as 0/1). view may be
 * NULL (empty). annotations is "key=value;key=value" or NULL. */
sm_bytes sm_state(uint64_t in_reply_to, const char *session_id, int status, uint64_t version, const char *players,
                  const char *view, const char *active, const char *annotations);

/* A ListSessionsResponse with n sessions: ids[i], joinable[i] */
sm_bytes sm_list(int n, const char *const *ids, const bool *joinable);
/* A MatchHistoryResponse: winners[i] NULL for a draw */
sm_bytes sm_history(const char *session_id, int n, const char *const *winners);

/* Feeds msg to the client (and frees it); asserts it parsed */
void feed(ttt_client *c, sm_bytes msg);

/* Takes the next queued ClientMessage, unpacked; NULL if none. Free with free_cm. */
Ramserver__V1__ClientMessage *take_cm(ttt_client *c);
void free_cm(Ramserver__V1__ClientMessage *m);
/* Drops every queued message */
void drain(ttt_client *c);

/* True if any log line contains needle */
bool log_has(const ttt_client *c, const char *needle);
/* The last log line's text */
const char *log_last(const ttt_client *c);

#endif
