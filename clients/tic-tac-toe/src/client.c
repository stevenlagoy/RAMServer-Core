#include "client.h"

#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

typedef Ramserver__V1__ServerMessage ServerMessage;

#define ERR(name) RAMSERVER__V1__ERROR_CODE__ERROR_CODE_##name
#define STATUS(name) RAMSERVER__V1__SESSION_STATUS__SESSION_STATUS_##name

const char *const TTT_CELL_NAMES[TTT_CELLS] = {
    "top-left",    "top-center", "top-right",    "middle-left", "center",
    "middle-right", "bottom-left", "bottom-center", "bottom-right",
};

/* ---- small helpers ---- */

static void copy_str(char *dst, size_t cap, const char *src) {
    if (src == NULL) src = "";
    size_t n = strlen(src);
    if (n >= cap) n = cap - 1;
    memcpy(dst, src, n);
    dst[n] = '\0';
}
#define COPY(dst, src) copy_str((dst), sizeof(dst), (src))

static bool str_eq(const char *a, const char *b) { return strcmp(a ? a : "", b ? b : "") == 0; }

static void changed(ttt_client *c) { c->changes++; }

void client_logf(ttt_client *c, log_level level, const char *fmt, ...) {
    int idx;
    if (c->log_count < TTT_LOG_LINES) {
        idx = (c->log_start + c->log_count) % TTT_LOG_LINES;
        c->log_count++;
    } else {
        idx = c->log_start;
        c->log_start = (c->log_start + 1) % TTT_LOG_LINES;
    }
    log_line *l = &c->log[idx];
    l->level = level;
    l->serial = ++c->log_serial;
    va_list ap;
    va_start(ap, fmt);
    vsnprintf(l->text, sizeof l->text, fmt, ap);
    va_end(ap);
    changed(c);
}

const log_line *client_log_at(const ttt_client *c, int i) {
    if (i < 0 || i >= c->log_count) return NULL;
    return &c->log[(c->log_start + i) % TTT_LOG_LINES];
}

/* ---- outbox and pending requests ---- */

static void enqueue(ttt_client *c, ttt_bytes msg, req_kind kind, uint64_t seq) {
    if (msg.data == NULL) {
        client_logf(c, LOG_ERROR, "Out of memory building a request");
        return;
    }
    if (c->outbox_count == TTT_MAX_OUTBOX) { /* the network loop isn't draining: drop the oldest */
        free(c->outbox[0].data);
        memmove(c->outbox, c->outbox + 1, sizeof c->outbox[0] * (TTT_MAX_OUTBOX - 1));
        c->outbox_count--;
    }
    c->outbox[c->outbox_count++] = msg;

    int slot = 0;
    for (int i = 0; i < TTT_MAX_PENDING; i++) {
        if (c->pending[i].kind == REQ_NONE) { slot = i; break; }
        if (c->pending[i].seq < c->pending[slot].seq) slot = i; /* full: reuse the oldest */
    }
    c->pending[slot].kind = kind;
    c->pending[slot].seq = seq;
}

static uint64_t next_seq(ttt_client *c) { return c->next_seq++; }

/* Removes and returns the kind of the request that in_reply_to answers, or REQ_NONE */
static req_kind take_pending(ttt_client *c, uint64_t in_reply_to) {
    if (in_reply_to == 0) return REQ_NONE;
    for (int i = 0; i < TTT_MAX_PENDING; i++) {
        if (c->pending[i].kind != REQ_NONE && c->pending[i].seq == in_reply_to) {
            req_kind k = c->pending[i].kind;
            c->pending[i].kind = REQ_NONE;
            return k;
        }
    }
    return REQ_NONE;
}

bool client_has_pending(const ttt_client *c, req_kind kind) {
    for (int i = 0; i < TTT_MAX_PENDING; i++)
        if (c->pending[i].kind == kind) return true;
    return false;
}

