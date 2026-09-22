# Project Management

## Epics

### 1. Protocol

**Stories:**
- **US-1:** AS A CLIENT DEVELOPER, I want a documented message schema (Protocol Buffers `.proto` files) for all client-server messages, so that I can implement a conforming client without reading server source.
    - **Acceptance:** `.proto` files exist in a shared `/proto/` directory; a markdown doc explains each message type, field, and when it's sent.
- **US-2:** AS A CLIENT DEVELOPER, I want defined message types for connecting, authentication, submitting actions, broadcasting state, handling errors, and disconnecting, so that the full match lifecycle is covered.
    - **Acceptance:** (MVP) `ConnectRequest`, `AuthRequest`/`AuthResponse`, `ActionRequest`, `StateUpdate`, `ErrorResponse`, `MatchResult` are defined with versioned schema.
- **US-3:** AS A PROTOCOL DESIGNER, I want a documented framing/transport convention (length-prefixed messages over TCP), so that partial reads/writes don't corrupt message boundaries.
    - **Acceptance:** Framing spec is written correctly; both reference clients parse a fuzz-tested stream of concatenated messages correctly.

### 2. Server Core

**Stories:**
- **US-4:** AS A PLAYER, I want a submitted action to be rejected if it's illegal in the current game state, so that opponents cannot submit cheating moves.
    - **Acceptance:** The server validates every `ActionRequest` against current state before applying; illegal actions return an `ErrorResponse` and are not applied or broadcasted.
- **US-5:** AS A PLAYER, I want the updated game state to broadcast to all clients immediately after a legal move, so that both sides stay in sync.
    - **Acceptance:** Broadcast latency is under a defined bound (e.g. 200ms on LAN) measured in integration tests; both clients receive identical state.
- **US-6:** AS A DEVELOPER EXTENDING THE SYSTEM, I want the game rules (state representation, legal-move checking, win conditions) implemented behind an interface, so that a new game can be added without touching networking or persistence code.
    - **Acceptance:** A `game` interface (or Go equivalent) with methods like `ApplyAction`, `IsLegal`, `CheckWinCondition`; (MVP) one reference game uses the interface.
- **US-7:** AS A NETWORK ENGINEER, I want the server to manage per-match session state (which clients are in a match, whose turn it is), so that out-of-turn player actions are rejected.
    - **Acceptance:** An out-of-turn action returns a specific error code; a spoofed action claiming to be another player's move is rejectd.
- **US-8:** AS A NETWORK ENGINEER, I want a connection lifecycle (handshake, heartbeat/timeout, graceful disconnect), so that a dropped client does not hang the match indefinitely.
    - **Acceptance:** The server detects a dead connection within a defined timeout and marks the match accordingly (forfeit or paused).

### 3. Reference Game

**Stories:**
- **US-9:** AS A PLAYER, I want to play a full match of the reference game from start to finish against another connected client.
    - **Acceptance:** A full match of a game completes with the correct win/draw detection and both clients receive the final state.
- **US-10:** AS A QA ENGINEER, I want at least one deliberately illegal or spoofed action tested against the reference game's rules, to demonstrate the anti-cheat guarantee.
    - **Acceptance:** A test (manual or automated) submits an illegal move and the server rejects and logs it, documented as part of a final demo.

### 4. Game Clients

**Stories:**
- **US-11:** AS A PLAYER USING THE COMPILED-LANGAUGE CLIENT, I want a simple UI showing the current game state and whose turn it is, so that I can play without needing the raw protocol.
    - **Acceptance:** The client renders state from `StateUpdate` messages; input is disallowed or disabled when it is not the player's turn.
- **US-12:** AS A PLAYER USING THE INTERPRETED-LANGUAGE CLIENT, I want the same gameplay experience as the compiled-language client, so that either client can be used interchangably (beyond visual differences).
    - **Acceptance:** The client is implemented independently against the protocol spec (not by porting code from the other client).
- **US-13:** AS A PLAYER, I want clear feedback when my action is rejected (e.g. with messages like "It is not your turn", "Illegal move"), so that I understand what went wrong.
    - **Acceptance:** `ErrorResponse` messages map to human-readable UI messages in both clients.

### 5. Persistence

**Stories:**
- **US-14:** AS A MATCH REVIEWER, I want the full action log and final result of a completed match persisted, so that matches can be audited or replayed later.
    - **Acceptance:** SQLite schema stores match ID, participants, ordered action log, and result; a completed match can be queried back out.

### 6. Auth / Security
- **US-15:** AS A PLAYER, I want to authenticate before I can submit actions, so that only valid players in a match can affect its state.
    - **Acceptance:** `AuthRequest`/`AuthResponse` exchange happens before any `ActionRequest` is accepted; an unauthenticated action is rejected.

### 7. DevOps / Testing
- **US-16:** AS A TEAM MEMBER, I want CI to run unit, integration, adversarial, and cross-language conformance tests on every PR, so that broken code never reaches `main`.
    - **Acceptance:** GitHub Actions workflow runs on all four tiers; a failing tier blocks merge.
- **US-17:** AS A DEVELOPER, I want a Docker environment definition, so that the server and clients run identically on every teammate's machine (Windows or Linux).
    - **Acceptance:** `docker-compose` brings up the server; setup docs include cmd, PowerShell, and Linux instructions.

## Non-Functional Requirements

- **Portability:** Server and both clients run on both Windows and Linux dev machines.
- **Language Independence:** Client pairs implement the protocol independently, without referencing each other's code.
- **Auditability:** Every action, legal or rejected, is logged.