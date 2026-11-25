# RAMServer-Core

RAMServer-Core is a standalone, reusable, authoritative multiplayer server core for turn-based games. Many student and hobbyist multiplayer projects let each client track its own copy of the game state, which makes cheating trivial and forces every new game to provide its own solutions for networking, validation, and synchronization. RAMServer-Core addresses this by keeping the single authoritative game state on the server. Clients submit actions as requests over a documented protocol on TCP. The server validates each request against the current state, applies it only if legal, and sends each participant its permitted view of the new state. Game rules sit behind a game-agnostic interface, so a new game can be added as a plugin without changes to the transport, session, or persistence layers.

**Project:** Reusable Authoritative Multiplayer Server Core (RAMServer-Core)

**Course:** CS 56000 Software Engineering

**Term:** Fall 2026

#### Team:
- Heffelmire, Jacob : Database and Persistence Designer [heffjl03@pfw.edu](mailto:heffjl03@pfw.edu) / [jlheffelmire@gmail.com](mailto:jlheffelmire@gmail.com)
- Jones, Hayden : Protocol Designer and Networking Engineer [jonehm05@pfw.edu](mailto:jonehm05@pfw.edu) / [jonehayhayd@gmail.com](mailto:jonehayhayd@gmail.com)
- LaGoy, Steven : Project Manager and Lead Developer [lagosm01@pfw.edu](mailto:lagosm01@pfw.edu) / [stevenlagoy@gmail.com](mailto:stevenlagoy@gmail.com)
- Mao, Aidan : Testing, Quality, and Release Engineer [maoal01@pfw.edu](mailto:maoal01@pfw.edu) / [aidanmao2005@gmail.com](mailto:aidanmao2005@gmail.com)

## Repository Layout

| Path                                | Contents                                                             |
|-------------------------------------|----------------------------------------------------------------------|
| `proto/`                            | Wire protocol definitions (source of truth)                          |
| `server/cmd/ramserver/`             | Primary server entry point. Run with `go run ./...`                  |
| `server/cmd/leanclient/`            | Lightweight test client implementation. Run with `go run ./...`      |
| `server/internal/`                  | Go server code (`github.com/stevenlagoy/ramserver-core/server`)      |
| `server/game/`                      | Game interface for any ruleset running on RAMServer-Core             |
| `server/rulesets/`                  | Implementations of the Game interface                                |
| `docs/`                             | Team documentation for project management and communication          |
| `scripts/`                          | Developer convenience scripts                                        |

## Prerequisites

Install these before doing anything else

| Tool           | Version | Needed for            |
|----------------|---------|-----------------------|
| Go             | 1.24+   | Server                |
| Docker Desktop | current | Running the server    |
| Git            | 2.40+   | Everything            |
| buf            | 1.4x    | Regenerating protobuf |

Install `buf` (works on all target platforms, requires Go):

```
go install github.com/bufbuild/buf/cmd/buf@latest
```

If `buf` is not found afterward, add Go's bin directory to your Path:

- **Linux / macOS**: `export PATH="$PATH:$(go env GOPATH)/bin"` in your shell profile
- **Windows**: add `%USERPROFILE%\go\bin` to your PATH environment variable

## Getting started

### 1. Clone and enter the repository

**Linux / macOS (bash)**
```bash
git clone https://github.com/stevenlagoy/ramserver-core.git
cd RAMServer-Core
```

### 2. Generate protobuf code

Generated code should not be committed. Run this after cloning and after any change to files under `proto/`. The command is identical on all target platforms:

```
buf generate
```

### 3. Run the server

#### With a Docker container

Identical on all target platforms:

```
docker compose up --build
```

The server listens on `localhost:8080` by default. Stop it with `Ctrl+C` and remove the container with `docker compose down`.

### Without a Docker container

**Linux / macOS (bash)**
```bash
cd server
go run ./cmd/ramserver
```

**Windows (cmd)**
```cmd
cd server
go run .\cmd\ramserver
```

### 4. Run the tests

Identical on all target platforms:

```bash
cd server
go test -race ./...
```

## Convenience scripts

**Linux / macOS (bash)**
```bash
./scripts/dev.sh generate # regenerate protobuf
./scripts/dev.sh test     # run server tests
./scripts/dev.sh up       # build and start the server in Docker
./scripts/leanclients.sh 3 # open 3 leanclients in one tmux window (needs tmux)
```

`leanclients.sh [count] [leanclient flags...]` opens one pane per client, named Player1 to PlayerN, and passes any extra flags (such as `-addr`) to every client. Close them all with `tmux kill-session -t leanclients`. The server allows 4 connections per IP address by default (`MaxConnectionsOneIP`), so more than 4 local clients are refused.

**Windows (PowerShell)**
```powershell
.\scripts\dev.ps1 generate # regenerate protobuf
.\scripts\dev.ps1 test     # run server tests
.\scripts\dev.ps1 up       # build and start the server in Docker
.\scripts\leanclients.ps1  # open 3 leanclients in one Windows Terminal window (needs wt.exe)
```

If PowerShell blocks any script, run:
`Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass`

`leanclients.ps1 [count] [leanclient flags...]` opens one pane per client, named Player1 to PlayerN, and passes any extra flags (such as `-addr`) to every client. The server allows 4 connections per IP address by default (`MaxConnectionsOneIP`), so more than 4 local clients are refused.

## Documentation

- [Wire protocol specification](docs/protocol.md)
- [Contributing guidelines](CONTRIBUTING.md)
- [Project Proposal](docs/PROJECT-PROPOSAL.md)
- [Team Policies](docs/TEAM-POLICIES.md)
- [Requirements Specification](docs/REQUIREMENTS-SPECIFICATION.md)
- [Chess Ruleset](server/rulesets/chess/README.md)
