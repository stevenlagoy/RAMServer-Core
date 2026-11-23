/* Full-screen terminal UI, drawn with ANSI escape sequences into a grid of cells. Each frame is
 * compared with the last, and only rows that changed are rewritten, so the screen doesn't flicker.
 *
 * Everything drawn comes from the app/client state, which comes from the server. Keys turn into
 * client commands; the board changes only when the server's StateUpdate arrives. */
#define _GNU_SOURCE
#include <errno.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/ioctl.h>
#include <termios.h>
#include <unistd.h>

#include "ui.h"

#define MAX_ROWS 120
#define MAX_COLS 300

/* ---- colors and styles ---- */

enum { C_DEFAULT = -1, C_BLACK = 0, C_RED, C_GREEN, C_YELLOW, C_BLUE, C_MAGENTA, C_CYAN, C_WHITE, C_GRAY = 8,
       C_BRED, C_BGREEN, C_BYELLOW, C_BBLUE, C_BMAGENTA, C_BCYAN, C_BWHITE };

enum { A_BOLD = 1, A_DIM = 2, A_UNDER = 4, A_REVERSE = 8 };

typedef struct {
    signed char fg, bg;
    unsigned char attr;
} style;

static const style S_NORMAL = {C_DEFAULT, C_DEFAULT, 0};

typedef struct {
    char ch[5]; /* one UTF-8 code point */
    style st;
} cell;

static style st(int fg, int bg, int attr) { return (style){(signed char)fg, (signed char)bg, (unsigned char)attr}; }

/* ---- UI state ---- */

typedef enum { SCR_SETUP, SCR_CONNECTING, SCR_OFFLINE, SCR_LOBBY, SCR_GAME } screen;
typedef enum { PROMPT_NONE, PROMPT_JOIN_ID, PROMPT_ADDRESS } prompt_kind;

typedef struct {
    tui_options opt;
    struct termios saved;
    bool started;
    int rows, cols;
    cell grid[MAX_ROWS][MAX_COLS];
    cell shown[MAX_ROWS][MAX_COLS];
    bool full_redraw;
    bool mouse_on;
    uint64_t last_changes;
    uint64_t last_frame_ms;

    bool setup_mode;
    int server_choice; /* 0..3 */
    char name[TTT_DISPLAY_NAME_MAX + 1];

    prompt_kind prompt;
    char prompt_buf[TTT_ID_MAX];

    int cursor; /* board cell under the cursor */
    bool help;
    char confirm;           /* key awaiting a second press: 'F' resign, 'L' leave, 'Q' quit */
    uint64_t confirm_until;
    uint64_t seen_version;  /* for "your turn" detection */
    bool was_my_turn;
    char toast[160];        /* a transient hint, e.g. "Session id copied" */
    uint64_t toast_until;

    /* input bytes not yet parsed (a split escape sequence) */
    unsigned char inbuf[64];
    size_t inlen;
} tui;

static tui T;

static const char *SERVER_SPECS[4] = {"local", "local-ws", "hosted", NULL};
static const char *SERVER_TITLES[4] = {"Local server, TCP", "Local server, WebSocket", "Hosted server", "Other address..."};

/* ---- output ---- */

static void out(const char *s, size_t n) {
    while (n > 0) {
        ssize_t w = write(STDOUT_FILENO, s, n);
        if (w < 0) {
            if (errno == EINTR) continue;
            return;
        }
        s += w;
        n -= (size_t)w;
    }
}
static void outs(const char *s) { out(s, strlen(s)); }

static void term_size(void) {
    struct winsize ws;
    if (ioctl(STDOUT_FILENO, TIOCGWINSZ, &ws) == 0 && ws.ws_row > 0 && ws.ws_col > 0) {
        T.rows = ws.ws_row > MAX_ROWS ? MAX_ROWS : ws.ws_row;
        T.cols = ws.ws_col > MAX_COLS ? MAX_COLS : ws.ws_col;
    } else {
        T.rows = 24;
        T.cols = 80;
    }
}

static void set_mouse(bool on) {
    if (on == T.mouse_on) return;
    T.mouse_on = on;
    outs(on ? "\x1b[?1000h\x1b[?1006h" : "\x1b[?1000l\x1b[?1006l");
}

static void restore_terminal(void) {
    if (!T.started) return;
    T.started = false;
    set_mouse(false);
    outs("\x1b[0m\x1b[?25h\x1b[?1049l"); /* reset style, show cursor, leave the alternate screen */
    tcsetattr(STDIN_FILENO, TCSAFLUSH, &T.saved);
}

/* ---- drawing into the grid ---- */

static void clear_grid(void) {
    for (int r = 0; r < T.rows; r++)
        for (int c = 0; c < T.cols; c++) {
            T.grid[r][c].ch[0] = ' ';
            T.grid[r][c].ch[1] = '\0';
            T.grid[r][c].st = S_NORMAL;
        }
}

/* Draws UTF-8 text at (r, c), one column per code point, clipped to the screen. Returns the next column. */
static int put(int r, int c, style s, const char *text) {
    if (r < 0 || r >= T.rows) return c;
    const unsigned char *p = (const unsigned char *)text;
    while (*p) {
        int len = 1;
        if (*p >= 0xF0) len = 4;
        else if (*p >= 0xE0) len = 3;
        else if (*p >= 0xC0) len = 2;
        if (c >= 0 && c < T.cols) {
            cell *x = &T.grid[r][c];
            int i = 0;
            for (; i < len && p[i]; i++) x->ch[i] = (char)p[i];
            x->ch[i] = '\0';
            x->st = s;
        }
        for (int i = 0; i < len && *p; i++) p++;
        c++;
    }
    return c;
}

static int putf(int r, int c, style s, const char *fmt, ...) __attribute__((format(printf, 4, 5)));
static int putf(int r, int c, style s, const char *fmt, ...) {
    char buf[512];
    va_list ap;
    va_start(ap, fmt);
    vsnprintf(buf, sizeof buf, fmt, ap);
    va_end(ap);
    return put(r, c, s, buf);
}

static int text_width(const char *s) {
    int n = 0;
    for (const unsigned char *p = (const unsigned char *)s; *p; p++)
        if ((*p & 0xC0) != 0x80) n++;
    return n;
}

static void fill(int r, int c, int w, style s) {
    for (int i = 0; i < w; i++) put(r, c + i, s, " ");
}

static void hline(int r, int c, int w, style s) {
    for (int i = 0; i < w; i++) put(r, c + i, s, T.opt.ascii ? "-" : "─");
}

