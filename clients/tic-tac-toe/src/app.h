/* The event loop: one poll() over the server connection and the player's input, so the connection is
 * read at all times, even while the player is typing or thinking. It moves frames between the
 * transport and the protocol logic (client.h), answers heartbeats, sends keepalives, drives the bot,
 * and asks the UI to draw. */
#ifndef TTT_APP_H
#define TTT_APP_H

#include <stdbool.h>
#include <stdint.h>

#include "addr.h"
#include "bot.h"
#include "client.h"
#include "frame.h"
#include "transport.h"

#define APP_KEEPALIVE_MS 15000 /* send a heartbeat if we've sent nothing for this long (limit: 30 s) */
#define APP_QUIT_GRACE_MS 2000 /* how long to wait for LeaveSessionResponse when quitting */

typedef struct app app;

/* What a user interface provides. Every function is called from the loop's thread. */
typedef struct {
    void *self;
    int (*input_fd)(void *self);                /* descriptor to poll for input, or -1 */
    void (*on_input)(void *self, app *a);       /* input_fd is readable */
    void (*render)(void *self, app *a, uint64_t now_ms); /* called every loop; redraw if needed */
} ui_ops;

typedef enum { CONN_IDLE, CONN_CONNECTING, CONN_OPEN } conn_state;

struct app {
    ttt_client client;

    server_addr addr;
    bool have_addr;
    transport *t;
    conn_state conn;
    uint64_t connect_started_ms;
    char conn_error[256]; /* why the last connection attempt failed or ended */
    frame_decoder dec;
    uint64_t last_send_ms;

    /* Options */
    bool guard;        /* refuse plainly illegal moves locally (default) */
    bool bot;          /* play automatically */
    bot_skill skill;
    int bot_games;     /* bot: quit after this many matches (0: never) */
    int bot_delay_ms;
    unsigned seed;
    bool exit_on_disconnect; /* end app_run (exit code 1) when the connection fails or drops */

    /* Bot bookkeeping */
    uint64_t bot_wait_until;
    uint64_t bot_acted_version;

    /* Quitting */
    bool quitting;
    uint64_t quit_deadline;
    bool done;
    int exit_code;
};

uint64_t app_now_ms(void);

void app_init(app *a, const char *display_name);
void app_free(app *a);

/* Starts connecting to addr (remembered for reconnecting). Failures are logged. */
bool app_connect(app *a, const server_addr *addr);
bool app_reconnect(app *a);
/* Drops the connection now, without leaving the session first */
void app_disconnect(app *a, const char *reason);

/* Leaves the session if in one (waiting briefly for the reply), then closes the connection and ends
 * app_run. A second request quits at once. */
void app_request_quit(app *a);

/* Runs until quit. Installs SIGINT/SIGTERM/SIGHUP handlers that quit cleanly. Returns the exit code. */
int app_run(app *a, ui_ops *ui);

/* Set by SIGWINCH; the UI clears it */
extern volatile int app_resized;

#endif
