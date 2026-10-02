# Requirements Specification

The name of the system is RAMServer-Core, a standalone, game-agnostic, authoritative multiplayer server core. 

## System Scope

### Core Features

These features are required for the minimum viable product:

- Authoritative server core with a game-agnostic state/validation interface.
- Client-to-server action submission and server-to-client state broadcast over a documented wire protocol.
- Two reference games implemented against the interface. Chess clients will be implemented independently in Java and Python. Go Fish clients will be implemented independently in two other languages (Lua, JavaScript, TypeScript, and C++ are options).
- Four independently-written clients in different languages (some compiled and some interpreted) connecting to the same server.
- Persistence of sessions, matches, client records, and action logs in an embedded SQLite database.
- Session management with sequential rematches. Clients join a lobby and can play several matches of a game with other clients during one session.

### Optional Features

These features are considered stretch goals which will be attempted after core requirements are implemented and tested:

- A third game implemented primarily by an agentic LLM demonstrating game agnosticism of the core and the ability for an external developer to implement a compliant client based on the protocol and server interfaces. Tic-Tac-Toe has been suggested as this final reference game.
- Reconnection/session-resume support following dropped client connections
- Scalability to more than 10 active sessions for 20 users
- Deployment to a cloud-hosted instance for a real network demonstration (non-LAN)

### Out-of-Scope

These features are not planned for the system, and will not be considered for system design:

- Art or assets beyond what is needed to clearly demonstrate gameplay
- Support for intensive, real-time games (like first person shooters [FPS], racing games, or sports games)
- Matchmaking systems; users will be able to select their own lobby to join but will not be assigned one by the system

## Stakeholders

### Development Team

The development team consists of the four software engineers implementing the server and its test clients. Team members have roles including Project Manager, Protocol Designer, Network Engineer, and Quality and Release Engineer. All team members will develop both the server and two game clients.

### Players

Players are people who interact with a game client connected to RAMServer-Core to play matches of turn-based games against one or more other players. This is the primary user in the system design. Players expect that neither side is able to cheat by falsifying game state or accessing hidden information.

### External Developers

Developers who want to build a new turn-based game or a client using an existing, tested authoritative server core rather than rebuilding networking and anti-cheat logic from scratch. This is considered a secondary user in the system design.

### Match Reviewers

People who audit and replay persisted matches. These could include game analyzers, tournament commentators, or other systems.

### Server Operator

A system which runs, configures, and stops the server. These include Docker environments and cloud hosting environments.

### Course Instructor

Person or people who assesses deliverables and the final project demonstration. They are concerned with the visible evidence of anti-cheat and protocol independence guarantees.

## Requirements

### Functional Requirements

### System Level

- **FR-4:** The system shall expose a documented wire protocol such that a conforming client written in any language can connect and play a full match.
- **FR-5:** The system shall define a game-agnostic interface such that a new game can be added without modifying the transport, session, or persistence layers.

#### Connection and Session

- **FR-8:** The system shall exchange a protocol version during the handshake and reject incompatable versions with an `ErrorResponse`.
- **FR-9:** The system shall allow an authenticated client to create a session for a named game and to list and join open sessions by ID.
- **FR-10:** The system shall enforce each game's participant limits and roles (e.g. player, spectator) and reject joins that exceed limits.
- **FR-11:** The system shall bind each participant to a server-issues identity and shall derive the acting player from that identity, never from fields in the message.
- **FR-12:** The system shall reject and log any message that is invalid for the connection's current state (e.g. `ActionRequest` before authentication). Connection states (Connected, Authenticated, InSession, InMatch) and the messages valid in each are defined in `docs/protocol.md`.
- **FR-36:** The system shall let a client list registered games and retrieve their metadata.

#### Match Play

