#include "bot.h"

#include <stdlib.h>

static const int LINES[8][3] = {{0, 1, 2}, {3, 4, 5}, {6, 7, 8}, {0, 3, 6}, {1, 4, 7}, {2, 5, 8}, {0, 4, 8}, {2, 4, 6}};

static char winner(const char b[TTT_CELLS]) {
    for (int i = 0; i < 8; i++) {
        char c = b[LINES[i][0]];
        if (c != '.' && b[LINES[i][1]] == c && b[LINES[i][2]] == c) return c;
    }
    return 0;
}

/* Score from me's point of view: +10 - depth for a win, depth - 10 for a loss, 0 for a draw */
static int minimax(char b[TTT_CELLS], char me, char turn, int depth) {
    char w = winner(b);
    if (w) return w == me ? 10 - depth : depth - 10;
    int best = turn == me ? -100 : 100;
    bool any = false;
    for (int i = 0; i < TTT_CELLS; i++) {
        if (b[i] != '.') continue;
        any = true;
        b[i] = turn;
        int s = minimax(b, me, turn == 'X' ? 'O' : 'X', depth + 1);
        b[i] = '.';
        if (turn == me ? s > best : s < best) best = s;
    }
    return any ? best : 0;
}

static unsigned next_rand(unsigned *seed) {
    *seed = *seed * 1103515245u + 12345u;
    return (*seed >> 16) & 0x7fff;
}

int bot_choose(const char cells[TTT_CELLS], char mark, bot_skill skill, unsigned *seed) {
    char b[TTT_CELLS];
    int empty[TTT_CELLS], n_empty = 0;
    for (int i = 0; i < TTT_CELLS; i++) {
        b[i] = cells[i];
        if (b[i] == '.') empty[n_empty++] = i;
    }
    if (n_empty == 0 || winner(b)) return n_empty ? empty[0] : -1;
    if (skill == BOT_CASUAL && next_rand(seed) % 3 == 0) return empty[next_rand(seed) % (unsigned)n_empty];

    int best[TTT_CELLS], n_best = 0, best_score = -1000;
    char other = mark == 'X' ? 'O' : 'X';
    for (int k = 0; k < n_empty; k++) {
        int i = empty[k];
        b[i] = mark;
        int s = minimax(b, mark, other, 1);
        b[i] = '.';
        if (s > best_score) {
            best_score = s;
            n_best = 0;
        }
        if (s == best_score) best[n_best++] = i;
    }
    return best[next_rand(seed) % (unsigned)n_best];
}
