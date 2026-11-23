/* Decoding the tic-tac-toe encoded_view and building encoded_action, exactly as specified in
 * rulesets/tictactoe/README.md. */
#ifndef TTT_VIEW_H
#define TTT_VIEW_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#define TTT_VIEW_LEN 11
#define TTT_CELLS 9

typedef enum {
    TTT_RUNNING_X, /* 'X': X to move */
    TTT_RUNNING_O, /* 'O': O to move */
    TTT_X_WON,     /* 'x' */
    TTT_O_WON,     /* 'o' */
    TTT_DRAW,      /* 'D' */
    TTT_NO_RESULT  /* '-' */
} ttt_status;

typedef struct {
    char cells[TTT_CELLS]; /* 'X', 'O' or '.' */
    ttt_status status;
    char me; /* 'X', 'O' or 'S' (spectator) */
} ttt_view;

/* Decodes an encoded_view. Returns false, leaving *out untouched, unless it is exactly 11 valid bytes
 * (an empty view, sent while the lobby waits, is not a board). */
bool ttt_view_decode(const uint8_t *data, size_t len, ttt_view *out);

bool ttt_view_running(const ttt_view *v);
/* The mark to move, 'X' or 'O', or 0 once the game is over */
char ttt_view_to_move(const ttt_view *v);
/* True when the recipient is a player and it is their move */
bool ttt_view_my_turn(const ttt_view *v);
/* The winning mark, 'X' or 'O', or 0 */
char ttt_view_winner(const ttt_view *v);

/* encoded_action for placing a mark in cell 0..8: one ASCII digit. Returns 1, or 0 for a bad cell. */
size_t ttt_action_move(int cell, uint8_t out[1]);
/* encoded_action for resigning: the 6 bytes "resign" (no terminator) */
extern const uint8_t TTT_ACTION_RESIGN[6];

/* Parses an annotation like winning_line "0,4,8" into three cells. Returns false if malformed. */
bool ttt_parse_line(const char *s, int out[3]);

#endif
