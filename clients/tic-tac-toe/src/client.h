/* The client's protocol logic and session state, with no I/O and no rendering.
 *
 * The network loop feeds it every frame it receives (client_on_frame) and sends whatever it queues
 * (client_outbox_*). The UI reads its state to draw, and calls the client_cmd_* functions for what the
 * player asks to do. Everything shown about a session comes from the server's messages: commands only
 * queue requests, and the state changes when the replies and broadcasts arrive. */
#ifndef TTT_CLIENT_H
#define TTT_CLIENT_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#include "proto.h"
#include "view.h"

#define TTT_ID_MAX 256
#define TTT_NAME_MAX 128
#define TTT_MAX_MEMBERS 32
#define TTT_LOG_LINES 64
#define TTT_LOG_TEXT 320
#define TTT_MAX_PENDING 64
#define TTT_MAX_RESULTS 128
#define TTT_MAX_LISTED 16
#define TTT_MAX_OUTBOX 64
#define TTT_DISPLAY_NAME_MAX 32 /* we keep names short; the server's own limit isn't documented */

typedef enum {
    PHASE_OFFLINE,    /* no connection: not yet connected, or the connection was lost */
    PHASE_HANDSHAKE,  /* connection open, ConnectRequest sent */
    PHASE_AUTH,       /* AuthRequest sent */
    PHASE_LOBBY,      /* signed in, not in a session */
    PHASE_JOINING,    /* JoinSessionRequest sent */
    PHASE_SESSION,    /* a member of session_id */
    PHASE_LEAVING     /* LeaveSessionRequest sent */
} client_phase;

typedef enum { REQ_NONE, REQ_CONNECT, REQ_AUTH, REQ_JOIN, REQ_LEAVE, REQ_READY, REQ_ACTION, REQ_LIST, REQ_HISTORY } req_kind;

typedef enum { JOIN_NONE, JOIN_QUICK, JOIN_CREATE_PRIVATE, JOIN_CREATE_PUBLIC, JOIN_BY_ID } join_mode;

typedef enum { LOG_INFO, LOG_GOOD, LOG_WARN, LOG_ERROR } log_level;

typedef struct {
    log_level level;
    char text[TTT_LOG_TEXT];
    uint64_t serial; /* increases by one per line, for renderers that print each line once */
} log_line;

typedef struct {
    char id[TTT_ID_MAX];
    char name[TTT_NAME_MAX];
    char role[32];
    bool connected;
    bool ready;
} member;

typedef struct {
    bool draw;
    char winner[TTT_ID_MAX]; /* first winner, if any */
    int end_reason;          /* MatchEndReason */
    char termination[64];    /* the game's termination_reason */
    unsigned turns;
} match_result;

typedef struct {
    char id[TTT_ID_MAX];
    int status;
    int n_players;
    char owner_name[TTT_NAME_MAX];
    bool joinable;
} listed_session;

typedef struct {
    req_kind kind;
    uint64_t seq;
} pending_req;