/* Shortens s to at most w columns with an ellipsis */
static const char *fit(const char *s, int w, char *buf, size_t cap) {
    if (w <= 0) return "";
    if (text_width(s) <= w) return s;
    size_t i = 0;
    int col = 0;
    const unsigned char *p = (const unsigned char *)s;
    while (*p && col < w - 1 && i + 4 < cap) {
        buf[i++] = (char)*p++;
        while ((*p & 0xC0) == 0x80 && i + 1 < cap) buf[i++] = (char)*p++;
        col++;
    }
    if (T.opt.ascii) buf[i++] = '~';
    else { memcpy(buf + i, "…", 3); i += 3; }
    buf[i] = '\0';
    return buf;
}

/* ---- presenting the grid ---- */

static void emit_style(char *buf, size_t *n, style s) {
    *n += (size_t)sprintf(buf + *n, "\x1b[0");
    if (!T.opt.no_color) {
        if (s.fg >= 0) *n += (size_t)sprintf(buf + *n, ";%d", s.fg < 8 ? 30 + s.fg : 90 + s.fg - 8);
        if (s.bg >= 0) *n += (size_t)sprintf(buf + *n, ";%d", s.bg < 8 ? 40 + s.bg : 100 + s.bg - 8);
    }
    if (s.attr & A_BOLD) *n += (size_t)sprintf(buf + *n, ";1");
    if ((s.attr & A_DIM) && !T.opt.no_color) *n += (size_t)sprintf(buf + *n, ";2");
    if (s.attr & A_UNDER) *n += (size_t)sprintf(buf + *n, ";4");
    if (s.attr & A_REVERSE) *n += (size_t)sprintf(buf + *n, ";7");
    buf[(*n)++] = 'm';
}

static bool style_eq(style a, style b) { return a.fg == b.fg && a.bg == b.bg && a.attr == b.attr; }

static void present(void) {
    static char buf[MAX_COLS * 24 + 64];
    if (T.full_redraw) {
        outs("\x1b[0m\x1b[2J");
        memset(T.shown, 0, sizeof T.shown);
        T.full_redraw = false;
    }
    for (int r = 0; r < T.rows; r++) {
        bool same = true;
        for (int c = 0; c < T.cols && same; c++)
            same = strcmp(T.grid[r][c].ch, T.shown[r][c].ch) == 0 && style_eq(T.grid[r][c].st, T.shown[r][c].st);
        if (same) continue;
        size_t n = (size_t)sprintf(buf, "\x1b[%d;1H", r + 1);
        style cur = {99, 99, 99};
        for (int c = 0; c < T.cols; c++) {
            cell *x = &T.grid[r][c];
            if (!style_eq(x->st, cur)) {
                emit_style(buf, &n, x->st);
                cur = x->st;
            }
            size_t l = strlen(x->ch);
            memcpy(buf + n, x->ch, l);
            n += l;
            T.shown[r][c] = *x;
        }
        n += (size_t)sprintf(buf + n, "\x1b[0m");
        out(buf, n);
    }
}

/* ---- pieces ---- */

static style mark_style(char m, bool dim) {
    int a = dim ? A_DIM : A_BOLD;
    if (m == 'X') return st(C_BCYAN, C_DEFAULT, a);
    if (m == 'O') return st(C_BYELLOW, C_DEFAULT, a);
    return st(C_GRAY, C_DEFAULT, 0);
}

static void draw_logo(int r, int c) {
    if (T.opt.ascii) {
        put(r, c, st(C_BCYAN, C_DEFAULT, A_BOLD), "TIC");
        put(r, c + 3, st(C_GRAY, C_DEFAULT, 0), "-");
        put(r, c + 4, st(C_BYELLOW, C_DEFAULT, A_BOLD), "TAC");
        put(r, c + 7, st(C_GRAY, C_DEFAULT, 0), "-");
        put(r, c + 8, st(C_BMAGENTA, C_DEFAULT, A_BOLD), "TOE");
        return;
    }
    static const char *L[3][3] = {
        {"╺┳╸╻┏━╸", " ┃ ┃┃  ", " ╹ ╹┗━╸"},
        {"╺┳╸┏━┓┏━╸", " ┃ ┣━┫┃  ", " ╹ ╹ ╹┗━╸"},
        {"╺┳╸┏━┓┏━╸", " ┃ ┃ ┃┣╸ ", " ╹ ┗━┛┗━╸"},
    };
    int colors[3] = {C_BCYAN, C_BYELLOW, C_BMAGENTA};
    int offs[3] = {0, 9, 20};
    for (int w = 0; w < 3; w++)
        for (int i = 0; i < 3; i++) put(r + i, c + offs[w], st(colors[w], C_DEFAULT, A_BOLD), L[w][i]);
}

/* The board: 3x3 cells of 9x3 characters inside grid lines, 31 columns by 13 rows */
#define BOARD_W 31
#define BOARD_H 13

static void board_cell_origin(int bi, int r0, int c0, int *r, int *c) {
    *r = r0 + 1 + (bi / 3) * 4;
    *c = c0 + 1 + (bi % 3) * 10;
}

static void draw_board(app *a, int r0, int c0, uint64_t now) {
    ttt_client *cl = &a->client;
    bool u = !T.opt.ascii;
    style grid = st(C_GRAY, C_DEFAULT, 0);
    /* grid lines */
    for (int i = 0; i <= 3; i++) {
        int r = r0 + i * 4;
        const char *l = u ? (i == 0 ? "┌" : i == 3 ? "└" : "├") : "+";
        const char *m = u ? (i == 0 ? "┬" : i == 3 ? "┴" : "┼") : "+";
        const char *rr = u ? (i == 0 ? "┐" : i == 3 ? "┘" : "┤") : "+";
        put(r, c0, grid, l);
        for (int k = 0; k < 3; k++) {
            hline(r, c0 + 1 + k * 10, 9, grid);
            put(r, c0 + 10 + k * 10, grid, k == 2 ? rr : m);
        }
        if (i < 3)
            for (int y = 1; y <= 3; y++)
                for (int k = 0; k <= 3; k++) put(r + y, c0 + k * 10, grid, u ? "│" : "|");
    }

    bool have = cl->has_board;
    bool live = client_in_match(cl);
    bool my_turn = client_my_turn(cl);
    bool flash = (now / 400) % 2 == 0;
    static const char *XA[3] = {" ╲   ╱ ", "   ╳   ", " ╱   ╲ "};
    static const char *OA[3] = {" ╭───╮ ", " │   │ ", " ╰───╯ "};
    static const char *XAa[3] = {" \\   / ", "   X   ", " /   \\ "};
    static const char *OAa[3] = {"  .-.  ", " (   ) ", "  '-'  "};

    for (int i = 0; i < TTT_CELLS; i++) {
        int r, c;
        board_cell_origin(i, r0, c0, &r, &c);
        char m = have ? cl->board.cells[i] : '.';
        bool in_line = cl->has_line && (cl->line[0] == i || cl->line[1] == i || cl->line[2] == i);
        bool cursor = live && my_turn && T.cursor == i;
        int bg = C_DEFAULT;
        if (in_line) bg = flash || live ? C_GREEN : C_DEFAULT;
        if (cursor) bg = m == '.' ? C_BLUE : C_RED;
        for (int y = 0; y < 3; y++) fill(r + y, c, 9, st(C_DEFAULT, bg, 0));
        if (m == 'X' || m == 'O') {
            bool dim = have && !live && !in_line && cl->has_result;
            style s = mark_style(m, dim);
            s.bg = (signed char)bg;
            const char **art = m == 'X' ? (u ? XA : XAa) : (u ? OA : OAa);
            for (int y = 0; y < 3; y++) put(r + y, c + 1, s, art[y]);
        } else if (live) {
            char num[2] = {(char)('1' + i), 0};
            put(r + 1, c + 4, st(cursor ? C_BWHITE : C_GRAY, bg, cursor ? A_BOLD : A_DIM), num);
        }
        if (cl->last_move == i && live) put(r + 2, c + 8, st(C_BMAGENTA, bg, A_BOLD), u ? "•" : "*");
    }
}