- **FR-1:** The system shall validate every player action against the current authoritative game state before applying it.
- **FR-2:** The system shall reject and log any client-submitted action that is illegal given the current game state.
- **FR-3:** The system shall send, within the latency bound in NFR-1, the updated state to all participants of the match after a validated action is applied, each receiving the view permitted by FR-14.
- **FR-7:** The system shall authenticate a connecting client before allowing it to submit actions for a match.
- **FR-13:** The system shall start a match when the game's start conditions are met (including necessary roles and player readiness indivations) and shall send each participant its role, initial state, and current turn when the match begins.
- **FR-14:** The system shall send each participant only the state that participant is permitted to see, and shall never transmit hidden state (such as a player's hand or randomness seed) to clients who should not see that information.
- **FR-15:** The system shall enforce turn order using the game's turn information and return a distinct error code for out-of-turn actions.
- **FR-16:** The system shall number actions and state updates sequentially and reject and log duplicate, replayed, or stale actions.
- **FR-17:** The system shall perform all randomness (e.g. shuffling) on the server and record the seed so a match can be replayed. The seed shall not be sent to player clients before the match ends.
- **FR-18:** The system shall send a `MatchResult` (winner, draw, scores, termination reason) to all participants when the game reports a terminal state.
- **FR-19:** The system shall support game-defined non-move actions such as resignation and draw offers where the game declares them.
- **FR-33:** After a match ends, the session shall return to a between-matches staet in which participants may start another match of the same game without reconnecting.
- **FR-34:** The system shall allow participants to leave a session, and shall close a session that has no participants or has been idle longer than a configured limit.
- **FR-35:** The system shall start a match only when the game's start conditions are met, including the required number of participants who have confirmed their readiness. 

#### Failure Handling

- **FR-20:** The system shall detect a dead connection through heartbeats within the timeout defined in NFRs and apply the disconnect policy declared by the game: forfeit, pause, fallback.
- **FR-21:** The system shall notify remaining participants of a leave, disconnect, or forfeit.
- **FR-22:** The system shall reject and log malformed, oversized, and unknown-type frames without terminating the server, and shall close a connection that repeatedly sends them.
- **FR-23:** The system shall return an `ErrorResponse` carrying a stable error code and human-readable text (defined in `docs/protocol.md`).

#### Game Plugin Interface

- **FR-24:** The system shall register games by identifier and select one at session creation, so adding a game requires only a new implementation and its registration.
- **FR-25:** A game implementation shall declare its metadata (name, version, min/max participants, role, game start policy, disconnect policy) and shall be deterministic given the same state, action, and seed.

#### Persistence and Auditability

- **FR-6:** The system shall persist each completed or terminated match with its session and match IDs, game, participants, ordered action log, seed, result, sequence number, and termination reason.
- **FR-26:** The system shall log every received action in the structured log, accepted or rejected, with timestamp, identity, session and match IDs, and rejection reason. Accepted actions are stored in the persisted action log.
- **FR-28:** The system shall allow a persisted match and its randomness seed to be retrieved by ID through a protocol request and replayed through the game implementation to reproduce its final state.

#### Operations

- **FR-29:** The system shall read its configuration (listen address, timeouts, database path, log level) from environment variables.
- **FR-30:** The system shall shut down gracefully, finishing in-flight persistence and notifying connected clients.

#### Clients

- **FR-31:** Each client shall display the current state, the player's role, and whose turn it is; disable input out-of-turn; show a readable message for every `ErrorResponse`; and show the final result.
- **FR-32:** The system shall assign each client a stable client ID that persists across connections and matches.
- **FR-37:** Clients shall allow players to enter a server address, choose a display name, create or join a session, and confirm their readiness to begin a match.

### Non-Functional Requirements

#### Performance

- **NFR-1:** Server-to-client broadcast latency is at most 200 ms at the 95th percentile on a LAN (measured in integration tests).
- **NFR-2:** Action validation and application completes within 50 ms for Chess and Go Fish.

#### Capacity

- **NFR-3:** MVP supports at least 2 concurrent sessions with 4 clients; the stretch target is 10 sessions and 20 clients.

#### Reliability

- **NFR-4:** A dead connection is detected within 15 seconds (e.g. 5 s heartbeat, 3 missed)
- **NFR-5:** Malformed input never crashes the server; verified by a fuzz test of the frame parser.
- **NFR-6:** A match's persistence write is atomic: a crash leaves either the full record or none.
- **NFR-7:** Matches in progress are lost at server restart unless the reconnection stretch goal is implemented.
- **NFR-24:** Sessions are isolated: an error or disconnect in one session does not affect any other session.

#### Security

- **NFR-8:** The server trusts no client-supplied identity or state; all authority derives from the server-held state.
- **NFR-9:** Frame size is capped (e.g. 64 KiB) and per-connection message rate is limited.
- **NFR-10:** Traffic is unencrypted TCP in the MVP; TLS with encryption is required before any cloud deployment.
- **NFR-11:**  No credentials or secrets are committed; containers run as non-root.

#### Portability

- **NFR-12:** The server and all clients run identically on Windows and Linux.
- **NFR-13:** One `docker compose up` command starts the server on Windows or Linux. Only the server is containerized and clients run natively.

#### Extensibility

- **NFR-14:** Adding a game changes no code in the transport, session, or persistence packages. Verify this by adding reference games without changes to internal packages.

#### Interoperability

- **NFR-15:** THe protocol is versioned; `buf breaking` runs in CI; breaking changes are announced before the PR.

#### Language Independence

- **NFR-16:** Client pairs implement the protocol without referencing each other's code; each client passes the conformance suite.

#### Maintainability

- **NFR-17:** `gofmt`, `go vet`, `golangci-lint`, and `buf lint` pass in CI; at least 80% coverage on validation and state-transition code.
- **NFR-23:** The server runs identically on every teammate's development machine.

#### Testability

- **NFR-18:** All randomness is seeded so tests are deterministic.

#### Auditability

- **NFR-19:** Every action, legal or rejected, appears in a structured log with timestamp and identity.

#### Usability

- **NFR-20:** A new player can connect, join, and make a first move without reading protocol documentation.

#### Documentation

- **NFR-21:** `docs/protocol.md` is sufficient to write a conforming client without reading server source, and is updated in the same PR as any protocol change.

#### Privacy

- **NFR-22:** Only gameplay data, client IDs, and display name are stored. Sensitive user data are not collected or stored by the server.

#### Footprint

- **NFR-25:** The database is a single file; an average match record is under 10 KB; retention is configurable.

## Use Cases

- **UC-1:** Connect and authenticate
    - **Actors:** Player (primary); Server (system)
    - **Preconditions:** Server is running; client knows the server's address.
    - **Actions:** Client opens a TCP connection and sends `ConnectRequest`.
    - **Main Flow:**
        1. Client sends `ConnectRequest` with its protocol version.
        2. Server confirms the version is compatable and replies.
        3. Client sends `AuthRequest` with its credential and display name.
        4. Server verifies the request, assigns or looks up the client ID, and replies `AuthResponse`.
        5. The connection becomes Authenticated and bound to the client ID.
    - **Alternate Flows:**
        - 2a. Version mismatch: server returns `ErrorResponse` 'version mismatch' and closes the connection.
        - 4a. Invalid credential: server returns `ErrorResponse` 'not authenticated', logs it, and closes the connection after repeated failures.
        - 3a. Any session or action message before authentication is rejected and logged.
    - **Postconditions:** An authenticated connection exists with no session membership.
    - **References:** FR-7, FR-8, FR-11, FR-12, FR-32, US-2, US-15
- **UC-2:** Create or join a session
    - **Actors:** Player (primary); Server (system)
    - **Preconditions:** Connection is authenticated and not in a session.
    - **Actions:** Player creates a session for a game or chooses an open session.
    - **Main Flow:**
        1. Client requests the game list and open sessions.
        2. Client sends a create request naming a game; server checks the game is registered, creates the session in Open state, adds the creator, and returns the session ID. Alternatively, the client sends a join request; server checks the session is open and below the participant limit, adds the player, and notifies existing participants.
        3. When the game's start conditions are met, the match starts.
    - **Alternate Flows:**
        - 2a. Unknown game or session: `ErrorResponse` 'unknown session'.
        - 2b. Session full or already in a match: `ErrorResponse` 'session full'.
        - 2c. Client already in a session: server rejects the request.
    - **Postconditions:** Participant appears in the session and others are notified.
    - **Referenes:** FR-9, FR-10, FR-13, FR-23, FR-24, FR-36
- **UC-3:** Submit an action
    - **Actors:** Player (primary); Server (system)
    - **Preconditions:** Player is authenticated, is a participant in an active match, and it is the player's turn.
    - **Actions:** Player performs a move in the client.
    - **Main Flow:**
        1. Client sends `ActionRequest`.
        2. Server confirms the connection is authenticated and bound to a participant in this match.
        3. Server confirms it is that participant's turn.
        4. Server asks the game to validate the action against the current state.
        5. Server applies the action and appends it to the action log.
        6. Server sends each participant its permitted view of the new state.
        7. If the game reports a terminal state, server follows UC-6.
    - **Alternate Flows:**
        - 2a. Not authenticated: server returns `ErrorResponse` 'not authenticated'; nothing is applied.
        - 3a. Out of turn: server returns `ErrorResponse` 'not your turn' and logs the attempt.
        - 3b. Duplicate or stale sequence number: server rejects and logs stale requests.
        - 4a. Illegal action: server returns `ErrorResponse` 'illegal action', logs it, and applies nothing.
        - 2b. Message claims another player's identity: server ignores the claim, rejects, and logs it.
    - **Postconditions:** State changes only if the action was legal; every attempt is logged.
    - **References:** FR-1, FR-2, FR-3, FR-11, FR-14, FR-15, FR-16, FR-26, US-4, US-5, US-7, US-10, US-13
- **UC-4:** Handle a rejected action
    - **Actors:** Player (primary); Client (system)
    - **Preconditions:** Player submitted an action and the server rejected it.
    - **Actions:** Client receives an `ErrorResponse`.
    - **Main Flow:**
        1. Client reads the error code and text.
        2. Client maps the code to a readable message.
        3. Client displays the message and leaves the displayed state unchanged.
        3. Client re-enables input if it is still the player's turn.
    - **Alternate Flows:**
        - 2a. Unknown code: client shows a generic message with the code.
        - 3a. Code is 'not authenticated' or 'unknown session': client returns to the appropriate screen.
    - **Postconditions:** State is unchanged and the player can try again.
    - **References:** FR-23, FR-31, US-13
- **UC-5:** Handle a dead connection
    - **Actors:** Server (system); Opponent (secondary)
    - **Preconditions:** A match is in progress and a participant's connection stops responding.
    - **Main Flow:**
        1. Server misses the configured number of heartbeats.
        2. Server marks the participant disconnected.
        3. Server applies the configured disconnect policy declared by the game.
        4. Server notifies the remaining participants.
        5. On forfeit, server records termination reason 'forfeit' and it persists the match.
    - **Alternate Flow:**
        - 3a. Participant leaves gracefully before the timeout; server skips detection and handles it as a leave.
        - 3b. Pause: The server waits for a disconnected client. If the client returns, play resumes, otherwise the fallback outcome applies after a pause limit.
    - **Postconditions:** The match is not left hanging.
    - **References:** FR-20, FR-21, FR-27, US-8
- **UC-6:** Complete a match and persist it
    - **Actors:** Server (system); Players (secondary)
    - **Preconditions:** A match is in progress
    - **Actions:** The game reports a terminal state, or a forfeit or termination occurs.
    - **Main Flow:**
        1. Server obtains the result (winner, draw, scores, termination reason) from the game.
        2. Server stops accepting actions for the match.
        3. Server sends `MatchResult` to all participants.
        4. Server writes the match record (IDs, game, participants, seed, ordered accepted actions, result, reason, timestamps) in one transaction.
        5. Session moves to the between-matches state.
    - **Alternate Flows:**
        - 4a. Write fails: server logs the error, retries, and flags the match unpersisted. Because writes are atomic, no partial record remains.
    - **Postconditions:** The match is stored atomically and the session can start a rematch.
    - **References:** FR-6, FR-18, FR-27, NFR-6, US-9, US-14
- **UC-7:** Review or replay a completed match
    - **Actors:** Match Reviewer (primary)
    - **Preconditions:** The match is persisted and the reviewer has the retrieval interface.
    - **Actions:** Reviewer requests a match by ID.
    - **Main Flow:**
        1. Reviewer supplies the match ID.
        2. System retrieves the record.
        3. System shows participants, result, and the ordered action log.
        4. On a replay request, system re-applies each action through the game from the recorded seed.
        5. System reports whether the replayed final state equals the recorded result.
    - **Alternate Flows:** 
        - 2a. Unknown ID: 'not found'
        - 4a. Replay diverges: system reports the first differing action, which indicates a non-deterministic game.
    - **Postconditions:** Stored data is unchanged.
    - **References:** FR-17, FR-25, FR-28, US-14
- **UC-8:** Add a new game
    - **Actors:** External Developer (primary)
    - **Preconditions:** Developer has the protocol documentation and the game interface.
    - **Actions:** Developer implements and registers a game.
    - **Main Flow:**
        1. Developer defines the game's state and action encodings per the protocol docs.
        2. Developer implements the interface, including metadata, per-recipient views, seeded randomness, and the disconnect policy.
        3. Developer registers the game under and identifier.
        4. Developer runs the game's unit tests and the replay check.
        5. The game appears in the game list and clients can create sessions for it.
    - **Alternate Flows:**
        - 2a. Interface not fully implemented: the build fails
        - 4a. Replay diverges: the game is non-deterministic and must be fixed.
    - **Postconditions:** The game is playable with no changes to transport, session, or persistence code.
    - **References:** FR-5, FR-24, FR-25, FR-36, NFR-14, US-6
- **UC-9:** Start a rematch
    - **Actors:** Players
    - **Preconditions:** Session is between matches and the participants are still connected.
    - **Actions:** A participant requests another match.
    - **Main Flow:**
        1. Server notifies the other participants.
        2. Each participant confirms readiness.
        3. Server creates a new match with the next sequence number, a new seed, and roles rotated per the game's rules.
        4. Server sends each participant its initial permitted view.
    - **Alternate Flows:**
        - 2a. A participant declines, leaves, or does not confirm in time: request is cancelled and the session stays in the between matches state.
        - 2b. Participants fall below the minimum: session returns to Open and waits for players.
    - **Postconditions:** A new match runs in the same session; the previous record is unchanged.
    - **References:** FR-13, FR-27, FR-33, FR-35
- **UC-10:** Leave a session
    - **Actors:** Player
    - **Preconditions:** Player is in a session.
    - **Actions:** Player sends a leave request.
    - **Main Flow:**
        1. Server removes the participant.
        2. If a match is in progress, server applies the game's disconnect policy for a departing player.
        3. Server notifies the remaining participants.
        4. If no participants remain, server closes the session.
    - **Postconditions:** Membership is updated and no match is left hanging.
    - **References:** FR-20, FR-21, FR-34
- **UC-11:** Reconnect to a match
    - **Actors:** Player
    - **Preconditions:** Player's connection dropped and the match is paused.
    - **Actions:** Client reconnects with its client ID.
    - **Main Flow:**
        1. Client authenticates.
        2. Server finds the participant slot by client ID and binds the new connection.
        3. Server sends the current permitted view and turn.
        4. Server notifies the others and resumes the match.
    - **Alternate Flows:**
        - 2a. Pause limit expired: the match has already ended; server reports the result.
    - **Postconditions:** Player is back in the match.
    - **References:** FR-11, FR-20, FR-32
- **UC-12:** Handle hostile or malformed input
    - **Actors:** QA Engineer (primary); Malicious client (secondary)
    - **Preconditions:** Server is running; test client can send arbitrary bytes.
    - **Actions:** Test client sends an oversized frame, malformed protobuf, unknown type, replayed sequence number, or an action claiming another player's identity.
    - **Main Flow:**
        1. Server rejects each with the matching ErrorResponse where the connection state allows it.
        2. Server logs the identity and reason.
        3. After the configured threshold, server closes the connection.
        4. Other sessions are unaffected.
    - **Postconditions:** Server is stable, state is unchanged, log entries exist.
    - **References:** FR-2, FR-11, FR-16, FR-22, FR-26, NFR-5, NFR-8, NFR-9, US-10
- **UC-13:** Verify client conformance
    - **Actors:** Client Developer (primary); CI (system)
    - **Preconditions:** A client build and a server build exist.
    - **Actions:** Developer or CI runs the conformance suite.
    - **Main Flow:**
        1. Suite connects the client to the server.
        2. Client completes a scripted full match including an illegal and an out-of-turn attempt.
        3. Suite compares final state and error handling to expectations.
        4. CI reports pass or fail per client.
    - **Postconditions:** A failing client blocks merge.
    - **References:** NFR-16, US-3, US-12, US-16
- **UC-14:** Configure, run, and stop the server
    - **Actors:** Client Operator (primary); Connected clients (secondary)
    - **Preconditions:**  A server image or binary has been built. The host provides a writable location for the SQLite file.
    - **Actions:** The operator starts the server.
    - **Main Flow:**
        1. Operator supplies configuration through environment variables: listen address, heartbeat and idle timeouts, database path, log level, and match retention.
        2. Operator starts the server.
        3. Server reads the environment, applies defaults for unset values, and validates every value.
        4. Server opens the SQLite file, creating it and applying the schema if it does not exist.
        5. Server registers the available games, begins listening on the configured address, and logs a startup message that lists the effective configuration without secrets.
        6. Server accepts client connections (UC-1) until a stop is requested.
        7. Operator requests a stop (SIGINT or SIGTERM, `Ctrl+C`, or `docker compose down`).
        8. Server stops accepting new connections and sends each connected client a shutdown notice.
        9. Server terminates every in-progress match with termination reason server shutdown, completes all in-flight persistence writes, and closes the database.
        10. Server logs a shutdown summary and exits with status 0.
    **Alternate Flows:**
        - 3a. Invalid or unparseable configuration value: server logs which variable is invalid, does not start listening, and exits with a nonzero status.
        - 4a. Database path is missing or not writable: server logs the error and exits with a nonzero status.
        - 5a. Listen address is already in use: server logs the error and exits with a nonzero status.
        - 9a. Shutdown exceeds the configured grace period: server logs the matches left unpersisted and exits with a nonzero status. Writes are atomic, so no partial record remains (NFR-6).
        - 7a. Operator sends a second stop signal during shutdown: server exits immediately, with the same atomicity guarantee as 9a.
    **Postconditions:** After startup, the server is listening with a valid configuration and an open database. After shutdown, no connections remain, every persisted match record is complete, and the database file is closed cleanly.
    **References:** FR-6, FR-29, FR-30, NFR-6, NFR-11, NFR-12, NFR-13, NFR-25, US-17

## User Stories

### Epic 1. Protocol

**Stories:**
- **US-1:** AS A CLIENT DEVELOPER, I want a documented message schema (Protocol Buffers `.proto` files) for all client-server messages, so that I can implement a conforming client without reading server source.
    - **Acceptance:** `.proto` files exist in a shared `/proto/` directory; a markdown doc explains each message type, field, and when it's sent.
- **US-2:** AS A CLIENT DEVELOPER, I want defined message types for connecting, authentication, submitting actions, broadcasting state, handling errors, and disconnecting, so that the full match lifecycle is covered.
    - **Acceptance:** (MVP) `ConnectRequest`, `AuthRequest`/`AuthResponse`, `ActionRequest`, `StateUpdate`, `ErrorResponse`, `MatchResult` are defined with versioned schema.
- **US-3:** AS A PROTOCOL DESIGNER, I want a documented framing/transport convention (length-prefixed messages over TCP), so that partial reads/writes don't corrupt message boundaries.
    - **Acceptance:** Framing spec is written correctly; all reference clients parse a fuzz-tested stream of concatenated messages correctly.

### Epic 2. Server Core

**Stories:**
- **US-4:** AS A PLAYER, I want a submitted action to be rejected if it's illegal in the current game state, so that other players cannot submit cheating moves.
    - **Acceptance:** The server validates every `ActionRequest` against current state before applying; illegal actions return an `ErrorResponse` and are not applied or broadcasted.
- **US-5:** AS A PLAYER, I want the updated game state to broadcast to all clients immediately after a legal move, so that all sides stay in sync.
    - **Acceptance:** Broadcast latency is under a defined bound (e.g. 200ms on LAN) measured in integration tests; all clients receive their permitted view of the same authoritative state.
- **US-6:** AS A DEVELOPER EXTENDING THE SYSTEM, I want the game rules (state representation, legal-move checking, win conditions) implemented behind an interface, so that a new game can be added without touching networking or persistence code.
    - **Acceptance:** A `game` interface (or Go equivalent) with methods like `ApplyAction`, `IsLegal`, `CheckWinCondition`; (MVP) two reference games use the interface.
- **US-7:** AS A NETWORK ENGINEER, I want the server to manage per-match session state (which clients are in a match, whose turn it is), so that out-of-turn player actions are rejected.
    - **Acceptance:** An out-of-turn action returns a specific error code; a spoofed action claiming to be another player's move is rejectd.
- **US-8:** AS A NETWORK ENGINEER, I want a connection lifecycle (handshake, heartbeat/timeout, graceful disconnect), so that a dropped client does not hang the match indefinitely.
    - **Acceptance:** The server detects a dead connection within a defined timeout and marks the match according to the game's declared policy.

### Epic 3. Reference Game

**Stories:**
- **US-9:** AS A PLAYER, I want to play a full match of the reference games from start to finish against another connected client.
    - **Acceptance:** A full match of a game completes with the correct win/draw detection and all clients receive the final state.
- **US-10:** AS A QA ENGINEER, I want at least one deliberately illegal or spoofed action tested against the reference games' rules, to demonstrate the anti-cheat guarantee.
    - **Acceptance:** A test (manual or automated) submits an illegal move and the server rejects and logs it, documented as part of a final demo.

### Epic 4. Game Clients

**Stories:**
- **US-11:** AS A PLAYER USING THE COMPILED-LANGAUGE CLIENT, I want a simple UI showing the current game state and whose turn it is, so that I can play without needing the raw protocol.
    - **Acceptance:** The client renders state from `StateUpdate` messages; input is disallowed or disabled when it is not the player's turn.
- **US-12:** AS A PLAYER USING THE INTERPRETED-LANGUAGE CLIENT, I want the same gameplay experience as the compiled-language client, so that either client can be used interchangeably (beyond visual differences).
    - **Acceptance:** The client is implemented independently against the protocol spec (not by porting code from the other client).
- **US-13:** AS A PLAYER, I want clear feedback when my action is rejected (e.g. with messages like "It is not your turn", "Illegal move"), so that I understand what went wrong.
    - **Acceptance:** `ErrorResponse` messages map to human-readable UI messages in all clients.

### Epic 5. Persistence

**Stories:**
- **US-14:** AS A MATCH REVIEWER, I want the full action log and final result of a completed match persisted, so that matches can be audited or replayed later.
    - **Acceptance:** SQLite schema stores match ID, participants, ordered action log, and result; a completed match can be queried back out.

### Epic 6. Auth / Security
- **US-15:** AS A PLAYER, I want to authenticate before I can submit actions, so that only valid players in a match can affect its state.
    - **Acceptance:** `AuthRequest`/`AuthResponse` exchange happens before any `ActionRequest` is accepted; an unauthenticated action is rejected.

### Epic 7. DevOps / Testing
- **US-16:** AS A TEAM MEMBER, I want CI to run unit, integration, adversarial, and cross-language conformance tests on every PR, so that broken code never reaches `main`.
    - **Acceptance:** GitHub Actions workflow runs on all four tiers; a failing tier blocks merge.
- **US-17:** AS A DEVELOPER, I want a Docker environment definition, so that the server and clients run identically on every teammate's machine (Windows or Linux).
    - **Acceptance:** `docker-compose` brings up the server; setup docs include cmd, PowerShell, and Linux instructions.