static void clear_pending(ttt_client *c) {
    for (int i = 0; i < TTT_MAX_PENDING; i++) c->pending[i].kind = REQ_NONE;
}

int client_outbox_count(const ttt_client *c) { return c->outbox_count; }

ttt_bytes client_outbox_take(ttt_client *c) {
    ttt_bytes out = {NULL, 0};
    if (c->outbox_count == 0) return out;
    out = c->outbox[0];
    memmove(c->outbox, c->outbox + 1, sizeof c->outbox[0] * (size_t)(c->outbox_count - 1));
    c->outbox_count--;
    return out;
}

bool client_take_heartbeat(ttt_client *c) {
    bool due = c->heartbeat_due;
    c->heartbeat_due = false;
    return due;
}

static void clear_outbox(ttt_client *c) {
    for (int i = 0; i < c->outbox_count; i++) free(c->outbox[i].data);
    c->outbox_count = 0;
    c->heartbeat_due = false;
}

/* ---- lifecycle ---- */

static void reset_board(ttt_client *c) {
    c->has_board = false;
    c->board_live = false;
    c->x_player[0] = c->o_player[0] = '\0';
    c->moves = 0;
    c->last_move = -1;
    c->has_line = false;
    c->resigned = 0;
}

static void reset_session(ttt_client *c) {
    c->session_id[0] = '\0';
    c->private_session = false;
    c->status = 0;
    c->state_version = 0;
    c->n_members = 0;
    c->owner_id[0] = c->active_id[0] = '\0';
    reset_board(c);
    c->n_results = 0;
    c->has_result = false;
}

void client_init(ttt_client *c, const char *display_name, const char *client_name) {
    memset(c, 0, sizeof *c);
    COPY(c->display_name, display_name);
    COPY(c->client_name, client_name);
    c->next_seq = 1; /* in_reply_to 0 means "unsolicited", so sequences start at 1 */
    c->phase = PHASE_OFFLINE;
    reset_session(c);
}

void client_free(ttt_client *c) { clear_outbox(c); }

void client_connected(ttt_client *c) {
    clear_outbox(c);
    clear_pending(c);
    c->fatal = false;
    c->phase = PHASE_HANDSHAKE;
    uint64_t seq = next_seq(c);
    enqueue(c, proto_connect(seq, TTT_PROTOCOL_VERSION, c->client_name), REQ_CONNECT, seq);
    client_logf(c, LOG_INFO, "Connected; saying hello (protocol %s)", TTT_PROTOCOL_VERSION);
}

void client_disconnected(ttt_client *c, const char *reason) {
    bool was_offline = c->phase == PHASE_OFFLINE;
    clear_outbox(c);
    clear_pending(c);
    c->phase = PHASE_OFFLINE;
    /* We can't be sure what happened to our seat. Forget the session; a resumed sign-in brings it back. */
    reset_session(c);
    c->listed_valid = false;
    if (!was_offline) client_logf(c, LOG_ERROR, "Connection lost: %s", reason && *reason ? reason : "closed");
    changed(c);
}

static void send_auth(ttt_client *c) {
    uint64_t seq = next_seq(c);
    c->resuming = c->player_id[0] != '\0' && c->reconnect_token[0] != '\0';
    if (c->resuming) {
        enqueue(c, proto_auth(seq, c->display_name, c->player_id, c->reconnect_token), REQ_AUTH, seq);
        client_logf(c, LOG_INFO, "Signing in again as %s", c->display_name);
    } else {
        enqueue(c, proto_auth(seq, c->display_name, NULL, NULL), REQ_AUTH, seq);
        client_logf(c, LOG_INFO, "Signing in as %s", c->display_name);
    }
    c->phase = PHASE_AUTH;
}

/* ---- commands ---- */

