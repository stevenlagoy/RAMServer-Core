# Interpreted Client

The interpreted client is implemented in Python with Pygame.

## Requirements

- Python 3.12 or newer
- [buf](https://buf.build/docs/installation), to generate the protobuf code
- A running RAMServer: Go to run it locally, or Docker

## Setup

Run these once, from this directory (`clients/chess/interpreted_client`):

```bash
python3 -m venv .venv
source .venv/bin/activate        # Windows: .venv\Scripts\activate
pip install -r requirements.txt
```

Generate the protobuf code from the repository root. Generated code is not committed, so run this again whenever `proto/` changes:

```bash
buf generate                     # writes clients/chess/interpreted_client/gen/
```

## Quick start

From the repository root (Linux, macOS or WSL), this builds and starts a local server and opens two clients that play each other:

```bash
./scripts/chess.sh               # 2 clients
./scripts/chess.sh 4             # 4 clients, two matches
```

The server stops once every client window is closed or you press Ctrl+C. The clients connect to `127.0.0.1:9000`, so stop any other server on that port first (`./scripts/dev.sh down`).

## Running the clients yourself

1. Start the server, from the repository root:

   ```bash
   cd server && go run ./cmd/ramserver   # or, from the repository root: docker compose up --build
   ```

2. Start one client per player, each in its own terminal, from this directory with the venv activated:

   ```bash
   python main.py
   ```

The client connects to `127.0.0.1:9000`, joins a chess session and readies up. The first client waits in a new session, the second one joins it, and the match starts. White is the player who joined first.

| Option              | Description                                                                 |
| ------------------- | --------------------------------------------------------------------------- |
| `-s`, `--session`   | Join a specific session by its ID instead of being matched with any player. |
| `--style`           | Board colors: `STANDARD` (default) or `COLORFUL`.                           |

## Playing

- You can only pick up your own pieces, and only on your turn.
- Move a piece by dragging it to a square and releasing it, or by clicking it and then clicking the square. A selected piece shows its legal moves: a dot on an empty square, a ring around a piece it can capture.
- Click the selected piece again, or anywhere off its moves, to deselect it.
- When a pawn reaches the last rank, pick the piece to promote to. Click elsewhere or press Esc to cancel the move.
- When the match ends, the result is shown with a **Play again** button. The rematch starts once both players have pressed it.
