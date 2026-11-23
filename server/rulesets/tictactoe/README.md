# Tic-tac-toe ruleset

The RAMServer ruleset for standard 3×3 tic-tac-toe. Package `tictactoe`, type `Game`, implementing
`game.Game` plus the optional `game.OutOfTurnActions` and `game.Annotator` interfaces.

This file is the complete description of the game's wire encodings. A client needs nothing else from
this package.

| | |
| --- | --- |
| `game_id` | `tictactoe` (the constant `tictactoe.Name`) |
| Roles | `player`: exactly 2. `spectator`: any number, including none. |
| Start condition | Exactly two players are in the session and both have sent `SetReadyRequest{ready: true}`. Spectators' ready flags are ignored. |
| A match is | One game. A rematch is a new match: both players ready up again. |
| `encoded_action` | One ASCII digit `0`–`8` (a cell), or the 6 ASCII bytes `resign`. |
| `encoded_view` | Always exactly 11 ASCII bytes: 9 cells, a status byte, and the recipient's own mark. |
| `termination_reason` | `THREE_IN_A_ROW`, `DRAW`, `RESIGNATION` (and `NOT_ENOUGH_PLAYERS`, which shouldn't occur) |

Integration, for the server team:

```go
games.Register(tictactoe.Name, func() game.Game { return &tictactoe.Game{} })
```

## Rules

- The board has 9 cells. Players take turns placing their own mark in an empty cell.
- **X** moves first. X is the first of the two players to have joined the session (the earliest
  member with the `player` role in join order). O is the other player.
- A player who completes a row, column, or diagonal of their own mark wins (`THREE_IN_A_ROW`). This is
  checked before the draw rule, so a ninth mark that completes a line is a win.
- If all nine cells are filled and nobody has a line, the game is a draw (`DRAW`).
- Either player may resign at any time while the game is running, even when it isn't their turn. The
  other player wins (`RESIGNATION`).
- Spectators may watch but may not move or resign.

## Roles, joining and starting

`Roles()` returns, in this order:

| Role | Min | Max |
| --- | --- | --- |
| `player` | 2 | 2 |
| `spectator` | 0 | unlimited |

The server gives each joiner the first role with room, so **the first two members to join are the
players and everyone after them is a spectator**. A member keeps that role for as long as they stay in
the session. If a player leaves a waiting lobby, the next person to join takes the free player seat. A
spectator doesn't move up to fill it.

The match starts as soon as both players are ready. Spectators never need to ready up and can't hold up
a match. They also can't start one by readying.

## Board and cell numbering

Cells are numbered 0–8, row by row from the top-left:

```
 0 | 1 | 2
---+---+---
 3 | 4 | 5
---+---+---
 6 | 7 | 8
```

Cell `n` is in row `n / 3` and column `n % 3` (0-based). The eight lines, in the order the game checks
them, are `0,1,2` `3,4,5` `6,7,8` `0,3,6` `1,4,7` `2,5,8` `0,4,8` `2,4,6`.

## `encoded_action`: what a player sends

`ActionRequest.encoded_action` must be **exactly** one of:

| Bytes | Length | Meaning |
| --- | --- | --- |
| `0x30`–`0x38` (ASCII `'0'`–`'8'`) | 1 | Place your mark in that cell. |
| `72 65 73 69 67 6e` (ASCII `resign`) | 6 | Resign. Allowed out of turn. |

There is no terminator, no whitespace, no length prefix, and case matters. Anything else, including
`"9"`, `"4\n"`, `" 4"`, `"04"`, `"RESIGN"`, and an empty payload, is rejected with
`ERROR_CODE_MALFORMED_MESSAGE`.

In C:

```c
uint8_t move = (uint8_t)('0' + cell);          /* cell in 0..8 */
action.encoded_action.data = &move;
action.encoded_action.len  = 1;

static const uint8_t RESIGN[] = { 'r','e','s','i','g','n' };
action.encoded_action.data = (uint8_t *)RESIGN;
action.encoded_action.len  = sizeof RESIGN;    /* 6: no NUL */
```

Examples:

| To do this | Send |
| --- | --- |
| Take the center | `"4"` (1 byte, `0x34`) |
| Take the top-left corner | `"0"` (1 byte, `0x30`) |
| Take the bottom-right corner | `"8"` (1 byte, `0x38`) |
| Resign | `"resign"` (6 bytes) |

### `LegalActionsResponse`

`LegalActionsRequest` returns exactly the payloads above that you may send right now. Each `payload`
can be sent back unchanged.

- **Player to move:** one entry per empty cell, in cell order, then `resign`. Descriptions are
  `top-left`, `top-center`, `top-right`, `middle-left`, `center`, `middle-right`, `bottom-left`,
  `bottom-center`, `bottom-right`, and `resign`.
- **The other player:** only `resign`.
- **Spectators, and anyone after the game has ended:** nothing.

Note that the waiting player's list isn't empty, because resigning is allowed out of turn (see
[Questions for the server team](#questions-and-gaps-in-the-documentation)).

## `encoded_view`: what every member receives

`StateUpdate.encoded_view` during a match is always **exactly 11 bytes of printable ASCII**, with no
terminator:

| Offset | Length | Contents |
| --- | --- | --- |
| 0–8 | 9 | The board. Byte `n` is cell `n`: `'X'`, `'O'`, or `'.'` (empty). |
| 9 | 1 | Status (see below). |
| 10 | 1 | You: `'X'` or `'O'` if the recipient is a player, `'S'` if a spectator. |

Status byte:

| Byte | Meaning |
| --- | --- |
| `'X'` | Game running; X to move. |
| `'O'` | Game running; O to move. |
| `'x'` (lowercase) | Game over; X won (by a line or because O resigned). |
| `'o'` (lowercase) | Game over; O won (by a line or because X resigned). |
| `'D'` | Game over; draw. |
| `'-'` | Game over with no result (`NOT_ENOUGH_PLAYERS`; shouldn't occur). |

So it's your turn exactly when `view[9] == view[10]`. Only the last byte differs between recipients:
the two players and the spectators get the same first 10 bytes.

While the session is waiting (before a match, and between matches), the server sends an **empty**
`encoded_view`. A client should treat any view that isn't exactly 11 bytes as "no board to show".

Examples (shown as strings; the quotes aren't part of the bytes):

| Situation | Board | View sent to X | to O | to a spectator |
| --- | --- | --- | --- | --- |
| Start of a match | empty | `.........XX` | `.........XO` | `.........XS` |
| X took the center; O to move | `....X....` | `....X....OX` | `....X....OO` | `....X....OS` |
| O took top-left; X to move | `O...X....` | `O...X....XX` | `O...X....XO` | `O...X....XS` |
| X won with the top row | `XXXOO....` | `XXXOO....xX` | `XXXOO....xO` | `XXXOO....xS` |
| Draw | `XOXXOOOXX` | `XOXXOOOXXDX` | `XOXXOOOXXDO` | `XOXXOOOXXDS` |

The draw board above, drawn:

```
 X | O | X
---+---+---
 X | O | O
---+---+---
 O | X | X
```

Decoding in C:

```c
/* view, len from StateUpdate.encoded_view */
if (len != 11) { /* waiting lobby: no board */ }
char cell[9]; memcpy(cell, view, 9);   /* 'X', 'O' or '.' */
char status = (char)view[9];
char me     = (char)view[10];          /* 'X', 'O' or 'S' */
int running  = (status == 'X' || status == 'O');
int my_turn  = running && status == me;
```

## Annotations

Every in-progress `StateUpdate` carries these `annotations`. They're the same for every recipient. All
values are ASCII strings.

| Key | Present | Value |
| --- | --- | --- |
| `x_player` | always | The `player_id` playing X. |
| `o_player` | always | The `player_id` playing O. |
| `moves` | always | Number of marks on the board, `"0"` to `"9"`. |
| `last_move` | after the first mark | Cell of the most recent mark, `"0"` to `"8"`. |
| `winning_line` | when won by `THREE_IN_A_ROW` | The three cells of the line, ascending, comma-separated, such as `"0,4,8"`. |
| `resigned` | when ended by `RESIGNATION` | The mark of the player who resigned, `"X"` or `"O"`. |

Use `x_player`/`o_player` with `StateUpdate.players` to show who plays which mark. A client should ignore
keys it doesn't know. More may be added later.

Example after X wins along the diagonal on their fourth mark:

```
x_player     = "pl_7f3a…"   (whatever ids the server issued)
o_player     = "pl_91c0…"
moves        = "7"
last_move    = "8"
winning_line = "0,4,8"
```

## End of a match: `MatchResult`

After the move that ends the game, the server broadcasts a final `StateUpdate`, then a `MatchResult`.

| `termination_reason` | When | `winner_player_ids` | `draw` |
| --- | --- | --- | --- |
| `THREE_IN_A_ROW` | A player completed a line. | the player who completed it | false |
| `DRAW` | The ninth mark completed no line. | empty | true |
| `RESIGNATION` | A player sent `resign`. | the other player | false |
| `NOT_ENOUGH_PLAYERS` | A match was started with fewer than two players. The start condition prevents this, so it's a safeguard only. | empty | true |

`turns` counts every applied action, including a resignation. After the `MatchResult`, the session
returns to waiting with all ready flags cleared. Both players send `SetReadyRequest{ready: true}` again
for a rematch, keeping the same seats (see below).

## Errors a player can see

Rejected actions change nothing. `ErrorResponse.message` is exactly the text below. Clients should
branch on `code`, and may show `message` to the player.

| `code` | `message` | Cause |
| --- | --- | --- |
| `ERROR_CODE_MALFORMED_MESSAGE` | `malformed action: send one digit "0" to "8" for a cell, or "resign"` | The payload is not exactly one of the encodings above. |
| `ERROR_CODE_ILLEGAL_ACTION` | `illegal action: cell N is already taken` (N is 0–8) | Moving into an occupied cell. |
| `ERROR_CODE_ILLEGAL_ACTION` | `illegal action: spectators cannot play` | A spectator sent a move or `resign`. |
| `ERROR_CODE_ILLEGAL_ACTION` | `illegal action: the game is over` | An action arrived after the game ended but before the server closed the match. Normally the server rejects these itself first. |
| `ERROR_CODE_ILLEGAL_ACTION` | `illegal action: it is not your turn` | Shouldn't reach a client, because the server's turn check answers first (below). Kept as a safeguard. |
| `ERROR_CODE_ILLEGAL_ACTION` | `illegal action: no game in progress` | Shouldn't reach a client. The game was handed a state it didn't create. |

Errors produced by the server rather than this game, for the same request:

- `ERROR_CODE_OUT_OF_TURN`: a cell move from the player who isn't to move, or from a spectator. The
  server checks turns before the game sees the action. Its `message` is the server's, not listed here.
  `resign` is exempt from this check, so a spectator's `resign` reaches the game and gets
  `spectators cannot play`.
- `ERROR_CODE_ILLEGAL_ACTION` for any action while the session is waiting (no match running).
- `ERROR_CODE_STALE_STATE`, `ERROR_CODE_NOT_IN_SESSION`: see `docs/protocol.md`.

The malformed check happens first. A spectator who sends `"9"` gets `MALFORMED_MESSAGE` if the action
reaches the game at all.

## Design decisions

- **Spectators are allowed, unlimited, and never need to be ready.** Watching costs the game nothing,
  and the server already broadcasts to every member. Players are listed first in `Roles()`, so the
  first two joiners play, as other RAMServer clients expect.
- **A match is a single game, not a series.** The protocol already handles a series: after each
  `MatchResult` the lobby waits, both players ready up again, and `MatchHistoryRequest` returns every
  finished game in the session. A best-of-N inside one match would need its own score encoding and
  would hide individual games from the match history.
- **X is the first player in join order, in every match.** `NewState` receives only the roster, not how
  many matches the session has played, so the game can't alternate who starts in a rematch. A client
  that wants alternation can have the players leave and rejoin in the other order. See the questions
  below.
- **Resigning is allowed out of turn.** If a player leaves or stops responding mid-match, the server
  doesn't tell the game, and the match would otherwise be stuck on their turn. With `resign` the
  remaining player can always end the match. It's offered in `LegalActions` to both players, because
  the compliance tests require that everything `Validate` accepts from a member who can send it is
  listed.
- **A fixed-length ASCII view and one-byte moves.** These are trivial to build and parse in C (no
  varints, no endianness, no JSON), and readable in logs and packet dumps. Putting the recipient's own
  mark in the view means a client doesn't need annotations or `player_order` to know whose turn it is.
- **The view carries the result too** (lowercase status). A client can draw the final board and the
  outcome from the last `StateUpdate` alone. `MatchResult` confirms it and gives the reason.
- **Strict parsing.** Exactly one encoding per action makes the README's format the only accepted one.
  The client gets `MALFORMED_MESSAGE` immediately if it adds a newline or a NUL terminator.
- **The state is a plain struct of arrays and scalars** (no slices, maps or pointers). Go copies it on
  assignment, so `Apply` can't change its input by construction. A unit test and the compliance tests
  check this anyway.
- **Defensive handling of things that can't happen.** If `NewState` gets fewer than two players, the
  match ends immediately with `NOT_ENOUGH_PLAYERS` instead of waiting forever. If a method is handed a
  state that isn't the game's own, it returns errors or empty results rather than panicking.

## Questions and gaps in the documentation

These weren't answered by the documentation in this repository. The choice made is noted for each.

1. **`LegalActionsResponse` for the waiting player.** `docs/protocol.md` says `legal_actions` is
   "empty … while it is not the requester's turn". But `ERROR_CODE_OUT_OF_TURN`'s description allows a
   game to let players resign at any time, and `gametest` requires such an action to be listed for the
   member who may send it. Chosen: `resign` is listed for the waiting player. Clarify the protocol text.
2. **Players who leave mid-match.** `docs/protocol.md` says leaving a match is a forfeit and "the game
   decides" whether it continues or ends with `MATCH_END_REASON_FORFEIT`. `docs/writing-a-ruleset.md`
   says the game is not told and the match is not ended. `game.Game` has no hook for it. Chosen: the
   remaining player can resign to end the match, with termination reason `RESIGNATION`. That names the
   leaver as the winner, which is wrong for a forfeit. A "member left" hook, or the server ending the
   match with `FORFEIT` and the other player as winner, would fix this.
3. **Session status after `MatchResult`.** `docs/protocol.md` says the session moves to
   `SESSION_STATUS_CLOSED`. `docs/writing-a-ruleset.md` and `docs/writing-a-client.md` say it returns to
   waiting for a rematch. This README follows the latter.
4. **`player_order`.** The protocol says it is "empty until the game sets one", but `game.Game` has no
   way to set it, and it isn't stated how the server fills it. Chosen: seats are given in the view (your
   own mark) and in the `x_player`/`o_player` annotations, so clients don't need `player_order`.
5. **Alternating the first move in rematches.** `NewState` isn't told the match number or previous
   results, so a game can't alternate X between matches without storing data in the `Game`, which isn't
   allowed. Chosen: X is always the first player in join order.
6. **Annotations on the final update.** The ruleset guide says annotations come with each in-progress
   `StateUpdate`. It's assumed the broadcast after the game-ending move is one of these, so
   `winning_line` and `resigned` reach clients. If it isn't, clients still get the result from the
   view's status byte and from `MatchResult`.
7. **Error message prefix.** The guide says "the error text becomes `ErrorResponse.message`". It's
   assumed this is the full `err.Error()`, including the `illegal action: ` / `malformed action: `
   prefix from the wrapped sentinel errors, so the messages above include it.
8. **Out-of-turn rejections and the active-player check.** It isn't documented what `message` the
   server sends with `ERROR_CODE_OUT_OF_TURN`, or how it treats an action from a spectator who isn't in
   `ActiveTurn`. Assumed: `OUT_OF_TURN`, the same as for the waiting player.
9. **Small documentation issues noticed.** In `docs/protocol.md`'s `ErrorCode` table, the
   `ERROR_CODE_OUT_OF_TURN` and `ERROR_CODE_ILLEGAL_ACTION` rows are joined on one line (`||`), so the
   table renders wrongly. Its session message table says `MatchHistoryResponse` is sent "In reply to
   `MatchHistoryResponse`" instead of `MatchHistoryRequest`. `Result.Scores` is documented as not sent
   to clients, so this game doesn't set it.

## Tests

From the repository root:

```
make test
```

This runs `gofmt` and `go vet` checks, this package's tests, and the SDK's own tests. Requires Go 1.24
or newer (tested with go1.24.7). Nothing is downloaded: `go.mod` points the server's import path at
`./sdk`.

- `tictactoe_test.go` holds the rule tests:
  - start conditions and roles
  - seat assignment from join order
  - turn order
  - a win along every one of the 8 lines, for X and for O
  - a ninth-move win compared with a draw
  - resignation by either player, in and out of turn
  - `LegalActions` for players, spectators and non-members
  - every malformed and illegal action and its exact message
  - that `Validate` and `Apply` never modify their input, including through a `*State`
  - that no method panics on a foreign state
- `compliance_test.go` runs `sdk/game/gametest` with two players and two spectators, plus extra seeds
  with players only.

Run one package's tests directly with `go test ./rulesets/tictactoe`.
