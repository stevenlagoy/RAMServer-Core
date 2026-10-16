# RAMServer Wire Protocol - v1

Schema file: [`ramserver/v1/ramserver.proto`](ramserver/v1/ramserver.proto) (package `ramserver.v1`)

The protocol defines every message exchanged between game clients and the RAMServer core over TCP. It lets clients written in any language connect, authenticate, join a game session, submit moves, and receive state updates and match results. Game-specific moves and state are carried as opaque bytes, so the core stays game-agnostic and only each game's ruleset interprets them.

For the full message reference, field descriptions, and session flow, see [docs/protocol.md](../docs/protocol.md).
