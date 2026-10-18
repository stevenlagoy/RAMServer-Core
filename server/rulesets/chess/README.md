# Chess

The chess ruleset for RAMServer, implemented as a `game.Game` plugin (package `chess`). The server core owns the connection, lobby, and persistence layers; this package decides what a legal chess move is and when a match is over.

Chess rules follow the FIDE Laws of Chess, Section I. There is no turn timer and there are no competition rules (like forfeiting on time). Clients connect to the server according to the wire protocol in `docs/protocol.md`.

## Contents

- [Status](#status)
- [Roles and players](#roles-and-players)
- [Messages from client to server](#messages-from-client-to-server)
- [Messages from server to client](#messages-from-server-to-client)
- [Errors](#errors)
- [How a match ends](#how-a-match-ends)
- [UCI notation](#uci-notation)
- [SAN notation](#san-notation)
- [Code layout](#code-layout)
- [Testing](#testing)

## Status

| Feature                                                               | State               |
|-----------------------------------------------------------------------|---------------------|
| All piece movement, castling, en passant, promotion                   | Implemented, tested |
| Check, pins, and moves which leave the king attacked                  | Implemented, tested |
| Checkmate, stalemate, insufficient material, 75-move rule, repetition | Implemented, tested |
| UCI moves from clients                                                | Implemented         |
| SAN output for move history                                           | Implemented         |
| King status (in check, checkmate, none) sent to clients               | Not implemented     |
| Resignation                                                           | Not implemented     |
| Draw offers, acceptance, and claims                                   | Implemented, tested |

The sections below say which behavior is planned when it is described.

## Roles and players

| Role      | Count     | Notes                                                                  |
|-----------|-----------|------------------------------------------------------------------------|
| player    | exactly 2 | The first player to join the lobby plays white, the second plays black |
| spectator | unlimited | Spectators do not need to be ready for the match to start              |

A match starts when both players are ready. The server rejects a match with fewer than two players.

## Messages from client to server

A move is sent as the opaque `encoded_action` of an `ActionRequest`. For chess its bytes are ASCII UCI:

- `e2e4` is a normal move.
- `e7e8q` is a promotion, with the piece to promote to as the last character.
- `e1g1` is castling. It is written as the king's two-square move, and the server moves the rook.
- `e2e4(=)` plays the move and offers a draw. Any move can carry the `(=)` suffix.
- `(=)` alone accepts the draw offered with the opponent's last move. It is legal only when an offer is pending.

Any move declines a pending offer, and a move marked `(=)` replaces it with an offer of the mover's own. A draw cannot be offered without a move, because only the player to move is permitted to send messages.

The server only reads UCI. Clients never send SAN. Resignation and draw offers are not part of the client message format yet.

A move is accepted only if all of these hold:

1. The sender is the player whose turn it is.
2. The origin square holds one of the sender's pieces.
3. The piece may move that way, and no piece is in the way.
4. The target square does not hold one of the sender's pieces.
5. A pawn which reaches the last rank includes a promotion letter, and no other move includes one.
6. The sender's own king is not attacked after the move.
7. For castling: the king and rook have not moved, the squares between them are empty, and the king does not castle out of, through, or into check.

## Messages from server to client

After every accepted move, and once when the match starts, the server broadcasts the state. The state consists of:

- The current game state in FEN notation
- The status of the white king (in check, checkmate, none) *(planned)*
- The status of the black king (in check, checkmate, none) *(planned)*
- Previous moves in Standard Algebraic Notation (SAN)
- Previous moves in Universal Chess Interface notation (UCI)

Chess has no hidden information, so every recipient receives the same view.

The FEN string has six fields: piece placement (rank 8 first), the color to move, castling rights (`KQkq` or `-`), the en passant target square (or `-`), the halfmove clock, and the fullmove number. The en passant target is set after every two-square pawn push, whether or not a capture is possible. The starting position is:

```
rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1
```

## Errors

A rejected move leaves the state unchanged, is logged, and is answered with an error to the sender only. Clients should branch on the error kind rather than on the message text.

| Error                      | Cause                                                                                                      | Protocol code                    |
|----------------------------|------------------------------------------------------------------------------------------------------------|----------------------------------|
| `game.ErrMalformedAction`  | The payload is not UCI: wrong length, an unknown square, or a promotion letter other than `q r b n`        | `ERROR_CODE_MALFORMED_MESSAGE`   |
| `game.ErrIllegalAction`    | The payload is valid UCI but not a legal move right now, including a move by the player who is not on turn | `ERROR_CODE_ILLEGAL_ACTION`      |

Uppercase UCI (`E2E4`) is malformed. A promotion letter on a move which does not promote is illegal, and so is a missing letter on a move which does.

## How a match ends

When a match ends, the clients receive:

- The final board state in FEN and a history of all moves in SAN and UCI
- The final result and its reason

| Reason                  | Result         | Condition                                                                                                                                           |
|-------------------------|----------------|-----------------------------------------------------------------------------------------------------------------------------------------------------|
| `CHECKMATE`             | `1-0` or `0-1` | The player to move has no legal move and their king is attacked. The other player wins                                                              |
| `STALEMATE`             | `1/2-1/2`      | The player to move has no legal move and their king is not attacked                                                                                 |
| `INSUFFICIENT_MATERIAL` | `1/2-1/2`      | Neither player can checkmate: king against king, king and one bishop or knight against king, or only bishops which all stand on one color of square |
| `AUTOMATIC_DRAW`        | `1/2-1/2`      | 75 moves (150 halfmoves) without a pawn move or capture, or the same position occurs for the third time                                             |
| `RESIGNATION`           | `1-0` or `0-1` | A player left the game. The other player wins *(planned)*                                                                                           |
| `MUTUAL_DRAW_OFFER`     | `1/2-1/2`      | An offer was accepted, or an offer claimed a draw after 100 halfmoves or on the third repetition                                                    |

Notes:

- Checkmate takes priority over every automatic draw. A move which mates on the 150th halfmove is a win.
- As in FIDE, threefold repeition and the 50-move rule are claims: the player to move claims by sending a move marked `(=)`, and the game ends at once. Fivefold repetition and 75 moves end the game with no offer. 
- Insufficient material is checked only for positions in which no checkmate can ever occur. Two knights against a bare king is not declared a draw, because mate is possible if the defender blunders.

## UCI notation

Universal Chess Interface notation is a simple notation. It has either 4 characters (for most moves) or 5 characters (for promotions). It gives the origin square followed by the destination square, and optionally a symbol for promotion.

- Files are lowercase letters `a` to `h`, from the left of the white player to the right.
- Ranks are digits `1` to `8`, with 1 closest to the white player (the bottom of the board).
- Promotion symbols are lowercase: `q` for queen, `r` for rook, `b` for bishop, and `n` for knight. A pawn cannot promote to a king or a pawn.

Examples: `e2e4`, `g1f3`, `a7a8q`, `e1g1` (white castles kingside), `e8c8` (black castles queenside).

## SAN notation

Standard Algebraic Notation is produced by the server for the move history. Clients may use it for display.

Pieces have a symbol:

| Piece  | Symbol |
|--------|--------|
| pawn   |        |
| rook   | R      |
| knight | N      |
| bishop | B      |
| queen  | Q      |
| king   | K      |

Pawns do not have an assigned symbol.

Each rank (horizontal row) has a number from 1 to 8, with 1 being closest to the white player's side (the official bottom of the board).

Each file (vertical column) has a letter from a to h, with a being to the left of the white player. These are always lowercase.

Moves are expressed as the symbol of the piece being moved and its new location on the board. For instance `Ra4` indicates that a rook is moving to a4. A pawn push has only the destination, like `e4`.

Captures are shown with an `x` after the moving piece's symbol. `Rxa4` indicates that a rook is moving to capture on a4. When a pawn captures, the file it is moving from is used to identify it, like `axb5`, which indicates that the pawn previously on file a is capturing a piece on b5. En passant is marked as a capture to the square the pawn is moving to, not where the captured pawn is. In `axb6`, the pawn previously on file a has captured a pawn which moved two squares from b7 and is now on b5.

When a pawn promotes, the symbol of the piece type it promotes to is appended at the end with an equals sign. `a8=Q` indicates that a pawn reached square a8 and promoted to a queen.

When a move is ambiguous, more information about the piece that is moving must be given. The rank, file, or both of the moving piece may be present, but no more information than is necessary should be included. Extra information is always put right after the piece symbol and before a capture mark.

- `Rce1` indicates that a rook on the c file is moving to e1.
- `R1e3` indicates that a rook on rank 1 is moving to e3, which is used when both rooks stand on the same file.
- `Qh3f1` indicates that a queen on h3 is moving to f1, which is used when one other queen shares the file and another shares the rank.

Prefer the file to disambiguate, and use the rank only if the ambiguous pieces are on the same file. Both are needed only in rare positions. A piece which cannot legally make the move (for example because it is pinned) does not make the move ambiguous. Any other rules may be combined with this (like promotion, and a `+` or `#` following the move).

Castling is indicated by `0-0` for kingside castling or `0-0-0` for queenside castling. These are zeros, as in this document. PGN uses the letter O instead, so if that is wanted, the two castling constants in `action.go` and this paragraph must change together.

A move which places the opponent in check must include a plus sign at the end. `Be4+` indicates that a bishop moves to e4 and places the opposing king in check (but not checkmate). At most one plus sign should be included. Checkmate is marked with a hash symbol at the end in place of the plus. `Be4#` indicates that a bishop moves to e4 and puts the opposing king in checkmate. A plus sign should not be included if a hash symbol is present. Plus signs and hash symbols always go at the very end of the move, after any promotion.

End-of-match symbols are `1-0` for a white win, `0-1` for a black win, `1/2-1/2` for a draw, and `*` for a game which has not finished.

### Grammar

The grammar for a SAN action is:

```
:action:    ::= :move: | (=/=)
:move:      ::= :castle: | :piecemove:
:castle:    ::= '0-0' | '0-0-0'
:piecemove: ::= [:symbol:][:file:][:rank:]['x']:file::rank:[:promotion:]['+'|'#'][' (=)']
:symbol:    ::= 'K' | 'Q' | 'B' | 'N' | 'R'
:file:      ::= 'a' | 'b' | 'c' | 'd' | 'e' | 'f' | 'g' | 'h'
:rank:      ::= '1' | '2' | '3' | '4' | '5' | '6' | '7' | '8'
:promotion: ::= '=' :symbol:
```

Read left to right, a piece move is: SYMBOL, FILE, RANK (the last two only when disambiguating), CAPTURE, destination FILE, destination RANK, PROMOTION, CHECK or CHECKMATE, DRAW OFFER. The symbol is omitted for pawns, and a pawn capture begins with the pawn's file.

## Code layout

| File            | Contents                                                                                                                 |
|-----------------|--------------------------------------------------------------------------------------------------------------------------|
| `chess.go`      | `Game`, the `game.Game` implementation. Delegates to `State` and wraps decode and legality errors                        |
| `state.go`      | `State` (board, turn, castling, en passant, counters, history), FEN reading and writing, legality, `apply` and `outcome` |
| `piece.go`      | Colors, piece types, and the movement patterns each piece follows                                                        |
| `action.go`     | `Move`, UCI decoding and encoding, SAN output                                                                            |
| `outcome.go`    | Termination reasons and `ChessResult`                                                                                    |
| `chess_test.go` | Unit, perft, adversarial, and end-to-end tests                                                                           |

Design notes:

- A `State` is never modified after it is built. `apply` returns a new `State`, so the server can keep or broadcast old states safely.
- Legality is decided by attack detection: a move is legal when a movement pattern allows it, nothing blocks it, and the mover's king is not attacked afterwards. Move generation tries every square for every piece and keeps the legal ones.
- Pieces are described by `MovementPattern` values, so a variant piece can be added without changing the rules engine.
- Decoding is separate from board lookups. `DecodeMove` only reads the bytes. `State.resolve` finds the piece, and `State.isLegal` decides whether the move may be played.

## Testing

Run from the `server/` directory:

```bash
go test -race ./...
```

The suite includes:

- **Unit tests** for FEN parsing and writing, notation, movement of each piece, castling, en passant, and promotion.
- **Perft tests** which count every position reachable to a fixed depth from seven known positions and compare the totals with the published numbers. Depth 3 runs by default. Set `RAMSERVER_PERFT_DEEP=1` for depth 4, which takes a couple of minutes.
- **Adversarial tests** which submit every pair of squares and check that exactly the legal moves are accepted.
- **End-to-end tests** which play a full game through the `game.Game` interface in the same order `Match` uses (`ActiveTurn`, `Validate`, `Outcome`, `Apply`).

Coverage of this package is above 98%. Bug fixes should come with a test which fails before the fix and passes after it.