static void draw_box_title(int r, int c, int w, const char *title) {
    put(r, c, st(C_GRAY, C_DEFAULT, A_BOLD), title);
    hline(r, c + text_width(title) + 1, w - text_width(title) - 1, st(C_GRAY, C_DEFAULT, A_DIM));
}

static void draw_log(app *a, int r0, int c0, int w, int h) {
    ttt_client *cl = &a->client;
    draw_box_title(r0, c0, w, "EVENTS");
    int n = cl->log_count;
    int first = n - (h - 1);
    if (first < 0) first = 0;
    char buf[TTT_LOG_TEXT + 8];
    for (int i = first, r = r0 + 1; i < n; i++, r++) {
        const log_line *l = client_log_at(cl, i);
        style s = S_NORMAL;
        const char *icon = T.opt.ascii ? "-" : "·";
        switch (l->level) {
        case LOG_INFO: s = st(C_DEFAULT, C_DEFAULT, 0); break;
        case LOG_GOOD: s = st(C_BGREEN, C_DEFAULT, 0); icon = T.opt.ascii ? "+" : "✓"; break;
        case LOG_WARN: s = st(C_BYELLOW, C_DEFAULT, 0); icon = "!"; break;
        case LOG_ERROR: s = st(C_BRED, C_DEFAULT, A_BOLD); icon = T.opt.ascii ? "x" : "✗"; break;
        }
        bool newest = i == n - 1;
        if (!newest && l->level == LOG_INFO) s.attr |= A_DIM;
        put(r, c0, s, icon);
        put(r, c0 + 2, s, fit(l->text, w - 2, buf, sizeof buf));
    }
}

static void draw_keys(int r, const char *const *pairs) {
    int c = 1;
    for (int i = 0; pairs[i]; i += 2) {
        c = put(r, c, st(C_BLACK, C_WHITE, A_BOLD), " ");
        c = put(r, c, st(C_BLACK, C_WHITE, A_BOLD), pairs[i]);
        c = put(r, c, st(C_BLACK, C_WHITE, A_BOLD), " ");
        c = put(r, c + 1, st(C_DEFAULT, C_DEFAULT, 0), pairs[i + 1]);
        c += 2;
    }
}

static void draw_status_bar(app *a) {
    style bar = st(C_BWHITE, C_BLUE, 0);
    fill(0, 0, T.cols, bar);
    put(0, 1, st(C_BWHITE, C_BLUE, A_BOLD), T.opt.ascii ? "RAMServer tic-tac-toe" : "RAMServer ▸ tic-tac-toe");
    char right[400], buf[400];
    const char *conn = a->conn == CONN_OPEN ? (T.opt.ascii ? "online" : "● online")
                       : a->conn == CONN_CONNECTING ? (T.opt.ascii ? "connecting" : "◌ connecting")
                                                    : (T.opt.ascii ? "offline" : "○ offline");
    if (a->have_addr) snprintf(right, sizeof right, "%s  %s  %s ", a->client.display_name, a->addr.label, conn);
    else snprintf(right, sizeof right, "%s ", conn);
    if (text_width(right) > T.cols - 26) snprintf(right, sizeof right, "%s  %s ", a->client.display_name, conn);
    const char *r = fit(right, T.cols - 26, buf, sizeof buf);
    put(0, T.cols - text_width(r), bar, r);
}

static void draw_prompt(void) {
    if (T.prompt == PROMPT_NONE) return;
    int r = T.rows - 3;
    const char *label = T.prompt == PROMPT_JOIN_ID ? "Session id to join: " : "Server address: ";
    fill(r, 0, T.cols, st(C_BLACK, C_BYELLOW, 0));
    int c = put(r, 1, st(C_BLACK, C_BYELLOW, A_BOLD), label);
    char buf[TTT_ID_MAX + 8];
    c = put(r, c, st(C_BLACK, C_BYELLOW, 0), fit(T.prompt_buf, T.cols - c - 2, buf, sizeof buf));
    put(r, c, st(C_BYELLOW, C_BLACK, 0), " ");
    put(r + 1, 1, st(C_GRAY, C_DEFAULT, 0), "Enter to confirm, Esc to cancel. Paste works.");
}

/* ---- screens ---- */

static screen current_screen(app *a) {
    if (T.setup_mode || !a->have_addr) return SCR_SETUP;
    if (a->conn == CONN_CONNECTING) return SCR_CONNECTING;
    if (a->conn == CONN_IDLE) return SCR_OFFLINE;
    switch (a->client.phase) {
    case PHASE_OFFLINE:
    case PHASE_HANDSHAKE:
    case PHASE_AUTH: return SCR_CONNECTING;
    case PHASE_LOBBY:
    case PHASE_JOINING: return SCR_LOBBY;
    case PHASE_SESSION:
    case PHASE_LEAVING: return SCR_GAME;
    }
    return SCR_LOBBY;
}

