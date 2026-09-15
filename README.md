# RAMServer Core

A Reusable Authoritative Multiplayer game server core. Game-agnostic server owns the game state and clients submit actions as requests.

**Project:** Reusable Authoritative Multiplayer Server Core (RAMServer)

**Course:** CS 56000 Software Engineering

**Term:** Fall 2026

#### Team:
- Heffelmire, Jacob : [heffjl03@pfw.edu](mailto:heffjl03@pfw.edu) / [jlheffelmire@gmail.com](mailto:jlheffelmire@gmail.com)
- Jones, Hayden : [jonehm05@pfw.edu](mailto:jonehm05@pfw.edu) / [jonehayhayd@gmail.com](mailto:jonehayhayd@gmail.com)
- LaGoy, Steven : [lagosm01@pfw.edu](mailto:lagosm01@pfw.edu) / [stevenlagoy@gmail.com](mailto:stevenlagoy@gmail.com)
- Mao, Aidan : [maoal01@pfw.edu](mailto:maoal01@pfw.edu) / [aidanmao2005@gmail.com](mailto:aidanmao2005@gmail.com)

## Repository Layout

| Path                | Contents                                    |
|---------------------|---------------------------------------------|
| `proto/`            | Wire protocol definitions (source of truth) |
| `server/`           | Go server code (`github.com/stevenlagoy/ramserver-core/server`) |
| `clients/client-1/` | Compiled-language client                    |
| `clients/client-2/` | Interpreted-language client                 |
| `docs/`             | Protocol specification and design notes     |
| `scripts/`          | Developer convenience scripts               |

## Prerequisites

Install these before doing anything else

| Tool           | Version | Needed for            |
|----------------|---------|-----------------------|
| Go             | 1.24+  | Server                |
| Docker Desktop | current | Running the server    |
| Git            | 2.40+   | Everything            |
| buf            | 1.4x    | Regenerating protobuf |

Install `buf` (works on all three platforms, requires Go):

```
go install github.com/bufbuild/buf/cmd/buf@latest
```

If `buf` is not found afterward, add Go's bin directory to your Path:

- **Linux / maxOS**: `export PATH="$PATH:$(go env GOPATH)/bin"` in your shell profile
- **Windows**: add `%USERPROFILE%\go\bin` to your PATH environment variable

## Getting started

### 1. Clone and enter the repository

**Linux / maxOS (bash)**
```bash
git clone https://github.com/stevenlagoy/ramserver-core.git
cd ramserver-core
```

### 2. Generate protobuf code

Generated code is not committed. Run this after cloning and after any change to files under `proto/`. The command is identical on all platforms:

```
buf generate
```

### 3. Run the server

Identical on all platforms:

```
docker compose up --build
```

The server listens on `localhost:9000`. Stop it with `Ctrl+C` and remove the container with `docker compose down`.

### 4. Run the server without Docker (faster iteration)

**Linux / maxOS (bash)**
```bash
cd server
go run ./cmd/ramserver --addr :9000
```

**Windows (cmd)**
```cmd
cd server
go run .\cmd\ramserver --addr :9000
```

### 5. Run the tests

**Linux / macOS (bash)**
```bash
cd server
go test -race ./...
```

**Windows (PowerShell / cmd)**
```cmd
cd server
go test -race ./...
```

`-race` requires a C toolchain on Windows. If it fails, drop the flag locally; CI runs the race detector on every pull request regardless.

## Convenience scripts

**Linux / macOS (bash)**
```bash
./scripts/dev.sh generate # regenerate protobuf
./scripts/dev.sh test     # run server tests
./scripts/dev.sh up       # build and start the server in Docker
```

**Windows (PowerShell)**
```powershell
.\scripts\dev.ps1 generate
.\scripts\dev.ps1 test
.\scripts\dev.ps1 up
```

If PowerShell blocks the script, run:
`Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass`

## Branches

| Branch     | Owners   | Scope                           |
|------------|----------|---------------------------------|
| `main`     | All four | Protected; requires 4 approvals |
| `server`   | All four | `server/`, `proto/`             |
| `client-1` | Pair TBD | `clients/client-1/`             |
| `client-2` | Pair TBD | `clients/client-2/`             |

Rebase your branch onto `main` before opening a pull request.

## Documentation

- [Wire protocol specification](docs/protocol.md)
- [Contributing guidelines](CONTRIBUTING.md)
