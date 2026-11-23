#include "helpers.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

typedef Ramserver__V1__ServerMessage SM;

static sm_bytes pack(SM *m) {
    sm_bytes b;
    b.len = ramserver__v1__server_message__get_packed_size(m);
    b.data = malloc(b.len ? b.len : 1);
    ramserver__v1__server_message__pack(m, b.data);
    return b;
}

sm_bytes sm_connect_response(uint64_t in_reply_to) {
    Ramserver__V1__ConnectResponse r = RAMSERVER__V1__CONNECT_RESPONSE__INIT;
    r.connection_id = "conn-1";
    r.server_protocol_version = "1.2";
    SM m = RAMSERVER__V1__SERVER_MESSAGE__INIT;
    m.in_reply_to = in_reply_to;
    m.payload_case = RAMSERVER__V1__SERVER_MESSAGE__PAYLOAD_CONNECT_RESPONSE;
    m.connect_response = &r;
    return pack(&m);
}

sm_bytes sm_auth_response(uint64_t in_reply_to, const char *player_id, const char *token, const char *session_id) {
    Ramserver__V1__AuthResponse r = RAMSERVER__V1__AUTH_RESPONSE__INIT;
    r.player_id = (char *)player_id;
    r.reconnect_token = (char *)token;
    r.session_id = (char *)(session_id ? session_id : "");
    SM m = RAMSERVER__V1__SERVER_MESSAGE__INIT;
    m.in_reply_to = in_reply_to;
    m.payload_case = RAMSERVER__V1__SERVER_MESSAGE__PAYLOAD_AUTH_RESPONSE;
    m.auth_response = &r;
    return pack(&m);
}

sm_bytes sm_error(uint64_t in_reply_to, int code, const char *message) {
    Ramserver__V1__ErrorResponse r = RAMSERVER__V1__ERROR_RESPONSE__INIT;
    r.code = (Ramserver__V1__ErrorCode)code;
    r.message = (char *)message;
    SM m = RAMSERVER__V1__SERVER_MESSAGE__INIT;
    m.in_reply_to = in_reply_to;
    m.payload_case = RAMSERVER__V1__SERVER_MESSAGE__PAYLOAD_ERROR_RESPONSE;
    m.error_response = &r;
    return pack(&m);
}

sm_bytes sm_leave_response(uint64_t in_reply_to, const char *session_id) {
    Ramserver__V1__LeaveSessionResponse r = RAMSERVER__V1__LEAVE_SESSION_RESPONSE__INIT;
    r.session_id = (char *)session_id;
    SM m = RAMSERVER__V1__SERVER_MESSAGE__INIT;
    m.in_reply_to = in_reply_to;
    m.payload_case = RAMSERVER__V1__SERVER_MESSAGE__PAYLOAD_LEAVE_SESSION_RESPONSE;
    m.leave_session_response = &r;
    return pack(&m);
}

sm_bytes sm_match_result(const char *session_id, const char *winner, const char *termination, int end_reason, unsigned turns) {
    Ramserver__V1__MatchResult r = RAMSERVER__V1__MATCH_RESULT__INIT;
    char *winners[1] = {(char *)winner};
    r.session_id = (char *)session_id;
    if (winner) {
        r.n_winner_player_ids = 1;
        r.winner_player_ids = winners;
    } else {
        r.draw = true;
    }
    r.termination_reason = (char *)termination;
    r.reason = (Ramserver__V1__MatchEndReason)end_reason;
    r.turns = turns;
    SM m = RAMSERVER__V1__SERVER_MESSAGE__INIT;
    m.payload_case = RAMSERVER__V1__SERVER_MESSAGE__PAYLOAD_MATCH_RESULT;
    m.match_result = &r;
    return pack(&m);
}

sm_bytes sm_state(uint64_t in_reply_to, const char *session_id, int status, uint64_t version, const char *players,
                  const char *view, const char *active, const char *annotations) {
    Ramserver__V1__StateUpdate u = RAMSERVER__V1__STATE_UPDATE__INIT;
    u.session_id = (char *)session_id;
    u.game_id = "tictactoe";
    u.status = (Ramserver__V1__SessionStatus)status;
    u.state_version = version;
    u.active_player_id = (char *)(active ? active : "");

    /* players */
    Ramserver__V1__SessionPlayer ps[8], *pp[8];
    char pbuf[1024];
    snprintf(pbuf, sizeof pbuf, "%s", players ? players : "");
    size_t np = 0;
    for (char *save = NULL, *tok = strtok_r(pbuf, ";", &save); tok && np < 8; tok = strtok_r(NULL, ";", &save)) {
        char *f[5] = {0};
        int k = 0;
        for (char *s2 = NULL, *x = strtok_r(tok, ":", &s2); x && k < 5; x = strtok_r(NULL, ":", &s2)) f[k++] = x;
        ramserver__v1__session_player__init(&ps[np]);
        ps[np].player_id = f[0];
        ps[np].display_name = f[1] ? f[1] : "";
        ps[np].role = f[2] ? f[2] : "player";
        ps[np].ready = f[3] && f[3][0] == '1';
        ps[np].connected = !f[4] || f[4][0] == '1';
        pp[np] = &ps[np];
        np++;
    }
    u.n_players = np;
    u.players = pp;
    if (np > 0) u.owner_player_id = ps[0].player_id;

    if (view) {
        u.encoded_view.data = (uint8_t *)view;
        u.encoded_view.len = strlen(view);
    }

    Ramserver__V1__StateUpdate__AnnotationsEntry es[8], *ep[8];
    char abuf[512];
    snprintf(abuf, sizeof abuf, "%s", annotations ? annotations : "");
    size_t na = 0;
    for (char *save = NULL, *tok = strtok_r(abuf, ";", &save); tok && na < 8; tok = strtok_r(NULL, ";", &save)) {
        char *eq = strchr(tok, '=');
        if (!eq) continue;
        *eq = '\0';
        ramserver__v1__state_update__annotations_entry__init(&es[na]);
        es[na].key = tok;
        es[na].value = eq + 1;
        ep[na] = &es[na];
        na++;
    }
    u.n_annotations = na;
    u.annotations = ep;

    SM m = RAMSERVER__V1__SERVER_MESSAGE__INIT;
    m.in_reply_to = in_reply_to;
    m.payload_case = RAMSERVER__V1__SERVER_MESSAGE__PAYLOAD_STATE_UPDATE;
    m.state_update = &u;
    return pack(&m);
}