static void draw_setup(app *a) {
    (void)a;
    int c0 = T.cols > 64 ? (T.cols - 60) / 2 : 2;
    int r = 2;
    draw_logo(r, c0);
    r += T.opt.ascii ? 2 : 4;
    put(r++, c0, st(C_GRAY, C_DEFAULT, 0), "A RAMServer client. Pick a server and a name, then press Enter.");
    r++;
    put(r++, c0, st(C_DEFAULT, C_DEFAULT, A_BOLD), "SERVER");
    for (int i = 0; i < 4; i++) {
        bool sel = T.server_choice == i;
        style s = sel ? st(C_BLACK, C_BCYAN, A_BOLD) : S_NORMAL;
        fill(r, c0, 58, sel ? s : S_NORMAL);
        put(r, c0 + 1, s, sel ? (T.opt.ascii ? ">" : "▶") : " ");
        put(r, c0 + 3, s, SERVER_TITLES[i]);
        const char *detail = i == 0 ? ADDR_LOCAL_TCP : i == 1 ? ADDR_LOCAL_WS : i == 2 ? "wss://ramserver-core-…/connect" : "tcp://, ws:// or wss://";
        if (T.opt.ascii && i == 2) detail = "wss://ramserver-core-.../connect";
        put(r, c0 + 28, sel ? s : st(C_GRAY, C_DEFAULT, 0), detail);
        r++;
    }
    r++;
    put(r++, c0, st(C_DEFAULT, C_DEFAULT, A_BOLD), "YOUR NAME");
    int c = put(r, c0 + 1, st(C_BWHITE, C_DEFAULT, A_UNDER | A_BOLD), T.name[0] ? T.name : "");
    put(r, c, st(C_BLACK, C_BWHITE, 0), " ");
    r += 2;
    put(r++, c0, st(C_GRAY, C_DEFAULT, 0), "Up/Down choose a server, type to edit your name, Enter to connect.");
    put(r++, c0, st(C_GRAY, C_DEFAULT, 0), "In Docker, a server on your own machine is at host.docker.internal:");
    put(r++, c0, st(C_GRAY, C_DEFAULT, 0), "choose \"Other address...\" and enter host.docker.internal:8080.");
    static const char *keys[] = {"Up/Down", "server", "Enter", "connect", "Esc", "quit", NULL};
    draw_keys(T.rows - 1, keys);
}

static void draw_connecting(app *a, uint64_t now) {
    static const char *spin_u[] = {"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"};
    static const char *spin_a[] = {"|", "/", "-", "\\"};
    const char *s = T.opt.ascii ? spin_a[(now / 120) % 4] : spin_u[(now / 90) % 10];
    int r = T.rows / 2 - 3, c0 = T.cols > 60 ? (T.cols - 56) / 2 : 2;
    draw_logo(r - 5, c0);
    const char *what = a->conn == CONN_CONNECTING ? "Connecting to" : a->client.phase == PHASE_AUTH ? "Signing in at" : "Saying hello to";
    int c = put(r, c0, st(C_BCYAN, C_DEFAULT, A_BOLD), s);
    c = putf(r, c + 1, S_NORMAL, "%s ", what);
    put(r, c, st(C_DEFAULT, C_DEFAULT, A_BOLD), a->addr.label);
    if (a->conn == CONN_CONNECTING) {
        unsigned secs = now > a->connect_started_ms ? (unsigned)((now - a->connect_started_ms) / 1000) : 0;
        putf(r + 2, c0, st(C_GRAY, C_DEFAULT, 0), "%us", secs);
        if (a->addr.kind == ADDR_WSS && secs >= 2)
            put(r + 3, c0, st(C_GRAY, C_DEFAULT, 0), "The hosted server can take a few seconds to wake up.");
    }
    draw_log(a, T.rows - 8, 1, T.cols - 2, 6);
    static const char *keys[] = {"Esc", "cancel", "Q", "quit", NULL};
    draw_keys(T.rows - 1, keys);
}

static void draw_offline(app *a) {
    int r = 3, c0 = T.cols > 64 ? (T.cols - 60) / 2 : 2;
    draw_logo(r, c0);
    r += T.opt.ascii ? 3 : 5;
    put(r++, c0, st(C_BRED, C_DEFAULT, A_BOLD), "Not connected");
    char buf[300];
    if (a->conn_error[0]) put(r++, c0, S_NORMAL, fit(a->conn_error, T.cols - c0 - 2, buf, sizeof buf));
    r++;
    put(r++, c0, st(C_GRAY, C_DEFAULT, 0), "Press Enter to try again (you'll sign in as a new player if the old");
    put(r++, c0, st(C_GRAY, C_DEFAULT, 0), "seat can't be resumed), S to choose another server, or Q to quit.");
    draw_log(a, T.rows - 10, 1, T.cols - 2, 8);
    static const char *keys[] = {"Enter", "reconnect", "S", "servers", "Q", "quit", NULL};
    draw_keys(T.rows - 1, keys);
}

static void draw_lobby(app *a) {
    ttt_client *cl = &a->client;
    int c0 = 3, r = 2;
    draw_logo(r, c0);
    r += T.opt.ascii ? 2 : 4;
    int c = put(r, c0, st(C_GRAY, C_DEFAULT, 0), "Signed in as ");
    put(r, c, st(C_DEFAULT, C_DEFAULT, A_BOLD), cl->display_name);
    r += 2;
    static const char *items[][3] = {
        {"M", "Quick match", "join an open game, or open one and wait"},
        {"C", "Create a private game", "get a session id to send a friend"},
        {"P", "Create a public game", "anyone can find it with quick match"},
        {"J", "Join by session id", "paste the id a friend sent you"},
        {"B", "Browse public games", "then press 1-9 to join one"},
        {"Q", "Quit", ""},
    };
    for (int i = 0; i < 6; i++) {
        put(r, c0, st(C_BLACK, C_BCYAN, A_BOLD), " ");
        put(r, c0 + 1, st(C_BLACK, C_BCYAN, A_BOLD), items[i][0]);
        put(r, c0 + 2, st(C_BLACK, C_BCYAN, A_BOLD), " ");
        put(r, c0 + 4, st(C_DEFAULT, C_DEFAULT, A_BOLD), items[i][1]);
        put(r, c0 + 28, st(C_GRAY, C_DEFAULT, 0), items[i][2]);
        r += 2;
    }
    if (cl->phase == PHASE_JOINING) put(r++, c0, st(C_BYELLOW, C_DEFAULT, A_BOLD), "Joining...");
    if (cl->listed_valid) {
        r++;
        draw_box_title(r++, c0, T.cols - c0 - 2 > 70 ? 70 : T.cols - c0 - 2, "PUBLIC GAMES");
        if (cl->n_listed == 0) put(r++, c0, st(C_GRAY, C_DEFAULT, 0), "None right now. Press P to open one, or M for quick match.");
        for (int i = 0; i < cl->n_listed && i < 9 && r < T.rows - 12; i++) {
            const listed_session *l = &cl->listed[i];
            char buf[64];
            style s = l->joinable ? S_NORMAL : st(C_GRAY, C_DEFAULT, A_DIM);
            putf(r, c0, st(C_BCYAN, C_DEFAULT, A_BOLD), "%d", i + 1);
            put(r, c0 + 3, s, fit(l->id, 24, buf, sizeof buf));
            putf(r, c0 + 29, s, "%-16.16s %d player%s  %s", l->owner_name[0] ? l->owner_name : "?", l->n_players,
                 l->n_players == 1 ? " " : "s", l->joinable ? "open" : proto_status_name(l->status));
            r++;
        }
    }
    draw_log(a, T.rows - 9, 1, T.cols - 2, 7);
    static const char *keys[] = {"M", "quick", "C", "private", "J", "join id", "B", "browse", "Q", "quit", NULL};
    draw_keys(T.rows - 1, keys);
}