static bool require_lobby(ttt_client *c) {
    switch (c->phase) {
    case PHASE_LOBBY: return true;
    case PHASE_SESSION:
    case PHASE_LEAVING: client_logf(c, LOG_WARN, "You're already in a session; leave it first (L)"); return false;
    case PHASE_JOINING: client_logf(c, LOG_WARN, "Still joining a session..."); return false;
    default: client_logf(c, LOG_WARN, "Not signed in yet"); return false;
    }
}

static bool join(ttt_client *c, const char *session_id, bool create_new, bool private_session) {
    if (!require_lobby(c)) return false;
    uint64_t seq = next_seq(c);
    enqueue(c, proto_join(seq, TTT_GAME_ID, session_id, create_new, private_session), REQ_JOIN, seq);
    reset_session(c);
    c->private_session = create_new && private_session;
    c->phase = PHASE_JOINING;
    changed(c);
    return true;
}

bool client_cmd_quick_match(ttt_client *c) {
    if (!join(c, NULL, false, false)) return false;
    client_logf(c, LOG_INFO, "Looking for a game (quick match)...");
    return true;
}

bool client_cmd_create(ttt_client *c, bool private_session) {
    if (!join(c, NULL, true, private_session)) return false;
    client_logf(c, LOG_INFO, "Creating a %s session...", private_session ? "private" : "public");
    return true;
}

bool client_cmd_join(ttt_client *c, const char *session_id) {
    if (session_id == NULL || *session_id == '\0') {
        client_logf(c, LOG_WARN, "Enter a session id to join");
        return false;
    }
    if (strlen(session_id) >= TTT_ID_MAX) {
        client_logf(c, LOG_WARN, "That session id is too long");
        return false;
    }
    if (!join(c, session_id, false, false)) return false;
    client_logf(c, LOG_INFO, "Joining session %s...", session_id);
    return true;
}

bool client_cmd_list_sessions(ttt_client *c) {
    if (c->phase < PHASE_LOBBY) {
        client_logf(c, LOG_WARN, "Not signed in yet");
        return false;
    }
    uint64_t seq = next_seq(c);
    enqueue(c, proto_list_sessions(seq, TTT_GAME_ID), REQ_LIST, seq);
    return true;
}

bool client_cmd_leave(ttt_client *c) {
    if (c->phase != PHASE_SESSION) {
        if (c->phase == PHASE_LEAVING) client_logf(c, LOG_WARN, "Already leaving...");
        else client_logf(c, LOG_WARN, "You're not in a session");
        return false;
    }
    uint64_t seq = next_seq(c);
    enqueue(c, proto_leave(seq, c->session_id), REQ_LEAVE, seq);
    c->phase = PHASE_LEAVING;
    client_logf(c, LOG_INFO, "Leaving the session...");
    return true;
}

bool client_cmd_set_ready(ttt_client *c, bool ready) {
    if (c->phase != PHASE_SESSION) {
        client_logf(c, LOG_WARN, "Join a session first");
        return false;
    }
    if (c->status == STATUS(IN_PROGRESS)) {
        client_logf(c, LOG_WARN, "The match is already being played");
        return false;
    }
    uint64_t seq = next_seq(c);
    enqueue(c, proto_ready(seq, c->session_id, ready), REQ_READY, seq);
    return true;
}

bool client_cmd_toggle_ready(ttt_client *c) {
    const member *me = client_me(c);
    return client_cmd_set_ready(c, !(me && me->ready));
}

static bool send_action(ttt_client *c, const uint8_t *action, size_t len) {
    uint64_t seq = next_seq(c);
    /* expected_state_version: never apply a move chosen against an out-of-date board */
    enqueue(c, proto_action(seq, c->session_id, action, len, c->state_version), REQ_ACTION, seq);
    return true;
}

