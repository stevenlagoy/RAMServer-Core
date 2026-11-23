# Creating Tic-Tac-Toe with an AI agent

This document records how the tic-tac-toe ruleset and C client were created by an agentic LLM, guided by
the creation team, without the agent having access to RAMServer's internal code. It answers each step and
question of the task, and links to everything the analysis team needs.

## Quick links

| What                                                              | Link                                                                                                                                                                                     |
| ----------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Agent conversation (ruleset and client)                           | [Claude Code session](https://claude.ai/code/cshk_01PDrsJtSvMfaArEQ4cTQdekLRC8tiBNkFt8deA2tvmiPc)                                                                                        |
| The agent's repository (the external developer kit)               | [jonehayd/RAMServer-tic-tac-toe](https://github.com/jonehayd/RAMServer-tic-tac-toe)                                                                                                      |
| The ruleset, as integrated into the server                        | [`server/rulesets/tictactoe`](https://github.com/stevenlagoy/RAMServer-Core/tree/feature/102-tic-tac-toe-ruleset/server/rulesets/tictactoe) on branch `feature/102-tic-tac-toe-ruleset`  |
| The client                                                        | [`clients/tic-tac-toe`](https://github.com/stevenlagoy/RAMServer-Core/tree/clients/C-tic-tac-toe/clients/tic-tac-toe) on branch `clients/C-tic-tac-toe`                                  |
| The ruleset's specification (encodings, rules, errors)            | [Ruleset README](https://github.com/stevenlagoy/RAMServer-Core/blob/feature/102-tic-tac-toe-ruleset/server/rulesets/tictactoe/README.md)                                                 |
| The client's documentation                                        | [Client README](https://github.com/stevenlagoy/RAMServer-Core/blob/clients/C-tic-tac-toe/clients/tic-tac-toe/README.md)                                                                  |
| Documentation given to the agent, as now maintained in the server | [Protocol reference](../proto/README.md), [Writing a ruleset](writing-a-ruleset.md), [Writing a client](writing-a-client.md), [WebSocket transport](websocket.md), [Framing](framing.md) |

## Summary

| Step                    | Result                                                                                                                                                                                           |
| ----------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Agentic tool            | Claude Code on the web, running **Claude Opus 5.5**, in a cloud sandbox attached to one repository.                                                                                              |
| Development environment | A separate public repository containing only what an external developer gets: documentation, the protocol schema, the public `game` interface, compliance tests, and a Docker build environment. |
| Ruleset                 | Go, standard rules, 2 players plus unlimited spectators, one board per match, resignation at any time. All tests pass, including the compliance tests.                                           |
| Client                  | C, full-screen terminal UI, TCP and secure WebSocket, a built-in bot. All tests pass, and complete matches were played against the real server.                                                  |
| Further prompts         | Planned, not yet run (see [Further prompts](#7-further-prompts-for-the-ruleset-and-client)).                                                                                                     |

## 1. Choosing the agentic tool

**Choice: Claude Code on the web** ([claude.ai/code](https://claude.ai/code)), with the model **Claude
Opus 5.5**.

| Requirement                                          | How it is met                                                                                                                                                                                                                                                                               |
| ---------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Conversation-based, shareable by link                | Each session is a conversation stored online and can be shared with a link (see Quick links).                                                                                                                                                                                               |
| Works autonomously in a dedicated repository         | A session is attached to one GitHub repository. It clones it into a cloud sandbox, works through a prompt on its own (reading, writing, building, and running tests), and pushes its work to a new branch for review.                                                                       |
| Limited web access, to avoid leaking the server code | The sandbox has no access to the team's machines, and its network access is configurable. Since RAMServer-Core is a public repository that limited network access may still reach, the agent was also instructed never to look for it, and its conversation can be checked for any attempt. |

## 2. The development environment

A new public repository, [jonehayd/RAMServer-tic-tac-toe](https://github.com/jonehayd/RAMServer-tic-tac-toe),
was created from copied files, not from a fork or clone, so no history of the server came with it.

**What it contains (an external developer's view):**

| Path                                                                      | Contents                                                                                                                                                                                                                                                                                                                                   |
| ------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `AGENTS.md`, `CLAUDE.md`                                                  | The agent's instructions: its role as an external developer, never to look for or read the server's code (on the web, in Go's module cache, or through package managers), and how to work (run all tests, report results honestly, record decisions).                                                                                      |
| `docs/`                                                                   | The protocol reference (version 1.2), framing, WebSocket transport, a guide to writing a ruleset, a guide to writing a client, and a guide to the local and hosted servers. Several of these were written for this kit, since they did not exist in the server yet; they have since been added to the server's own documentation.          |
| `proto/`                                                                  | The protocol schema.                                                                                                                                                                                                                                                                                                                       |
| `sdk/game/`                                                               | The server's public `game` package: the `Game` interface, roles, the registry, and its error values.                                                                                                                                                                                                                                       |
| `sdk/game/gametest/`                                                      | Ruleset compliance tests, written for this project. They play many random matches through the same calls the server makes and check the interface's contract: immutability, legal actions validating, readiness rules, every match ending, and no panics. They are tested against a sample game and 13 deliberately broken versions of it. |
| `go.mod`                                                                  | Points the server's import path at `sdk/`, so the agent's code compiles exactly as it will inside the server, and Go never downloads the real server module. This was checked with downloads disabled.                                                                                                                                     |
| `Dockerfile`, `docker-compose.yml`, `scripts/install-deps.sh`, `Makefile` | A reproducible environment (Go 1.24, a C toolchain, protobuf-c, libwebsockets, OpenSSL, cmocka) and one command, `make test`, that runs every test, so results are the same on every developer's machine.                                                                                                                                  |

**The server facade** (`server/ramserver`) was not included: it imports the server's internal packages,
so it cannot be shipped without them. Instead, the documentation describes the server's behavior, and
running the agent's ruleset in a real server was done by the team, who copied it into RAMServer-Core and
registered it.

**Connecting to the live server.** The hosted server's address and the WebSocket transport are
documented in the kit, so the agent's client could connect to it directly.

**No access to internal code.** The kit contains no internal packages, entry points, other rulesets,
or the leanclient, and every file was checked for references to them before the agent started. Only the
public import path of the `game` package appears.

## 3. The ruleset prompt

The full prompt is recorded in the kit's [README](https://github.com/jonehayd/RAMServer-tic-tac-toe#ruleset-prompt).
It:

- tells the agent it is **an external developer**, and **not to look for, download, or read the server's
  source code**, anywhere,
- asks for a ruleset in Go **implementing `game.Game`**, with **standard rules** on a 3x3 board,
- requires **at least two players**, and leaves spectators and the match format (one game or a series)
  to the agent, with its reasons documented,
- asks for simple action and view encodings, specified precisely enough that a C client could be written
  from the README alone,
- requires **tests** (every way a game ends, illegal and malformed moves, turn order, immutability) and
  the **compliance tests**, and **documentation** (game id, roles, start condition, encodings, annotations,
  termination reasons, and error messages),
- asks for a report of what was built, its decisions, the tests run with their results, and any gaps in
  the documentation.

## 4. Evaluating the ruleset

**Do the tests pass?** Yes. All 38 tests pass in the kit, including the compliance tests, and `gofmt` and
`go vet` are clean. After being copied into RAMServer-Core, its tests pass there with the rest of the
server's tests.

**How many players does one game involve? Are spectators allowed?** Exactly **two players**. The first two
members to join play; X is the first to join and moves first. **Spectators are allowed, without limit**:
anyone who joins after the two players watches, and sees every move.

**One board, or a series?** **One board per match.** The agent chose not to implement a series. Players
play again by readying up for a rematch in the same session, which the server supports for every game. X
is always the first player to have joined, because the `Game` interface doesn't tell a ruleset which
match it is starting, so it can't alternate who moves first.

## 5. The client prompt

The full prompt is recorded in the kit's [README](https://github.com/jonehayd/RAMServer-tic-tac-toe#client-prompt).
It:

- asks for **a client in C** that plays tic-tac-toe, using the ruleset's README for the game's encodings,
  and repeats that the agent must not look for the server's code,
- requires connecting to **a local server over TCP and the hosted server over WebSocket with TLS**,
- requires handling **the whole session**: the handshake, quick match, creating a private session,
  joining by id, **readying up**, taking turns, results, rematches, **leaving a session, and quitting or
  losing the connection** cleanly, including heartbeats and the server's timing rules,
- treats the server as the source of truth, and shows every server error to the player,
- allows **a terminal UI or a GUI library**, and **encourages creative choices** as long as they follow
  the protocol, with the requirement that it builds and passes its tests without a display,
- requires **tests** (framing, every request, decoding views, and session state driven by sequences of
  server messages) and **documentation** (building, running, playing two instances, controls, design,
  and creative choices).

## 6. Evaluating the client

**Do the tests pass?** Yes. All 56 unit tests pass, and the build has no compiler warnings with
`-Wall -Wextra`. The agent's integration test, in which real client processes play complete matches
against a stand-in server it wrote, also passed. On the client branch, the integration test was changed
to run against **the real server** instead, and passes there: three matches with rematches and a
spectator, joining by id, quick match, an out-of-turn move showing the server's error, and resigning.

**Can you connect to a local server and to the hosted server?**

- **Local server: yes**, over both **TCP and WebSocket**. Two instances played complete matches against a
  RAMServer built from the ruleset branch, including rematches, and left cleanly. It also runs in Docker
  and plays against a server running in Docker.
- **Hosted server: it connects, signs in, and starts a session over secure WebSocket**, from the agent's
  sandbox, a local build, and the client's Docker image. Complete matches on the hosted server need the
  tic-tac-toe ruleset to be deployed there (branch `feature/102-tic-tac-toe-ruleset`); until then the
  hosted server answers that the game is unknown.

**Is the system clean and responsive?** Mostly.

- The code is organized so that the protocol, framing, game state, and bot contain no networking or
  terminal code, and the unit tests cover exactly those parts.
- One event loop always keeps reading from the server, even while the player types or reads the help
  screen, and replies to every heartbeat.
- The screen is redrawn only where it changed, so it doesn't flicker.
- Quitting leaves the session cleanly and restores the terminal, and a lost connection is reported
  clearly with an offer to reconnect.

**What creative choices were made?**

- **A full-screen terminal UI rather than a GUI window**, so it runs the same in WSL, in Docker, and over
  SSH, with large box-drawn X and O pieces, colors, and mouse support.
- **Several ways to play**: arrow keys, WASD, vi keys, number keys, or mouse clicks.
- **A built-in bot opponent** that plays through the server like any other client, either perfectly or
  casually.
- **One-key sharing of private games**: the session id is copied to the clipboard.
- **A scoreboard** kept across rematches, and an event log that narrates the session in plain words.
- **No accidental forfeits**: resigning, and leaving during a match, need a second key press.
- **A line-based mode** for scripts and testing, and options for terminals without color or Unicode.

**Changes made by the team when moving the client to its branch.** The client was written for the kit's
layout, so on `clients/C-tic-tac-toe` its build downloads the protocol schema from RAMServer-Core's `main`
branch, its integration test runs against a real server (the stand-in server needed the kit's Go code),
and a Dockerfile was added so it runs anywhere with only Docker installed.

## 7. Further prompts for the ruleset and client

**Status: not yet run.** This section records the server features the ruleset and client don't use, the
features chosen for the next prompts, and the significant expansion.

**Server features not used:**

| Feature                                                  | Ruleset                                     | Client                                                               |
| -------------------------------------------------------- | ------------------------------------------- | -------------------------------------------------------------------- |
| `LegalActionsRequest` (the server's list of legal moves) | provides it                                 | **not used**: the client works out legal moves from the board itself |
| `player_order` in state updates                          | —                                           | **not used**: seats come from the view and annotations               |
| `Result.Scores` in match results                         | **not set**                                 | —                                                                    |
| Alternating the first player between matches             | **not possible** with the current interface | —                                                                    |
| Resuming a dropped connection                            | —                                           | implemented, but **the server doesn't support it yet**               |

**Chosen for the next prompts:**

1. **Client: use `LegalActionsRequest`** to highlight the moves the server says are legal, instead of
   working them out locally, and fall back to its own check if the request fails.
2. **Ruleset and client, the significant expansion: best-of-N series** (for example best of 3 or best
   of 5) within one match. The ruleset tracks the series in its state, alternates who moves first
   between games (which also fixes "X always moves first"), reports scores in `Result.Scores`, and only
   ends the match when the series is decided. The client shows the series score and which game is being
   played.

## 8. Access for the analysis team

- **The ruleset and its tests:** [`server/rulesets/tictactoe`](https://github.com/stevenlagoy/RAMServer-Core/tree/feature/102-tic-tac-toe-ruleset/server/rulesets/tictactoe) on branch `feature/102-tic-tac-toe-ruleset`.
- **The client, its tests, and a Dockerfile:** [`clients/tic-tac-toe`](https://github.com/stevenlagoy/RAMServer-Core/tree/clients/C-tic-tac-toe/clients/tic-tac-toe) on branch `clients/C-tic-tac-toe`. The branch's root README explains how to build and run it with Docker.
- **The agent's repository**, with the full history of its work and the prompts used: [jonehayd/RAMServer-tic-tac-toe](https://github.com/jonehayd/RAMServer-tic-tac-toe).
- **The agent conversation:** [Claude Code session](https://claude.ai/code/cshk_01PDrsJtSvMfaArEQ4cTQdekLRC8tiBNkFt8deA2tvmiPc).
