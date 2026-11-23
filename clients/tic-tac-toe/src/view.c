#include "view.h"

const uint8_t TTT_ACTION_RESIGN[6] = {'r', 'e', 's', 'i', 'g', 'n'};

bool ttt_view_decode(const uint8_t *data, size_t len, ttt_view *out) {
    if (data == NULL || len != TTT_VIEW_LEN) return false;
    ttt_view v;
    for (int i = 0; i < TTT_CELLS; i++) {
        char c = (char)data[i];
        if (c != 'X' && c != 'O' && c != '.') return false;
        v.cells[i] = c;
    }
    switch ((char)data[9]) {
    case 'X': v.status = TTT_RUNNING_X; break;
    case 'O': v.status = TTT_RUNNING_O; break;
    case 'x': v.status = TTT_X_WON; break;
    case 'o': v.status = TTT_O_WON; break;
    case 'D': v.status = TTT_DRAW; break;
    case '-': v.status = TTT_NO_RESULT; break;
    default: return false;
    }
    char me = (char)data[10];
    if (me != 'X' && me != 'O' && me != 'S') return false;
    v.me = me;
    *out = v;
    return true;
}

bool ttt_view_running(const ttt_view *v) { return v->status == TTT_RUNNING_X || v->status == TTT_RUNNING_O; }

char ttt_view_to_move(const ttt_view *v) {
    if (v->status == TTT_RUNNING_X) return 'X';
    if (v->status == TTT_RUNNING_O) return 'O';
    return 0;
}

bool ttt_view_my_turn(const ttt_view *v) { return ttt_view_running(v) && ttt_view_to_move(v) == v->me; }

char ttt_view_winner(const ttt_view *v) {
    if (v->status == TTT_X_WON) return 'X';
    if (v->status == TTT_O_WON) return 'O';
    return 0;
}

size_t ttt_action_move(int cell, uint8_t out[1]) {
    if (cell < 0 || cell >= TTT_CELLS) return 0;
    out[0] = (uint8_t)('0' + cell);
    return 1;
}

bool ttt_parse_line(const char *s, int out[3]) {
    /* exactly "d,d,d" with digits 0..8 */
    if (s == NULL) return false;
    for (int i = 0; i < 3; i++) {
        if (s[0] < '0' || s[0] > '8') return false;
        out[i] = s[0] - '0';
        s++;
        if (i < 2) {
            if (*s != ',') return false;
            s++;
        }
    }
    return *s == '\0';
}