bool client_cmd_move(ttt_client *c, int cell, bool guard) {
    if (cell < 0 || cell >= TTT_CELLS) return false;
    if (c->phase != PHASE_SESSION || (guard && !client_in_match(c))) {
        client_logf(c, LOG_WARN, "There's no match in progress");
        return false;
    }
    if (guard) {
        if (!client_is_player(c)) {
            client_logf(c, LOG_WARN, "Spectators can't move");
            return false;
        }
        if (!client_my_turn(c)) {
            client_logf(c, LOG_WARN, "It's %s's turn", client_name_of(c, c->active_id));
            return false;
        }
        if (c->board.cells[cell] != '.') {
            client_logf(c, LOG_WARN, "%s is taken", TTT_CELL_NAMES[cell]);
            return false;
        }
        if (client_has_pending(c, REQ_ACTION)) {
            client_logf(c, LOG_WARN, "Waiting for the server to answer your last move...");
            return false;
        }
    }
    uint8_t action[1];
    ttt_action_move(cell, action);
    return send_action(c, action, sizeof action);
}

bool client_cmd_resign(ttt_client *c) {
    if (c->phase != PHASE_SESSION || !client_in_match(c)) {
        client_logf(c, LOG_WARN, "There's no match in progress");
        return false;
    }
    if (!client_is_player(c)) {
        client_logf(c, LOG_WARN, "Spectators can't resign");
        return false;
    }
    client_logf(c, LOG_INFO, "Resigning...");
    return send_action(c, TTT_ACTION_RESIGN, sizeof TTT_ACTION_RESIGN);
}

static void do_auto_join(ttt_client *c) {
    join_mode mode = c->auto_join;
    c->auto_join = JOIN_NONE; /* only once */
    switch (mode) {
    case JOIN_QUICK: client_cmd_quick_match(c); break;
    case JOIN_CREATE_PRIVATE: client_cmd_create(c, true); break;
    case JOIN_CREATE_PUBLIC: client_cmd_create(c, false); break;
    case JOIN_BY_ID: client_cmd_join(c, c->auto_join_id); break;
    case JOIN_NONE: break;
    }
}

/* ---- queries ---- */

const member *client_member(const ttt_client *c, const char *id) {
    if (id == NULL || *id == '\0') return NULL;
    for (int i = 0; i < c->n_members; i++)
        if (strcmp(c->members[i].id, id) == 0) return &c->members[i];
    return NULL;
}

const member *client_me(const ttt_client *c) { return client_member(c, c->player_id); }

const char *client_name_of(const ttt_client *c, const char *id) {
    const member *m = client_member(c, id);
    if (m && m->name[0]) return m->name;
    if (id && str_eq(id, c->player_id)) return c->display_name;
    return id && *id ? id : "nobody";
}

bool client_in_match(const ttt_client *c) {
    return c->phase == PHASE_SESSION && c->status == STATUS(IN_PROGRESS) && c->has_board && c->board_live &&
           ttt_view_running(&c->board);
}

bool client_is_player(const ttt_client *c) {
    if (c->has_board) return c->board.me == 'X' || c->board.me == 'O';
    const member *me = client_me(c);
    return me && strcmp(me->role, "player") == 0;
}

bool client_my_turn(const ttt_client *c) { return client_in_match(c) && ttt_view_my_turn(&c->board); }

int client_wins(const ttt_client *c, const char *id) {
    int n = 0;
    for (int i = 0; i < c->n_results; i++)
        if (!c->results[i].draw && str_eq(c->results[i].winner, id)) n++;
    return n;
}

int client_draws(const ttt_client *c) {
    int n = 0;
    for (int i = 0; i < c->n_results; i++)
        if (c->results[i].draw) n++;
    return n;
}

/* ---- handling server messages ---- */

static void on_connect_response(ttt_client *c, const Ramserver__V1__ConnectResponse *r) {
    if (c->phase != PHASE_HANDSHAKE) return;
    client_logf(c, LOG_INFO, "Server speaks protocol %s", r->server_protocol_version);
    send_auth(c);
}

