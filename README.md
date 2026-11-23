# ttt: Tic-Tac-Toe Client

`ttt` is a terminal client for playing tic-tac-toe on a RAMServer, written in C. It runs as a full-screen,
colored, keyboard- and mouse-driven interface in any modern terminal, connects to a local server over
TCP or WebSocket and to the hosted server over secure WebSocket, and includes a built-in bot opponent.

The code is in [`clients/tic-tac-toe/`](clients/tic-tac-toe/), and its
[README](clients/tic-tac-toe/README.md) covers everything in detail: controls, options, design, and
tests.

### Creators

This client was created by Hayden Jones and Aiden Mao, as the creation team for the RAMServer project's tic-tac-toe
game, with the code written by Claude Opus 5.5.

**AI use disclosure.** The client's code, tests, and README were written by Claude (Anthropic), working
as an autonomous agent in [Claude Code on the web](https://claude.ai/code). The human creation team set
up its environment, wrote its prompts, reviewed and tested its work, and moved the result into this
branch.

### How the client was made

The client was built the way a third-party developer would build one: without access to RAMServer's
own code.

1. **An external developer's repository.** The team created a separate public repository,
   [jonehayd/RAMServer-tic-tac-toe](https://github.com/jonehayd/RAMServer-tic-tac-toe), containing only
   what an outside developer would get: the protocol documentation and schema, the server's public
   `game` interface, compliance tests for rulesets, and a reproducible build environment. None of
   RAMServer's internal code was included.
2. **An agent working in that repository.** A Claude Code on the web session worked autonomously in
   that repository, in a cloud sandbox with no access to the team's machines. It was instructed to act
   as an external developer and never look for the server's code. The full conversation is shared here:
   [Claude Code session](https://claude.ai/code/cshk_01PDrsJtSvMfaArEQ4cTQdekLRC8tiBNkFt8deA2tvmiPc).
3. **The ruleset first, then the client.** The agent first wrote the tic-tac-toe ruleset in Go, which
   the team reviewed and added to the server. It then wrote this client in C from the protocol
   documentation and the ruleset's README alone, choosing a terminal interface, a bot opponent, and the
   rest of its design itself.
4. **Review and integration.** The team reviewed the code, played it against the real server over TCP
   and WebSocket, and moved it to this branch: the build now downloads the protocol schema from
   RAMServer-Core's `main` branch, the integration test runs against a real server, and a Dockerfile
   was added.

### Games

This client plays tic-tac-toe (`game_id` `tictactoe`): two players, any number of spectators, and
rematches in the same session.

### Technologies

- **Language:** C11.
- **Libraries:** protobuf-c (protocol messages), libwebsockets with OpenSSL (WebSocket and TLS), cmocka
  (unit tests).
- **Tools:** `buf` and `protoc` with `protoc-c` (generating message code from the protocol schema),
  GNU Make, Docker.

## How To Use

### Requirements

- **Any OS (Windows, macOS, Linux):** [Docker](https://docs.docker.com/get-docker/). Docker Desktop on
  Windows and macOS. Nothing else is needed.
- **Building natively instead (Linux or WSL):** the packages and steps in the
  [client README](clients/tic-tac-toe/README.md#building).

### Execution

Build the image once, from the repository root. The commands are the same in PowerShell, cmd, and
Linux/macOS shells:

```bash
docker build -t ttt clients/tic-tac-toe
```

Then run the client. `-it` is required, so it draws in your terminal :
_note: the terminal needs to be resized to fit the board, if it's too small it will show an error_

```bash
docker run --rm -it ttt --server hosted --name Ada
```

### Connection

Choose the server with `--server`. Without it, the client asks.

| Server                                             | Command                                                                         |
| -------------------------------------------------- | ------------------------------------------------------------------------------- |
| The hosted server                                  | `docker run --rm -it ttt --server hosted`                                       |
| A server from RAMServer-Core's `docker compose up` | `docker run --rm -it --network ramserver-core_default ttt --server server:8080` |
| Another server with a published port               | `docker run --rm -it ttt --server host.docker.internal:8080`                    |

After signing in, the lobby offers:

- **M**: quick match, joining an open game or creating one,
- **C**: create a private game, whose session id appears at the top right (press **Y** to copy it),
- **J**: join a game by its session id,
- **B**: browse public games.

Press **R** in the waiting room to show you're ready. The match starts once both players are ready.

### Play

- Place your mark with the arrow keys and Enter, the number keys **1**-**9**, or a mouse click.
- Empty cells show their number while it's your turn, and the status panel says whose turn it is. The
  client won't send a move that can't be legal, and explains why.
- Your opponent's moves appear as soon as the server confirms them, with the newest move marked. An
  event log describes every move, join, and result.

### Disconnection and Shutdown

- **L** leaves the session and returns to the lobby (press twice during a match, since it forfeits).
- **Q** or Ctrl-C leaves the session, disconnects from the server, and quits. The container is removed
  automatically (`--rm`).
- If the connection drops, the client offers to reconnect.

## Features

- **A built-in bot**, playing through the server like any other client: `--bot` plays perfectly, and
  `--bot --casual` makes mistakes. For example, to play against it on the hosted server:
  `docker run --rm -it ttt --server hosted --create`, then
  `docker run --rm -it ttt --server hosted --join <session id> --bot --casual`.
- **Spectators** can watch a match by joining after both players.
- **A running scoreboard** across rematches, filled in from the session's match history.
- **A line-based mode** (`--plain`) for scripts and testing, and options for terminals without color or
  Unicode (`--no-color`, `--ascii`).
