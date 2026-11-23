/* Line-based UI: prints each event once, and the board whenever the server sends a new one; reads
 * one command per line from stdin. Used with --plain, when stdout isn't a terminal, and by --bot. */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include "ui.h"

typedef struct {
    uint64_t printed_serial;
    uint64_t printed_version;
    uint64_t printed_list;
    bool printed_board_live;
    char line[512];
    size_t len;
    bool eof;
    bool quit_on_eof;
} plain_ui;

static plain_ui P;

static void print_board(app *a) {
    ttt_client *c = &a->client;
    const ttt_view *v = &c->board;
    printf("\n");
    for (int r = 0; r < 3; r++) {
        printf("   ");
        for (int k = 0; k < 3; k++) {
            int i = r * 3 + k;
            char ch = v->cells[i] == '.' ? (char)('1' + i) : v->cells[i];
            printf(" %c %s", ch, k < 2 ? "|" : "");
        }
        printf("\n");
        if (r < 2) printf("   ---+---+---\n");
    }
    if (client_in_match(c)) {
        char tm = ttt_view_to_move(v);
        if (client_my_turn(c)) printf("Your move (%c): type 1-9\n", tm);
        else printf("%s (%c) to move%s\n", client_name_of(c, c->active_id), tm, v->me == 'S' ? "; you're watching" : "");
    }
    printf("\n");
}

static void plain_render(void *self, app *a, uint64_t now) {
    (void)self;
    (void)now;
    ttt_client *c = &a->client;
    bool any = false;
    for (int i = 0; i < c->log_count; i++) {
        const log_line *l = client_log_at(c, i);
        if (l->serial <= P.printed_serial) continue;
        const char *tag = l->level == LOG_ERROR ? "error: " : l->level == LOG_WARN ? "! " : "";
        printf("%s%s\n", tag, l->text);
        P.printed_serial = l->serial;
        any = true;
    }
    if (c->listed_serial != P.printed_list) {
        P.printed_list = c->listed_serial;
        for (int i = 0; i < c->n_listed; i++) {
            const listed_session *l = &c->listed[i];
            printf("  %s  owner %s, %d player%s, %s\n", l->id, l->owner_name[0] ? l->owner_name : "?", l->n_players,
                   l->n_players == 1 ? "" : "s", l->joinable ? "joinable" : proto_status_name(l->status));
        }
        any = true;
    }
    if (c->has_board && (c->state_version != P.printed_version || c->board_live != P.printed_board_live) &&
        c->phase == PHASE_SESSION && c->board_live) {
        print_board(a);
        any = true;
    }
    P.printed_version = c->state_version;
    P.printed_board_live = c->board_live;
    if (any) fflush(stdout);
}

static void help(void) {
    printf("commands:\n"
           "  quick | create | create public | join <id> | list     find a game\n"
           "  ready | unready                                       ready up (again, for a rematch)\n"
           "  1-9 | move <1-9>                                      place your mark (1 = top-left)\n"
           "  resign | leave | board | who | reconnect | quit\n");
    fflush(stdout);
}

static void who(app *a) {
    ttt_client *c = &a->client;
    if (c->phase != PHASE_SESSION) {
        printf("not in a session\n");
        return;
    }
    printf("session %s (%s)\n", c->session_id, proto_status_name(c->status));
    for (int i = 0; i < c->n_members; i++) {
        const member *m = &c->members[i];
        printf("  %-20s %-9s %s%s%s  %d wins\n", m->name, m->role, m->ready ? "ready" : "not ready",
               m->connected ? "" : ", offline", strcmp(m->id, c->player_id) == 0 ? ", you" : "", client_wins(c, m->id));
    }
}

static void command(app *a, char *line) {
    ttt_client *c = &a->client;
    while (*line == ' ' || *line == '\t') line++;
    size_t n = strlen(line);
    while (n > 0 && (line[n - 1] == ' ' || line[n - 1] == '\r' || line[n - 1] == '\t')) line[--n] = '\0';
    if (n == 0) return;
    char *arg = strchr(line, ' ');
    if (arg) {
        *arg++ = '\0';
        while (*arg == ' ') arg++;
    }
    if (strcmp(line, "help") == 0 || strcmp(line, "?") == 0) help();
    else if (strcmp(line, "quick") == 0) client_cmd_quick_match(c);
    else if (strcmp(line, "create") == 0) client_cmd_create(c, !(arg && strcmp(arg, "public") == 0));
    else if (strcmp(line, "join") == 0) client_cmd_join(c, arg);
    else if (strcmp(line, "list") == 0) {
        if (client_cmd_list_sessions(c)) { /* printed when the reply arrives, below */ }
    } else if (strcmp(line, "ready") == 0) client_cmd_set_ready(c, true);
    else if (strcmp(line, "unready") == 0) client_cmd_set_ready(c, false);
    else if (strcmp(line, "resign") == 0) client_cmd_resign(c);
    else if (strcmp(line, "leave") == 0) client_cmd_leave(c);
    else if (strcmp(line, "quit") == 0 || strcmp(line, "exit") == 0) app_request_quit(a);
    else if (strcmp(line, "reconnect") == 0) app_reconnect(a);
    else if (strcmp(line, "who") == 0) who(a);
    else if (strcmp(line, "board") == 0) {
        if (c->has_board) print_board(a);
        else printf("no board yet\n");
    } else if ((strcmp(line, "move") == 0 && arg && arg[0] >= '1' && arg[0] <= '9' && !arg[1]) ||
               (line[0] >= '1' && line[0] <= '9' && !line[1])) {
        int cell = (arg ? arg[0] : line[0]) - '1';
        client_cmd_move(c, cell, a->guard);
    } else {
        printf("unknown command \"%s\"; type help\n", line);
    }
    fflush(stdout);
}

static void plain_on_input(void *self, app *a) {
    (void)self;
    char buf[256];
    ssize_t n = read(STDIN_FILENO, buf, sizeof buf);
    if (n <= 0) {
        P.eof = true;
        if (P.quit_on_eof && !a->quitting) app_request_quit(a); /* a "quit" already asked: let it finish */
        return;
    }
    for (ssize_t i = 0; i < n; i++) {
        if (buf[i] == '\n') {
            P.line[P.len] = '\0';
            command(a, P.line);
            P.len = 0;
        } else if (P.len + 1 < sizeof P.line) {
            P.line[P.len++] = buf[i];
        }
    }
}

static int plain_input_fd(void *self) {
    (void)self;
    return P.eof ? -1 : STDIN_FILENO;
}

void plain_start(ui_ops *ops, bool quit_on_eof) {
    memset(&P, 0, sizeof P);
    P.quit_on_eof = quit_on_eof;
    ops->self = &P;
    ops->input_fd = plain_input_fd;
    ops->on_input = plain_on_input;
    ops->render = plain_render;
}
