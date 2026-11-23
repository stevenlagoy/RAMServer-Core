/* User interfaces. Each reads the app's state to draw and calls app/client commands for the player's
 * input; none of them touches the network or the protocol directly. */
#ifndef TTT_UI_H
#define TTT_UI_H

#include <stdbool.h>

#include "app.h"

typedef struct {
    bool ascii;     /* no Unicode box drawing */
    bool no_color;
    bool bell;      /* ring the terminal bell when it becomes your turn */
    bool setup;     /* start on the server-and-name screen */
    char name[TTT_NAME_MAX];
} tui_options;

/* Full-screen terminal UI. Returns false if the terminal can't be set up (stdin/stdout not a TTY). */
bool tui_start(ui_ops *ops, const tui_options *opts);
void tui_stop(void);

/* Line-based UI for scripts, logs, and terminals without cursor control: prints events and boards, and
 * reads commands from stdin ("help" lists them). */
void plain_start(ui_ops *ops, bool quit_on_eof);

#endif
