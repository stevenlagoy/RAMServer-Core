#define _POSIX_C_SOURCE 200809L
#include "app.h"

#include <errno.h>
#include <poll.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

volatile int app_resized = 0;
static volatile sig_atomic_t quit_signal = 0;

static void on_quit_signal(int sig) {
    (void)sig;
    quit_signal++;
}
static void on_winch(int sig) {
    (void)sig;
    app_resized = 1;
}

uint64_t app_now_ms(void) {
    struct timespec ts;
    clock_gettime(CLOCK_MONOTONIC, &ts);
    return (uint64_t)ts.tv_sec * 1000u + (uint64_t)ts.tv_nsec / 1000000u;
}

void app_init(app *a, const char *display_name) {
    memset(a, 0, sizeof *a);
    client_init(&a->client, display_name, "ramserver-ttt-c/1.0");
    frame_decoder_init(&a->dec);
    a->guard = true;
    a->skill = BOT_PERFECT;
    a->bot_delay_ms = 700;
    a->seed = (unsigned)app_now_ms();
}

void app_free(app *a) {
    if (a->t) transport_close(a->t);
    a->t = NULL;
    client_free(&a->client);
}

bool app_connect(app *a, const server_addr *addr) {
    if (a->t) app_disconnect(a, "reconnecting");
    if (addr != &a->addr) a->addr = *addr;
    a->have_addr = true;
    a->conn_error[0] = '\0';
    frame_decoder_init(&a->dec);
    client_logf(&a->client, LOG_INFO, "Connecting to %s...", a->addr.label);
    char err[256] = "";
    a->t = transport_open(&a->addr, err, sizeof err);
    if (!a->t) {
        snprintf(a->conn_error, sizeof a->conn_error, "%s", err);
        client_logf(&a->client, LOG_ERROR, "Couldn't connect: %s", err);
        a->conn = CONN_IDLE;
        return false;
    }
    a->conn = CONN_CONNECTING;
    a->connect_started_ms = app_now_ms();
    return true;
}

bool app_reconnect(app *a) {
    if (!a->have_addr) return false;
    return app_connect(a, &a->addr);
}

void app_disconnect(app *a, const char *reason) {
    if (a->t) {
        transport_close(a->t);
        a->t = NULL;
    }
    if (a->conn != CONN_IDLE) {
        snprintf(a->conn_error, sizeof a->conn_error, "%s", reason ? reason : "closed");
        if (a->conn == CONN_OPEN) client_disconnected(&a->client, reason);
        else client_logf(&a->client, LOG_ERROR, "Couldn't connect: %s", reason ? reason : "closed");
    }
    a->conn = CONN_IDLE;
}

void app_request_quit(app *a) {
    if (a->quitting) { /* asked twice: don't wait any longer */
        a->done = true;
        return;
    }
    a->quitting = true;
    a->quit_deadline = app_now_ms() + APP_QUIT_GRACE_MS;
    if (a->conn == CONN_OPEN && a->client.phase == PHASE_SESSION) {
        client_cmd_leave(&a->client); /* leave cleanly before closing (docs/websocket.md, Closing) */
    }
}

/* ---- tracing: TTT_TRACE=FILE logs every frame, for debugging against a server ---- */

static FILE *trace_file(void) {
    static FILE *f;
    static bool init;
    if (!init) {
        init = true;
        const char *path = getenv("TTT_TRACE");
        if (path && *path && (f = fopen(path, "a")) != NULL) setvbuf(f, NULL, _IOLBF, 0);
    }
    return f;
}

static const char *const CLIENT_KINDS[] = {"?", "?", "ConnectRequest", "AuthRequest", "JoinSessionRequest",
    "LeaveSessionRequest", "ActionRequest", "SetReadyRequest", "LegalActionsRequest", "ListSessionsRequest",
    "MatchHistoryRequest"};
static const char *const SERVER_KINDS[] = {"?", "?", "ConnectResponse", "AuthResponse", "StateUpdate", "ErrorResponse",
    "MatchResult", "LeaveSessionResponse", "LegalActionsResponse", "ListSessionsResponse", "MatchHistoryResponse"};