static int count_role(ttt_client *cl, const char *role) {
    int n = 0;
    for (int i = 0; i < cl->n_members; i++) n += strcmp(cl->members[i].role, role) == 0;
    return n;
}

/* The mark a member played (from the last match's annotations), or 0 */
static char mark_of(ttt_client *cl, const char *id) {
    if (cl->x_player[0] && strcmp(cl->x_player, id) == 0) return 'X';
    if (cl->o_player[0] && strcmp(cl->o_player, id) == 0) return 'O';
    return 0;
}

static void draw_status_message(app *a, int r, int c0, int w, uint64_t now) {
    ttt_client *cl = &a->client;
    char buf[200], who[160];
    bool pulse = (now / 500) % 2 == 0;
    if (cl->phase == PHASE_LEAVING) {
        put(r, c0, st(C_BYELLOW, C_DEFAULT, A_BOLD), "Leaving...");
        return;
    }
    if (client_in_match(cl)) {
        char tm = ttt_view_to_move(&cl->board);
        if (client_my_turn(cl)) {
            style s = st(C_BLACK, tm == 'X' ? C_BCYAN : C_BYELLOW, A_BOLD);
            if (!pulse) s = st(tm == 'X' ? C_BCYAN : C_BYELLOW, C_DEFAULT, A_BOLD);
            snprintf(buf, sizeof buf, T.opt.ascii ? " > YOUR MOVE (%c) " : " ▶ YOUR MOVE (%c) ", tm);
            put(r, c0, s, buf);
            put(r + 1, c0, st(C_GRAY, C_DEFAULT, 0), "Arrows/WASD + Enter, a digit 1-9, or click a cell.");
        } else {
            snprintf(who, sizeof who, "%s", client_name_of(cl, cl->active_id));
            snprintf(buf, sizeof buf, "Waiting for %s (%c)%s", fit(who, w - 20, buf + 100, 100), tm, pulse ? "..." : "   ");
            put(r, c0, mark_style(tm, false), buf);
            if (!client_is_player(cl)) put(r + 1, c0, st(C_GRAY, C_DEFAULT, 0), "You're watching this match.");
        }
        return;
    }
    if (cl->has_result) {
        const match_result *res = &cl->result;
        const char *how = client_result_how(res);
        bool won = !res->draw && strcmp(res->winner, cl->player_id) == 0;
        bool lost = !res->draw && !won && client_is_player(cl);
        style s;
        if (res->draw) { s = st(C_BLACK, C_WHITE, A_BOLD); snprintf(buf, sizeof buf, " DRAW "); }
        else if (won) { s = st(C_BLACK, pulse ? C_BGREEN : C_GREEN, A_BOLD); snprintf(buf, sizeof buf, T.opt.ascii ? " * YOU WIN * " : " ★ YOU WIN ★ "); }
        else if (lost) { s = st(C_BWHITE, C_RED, A_BOLD); snprintf(buf, sizeof buf, " YOU LOSE "); }
        else { s = st(C_BLACK, C_WHITE, A_BOLD); snprintf(buf, sizeof buf, " %.60s WINS ", client_name_of(cl, res->winner)); }
        int c = put(r, c0, s, buf);
        putf(r, c + 1, st(C_GRAY, C_DEFAULT, 0), "by %s, %u turn%s", how, res->turns, res->turns == 1 ? "" : "s");
    }
    int rr = r + (cl->has_result ? 2 : 0);
    if (cl->status == RAMSERVER__V1__SESSION_STATUS__SESSION_STATUS_WAITING) {
        const member *me = client_me(cl);
        int players = count_role(cl, "player");
        if (me && strcmp(me->role, "player") == 0) {
            if (!me->ready) put(rr, c0, st(C_BYELLOW, C_DEFAULT, A_BOLD), cl->has_result ? "Press R for a rematch." : "Press R when you're ready to play.");
            else if (players < 2) put(rr, c0, st(C_DEFAULT, C_DEFAULT, 0), "Ready. Waiting for an opponent to join...");
            else put(rr, c0, st(C_DEFAULT, C_DEFAULT, 0), "Ready. Waiting for your opponent to ready up...");
        } else {
            put(rr, c0, st(C_GRAY, C_DEFAULT, 0), "You're spectating. The match starts when both players are ready.");
        }
    } else if (cl->status == RAMSERVER__V1__SESSION_STATUS__SESSION_STATUS_CLOSED) {
        put(rr, c0, st(C_BRED, C_DEFAULT, 0), "This session has closed. Press L to go back to the lobby.");
    }
}

