/* ttt: a tic-tac-toe client for RAMServer */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include "app.h"
#include "ui.h"

#define VERSION "1.0"

static void usage(FILE *f) {
    fprintf(f,
            "Usage: ttt [options]\n"
            "\n"
            "Play tic-tac-toe against other people through a RAMServer.\n"
            "\n"
            "Server (without one, the full-screen UI asks):\n"
            "  -s, --server SPEC   local      plain TCP to 127.0.0.1:8080\n"
            "                      local-ws   ws://127.0.0.1:8081/connect\n"
            "                      hosted     wss://ramserver-core-80752473398.us-east5.run.app/connect\n"
            "                      HOST:PORT, tcp://HOST:PORT, ws://HOST[:PORT][/PATH], wss://...\n"
            "  -n, --name NAME     your display name (default: $TTT_NAME or $USER)\n"
            "\n"
            "Finding a game once signed in (otherwise choose in the lobby):\n"
            "      --quick         quick match\n"
            "      --create        create a private session (share its id)\n"
            "      --create-public create a public session\n"
            "      --join ID       join a session by id\n"
            "\n"
            "Interface:\n"
            "      --plain         line-based interface (the default when output isn't a terminal)\n"
            "      --ascii         no Unicode box drawing\n"
            "      --no-color      no colors\n"
            "      --bell          ring the terminal bell when it becomes your turn\n"
            "      --no-guard      send every move to the server, even ones it will refuse\n"
            "\n"
            "Bot (plays for you, through the server like anyone else):\n"
            "      --bot           ready up and play automatically, perfectly\n"
            "      --casual        with --bot: make a random move a third of the time\n"
            "      --games N       with --bot: leave and quit after N matches\n"
            "      --delay MS      with --bot: think for MS milliseconds per move (default 700)\n"
            "\n"
            "  -h, --help          show this help\n"
            "      --version       show the version\n");
}

static const char *need_arg(int argc, char **argv, int *i) {
    if (*i + 1 >= argc) {
        fprintf(stderr, "ttt: %s needs a value\n", argv[*i]);
        exit(2);
    }
    return argv[++*i];
}

int main(int argc, char **argv) {
    const char *server = getenv("TTT_SERVER");
    const char *name = getenv("TTT_NAME");
    if (!name || !*name) name = getenv("USER");
    if (!name || !*name) name = "player";
    bool plain = false, bot = false, casual = false;
    int games = 0, delay = -1;
    join_mode join = JOIN_NONE;
    const char *join_id = NULL;
    tui_options topt;
    memset(&topt, 0, sizeof topt);
    bool guard = true;

    for (int i = 1; i < argc; i++) {
        const char *a = argv[i];
        if (!strcmp(a, "-s") || !strcmp(a, "--server")) server = need_arg(argc, argv, &i);
        else if (!strcmp(a, "-n") || !strcmp(a, "--name")) name = need_arg(argc, argv, &i);
        else if (!strcmp(a, "--quick")) join = JOIN_QUICK;
        else if (!strcmp(a, "--create")) join = JOIN_CREATE_PRIVATE;
        else if (!strcmp(a, "--create-public")) join = JOIN_CREATE_PUBLIC;
        else if (!strcmp(a, "--join")) join = JOIN_BY_ID, join_id = need_arg(argc, argv, &i);
        else if (!strcmp(a, "--plain")) plain = true;
        else if (!strcmp(a, "--ascii")) topt.ascii = true;
        else if (!strcmp(a, "--no-color")) topt.no_color = true;
        else if (!strcmp(a, "--bell")) topt.bell = true;
        else if (!strcmp(a, "--no-guard")) guard = false;
        else if (!strcmp(a, "--bot")) bot = true;
        else if (!strcmp(a, "--casual")) casual = true;
        else if (!strcmp(a, "--games")) games = atoi(need_arg(argc, argv, &i));
        else if (!strcmp(a, "--delay")) delay = atoi(need_arg(argc, argv, &i));
        else if (!strcmp(a, "-h") || !strcmp(a, "--help")) { usage(stdout); return 0; }
        else if (!strcmp(a, "--version")) { printf("ttt %s (protocol %s)\n", VERSION, TTT_PROTOCOL_VERSION); return 0; }
        else {
            fprintf(stderr, "ttt: unknown option %s\n\n", a);
            usage(stderr);
            return 2;
        }
    }

    char display[TTT_DISPLAY_NAME_MAX + 1];
    snprintf(display, sizeof display, "%s", name);
    if (getenv("NO_COLOR")) topt.no_color = true;

    static app a; /* large: keep it off the stack */
    app_init(&a, display);
    a.guard = guard;
    a.bot = bot;
    a.skill = casual ? BOT_CASUAL : BOT_PERFECT;
    a.bot_games = games;
    if (delay >= 0) a.bot_delay_ms = delay;
    a.client.auto_join = join;
    if (join_id) snprintf(a.client.auto_join_id, sizeof a.client.auto_join_id, "%s", join_id);

    bool tty = isatty(STDIN_FILENO) && isatty(STDOUT_FILENO);
    if (!tty) plain = true;
    if (plain && !server) server = "local";

    server_addr addr;
    if (server) {
        char err[256];
        if (!addr_parse(server, &addr, err, sizeof err)) {
            fprintf(stderr, "ttt: %s\n", err);
            return 2;
        }
    }

    ui_ops ui;
    if (plain) {
        plain_start(&ui, !bot);
        a.exit_on_disconnect = bot; /* a bot has no one to ask what to do next */
    } else {
        snprintf(topt.name, sizeof topt.name, "%s", display);
        topt.setup = server == NULL;
        if (!tui_start(&ui, &topt)) {
            fprintf(stderr, "ttt: can't use this terminal; try --plain\n");
            return 1;
        }
    }

    if (server) app_connect(&a, &addr);
    int rc = app_run(&a, &ui);
    if (!plain) tui_stop();

    if (a.client.fatal) rc = rc ? rc : 1;
    if (bot && games > 0 && a.client.results_received < (uint64_t)games) rc = rc ? rc : 1;
    if (!plain && a.conn_error[0] && a.client.player_id[0] == '\0') fprintf(stderr, "ttt: %s\n", a.conn_error);
    app_free(&a);
    return rc;
}
