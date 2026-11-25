# RAMServer-Core

**A reusable, authoritative multiplayer server core for turn-based games.**

RAMServer-Core provides the server-side infrastructure for multiplayer turn-based games. It maintains the authoritative game state, validates player actions through a game ruleset, and communicates state updates to clients through a documented, language-neutral protocol.

Instead of implementing networking, session management, action validation, and synchronization separately for every game, developers can build on the shared server core and implement game-specific rules behind the `game.Game` interface.

> **Release status:** This is the first developer-facing release of RAMServer-Core. The project is intended to grow through subsequent releases, including additional rulesets, client examples, and server capabilities. Consult the release notes and issue tracker for the capabilities included in a particular version.

## What RAMServer-Core provides

- **Authoritative gameplay:** the server owns the game state and decides whether submitted actions are valid.
- **Game extensibility:** rulesets implement a common Go interface rather than modifying the networking and session layers.
- **Language-independent clients:** clients communicate using Protocol Buffers over the documented wire protocol.
- **Session management:** the core coordinates participants, sessions, matches, turns, and state updates.
- **A reference implementation:** Chess demonstrates how a game can be implemented against the ruleset interface.
- **Development and testing support:** StubGame and the lightweight `leanclient` support development and testing.

The server validates actions against the current match state. Clients should render the state they receive from the server; they must not be treated as authoritative sources of game state or move legality.

## Included components

| Component                | Purpose                                   |
|--------------------------|-------------------------------------------|
| `server/`                | Go server module and game API             |
| `server/rulesets/chess/` | Chess ruleset                             |
| `server/rulesets/stub/`  | Stub ruleset for testing and examples     |
| `proto/`                 | Source Protocol Buffers definitions       |
| `docs/protocol.md`       | Client-facing wire protocol specification |
| `server/cmd/leanclient/` | Lightweight command-line test client      |

The set of included games and clients may change between releases. See [Featured Clients](docs/FEATURED-CLIENTS.md) for the maintained client examples.

## Architecture at a glance

A client connects to the server and exchanges protocol messages. The server associates the connection with a server-issued identity, checks session membership and turn constraints, and delegates game-specific validation and state transitions to the selected ruleset. The server then sends each participant the view of the state that the ruleset permits that participant to see.

The public game extension point is `server/game.Game`. The protocol is a separate interface used by clients. A ruleset implementation and a client implementation are therefore different contributions with different review requirements.

## Quick start

### Prerequisites

- Go 1.24 or later
- Docker Desktop (for containerized local operation)
- Git
- [Buf](https://buf.build/) (only needed to regenerate Protocol Buffers code)

Install Buf with Go:

```sh
go install github.com/bufbuild/buf/cmd/buf@latest
```

Ensure `$(go env GOPATH)/bin` (or `%USERPROFILE%\go\bin` on Windows) is on your `PATH`.

### Clone the repository

```sh
git clone https://github.com/stevenlagoy/ramserver-core.git
cd RAMServer-Core
```

### Generate Protocol Buffers code

Generated protobuf code is not committed. Generate it after cloning and after changing files under `proto/`:

```sh
buf generate
```

### Run with Docker

From the repository root:

```sh
docker compose up --build
```

Stop the server with `Ctrl+C`. To stop and remove the Compose containers:

```sh
docker compose down
```

The local server listens on the configured address and port. Check the Compose configuration and [Server Operations](docs/SERVER-OPERATIONS.md) rather than assuming the development port is the same as a hosted deployment.

### Run natively

```sh
cd server
go run ./cmd/ramserver
```

On Windows Command Prompt:

```bat
cd server
go run .\cmd\ramserver
```

### Run tests

```sh
cd server
go test -race ./...
```

See [Contributing](CONTRIBUTING.md) for the expected checks before submitting changes.

## Using RAMServer-Core

Choose the workflow that matches what you want to build:

- **Operate your own server:** fork the repository, maintain your own deployment, and review [Server Operations](docs/SERVER-OPERATIONS.md).
- **Add a game to your own fork:** implement `game.Game` and register the ruleset in your server executable. Start with [Game Development](docs/GAME-DEVELOPMENT.md).
- **Propose a game for the official repository or hosted service:** follow [Game Contribution](docs/GAME-CONTRIBUTION.md). Official acceptance is subject to maintainer review.
- **Build a client:** implement the wire protocol described in [Protocol](docs/protocol.md). Use [Client Development](docs/CLIENT-DEVELOPMENT.md) for the client repository conventions.
- **Propose a core feature or report a bug:** use the GitHub issue templates and read [Contributing](CONTRIBUTING.md).

## Documentation

| Document                                         | Description                                                          |
|--------------------------------------------------|----------------------------------------------------------------------|
| [Contributing](CONTRIBUTING.md)                  | Issues, pull requests, review, and contribution standards            |
| [Server Operations](docs/SERVER-OPERATIONS.md)   | Forking, configuring, deploying, and operating an independent server |
| [Game Development](docs/GAME-DEVELOPMENT.md)     | Implementing and registering a ruleset                               |
| [Game Contribution](docs/GAME-CONTRIBUTION.md)   | Submitting a ruleset to the official project                         |
| [Client Development](docs/CLIENT-DEVELOPMENT.md) | Implementing clients and following the client repository pattern     |
| [Featured Clients](docs/FEATURED-CLIENTS.md)     | Criteria and process for official example clients                    |
| [Wire Protocol](docs/protocol.md)                | Protocol messages, framing, and client/server behavior               |
| [Security](docs/SECURITY.md)                     | Security boundaries and vulnerability reporting                      |
| [Team Policies](docs/TEAM-POLICIES.md)           | Internal team working agreements                                     |

## Project status and roadmap

RAMServer-Core is designed to support additional turn-based games and independently implemented clients. The initial release establishes the reusable server core, its ruleset interface, the protocol, and reference implementations. Future releases may add more games, client examples, operational capabilities, and protocol improvements.

A feature being discussed or present on an unreleased branch should not be assumed to be part of the current release. Check the [GitHub releases](https://github.com/stevenlagoy/ramserver-core/releases), repository tags, and issue tracker for current status.

## Contributing

Contributions are welcome through the project's documented review process. Open an issue before beginning substantial work. New rulesets and client examples have dedicated submission requirements; see [Contributing](CONTRIBUTING.md).

## Project identity and attribution

RAMServer-Core is the project name and brand. Forks and derivative projects must identify their own maintainers and deployments clearly and must not imply that they are operated, endorsed, or supported by the RAMServer maintainers. Please retain existing RAMServer attribution and branding assets when redistributing project materials, subject to the repository's license and any applicable asset-specific terms.

See [Server Operations](docs/SERVER-OPERATIONS.md) for the expected attribution and responsibility boundaries.

## License

The applicable license and third-party notices are provided in [LICENSE](LICENSE) and any accompanying notices. If no license file is present in a particular distribution, do not assume that the source code is licensed for unrestricted reuse. The project maintainers should select and publish an explicit license before announcing the release.