static void on_auth_response(ttt_client *c, const Ramserver__V1__AuthResponse *r) {
    if (c->phase != PHASE_AUTH) return;
    COPY(c->player_id, r->player_id);
    COPY(c->reconnect_token, r->reconnect_token);
    c->resuming = false;
    if (r->session_id && *r->session_id) {
        /* Resumed into a session: its StateUpdate follows */
        COPY(c->session_id, r->session_id);
        c->phase = PHASE_SESSION;
        client_logf(c, LOG_GOOD, "Signed in as %s; back in session %s", c->display_name, c->session_id);
        uint64_t seq = next_seq(c);
        enqueue(c, proto_match_history(seq, c->session_id), REQ_HISTORY, seq);
    } else {
        c->phase = PHASE_LOBBY;
        client_logf(c, LOG_GOOD, "Signed in as %s", c->display_name);
        do_auto_join(c);
    }
    changed(c);
}

static void copy_result(match_result *dst, const Ramserver__V1__MatchResult *r) {
    memset(dst, 0, sizeof *dst);
    dst->draw = r->draw || r->n_winner_player_ids == 0;
    if (r->n_winner_player_ids > 0) COPY(dst->winner, r->winner_player_ids[0]);
    dst->end_reason = (int)r->reason;
    COPY(dst->termination, r->termination_reason);
    dst->turns = r->turns;
}

static void push_result(ttt_client *c, const match_result *r) {
    if (c->n_results == TTT_MAX_RESULTS) {
        memmove(c->results, c->results + 1, sizeof c->results[0] * (TTT_MAX_RESULTS - 1));
        c->n_results--;
    }
    c->results[c->n_results++] = *r;
}

const char *client_result_how(const match_result *r) {
    if (strcmp(r->termination, "THREE_IN_A_ROW") == 0) return "three in a row";
    if (strcmp(r->termination, "RESIGNATION") == 0) return "resignation";
    if (strcmp(r->termination, "DRAW") == 0) return "a full board";
    if (strcmp(r->termination, "NOT_ENOUGH_PLAYERS") == 0) return "not enough players";
    if (r->end_reason == RAMSERVER__V1__MATCH_END_REASON__MATCH_END_REASON_FORFEIT) return "forfeit";
    if (r->end_reason == RAMSERVER__V1__MATCH_END_REASON__MATCH_END_REASON_DISCONNECT) return "disconnect";
    if (r->end_reason == RAMSERVER__V1__MATCH_END_REASON__MATCH_END_REASON_ABORTED) return "the server aborting it";
    return r->termination[0] ? r->termination : proto_end_reason_name(r->end_reason);
}

static void on_match_result(ttt_client *c, const Ramserver__V1__MatchResult *r) {
    if (c->phase != PHASE_SESSION && c->phase != PHASE_LEAVING) return;
    if (r->session_id && *r->session_id && !str_eq(r->session_id, c->session_id)) return;
    match_result res;
    copy_result(&res, r);
    push_result(c, &res);
    c->results_received++;
    c->result = res;
    c->has_result = true;
    const char *how = client_result_how(&res);
    if (res.draw) {
        client_logf(c, LOG_INFO, "Draw, by %s, after %u turns", how, res.turns);
    } else if (str_eq(res.winner, c->player_id)) {
        client_logf(c, LOG_GOOD, "You win by %s!", how);
    } else if (client_is_player(c)) {
        client_logf(c, LOG_WARN, "%s wins by %s", client_name_of(c, res.winner), how);
    } else {
        client_logf(c, LOG_INFO, "%s wins by %s", client_name_of(c, res.winner), how);
    }
    changed(c);
}