static void trace_frame(bool out, const uint8_t *payload, size_t len) {
    FILE *f = trace_file();
    if (!f) return;
    unsigned long long t = (unsigned long long)app_now_ms();
    if (len == 0) {
        fprintf(f, "%llu %s heartbeat\n", t, out ? "->" : "<-");
        return;
    }
    if (out) {
        Ramserver__V1__ClientMessage *m = ramserver__v1__client_message__unpack(NULL, len, payload);
        if (!m) return;
        int k = (int)m->payload_case;
        fprintf(f, "%llu -> seq=%llu %s", t, (unsigned long long)m->sequence, k > 0 && k <= 10 ? CLIENT_KINDS[k] : "?");
        if (k == RAMSERVER__V1__CLIENT_MESSAGE__PAYLOAD_ACTION_REQUEST)
            fprintf(f, " action=\"%.*s\" expected_version=%llu", (int)m->action_request->encoded_action.len,
                    (const char *)m->action_request->encoded_action.data,
                    (unsigned long long)m->action_request->expected_state_version);
        fprintf(f, " (%zu bytes)\n", len);
        ramserver__v1__client_message__free_unpacked(m, NULL);
    } else {
        Ramserver__V1__ServerMessage *m = ramserver__v1__server_message__unpack(NULL, len, payload);
        if (!m) {
            fprintf(f, "%llu <- undecodable (%zu bytes)\n", t, len);
            return;
        }
        int k = (int)m->payload_case;
        fprintf(f, "%llu <- in_reply_to=%llu %s", t, (unsigned long long)m->in_reply_to, k > 0 && k <= 10 ? SERVER_KINDS[k] : "?");
        if (k == RAMSERVER__V1__SERVER_MESSAGE__PAYLOAD_STATE_UPDATE) {
            Ramserver__V1__StateUpdate *u = m->state_update;
            fprintf(f, " version=%llu status=%s view=\"%.*s\" active=%s players=%zu", (unsigned long long)u->state_version,
                    proto_status_name((int)u->status), (int)u->encoded_view.len, (const char *)u->encoded_view.data,
                    u->active_player_id, u->n_players);
        } else if (k == RAMSERVER__V1__SERVER_MESSAGE__PAYLOAD_ERROR_RESPONSE) {
            fprintf(f, " %s \"%s\"", proto_error_name((int)m->error_response->code), m->error_response->message);
        } else if (k == RAMSERVER__V1__SERVER_MESSAGE__PAYLOAD_MATCH_RESULT) {
            fprintf(f, " %s draw=%d turns=%u", m->match_result->termination_reason, m->match_result->draw, m->match_result->turns);
        }
        fprintf(f, " (%zu bytes)\n", len);
        ramserver__v1__server_message__free_unpacked(m, NULL);
    }
}

/* ---- sending ---- */

static void send_frame(app *a, const uint8_t *payload, size_t len) {
    static uint8_t frame[FRAME_PREFIX_LEN + FRAME_MAX_PAYLOAD];
    if (!a->t || a->conn != CONN_OPEN) return;
    size_t n = frame_encode(payload, len, frame);
    if (n == 0) {
        client_logf(&a->client, LOG_ERROR, "A request was too large to send (%zu bytes)", len);
        return;
    }
    trace_frame(true, payload, len);
    if (transport_send_frame(a->t, frame, n) < 0) {
        char reason[256];
        transport_get_state(a->t, reason, sizeof reason);
        app_disconnect(a, reason[0] ? reason : "sending failed");
        return;
    }
    a->last_send_ms = app_now_ms();
}

static void flush(app *a) {
    if (a->conn != CONN_OPEN) return;
    if (client_take_heartbeat(&a->client)) send_frame(a, NULL, 0);
    while (a->conn == CONN_OPEN && client_outbox_count(&a->client) > 0) {
        ttt_bytes m = client_outbox_take(&a->client);
        send_frame(a, m.data, m.len);
        free(m.data);
    }
}

/* ---- receiving ---- */

static void read_connection(app *a) {
    uint8_t buf[16384];
    for (;;) {
        if (!a->t) return;
        ssize_t n = transport_read(a->t, buf, sizeof buf);
        if (n < 0) {
            if (errno == EAGAIN || errno == EWOULDBLOCK) return;
            app_disconnect(a, strerror(errno));
            return;
        }
        if (n == 0) {
            char reason[256];
            transport_get_state(a->t, reason, sizeof reason);
            app_disconnect(a, reason);
            return;
        }
        size_t off = 0;
        while (off < (size_t)n) {
            off += frame_decoder_feed(&a->dec, buf + off, (size_t)n - off);
            const uint8_t *payload;
            size_t len;
            frame_status st;
            while ((st = frame_next(&a->dec, &payload, &len)) == FRAME_OK) {
                trace_frame(false, payload, len);
                client_on_frame(&a->client, payload, len);
            }
            if (st == FRAME_TOO_LARGE) {
                app_disconnect(a, "the server sent a frame over the 64 KiB limit");
                return;
            }
        }
        flush(a); /* answer heartbeats and send follow-ups (like AuthRequest) promptly */
    }
}

static void check_transport(app *a) {
    if (!a->t) return;
    char reason[256];
    transport_state s = transport_get_state(a->t, reason, sizeof reason);
    if (a->conn == CONN_CONNECTING && s == TRANSPORT_OPEN) {
        a->conn = CONN_OPEN;
        a->last_send_ms = app_now_ms();
        client_connected(&a->client);
    }
    /* A closed transport is noticed when its descriptor reaches EOF, after every byte was read. */
}

