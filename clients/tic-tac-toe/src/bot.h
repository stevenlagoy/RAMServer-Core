/* A tic-tac-toe opponent for --bot mode: chooses a move from the board in a view, never touching the
 * network itself. */
#ifndef TTT_BOT_H
#define TTT_BOT_H

#include "view.h"

typedef enum { BOT_PERFECT, BOT_CASUAL } bot_skill;

/* Returns the cell (0..8) to play for mark ('X' or 'O') on cells, or -1 if the board is full.
 * BOT_PERFECT never loses; BOT_CASUAL plays a random move a third of the time. seed varies the choice
 * among equally good moves. */
int bot_choose(const char cells[TTT_CELLS], char mark, bot_skill skill, unsigned *seed);

#endif