/* Logs what changed between the old and new roster: joins, leaves, readiness, connection */
static void diff_members(ttt_client *c, const member *old, int n_old, bool ready_reset) {
    for (int i = 0; i < c->n_members; i++) {
        const member *m = &c->members[i];
        const member *was = NULL;
        for (int j = 0; j < n_old; j++)
            if (strcmp(old[j].id, m->id) == 0) was = &old[j];
        bool me = str_eq(m->id, c->player_id);
        if (was == NULL) {
            if (!me) client_logf(c, LOG_INFO, "%s joined as %s", m->name, m->role);
            continue;
        }
        if (was->ready != m->ready && strcmp(m->role, "player") == 0 && !(ready_reset && !m->ready))
            client_logf(c, LOG_INFO, "%s %s", me ? "You are" : m->name, m->ready ? "ready" : "not ready");
        if (was->connected != m->connected && !me)
            client_logf(c, m->connected ? LOG_INFO : LOG_WARN, "%s %s", m->name,
                        m->connected ? "reconnected" : "lost their connection");
    }
    for (int j = 0; j < n_old; j++) {
        if (client_member(c, old[j].id) == NULL && !str_eq(old[j].id, c->player_id))
            client_logf(c, LOG_WARN, "%s left", old[j].name);
    }
}

static void read_annotations(ttt_client *c, const Ramserver__V1__StateUpdate *u) {
    c->last_move = -1;
    c->has_line = false;
    c->resigned = 0;
    for (size_t i = 0; i < u->n_annotations; i++) {
        const char *k = u->annotations[i]->key ? u->annotations[i]->key : "";
        const char *v = u->annotations[i]->value ? u->annotations[i]->value : "";
        if (strcmp(k, "x_player") == 0) COPY(c->x_player, v);
        else if (strcmp(k, "o_player") == 0) COPY(c->o_player, v);
        else if (strcmp(k, "moves") == 0) c->moves = atoi(v);
        else if (strcmp(k, "last_move") == 0) c->last_move = (v[0] >= '0' && v[0] <= '8' && !v[1]) ? v[0] - '0' : -1;
        else if (strcmp(k, "winning_line") == 0) c->has_line = ttt_parse_line(v, c->line);
        else if (strcmp(k, "resigned") == 0) c->resigned = (v[0] == 'X' || v[0] == 'O') ? v[0] : 0;
        /* unknown keys are ignored */
    }
}

static void on_state_update(ttt_client *c, const Ramserver__V1__StateUpdate *u, req_kind replied_to) {
    const char *sid = u->session_id ? u->session_id : "";
    if (c->phase == PHASE_JOINING) {
        /* The requester's copy of the broadcast that includes it is the join's reply (docs/protocol.md) */
        if (replied_to != REQ_JOIN) return;
        COPY(c->session_id, sid);
        c->phase = PHASE_SESSION;
        c->state_version = 0;
        client_logf(c, LOG_GOOD, "Joined %ssession %s", c->private_session ? "private " : "", sid);
        uint64_t seq = next_seq(c);
        enqueue(c, proto_match_history(seq, c->session_id), REQ_HISTORY, seq);
    } else if (c->phase == PHASE_SESSION || c->phase == PHASE_LEAVING) {
        if (!str_eq(sid, c->session_id)) return;        /* not our session (a late broadcast) */
        if (u->state_version < c->state_version) return; /* older than what we have */
    } else {
        return; /* not in a session: a late broadcast after leaving */
    }

    /* Roster */
    member old[TTT_MAX_MEMBERS];
    int n_old = c->n_members;
    memcpy(old, c->members, sizeof(member) * (size_t)n_old);
    c->n_members = 0;
    for (size_t i = 0; i < u->n_players && c->n_members < TTT_MAX_MEMBERS; i++) {
        const Ramserver__V1__SessionPlayer *p = u->players[i];
        member *m = &c->members[c->n_members++];
        COPY(m->id, p->player_id);
        COPY(m->name, p->display_name && *p->display_name ? p->display_name : p->player_id);
        COPY(m->role, p->role);
        m->connected = p->connected;
        m->ready = p->ready;
    }
    bool first = n_old == 0 && c->state_version == 0;
    /* When a match ends the server clears every ready flag: not news */
    bool ready_reset = c->status == STATUS(IN_PROGRESS) && u->status != STATUS(IN_PROGRESS);
    if (!first) diff_members(c, old, n_old, ready_reset);

    int old_status = c->status;
    c->status = (int)u->status;
    c->state_version = u->state_version;
    COPY(c->owner_id, u->owner_player_id);
    COPY(c->active_id, u->active_player_id);

    /* Board. A valid view is always kept (so a final position is shown however the server labels that
     * update); it is playable only while the session is in progress. */
    ttt_view v;
    bool decoded = ttt_view_decode(u->encoded_view.data, u->encoded_view.len, &v);
    bool live = decoded && c->status == STATUS(IN_PROGRESS);
    if (decoded) {
        bool new_match = live && (old_status != STATUS(IN_PROGRESS) || !c->board_live);
        if (new_match) {
            reset_board(c);
            c->has_result = false;
        }
        c->board = v;
        c->has_board = true;
        c->board_live = live;
        read_annotations(c, u);
        if (new_match) {
            if (v.me == 'S') client_logf(c, LOG_GOOD, "Match started: you're watching");
            else client_logf(c, LOG_GOOD, "Match started: you are %c%s", v.me, v.me == 'X' ? " and move first" : "");
        }
    } else {
        if (u->encoded_view.len > 0)
            client_logf(c, LOG_ERROR, "Couldn't read the board the server sent (%zu bytes)", u->encoded_view.len);
        c->board_live = false; /* keep the last board to show with the result */
    }
    if (old_status == STATUS(IN_PROGRESS) && c->status == STATUS(WAITING))
        client_logf(c, LOG_INFO, "Back in the lobby. Press R to ready up for a rematch");
    if (c->status == STATUS(CLOSED) && old_status != STATUS(CLOSED)) client_logf(c, LOG_WARN, "The session has closed");
    changed(c);
}