/* ---- bot ---- */

static void bot_step(app *a, uint64_t now) {
    ttt_client *c = &a->client;
    if (!a->bot || a->quitting || c->phase != PHASE_SESSION) return;

    if (client_in_match(c)) {
        if (!client_my_turn(c) || client_has_pending(c, REQ_ACTION) || c->state_version == a->bot_acted_version) {
            a->bot_wait_until = 0;
            return;
        }
        if (a->bot_wait_until == 0) {
            a->bot_wait_until = now + (uint64_t)a->bot_delay_ms;
            return;
        }
        if (now < a->bot_wait_until) return;
        int cell = bot_choose(c->board.cells, c->board.me, a->skill, &a->seed);
        if (cell >= 0) {
            client_logf(c, LOG_INFO, "Bot plays %d (%s)", cell + 1, TTT_CELL_NAMES[cell]);
            client_cmd_move(c, cell, a->guard);
        }
        a->bot_acted_version = c->state_version;
        a->bot_wait_until = 0;
        return;
    }

    if (c->status == RAMSERVER__V1__SESSION_STATUS__SESSION_STATUS_WAITING) {
        if (a->bot_games > 0 && c->results_received >= (uint64_t)a->bot_games) {
            client_logf(c, LOG_INFO, "Bot finished %d match%s; leaving", a->bot_games, a->bot_games == 1 ? "" : "es");
            app_request_quit(a);
            return;
        }
        const member *me = client_me(c);
        if (me && strcmp(me->role, "player") == 0 && !me->ready && !client_has_pending(c, REQ_READY)) {
            if (a->bot_wait_until == 0) {
                a->bot_wait_until = now + (uint64_t)a->bot_delay_ms;
                return;
            }
            if (now < a->bot_wait_until) return;
            client_cmd_set_ready(c, true);
            a->bot_wait_until = 0;
        }
    }
}

/* ---- the loop ---- */

int app_run(app *a, ui_ops *ui) {
    struct sigaction sa;
    memset(&sa, 0, sizeof sa);
    sa.sa_handler = on_quit_signal;
    sigemptyset(&sa.sa_mask);
    sigaction(SIGINT, &sa, NULL);
    sigaction(SIGTERM, &sa, NULL);
    sigaction(SIGHUP, &sa, NULL);
    sa.sa_handler = on_winch;
    sigaction(SIGWINCH, &sa, NULL);
    signal(SIGPIPE, SIG_IGN);

    sig_atomic_t quits_seen = 0;
    while (!a->done) {
        uint64_t now = app_now_ms();
        if (quit_signal != quits_seen) {
            quits_seen = quit_signal;
            app_request_quit(a);
        }

        check_transport(a);
        if (a->conn == CONN_CONNECTING && a->t && now > a->connect_started_ms + 45000)
            app_disconnect(a, "timed out connecting");

        if (a->client.fatal && a->t) {
            app_disconnect(a, "the server refused this client");
        }

        bot_step(a, now);
        flush(a);
        if (a->conn == CONN_OPEN && a->last_send_ms + APP_KEEPALIVE_MS <= now) send_frame(a, NULL, 0); /* last_send_ms may be newer than now */

        if (a->exit_on_disconnect && a->have_addr && a->conn == CONN_IDLE && !a->quitting) {
            a->exit_code = 1;
            a->done = true;
        }
        if (a->quitting) {
            client_phase ph = a->client.phase;
            bool waiting_to_leave = a->conn == CONN_OPEN && (ph == PHASE_LEAVING || ph == PHASE_SESSION);
            if (!waiting_to_leave || now >= a->quit_deadline) a->done = true;
        }

        ui->render(ui->self, a, now);
        if (a->done) break;

        struct pollfd fds[2];
        int nfds = 0, conn_i = -1, in_i = -1;
        if (a->t) {
            conn_i = nfds;
            fds[nfds++] = (struct pollfd){transport_fd(a->t), POLLIN, 0};
        }
        int in_fd = ui->input_fd(ui->self);
        if (in_fd >= 0) {
            in_i = nfds;
            fds[nfds++] = (struct pollfd){in_fd, POLLIN, 0};
        }
        int rc = poll(fds, (nfds_t)nfds, 100);
        if (rc < 0 && errno != EINTR) break;
        if (rc > 0) {
            if (conn_i >= 0 && (fds[conn_i].revents & (POLLIN | POLLHUP | POLLERR))) read_connection(a);
            if (in_i >= 0 && (fds[in_i].revents & (POLLIN | POLLHUP | POLLERR))) ui->on_input(ui->self, a);
        }
    }

    /* Close cleanly: over WebSocket this sends a close frame */
    if (a->t) {
        flush(a);
        transport_close(a->t);
        a->t = NULL;
        a->conn = CONN_IDLE;
    }
    return a->exit_code;
}