sm_bytes sm_list(int n, const char *const *ids, const bool *joinable) {
    Ramserver__V1__SessionInfo infos[8], *ip[8];
    Ramserver__V1__SessionPlayer owner = RAMSERVER__V1__SESSION_PLAYER__INIT, *op[1] = {&owner};
    owner.player_id = "owner-1";
    owner.display_name = "Owner";
    owner.role = "player";
    for (int i = 0; i < n && i < 8; i++) {
        ramserver__v1__session_info__init(&infos[i]);
        infos[i].session_id = (char *)ids[i];
        infos[i].game_id = "tictactoe";
        infos[i].status = RAMSERVER__V1__SESSION_STATUS__SESSION_STATUS_WAITING;
        infos[i].joinable = joinable[i];
        infos[i].owner_player_id = "owner-1";
        infos[i].n_players = 1;
        infos[i].players = op;
        ip[i] = &infos[i];
    }
    Ramserver__V1__ListSessionsResponse r = RAMSERVER__V1__LIST_SESSIONS_RESPONSE__INIT;
    r.n_sessions = (size_t)n;
    r.sessions = ip;
    SM m = RAMSERVER__V1__SERVER_MESSAGE__INIT;
    m.payload_case = RAMSERVER__V1__SERVER_MESSAGE__PAYLOAD_LIST_SESSIONS_RESPONSE;
    m.list_sessions_response = &r;
    return pack(&m);
}

sm_bytes sm_history(const char *session_id, int n, const char *const *winners) {
    Ramserver__V1__MatchResult rs[8], *rp[8];
    char *w[8][1];
    for (int i = 0; i < n && i < 8; i++) {
        ramserver__v1__match_result__init(&rs[i]);
        rs[i].session_id = (char *)session_id;
        if (winners[i]) {
            w[i][0] = (char *)winners[i];
            rs[i].winner_player_ids = w[i];
            rs[i].n_winner_player_ids = 1;
            rs[i].termination_reason = "THREE_IN_A_ROW";
        } else {
            rs[i].draw = true;
            rs[i].termination_reason = "DRAW";
        }
        rs[i].reason = RAMSERVER__V1__MATCH_END_REASON__MATCH_END_REASON_COMPLETED;
        rp[i] = &rs[i];
    }
    Ramserver__V1__MatchHistoryResponse r = RAMSERVER__V1__MATCH_HISTORY_RESPONSE__INIT;
    r.session_id = (char *)session_id;
    r.n_matches = (size_t)n;
    r.matches = rp;
    SM m = RAMSERVER__V1__SERVER_MESSAGE__INIT;
    m.payload_case = RAMSERVER__V1__SERVER_MESSAGE__PAYLOAD_MATCH_HISTORY_RESPONSE;
    m.match_history_response = &r;
    return pack(&m);
}

void feed(ttt_client *c, sm_bytes msg) {
    assert_true(client_on_frame(c, msg.data, msg.len));
    free(msg.data);
}

Ramserver__V1__ClientMessage *take_cm(ttt_client *c) {
    if (client_outbox_count(c) == 0) return NULL;
    ttt_bytes b = client_outbox_take(c);
    Ramserver__V1__ClientMessage *m = ramserver__v1__client_message__unpack(NULL, b.len, b.data);
    free(b.data);
    assert_non_null(m);
    return m;
}

void free_cm(Ramserver__V1__ClientMessage *m) {
    if (m) ramserver__v1__client_message__free_unpacked(m, NULL);
}

void drain(ttt_client *c) {
    while (client_outbox_count(c) > 0) {
        ttt_bytes b = client_outbox_take(c);
        free(b.data);
    }
}

bool log_has(const ttt_client *c, const char *needle) {
    for (int i = 0; i < c->log_count; i++)
        if (strstr(client_log_at(c, i)->text, needle)) return true;
    return false;
}

const char *log_last(const ttt_client *c) {
    if (c->log_count == 0) return "";
    return client_log_at(c, c->log_count - 1)->text;
}