static void on_leave_response(ttt_client *c, const Ramserver__V1__LeaveSessionResponse *r) {
    (void)r;
    if (c->phase != PHASE_LEAVING && c->phase != PHASE_SESSION) return;
    reset_session(c);
    c->phase = PHASE_LOBBY;
    client_logf(c, LOG_INFO, "You left the session");
}

static void on_list_sessions(ttt_client *c, const Ramserver__V1__ListSessionsResponse *r) {
    c->n_listed = 0;
    for (size_t i = 0; i < r->n_sessions && c->n_listed < TTT_MAX_LISTED; i++) {
        const Ramserver__V1__SessionInfo *s = r->sessions[i];
        if (!str_eq(s->game_id, TTT_GAME_ID)) continue;
        listed_session *l = &c->listed[c->n_listed++];
        COPY(l->id, s->session_id);
        l->status = (int)s->status;
        l->n_players = (int)s->n_players;
        l->joinable = s->joinable;
        l->owner_name[0] = '\0';
        for (size_t j = 0; j < s->n_players; j++)
            if (str_eq(s->players[j]->player_id, s->owner_player_id)) COPY(l->owner_name, s->players[j]->display_name);
    }
    c->listed_valid = true;
    c->listed_serial++;
    client_logf(c, LOG_INFO, "%d public tic-tac-toe session%s", c->n_listed, c->n_listed == 1 ? "" : "s");
}

static void on_match_history(ttt_client *c, const Ramserver__V1__MatchHistoryResponse *r) {
    if (c->phase != PHASE_SESSION || !str_eq(r->session_id, c->session_id)) return;
    /* History holds every finished match, including any MatchResult we already counted */
    c->n_results = 0;
    for (size_t i = 0; i < r->n_matches; i++) {
        match_result res;
        copy_result(&res, r->matches[i]);
        push_result(c, &res);
    }
    if (r->n_matches > 0)
        client_logf(c, LOG_INFO, "This session has played %zu match%s so far", r->n_matches, r->n_matches == 1 ? "" : "es");
    changed(c);
}

