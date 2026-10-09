# Chess

Chess rules are according to the FIDE rules, Section I. There is no turn timer or additional competition rules (like forfeiting).

Clients will connect to the server according to the protocol.

Transmissions from the server to clients will consist of:
- The current game state in FEN notation
- The status of the white king (in check, checkmate, none)
- The status of the black king (in check, checkmate, none)
- Previous moves in Standard Algebraic Notation (SAN)
- Previous moves in Universal Chess Interface (UCI)

Transmissions of moves from clients to the server should consist of:
- The desired move in SAN or UCI

The server will validate the move and send the updated board state (FEN) to all clients if successful, or return an error if unsuccessful.

When a match ends, the clients will receive:
- The final board state in FEN and a history of all moves in SAN and UCI
- The final result
    - white win, when the black king is checkmated
    - black win, when the white king is checkmated
    - stalemate, when the player whose turn it is to move has no legal moves available, and their king is not currently in check
    - white resignation, when the white player left the game
    - black resignation, when the black player left the game
    - insufficient material, when neither player has the pieces necessary to checkmate the other

## SAN Notation:

Back-rank pieces have a symbol:
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

Moves are expressed as the symbol of the piece being moved and its new location on the board. For instance `Ra4` indicates that a Rook is moving to a4.

Captures are shown with an `x` after the moving piece's symbol. `Rxa4` indicates that a bishop is moving to capture on a4. When a pawn captures, the file it is moving from is used to identify it, like `axb5` which indicates that the pawn previously on the a file is capturing a piece on b5. En passant is marked as a capture to the square the pawn is moving to, not where the captured piece is, like in `axb6` which indicates that the pawn previously on rank a has captured a pawn which moved two spaces from b7 and is now on b5.

When a pawn promotes, the symbol of the piece type it promotes to is appended at the end with an equals sign. `a8=Q` indicates that a pawn reached square a8 and promoted to a Queen.

When a move is ambiguous, more information about the piece that is moving must be given. The rank, file, or both of the moving piece may be present, but no more information than is necessary should be included. Extra information is always put right after the piece symbol and before a capture mark. `Rce1` indicates that a rook on the c file is moving to e1. `B7xe5` indicates that a Bishop on rank 7 is capturing on e5. `Qh3f1` indicates that a Queen on h3 is moving to f1. The case in which both rank and file are needed to disambiguate moving pieces is exceptionally rare, and usually only the rank or file is needed. Prefer using the file to disambiguate, and use the rank only if the ambiguous pieces are on the same file. Any other rules may be combined with this (like promotion and/or a + or # following the move).

Castling is indicated by `0-0` for kingside castling or `0-0-0` for queenside castling.

A move which places the opponent in check must include a plus sign at the end. `Be4+` indicates that a bishop moves to e4 and places the opposing king in check (but not checkmate). At most one plus sign should be included. Checkmate is marked with a hash symbol at the end in place of the plus. `Be4#` indicates that a bishop moves to e4 and puts the opposing king in checkmate. A plus sign should not be included if a hash symbol is present. Plus signs or hash symbols always go at the very end of the move, after any promotions.

A draw offer is indicated by appending `(=)` after the move. Responses can either be to send back `1/2-1/2` to accept the draw, or to send the next move with nothing added to reject it. When offering a draw without making a move, just `(=)` may be sent, and valid responses are either `1/2-1/2` to accept the draw, or `(=/=)` to reject it.

End-of-match symbols are: `1-0` for a white win, `0-1` for a black win, or `1/2-1/2` for a draw.

The grammar for a SAN action is:
:action: ::= :move:|(=/=)
:move: ::= [:symbol:][:file:][:rank:]['x']:file::rank:[:promotion:]['+'|'#'][' (=)']
:symbol: ::= ['K'|'Q'|'B'|'N'|'R']
:file: ::= ['a'|'b'|'c'|'d'|'e'|'f'|'g'|'h']
:rank: ::= ['1'|'2'|'3'|'4'|'5'|'6'|'7'|'8']
:promotion: ::= '=':symbol:

SYMBOL FILE RANK CAPTURE FILE RANK PROMOTION CHECK(MATE) DRAWOFFER

# UCI Notation:

Universal Chess Notation is a simpler notation. It has either 4 characters (for most moves) or 5 characters (for promotions). It gives the origin square with rank and file followed by the destination square with rank and file, and optionally a symbol for promotion.

Symbols used for promotion are `q` for queen, `b` for bishop, `n` for knight, and `r` for rook.