static void draw_game(app *a, uint64_t now) {
    ttt_client *cl = &a->client;
    bool wide = T.cols >= 80;
    int br = 2, bc = 2;
    draw_board(a, br, bc, now);

    int pr = wide ? 2 : br + BOARD_H + 1;
    int pc = wide ? bc + BOARD_W + 4 : 2;
    int pw = T.cols - pc - 2;
    char buf[300];

    draw_box_title(pr, pc, pw, "SESSION");
    int c = put(pr + 1, pc, st(C_BWHITE, C_DEFAULT, A_BOLD), fit(cl->session_id, pw - 12, buf, sizeof buf));
    if (cl->private_session) put(pr + 1, c + 1, st(C_BMAGENTA, C_DEFAULT, 0), "private");
    put(pr + 2, pc, st(C_GRAY, C_DEFAULT, 0), "Y copies the id. A friend joins with J.");

    int r = pr + 4;
    draw_box_title(r++, pc, pw, "PLAYERS");
    for (int i = 0; i < cl->n_members; i++) {
        const member *m = &cl->members[i];
        if (strcmp(m->role, "player") != 0) continue;
        char mk = mark_of(cl, m->id);
        put(r, pc, mark_style(mk ? mk : '.', false), mk ? (char[]){mk, 0} : (T.opt.ascii ? "-" : "·"));
        bool me = strcmp(m->id, cl->player_id) == 0;
        c = put(r, pc + 2, st(C_DEFAULT, C_DEFAULT, me ? A_BOLD : 0), fit(m->name, 16, buf, sizeof buf));
        if (me) put(r, c, st(C_GRAY, C_DEFAULT, 0), " (you)");
        int x = pc + 25;
        if (!m->connected) put(r, x, st(C_BRED, C_DEFAULT, 0), "offline");
        else if (cl->status != RAMSERVER__V1__SESSION_STATUS__SESSION_STATUS_IN_PROGRESS)
            put(r, x, m->ready ? st(C_BGREEN, C_DEFAULT, 0) : st(C_GRAY, C_DEFAULT, 0),
                m->ready ? (T.opt.ascii ? "ready" : "✓ ready") : "not ready");
        else if (strcmp(m->id, cl->active_id) == 0) put(r, x, st(C_BWHITE, C_DEFAULT, A_BOLD), T.opt.ascii ? "<- to move" : "◀ to move");
        int wins = client_wins(cl, m->id);
        if (pw > 45) putf(r, pc + 37, st(C_GRAY, C_DEFAULT, 0), "%d win%s", wins, wins == 1 ? "" : "s");
        r++;
    }
    if (count_role(cl, "player") < 2) put(r++, pc + 2, st(C_GRAY, C_DEFAULT, A_DIM), "(waiting for a second player)");
    int specs = count_role(cl, "spectator");
    if (specs > 0) {
        c = putf(r, pc, st(C_GRAY, C_DEFAULT, 0), "%d watching: ", specs);
        for (int i = 0; i < cl->n_members && c < pc + pw - 4; i++)
            if (strcmp(cl->members[i].role, "spectator") == 0) c = putf(r, c, st(C_GRAY, C_DEFAULT, 0), "%s ", cl->members[i].name);
        r++;
    }
    if (cl->n_results > 0)
        putf(r++, pc, st(C_GRAY, C_DEFAULT, 0), "%d match%s played, %d drawn", cl->n_results, cl->n_results == 1 ? "" : "es", client_draws(cl));

    r++;
    draw_status_message(a, r, pc, pw, now);

    int log_top = wide ? (br + BOARD_H + 1) : (r + 4);
    int log_h = T.rows - 2 - log_top;
    if (T.prompt != PROMPT_NONE) log_h -= 3;
    if (log_h >= 3) draw_log(a, log_top, 1, T.cols - 2, log_h);

    if (client_in_match(cl)) {
        if (client_is_player(cl)) {
            static const char *keys[] = {"←↑→↓", "move", "Enter", "place", "1-9", "place", "F", "resign", "L", "leave", "?", "help", NULL};
            static const char *keys_a[] = {"Arrows", "move", "Enter", "place", "1-9", "place", "F", "resign", "L", "leave", "?", "help", NULL};
            draw_keys(T.rows - 1, T.opt.ascii ? keys_a : keys);
        } else {
            static const char *keys[] = {"L", "leave", "Q", "quit", "?", "help", NULL};
            draw_keys(T.rows - 1, keys);
        }
    } else {
        static const char *keys[] = {"R", "ready", "Y", "copy id", "L", "leave", "Q", "quit", "?", "help", NULL};
        draw_keys(T.rows - 1, keys);
    }
}

static void draw_help(void) {
    static const char *lines[] = {
        "KEYS",
        "",
        "Lobby     M quick match   C create private   P create public",
        "          J join by id    B browse (then 1-9 to join)",
        "Waiting   R ready / not ready   Y copy the session id",
        "Playing   arrows, WASD or hjkl move the cursor",
        "          Enter or Space places your mark there",
        "          1-9 place directly (1 = top-left, 9 = bottom-right)",
        "          or click a cell with the mouse",
        "          F twice resigns",
        "Anywhere  L leave the session (twice during a match)",
        "          Q quit (leaves the session first)   ? close this",
        "",
        "The board is drawn only from what the server sends:",
        "a move shows up when the server accepts it.",
    };
    int n = (int)(sizeof lines / sizeof lines[0]);
    int w = 66, h = n + 2;
    int r0 = (T.rows - h) / 2, c0 = (T.cols - w) / 2;
    if (r0 < 1) r0 = 1;
    if (c0 < 0) c0 = 0;
    style box = st(C_BWHITE, C_BLACK, 0);
    for (int y = 0; y < h; y++) fill(r0 + y, c0, w, box);
    for (int i = 0; i < n; i++) put(r0 + 1 + i, c0 + 2, i == 0 ? st(C_BCYAN, C_BLACK, A_BOLD) : box, lines[i]);
}

static void draw_toast(uint64_t now) {
    if (now >= T.toast_until || !T.toast[0]) return;
    int w = text_width(T.toast) + 4;
    int c = (T.cols - w) / 2;
    style s = st(C_BLACK, C_BYELLOW, A_BOLD);
    fill(T.rows - 3, c, w, s);
    put(T.rows - 3, c + 2, s, T.toast);
}

static void toast(const char *msg, uint64_t now) {
    snprintf(T.toast, sizeof T.toast, "%s", msg);
    T.toast_until = now + 2500;
}

/* ---- rendering entry point ---- */

static void tui_render(void *self, app *a, uint64_t now) {
    (void)self;
    ttt_client *cl = &a->client;
    if (app_resized) {
        app_resized = 0;
        term_size();
        T.full_redraw = true;
    }
    if (T.confirm && now > T.confirm_until) T.confirm = 0;

    /* Your-turn detection: center the cursor on a fresh match, ring the bell if asked */
    bool my_turn = client_my_turn(cl);
    if (my_turn && !T.was_my_turn) {
        if (cl->moves == 0) T.cursor = 4;
        if (cl->board.cells[T.cursor] != '.') {
            for (int i = 0; i < TTT_CELLS; i++)
                if (cl->board.cells[(4 + i) % TTT_CELLS] == '.') { T.cursor = (4 + i) % TTT_CELLS; break; }
        }
        if (T.opt.bell) outs("\a");
    }
    T.was_my_turn = my_turn;
    set_mouse(client_in_match(cl) && client_is_player(cl));

    clear_grid();
    if (T.rows < 20 || T.cols < 44) {
        put(0, 0, st(C_BYELLOW, C_DEFAULT, A_BOLD), "Make the terminal at least 44x20.");
        putf(1, 0, S_NORMAL, "It is %dx%d now. Q quits.", T.cols, T.rows);
        present();
        return;
    }
    draw_status_bar(a);
    switch (current_screen(a)) {
    case SCR_SETUP: draw_setup(a); break;
    case SCR_CONNECTING: draw_connecting(a, now); break;
    case SCR_OFFLINE: draw_offline(a); break;
    case SCR_LOBBY: draw_lobby(a); break;
    case SCR_GAME: draw_game(a, now); break;
    }
    if (T.confirm) {
        const char *msg = T.confirm == 'F' ? "Press F again to resign" : T.confirm == 'L' ? "Press L again to leave (this forfeits the match)" : "Press Q again to quit (this forfeits the match)";
        toast(msg, now);
        T.toast_until = T.confirm_until;
    }
    draw_toast(now);
    draw_prompt();
    if (T.help) draw_help();
    present();
    T.last_changes = cl->changes;
    T.last_frame_ms = now;
}

/* ---- input ---- */

enum { K_NONE = 0, K_UP = 1000, K_DOWN, K_LEFT, K_RIGHT, K_ENTER, K_ESC, K_BACKSPACE, K_TAB, K_MOUSE };

