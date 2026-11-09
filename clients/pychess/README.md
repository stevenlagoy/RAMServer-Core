# Interpreted Client

The interpreted client is implemented in Python with Pygame.

## Requirements

- Python 3.12 or newer
- [buf](https://buf.build/docs/installation), to generate the protobuf code
- A running RAMServer: Go to run it locally, or Docker; or the hosted server, which needs no setup
- `websocket-client`, which `pip install -r requirements.txt` installs, to reach the hosted server (plain TCP needs no extra package)

## Setup

Run these once, from this directory (`clients/chess/interpreted_client`):
 
```bash
python3 -m venv .venv
source .venv/bin/activate # Windows: .venv\Scripts\activate
pip install -r requirements.txt
```
 
Generate the protobuf code from the repository root. Generated code is not committed, so run this again whenever `proto/` changes:
 
```bash
buf generate # writes clients/chess/interpreted_client/gen/
```
 
## Quick start

From the repository root (Linux, macOS or WSL), this builds and starts a local server and opens two clients that play each other:
 
```bash
./scripts/chess.sh # 2 clients
./scripts/chess.sh 4 # 4 clients, two matches
```
 
The script starts each client with `--auto`, which connects, joins a session and readies up without any input. The server stops once every client window is closed or you press Ctrl+C. The clients connect to `127.0.0.1:8080`, so stop any other server on that port first (`./scripts/dev.sh down`).
 
## Running the clients yourself

1. Start the server, from the repository root:
```bash
   cd server && go run ./cmd/ramserver # or, from the repository root: docker compose up --build
```
 
2. Start one client per player, each in its own terminal, from this directory with the venv activated:
```bash
   python main.py
```
 
| Option              | Description                                                                                                                   |
|---------------------|-------------------------------------------------------------------------------------------------------------------------------|
| `--server`          | Server for this run: `host:port` for TCP, or a `ws://` or `wss://` URL. Defaults to the last one used, then `127.0.0.1:8080`. |
| `--hosted`          | Use the hosted server (`wss://ramserver-core.cloud.run/connect`).                                          |
| `--name`            | Display name for this run. Defaults to the last name used.                                                                    |
| `-s`, `--session`   | Join a specific session by its ID. Implies `--auto`.                                                                          |
| `--auto`            | Connect, join an open session (or create one) and ready up with no input.                                                     |
| `--style`           | Board style for this run: `STANDARD`, `COLORFUL`, `OCEAN`, `TOURNAMENT`, `HIGH_CONTRAST` or `CUSTOM`.                         |
 
## Connecting over TCP or WebSocket

Both transports carry the same length-prefixed frames described in `docs/framing.md`. Over a WebSocket each frame travels inside a binary message, and one message may hold part of a frame or several, so the client reads from a buffer and the framing code is shared. Text messages are refused. The address decides the transport: a `ws://` or `wss://` URL uses a WebSocket, anything else uses TCP.
 
The hosted server must run the protocol 1.2 server changes for the lobby list, match history, resigning, and the check display. Against an older server the client still plays, and says where a feature is missing.
 
## Screens

**Connect.** Enter the server and a display name; both are remembered for next time. The server is `host:port` for a local or LAN server (the port defaults to 8080), or a `ws://` or `wss://` URL for a WebSocket server such as the hosted one. The **Local server** and **Hosted server** buttons fill in those two addresses. Tab moves between fields and Enter connects. A failed connection says where it tried and why.
 
**Lobby.** Before a match, each player is in a lobby (a session) and marks themselves ready; the match starts when the game's start condition is met.
 
- *Not in a lobby:* Quick match joins an open lobby or creates one. Create lobby and Create private lobby always make a new one; a private lobby is not listed and is joined by its ID. The list on the right shows open lobbies with a Join button, refreshed every few seconds, and a lobby can also be joined by typing its ID.
- *In a lobby:* the roster shows each player's role, whether they are the host, and whether they are ready. Ready and Cancel ready change your own flag. Copy ID copies the lobby's ID so another player can join it. The history panel lists the results of earlier matches in this lobby.
**Match.** The board is on the left (or on top in a tall window) and a panel on the right (or below) holds:
 
- the two players, with a marker on the player to move (the board's border is also marked on that side);
- the move number and the number of half-moves played;
- the status: whose turn it is, and "in check";
- **Offer draw with next move**, **Resign**, and **Settings**.
When the match ends, the result and how it ended are shown with **Play again** and **Back to lobby**.
 
### Playing

- You can only pick up your own pieces, and only on your turn.
- Move a piece by dragging it to a square and releasing it, or by clicking it and then clicking the square. A selected piece shows its legal moves: a dot on an empty square, a ring around a piece it can capture.
- Click the selected piece again, or anywhere off its moves, to deselect it. Esc also deselects, and with nothing selected it opens Settings.
- When a pawn reaches the last rank, pick the piece to promote to. Click elsewhere or press Esc to cancel the move.

### Draws and resigning

A draw offer travels with a move, because only the player to move can send anything. Press **Offer draw with next move** on your turn (press it again to cancel), then make your move. Your opponent sees the offer on their turn and can **Accept** it, or **Decline** it, which only dismisses the banner: making any move also declines the offer. Resigning asks for confirmation and works at any time, on either player's turn.
 
### Check display

A king in check has a red outline. The pieces giving check are ringed and joined to the king by a line. Squares which the king may not move to are marked with a red cross, either always or when you select your king (see Settings). The server sends this information with each state, so it is always the server's view of the position.

## Settings

Open Settings from the connect screen, the lobby, or a match (the button, or Esc). Changes apply at once and are saved when the screen closes, in `~/.ramchess/chess_settings.json` (set `RAMCHESS_CONFIG_DIR` to use another folder).
 
| Section       | Options                                                                                                                                                              |
|---------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Appearance    | Board style (five presets, or Custom with a color editor for light squares, dark squares, background and border); coordinates; highlight last move; show legal moves |
| Animations    | Animate moves on or off; speed from 0.5x to 3x                                                                                                                       |
| Check display | Red king outline; show checking pieces; unsafe king squares (off, when king selected, always)                                                                        |
 
Animations show the opponent's moves only, since your own move is already on the board when you make it. Castling moves the rook with the king, en passant and captures keep the captured piece until the capture lands, promotions stay pawns until they arrive, and knights lift above the pieces they jump over.

## Tests

```bash
pip install -r requirements-dev.txt
python -m pytest tests
```
 
The tests need the generated protobuf code and no server or window: pygame runs with its dummy video driver, and a fake connection feeds the screens scripted server messages. Set `SHOTS_DIR=some/folder` to also save a screenshot of each screen state.

## Code layout

| File                      | Contents                                                                                                                       |
|---------------------------|--------------------------------------------------------------------------------------------------------------------------------|
| `main.py`                 | Command line options, window, and the first screen                                                                             |
| `app.py`                  | The scene stack, main loop, and server event dispatch                                                                          |
| `networking.py`           | Server addresses, the TCP and WebSocket transports, framing, protobuf messages, a thread-safe event queue, and `NetworkClient` |
| `scenes/`                 | `connect`, `lobby`, `game`, `settings_scene`, and `dialogs`, on a shared `Scene` base class                                    |
| `board.py`, `piece.py`    | The board and its drawing: pieces, overlays, coordinates, and animation                                                        |
| `animation.py`            | Works out which pieces move for a move, and the timing (no pygame)                                                             |
| `fen.py`, `move.py`       | FEN parsing and UCI moves, including the draw offer suffix                                                                     |
| `outcome.py`              | Wording for how a match ended (no pygame)                                                                                      |
| `settings.py`, `styles.py`| Saved settings and the board style presets                                                                                     |
| `ui.py`, `end_screen.py`  | Buttons, toggles, sliders, selectors, text fields, toasts, and the end screen                                                  |