typedef struct {
    /* Identity and configuration */
    char display_name[TTT_NAME_MAX];
    char client_name[64];
    char player_id[TTT_ID_MAX];
    char reconnect_token[TTT_ID_MAX];
    bool resuming; /* the AuthRequest in flight tries to resume player_id */

    client_phase phase;
    uint64_t next_seq;
    pending_req pending[TTT_MAX_PENDING];

    /* What to do once signed in (from the command line), done once */
    join_mode auto_join;
    char auto_join_id[TTT_ID_MAX];

    /* The current session, exactly as the last StateUpdate described it */
    char session_id[TTT_ID_MAX];
    bool private_session; /* we created it as private */
    int status;           /* SessionStatus */
    uint64_t state_version;
    member members[TTT_MAX_MEMBERS];
    int n_members;
    char owner_id[TTT_ID_MAX];
    char active_id[TTT_ID_MAX];

    /* The board: from the last in-progress StateUpdate. It is kept after the match ends (board_live
     * false) so the final position can be shown with the result. */
    bool has_board;
    bool board_live;
    ttt_view board;
    char x_player[TTT_ID_MAX];
    char o_player[TTT_ID_MAX];
    int moves;
    int last_move;   /* -1 if none */
    bool has_line;
    int line[3];
    char resigned;   /* 'X', 'O' or 0 */

    /* Finished matches in this session: the history when we joined, then each MatchResult */
    match_result results[TTT_MAX_RESULTS];
    int n_results;
    uint64_t results_received; /* MatchResults received since this client started */
    bool has_result; /* the latest MatchResult, shown until the next match starts */
    match_result result;

    /* Public sessions from the last ListSessionsResponse */
    listed_session listed[TTT_MAX_LISTED];
    int n_listed;
    bool listed_valid;
    uint64_t listed_serial; /* increases with each ListSessionsResponse */

    /* Events and errors to show the player, oldest first in a ring */
    log_line log[TTT_LOG_LINES];
    int log_start, log_count;
    uint64_t log_serial;

    /* Serialized ClientMessages waiting to be framed and sent */
    ttt_bytes outbox[TTT_MAX_OUTBOX];
    int outbox_count;
    bool heartbeat_due; /* a heartbeat reply is owed */

    bool fatal; /* the connection can't be used any more (for example, a protocol version mismatch) */
    uint64_t changes; /* incremented on every visible change, so a UI knows when to redraw */
} ttt_client;

void client_init(ttt_client *c, const char *display_name, const char *client_name);
void client_free(ttt_client *c);

/* The connection is open: queues ConnectRequest. */
void client_connected(ttt_client *c);
/* The connection closed or failed. Membership is kept as "unknown": the next sign-in resumes if it can. */
void client_disconnected(ttt_client *c, const char *reason);

/* Handles one received frame payload (len 0 is a heartbeat). Returns false for a payload that isn't a
 * ServerMessage; that is logged and otherwise ignored. */
bool client_on_frame(ttt_client *c, const uint8_t *payload, size_t len);

/* Player commands. Each returns true if it queued a request. When the client can tell the request makes
 * no sense right now (say, moving while not in a match), it logs why and returns false. */
bool client_cmd_quick_match(ttt_client *c);
bool client_cmd_create(ttt_client *c, bool private_session);
bool client_cmd_join(ttt_client *c, const char *session_id);
bool client_cmd_list_sessions(ttt_client *c);
bool client_cmd_leave(ttt_client *c);
bool client_cmd_set_ready(ttt_client *c, bool ready);
bool client_cmd_toggle_ready(ttt_client *c);
/* guard: refuse locally when it's plainly not legal (not your turn, cell taken). With guard false the
 * move is sent anyway, so the server's own error is shown. */
bool client_cmd_move(ttt_client *c, int cell, bool guard);
bool client_cmd_resign(ttt_client *c);

/* Outbox: the network loop takes each message, frames it, sends it, and frees it. */
int client_outbox_count(const ttt_client *c);
ttt_bytes client_outbox_take(ttt_client *c); /* oldest first; the caller frees .data */
bool client_take_heartbeat(ttt_client *c);   /* true once per heartbeat owed */

/* Queries for the UI */
const member *client_me(const ttt_client *c);
const member *client_member(const ttt_client *c, const char *id);
const char *client_name_of(const ttt_client *c, const char *id); /* display name, or the id */
bool client_in_match(const ttt_client *c);   /* a live board in an in-progress session */
bool client_my_turn(const ttt_client *c);
bool client_is_player(const ttt_client *c);
bool client_has_pending(const ttt_client *c, req_kind kind);
/* Wins for id, and draws, among this session's known results */
int client_wins(const ttt_client *c, const char *id);
int client_draws(const ttt_client *c);
/* How a match ended, for people: "three in a row", "resignation", "forfeit", ... */
const char *client_result_how(const match_result *r);
const log_line *client_log_at(const ttt_client *c, int i); /* 0 = oldest kept */

void client_logf(ttt_client *c, log_level level, const char *fmt, ...) __attribute__((format(printf, 3, 4)));

extern const char *const TTT_CELL_NAMES[TTT_CELLS];

#endif