/* Copies text to the system clipboard with OSC 52, which Windows Terminal and most terminals support */
static void copy_to_clipboard(const char *s) {
    static const char b64[] = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
    char buf[TTT_ID_MAX * 2 + 32];
    size_t n = (size_t)sprintf(buf, "\x1b]52;c;");
    size_t len = strlen(s);
    for (size_t i = 0; i < len; i += 3) {
        unsigned v = (unsigned)(unsigned char)s[i] << 16;
        if (i + 1 < len) v |= (unsigned)(unsigned char)s[i + 1] << 8;
        if (i + 2 < len) v |= (unsigned char)s[i + 2];
        buf[n++] = b64[(v >> 18) & 63];
        buf[n++] = b64[(v >> 12) & 63];
        buf[n++] = i + 1 < len ? b64[(v >> 6) & 63] : '=';
        buf[n++] = i + 2 < len ? b64[v & 63] : '=';
    }
    buf[n++] = '\a';
    out(buf, n);
}

static void connect_from_setup(app *a, const char *spec) {
    char err[256];
    server_addr addr;
    if (!addr_parse(spec, &addr, err, sizeof err)) {
        client_logf(&a->client, LOG_ERROR, "%s", err);
        toast(err, app_now_ms());
        return;
    }
    if (T.name[0] == '\0') {
        toast("Type a name first", app_now_ms());
        return;
    }
    snprintf(a->client.display_name, sizeof a->client.display_name, "%s", T.name);
    T.setup_mode = false;
    app_connect(a, &addr);
}

static void handle_prompt_key(app *a, int key) {
    size_t n = strlen(T.prompt_buf);
    if (key == K_ESC) {
        T.prompt = PROMPT_NONE;
    } else if (key == K_ENTER) {
        prompt_kind k = T.prompt;
        T.prompt = PROMPT_NONE;
        /* trim spaces around a pasted id */
        char *s = T.prompt_buf;
        while (*s == ' ') s++;
        size_t l = strlen(s);
        while (l > 0 && (s[l - 1] == ' ' || s[l - 1] == '\r' || s[l - 1] == '\n')) s[--l] = '\0';
        if (k == PROMPT_JOIN_ID) client_cmd_join(&a->client, s);
        else if (k == PROMPT_ADDRESS) connect_from_setup(a, s);
    } else if (key == K_BACKSPACE) {
        if (n > 0) T.prompt_buf[n - 1] = '\0';
    } else if (key >= 32 && key < 127 && n + 1 < sizeof T.prompt_buf) {
        T.prompt_buf[n] = (char)key;
        T.prompt_buf[n + 1] = '\0';
    }
}

static void open_prompt(prompt_kind k) {
    T.prompt = k;
    T.prompt_buf[0] = '\0';
}

static void handle_setup_key(app *a, int key) {
    size_t n = strlen(T.name);
    switch (key) {
    case K_UP: T.server_choice = (T.server_choice + 3) % 4; break;
    case K_DOWN:
    case K_TAB: T.server_choice = (T.server_choice + 1) % 4; break;
    case K_ESC: app_request_quit(a); app_request_quit(a); break;
    case K_BACKSPACE: if (n > 0) T.name[n - 1] = '\0'; break;
    case K_ENTER:
        if (T.server_choice == 3) open_prompt(PROMPT_ADDRESS);
        else connect_from_setup(a, SERVER_SPECS[T.server_choice]);
        break;
    default:
        if (key >= 32 && key < 127 && n < TTT_DISPLAY_NAME_MAX && (key != ' ' || n > 0)) {
            T.name[n] = (char)key;
            T.name[n + 1] = '\0';
        }
    }
}

static bool confirm(char k, uint64_t now) {
    if (T.confirm == k && now <= T.confirm_until) {
        T.confirm = 0;
        T.toast_until = 0;
        return true;
    }
    T.confirm = k;
    T.confirm_until = now + 3000;
    return false;
}

static void quit_key(app *a, uint64_t now) {
    if (client_in_match(&a->client) && client_is_player(&a->client) && !confirm('Q', now)) return;
    app_request_quit(a);
}

static int lower(int k) { return (k >= 'A' && k <= 'Z') ? k + 32 : k; }

static void move_cursor(int dr, int dc) {
    int r = T.cursor / 3 + dr, c = T.cursor % 3 + dc;
    if (r < 0) r = 2;
    if (r > 2) r = 0;
    if (c < 0) c = 2;
    if (c > 2) c = 0;
    T.cursor = r * 3 + c;
}

static void game_key(app *a, int key, uint64_t now) {
    ttt_client *cl = &a->client;
    int k = lower(key);
    bool match = client_in_match(cl);
    switch (key) {
    case K_UP: move_cursor(-1, 0); return;
    case K_DOWN: move_cursor(1, 0); return;
    case K_LEFT: move_cursor(0, -1); return;
    case K_RIGHT: move_cursor(0, 1); return;
    case K_ENTER: if (match) client_cmd_move(cl, T.cursor, a->guard); return;
    default: break;
    }
    if (match && client_is_player(cl)) {
        if (k == 'w' || k == 'k') { move_cursor(-1, 0); return; }
        if (k == 's' || k == 'j') { move_cursor(1, 0); return; }
        if (k == 'a' || k == 'h') { move_cursor(0, -1); return; }
        if (k == 'd') { move_cursor(0, 1); return; }
        if (k == ' ') { client_cmd_move(cl, T.cursor, a->guard); return; }
        if (key >= '1' && key <= '9') {
            T.cursor = key - '1';
            client_cmd_move(cl, key - '1', a->guard);
            return;
        }
        if (k == 'f') {
            if (confirm('F', now)) client_cmd_resign(cl);
            return;
        }
    }
    switch (k) {
    case 'r': client_cmd_toggle_ready(cl); break;
    case 'y':
        copy_to_clipboard(cl->session_id);
        toast("Session id copied to the clipboard", now);
        client_logf(cl, LOG_INFO, "Session id: %s", cl->session_id);
        break;
    case 'l':
        if (match && client_is_player(cl) && !confirm('L', now)) break;
        client_cmd_leave(cl);
        break;
    case 'q': quit_key(a, now); break;
    default: break;
    }
}

static void lobby_key(app *a, int key, uint64_t now) {
    ttt_client *cl = &a->client;
    int k = lower(key);
    if (k == 'm') client_cmd_quick_match(cl);
    else if (k == 'c') client_cmd_create(cl, true);
    else if (k == 'p') client_cmd_create(cl, false);
    else if (k == 'j') open_prompt(PROMPT_JOIN_ID);
    else if (k == 'b') client_cmd_list_sessions(cl);
    else if (k == 'q' || key == K_ESC) quit_key(a, now);
    else if (key >= '1' && key <= '9' && cl->listed_valid) {
        int i = key - '1';
        if (i < cl->n_listed) client_cmd_join(cl, cl->listed[i].id);
    }
}