static void on_error(ttt_client *c, const Ramserver__V1__ErrorResponse *e, req_kind kind) {
    int code = (int)e->code;
    const char *msg = e->message && *e->message ? e->message : "(no details)";

    if (kind == REQ_AUTH && c->resuming && code == ERR(AUTH_FAILED)) {
        /* docs/writing-a-client.md: resuming isn't implemented yet; sign in as a new player instead */
        client_logf(c, LOG_WARN, "Couldn't resume the old identity (%s); signing in as a new player", msg);
        c->player_id[0] = c->reconnect_token[0] = '\0';
        send_auth(c);
        return;
    }

    client_logf(c, LOG_ERROR, "Server: %s: %s", proto_error_name(code), msg);
    switch (kind) {
    case REQ_CONNECT:
        c->fatal = true; /* e.g. INVALID_PROTOCOL_VERSION: the server closes the connection */
        break;
    case REQ_AUTH:
        c->fatal = true; /* e.g. INVALID_ARGUMENT for the display name */
        c->phase = PHASE_HANDSHAKE;
        break;
    case REQ_JOIN:
        if (c->phase == PHASE_JOINING) {
            reset_session(c);
            c->phase = PHASE_LOBBY;
            if (code == ERR(UNKNOWN_GAME))
                client_logf(c, LOG_WARN, "This server doesn't have tic-tac-toe installed yet");
        }
        break;
    case REQ_LEAVE:
        if (code == ERR(NOT_IN_SESSION)) {
            reset_session(c);
            c->phase = PHASE_LOBBY;
        } else if (c->phase == PHASE_LEAVING) {
            c->phase = PHASE_SESSION;
        }
        break;
    case REQ_ACTION:
        if (code == ERR(STALE_STATE)) client_logf(c, LOG_WARN, "The board changed before your move arrived; try again");
        break;
    default: break;
    }
    changed(c);
}

bool client_on_frame(ttt_client *c, const uint8_t *payload, size_t len) {
    if (len == 0) { /* heartbeat: answer it with one */
        c->heartbeat_due = true;
        return true;
    }
    ServerMessage *m = ramserver__v1__server_message__unpack(NULL, len, payload);
    if (m == NULL) {
        client_logf(c, LOG_ERROR, "Received a message that isn't a valid ServerMessage (%zu bytes); ignored", len);
        return false;
    }
    req_kind kind = take_pending(c, m->in_reply_to);
    switch (m->payload_case) {
    case RAMSERVER__V1__SERVER_MESSAGE__PAYLOAD_CONNECT_RESPONSE: on_connect_response(c, m->connect_response); break;
    case RAMSERVER__V1__SERVER_MESSAGE__PAYLOAD_AUTH_RESPONSE: on_auth_response(c, m->auth_response); break;
    case RAMSERVER__V1__SERVER_MESSAGE__PAYLOAD_STATE_UPDATE: on_state_update(c, m->state_update, kind); break;
    case RAMSERVER__V1__SERVER_MESSAGE__PAYLOAD_ERROR_RESPONSE: on_error(c, m->error_response, kind); break;
    case RAMSERVER__V1__SERVER_MESSAGE__PAYLOAD_MATCH_RESULT: on_match_result(c, m->match_result); break;
    case RAMSERVER__V1__SERVER_MESSAGE__PAYLOAD_LEAVE_SESSION_RESPONSE: on_leave_response(c, m->leave_session_response); break;
    case RAMSERVER__V1__SERVER_MESSAGE__PAYLOAD_LIST_SESSIONS_RESPONSE: on_list_sessions(c, m->list_sessions_response); break;
    case RAMSERVER__V1__SERVER_MESSAGE__PAYLOAD_MATCH_HISTORY_RESPONSE: on_match_history(c, m->match_history_response); break;
    default: break; /* LegalActionsResponse (never requested) and anything newer: ignore */
    }
    ramserver__v1__server_message__free_unpacked(m, NULL);
    return true;
}