static void mouse_click(app *a, int row, int col) {
    /* row/col are 1-based; the board's top-left is at grid (2, 2) */
    int r = row - 1 - 2, c = col - 1 - 2;
    if (r <= 0 || c <= 0 || r >= BOARD_H - 1 || c >= BOARD_W - 1) return;
    if (r % 4 == 0 || c % 10 == 0) return; /* a grid line */
    int idx = (r / 4) * 3 + (c / 10);
    T.cursor = idx;
    client_cmd_move(&a->client, idx, a->guard);
}

static void dispatch(app *a, int key, uint64_t now) {
    if (key == K_NONE) return;
    if (T.help) {
        T.help = false;
        return;
    }
    if (T.prompt != PROMPT_NONE) {
        handle_prompt_key(a, key);
        return;
    }
    screen s = current_screen(a);
    if (key == '?' && s != SCR_SETUP) {
        T.help = true;
        return;
    }
    switch (s) {
    case SCR_SETUP: handle_setup_key(a, key); break;
    case SCR_CONNECTING:
        if (key == K_ESC) {
            app_disconnect(a, "cancelled");
            T.setup_mode = true;
        } else if (lower(key) == 'q') app_request_quit(a);
        break;
    case SCR_OFFLINE:
        if (key == K_ENTER || lower(key) == 'n') app_reconnect(a);
        else if (lower(key) == 's') T.setup_mode = true;
        else if (lower(key) == 'q' || key == K_ESC) app_request_quit(a);
        break;
    case SCR_LOBBY: lobby_key(a, key, now); break;
    case SCR_GAME: game_key(a, key, now); break;
    }
}

/* Parses one key from the buffer. Returns bytes used, 0 if more bytes are needed. */
static size_t parse_key(const unsigned char *b, size_t n, int *key, int *mrow, int *mcol, bool *mpress, bool final) {
    *key = K_NONE;
    if (n == 0) return 0;
    if (b[0] != 0x1b) {
        if (b[0] == '\r' || b[0] == '\n') *key = K_ENTER;
        else if (b[0] == 127 || b[0] == 8) *key = K_BACKSPACE;
        else if (b[0] == '\t') *key = K_TAB;
        else if (b[0] >= 32 && b[0] < 127) *key = b[0];
        else if (b[0] >= 0x80) { /* skip a UTF-8 sequence: names and ids are ASCII */
            size_t l = 1;
            while (l < n && (b[l] & 0xC0) == 0x80) l++;
            return l;
        }
        return 1;
    }
    if (n == 1) {
        if (!final) return 0;
        *key = K_ESC;
        return 1;
    }
    if (b[1] == '[' || b[1] == 'O') {
        if (n < 3) return final ? 2 : 0;
        if (b[2] == 'A') *key = K_UP;
        else if (b[2] == 'B') *key = K_DOWN;
        else if (b[2] == 'C') *key = K_RIGHT;
        else if (b[2] == 'D') *key = K_LEFT;
        if (*key != K_NONE) return 3;
        if (b[1] == '[' && b[2] == '<') { /* SGR mouse: ESC [ < btn ; col ; row (M|m) */
            size_t i = 3;
            int v[3] = {0, 0, 0}, k = 0;
            for (; i < n; i++) {
                if (b[i] >= '0' && b[i] <= '9') v[k] = v[k] * 10 + (b[i] - '0');
                else if (b[i] == ';' && k < 2) k++;
                else if (b[i] == 'M' || b[i] == 'm') {
                    *key = K_MOUSE;
                    *mcol = v[1];
                    *mrow = v[2];
                    *mpress = b[i] == 'M' && v[0] == 0;
                    return i + 1;
                } else return i + 1;
            }
            return final ? n : 0;
        }
        /* other CSI sequences (Home, F-keys, ...): skip to the final byte */
        for (size_t i = 2; i < n; i++)
            if (b[i] >= 0x40 && b[i] <= 0x7e) return i + 1;
        return final ? n : 0;
    }
    *key = K_ESC; /* Alt+key or a lone Esc followed by a key */
    return 1;
}

static void tui_on_input(void *self, app *a) {
    (void)self;
    unsigned char buf[256];
    ssize_t n = read(STDIN_FILENO, buf, sizeof buf);
    if (n <= 0) {
        if (n == 0) app_request_quit(a);
        return;
    }
    uint64_t now = app_now_ms();
    for (ssize_t i = 0; i < n; i++) {
        if (T.inlen < sizeof T.inbuf) T.inbuf[T.inlen++] = buf[i];
    }
    size_t off = 0;
    for (;;) {
        int key, mrow = 0, mcol = 0;
        bool mpress = false;
        /* A read normally holds whole sequences, so a lone ESC at the end really is Esc */
        size_t used = parse_key(T.inbuf + off, T.inlen - off, &key, &mrow, &mcol, &mpress, true);
        if (used == 0) break;
        off += used;
        if (key == K_MOUSE) {
            if (mpress && T.prompt == PROMPT_NONE && !T.help && current_screen(a) == SCR_GAME) mouse_click(a, mrow, mcol);
        } else {
            dispatch(a, key, now);
        }
    }
    memmove(T.inbuf, T.inbuf + off, T.inlen - off);
    T.inlen -= off;
}

static int tui_input_fd(void *self) {
    (void)self;
    return STDIN_FILENO;
}

static void on_exit_restore(void) { restore_terminal(); }

bool tui_start(ui_ops *ops, const tui_options *opts) {
    if (!isatty(STDIN_FILENO) || !isatty(STDOUT_FILENO)) return false;
    memset(&T, 0, sizeof T);
    T.opt = *opts;
    snprintf(T.name, sizeof T.name, "%s", opts->name);
    T.setup_mode = opts->setup;
    T.server_choice = 0;
    T.cursor = 4;
    if (tcgetattr(STDIN_FILENO, &T.saved) < 0) return false;
    struct termios raw = T.saved;
    raw.c_lflag &= (tcflag_t) ~(ICANON | ECHO | IEXTEN); /* keep ISIG: Ctrl-C quits cleanly through SIGINT */
    raw.c_iflag &= (tcflag_t) ~(IXON | ICRNL);
    raw.c_cc[VMIN] = 1;
    raw.c_cc[VTIME] = 0;
    if (tcsetattr(STDIN_FILENO, TCSAFLUSH, &raw) < 0) return false;
    T.started = true;
    atexit(on_exit_restore);
    outs("\x1b[?1049h\x1b[?25l"); /* alternate screen, hide the cursor */
    term_size();
    T.full_redraw = true;
    ops->self = &T;
    ops->input_fd = tui_input_fd;
    ops->on_input = tui_on_input;
    ops->render = tui_render;
    return true;
}

void tui_stop(void) { restore_terminal(); }
