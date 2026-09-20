# RAMServer Wire Protocol — v1

Schema file: `ramserver.proto` (package `ramserver.v1`)

## Framing

Every message on the wire is a length-prefixed, serialized `ClientMessage`
(client → server) or `ServerMessage` (server → client). Both are envelope
messages — an actual message never appears on the wire on its own, only
wrapped in one of these two `oneof`s.

- `ClientMessage.sequence` — a client-assigned, monotonically increasing
  counter. Used to correlate a response to the request that caused it.
- `ServerMessage.in_reply_to` — echoes the `sequence` of the client message
  this is a direct response to. For unsolicited server pushes (broadcasts to
  everyone in a lobby or match, not triggered by this client's own request),
  this is `0`.

## Message lifecycle

A client moves through four phases in order: **connect → authenticate →
lobby → match**. Each phase's messages are only valid once the previous
phase has completed.

### 1. Connect

| Message           | Direction       | Sent when                                                              |
| ----------------- | --------------- | ---------------------------------------------------------------------- |
| `ConnectRequest`  | client → server | Immediately after the TCP connection opens; must be the first message. |
| `ConnectResponse` | server → client | In reply to `ConnectRequest`.                                          |

**`ConnectRequest`**

- `protocol_version` — the schema version the client was built against. The
  server compares this against its own version and rejects a mismatch.
- `client_name` — free-text identifier for logging/debugging (e.g.
  `"ramserver-java-client"`).

**`ConnectResponse`**

- `accepted` — `false` if the protocol version is incompatible. If `false`,
  an `ErrorResponse` (`ERROR_CODE_INVALID_PROTOCOL_VERSION`) follows and the
  server closes the connection.
- `session_id` — server-assigned identifier for this TCP session.
- `server_protocol_version` — the server's schema version, for client-side logging.

### 2. Authenticate

| Message        | Direction       | Sent when                             |
| -------------- | --------------- | ------------------------------------- |
| `AuthRequest`  | client → server | After a successful `ConnectResponse`. |
| `AuthResponse` | server → client | In reply to `AuthRequest`.            |

**Identity vs. display name — how impersonation is prevented**

`display_name` is cosmetic only: shown to other players, allowed to collide,
never used to establish who someone is. Identity is handled separately:

- **New player:** send `AuthRequest` with just `display_name` (leave
  `player_id` and `reconnect_token` empty). The server generates a new
  `player_id` and a secret `reconnect_token`, returned only to that client in
  `AuthResponse`.
- **Resuming player** (e.g. after a dropped connection): send `AuthRequest`
  with the previously issued `player_id` **and** `reconnect_token`. The
  server only grants that identity back if the token matches what it issued
  — a client that only knows another player's `player_id` (which is visible
  to opponents in lobby/match broadcasts) cannot assume that identity without
  also knowing the secret token (which is never broadcast to anyone).
- The token is never included in any broadcast message (`LobbyUpdate`,
  `StateUpdate`, `MatchResult`) — only ever sent once, directly to its owner,
  in `AuthResponse`.
- Once authenticated, every later message on that connection is attributed
  to that `player_id` by the server itself — no later message (including
  `ActionRequest`) carries a client-supplied player identity field, so a
  connection can never act as a different player mid-session.

**`AuthRequest`**

- `display_name` — cosmetic label shown to other players.
- `player_id` — identity to resume, if any; empty to register new.
- `reconnect_token` — secret proving ownership of `player_id`; empty to register new.

**`AuthResponse`**

- `authenticated` — `false` on failure (`ERROR_CODE_AUTH_FAILED`): unknown
  `player_id`, or a `reconnect_token` that doesn't match.
- `player_id` — the newly issued or resumed identity.
- `reconnect_token` — secret for this identity. Store it client-side only
  (e.g. a local config file) if you want to support resuming later.

### 3. Lobby

A lobby holds players waiting for a specific game before a match starts. The
server enforces each game's minimum and maximum player count — the client
never decides this; it only reads the values the server reports. The first
player to join a lobby becomes its **host**.

| Message             | Direction                                               | Sent when                                                                                                          |
| ------------------- | ------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| `JoinLobbyRequest`  | client → server                                         | Player wants to join or create a lobby for a game. Only accepted while the lobby's status is `WAITING` or `READY`. |
| `LeaveLobbyRequest` | client → server                                         | Player leaves a lobby before the match starts.                                                                     |
| `StartGameRequest`  | client → server                                         | The host manually starts the match once `min_players` is met.                                                      |
| `LobbyUpdate`       | server → client (broadcast to all players in the lobby) | Whenever lobby membership or status changes, including the transition into a match.                                |

**When a match actually starts** — two paths, both ending in
`LOBBY_STATUS_STARTING`:

1. **Host-triggered:** once the lobby reaches `min_players` (status
   `READY`), the host sends `StartGameRequest`.
2. **Auto-start on full:** if the lobby reaches `max_players` at any point,
   it starts automatically — no `StartGameRequest` needed.

**`JoinLobbyRequest`**

- `game_id` — `"tictactoe"`, `"chess"`, or `"gofish"`.
- `lobby_id` — leave empty to join any open lobby for `game_id`, or to create
  a new one if none is open.
- Rejected with `ERROR_CODE_LOBBY_ALREADY_STARTED` if the target lobby's
  status is already `STARTING` or `CLOSED` — joining mid-match or
  post-match is never allowed.

**`StartGameRequest`**

- `lobby_id` — the lobby to start.
- Rejected with `ERROR_CODE_NOT_LOBBY_HOST` if the sender isn't the host.
- Rejected with `ERROR_CODE_LOBBY_NOT_READY` if `min_players` hasn't been met yet.

**`LobbyUpdate`**

- `players` — current lobby roster (`player_id`, `display_name` each).
- `host_player_id` — the one player allowed to send `StartGameRequest`; a
  client uses this to decide whether to show a "Start Game" button.
- `min_players` / `max_players` — the constraints for `game_id`, as declared
  by that game's registered ruleset on the server (e.g. chess: 2/2, Go Fish: 2/4).
- `status` — see `LobbyStatus` below.
- `match_id` — populated once `status == LOBBY_STATUS_STARTING`; this is the
  id to use in all subsequent `ActionRequest` messages for the match.

**`LobbyStatus`**

- `LOBBY_STATUS_WAITING` — below `min_players`.
- `LOBBY_STATUS_READY` — `min_players` met, below `max_players`; the host may
  now send `StartGameRequest`.
- `LOBBY_STATUS_STARTING` — match initializing, triggered by the host or by
  reaching `max_players`. A `StateUpdate` with the initial game state follows
  shortly.
- `LOBBY_STATUS_CLOSED` — abandoned before starting, or superseded once the
  match begins.

### 4. Match

| Message         | Direction                                               | Sent when                                                        |
| --------------- | ------------------------------------------------------- | ---------------------------------------------------------------- |
| `ActionRequest` | client → server                                         | Player submits a move.                                           |
| `StateUpdate`   | server → client (broadcast to all players in the match) | After every validated action, and once at match start.           |
| `ErrorResponse` | server → client                                         | An action is rejected, or any other protocol-level error occurs. |
| `MatchResult`   | server → client (broadcast to all players in the match) | The match reaches a win, loss, or draw condition.                |

**`ActionRequest`**

- `match_id` — from the `LobbyUpdate` that started this match.
- `encoded_action` — opaque bytes. The core does not interpret this; it is
  handed directly to the match's registered `Ruleset` for validation.
- Carries no player identity field — see the impersonation note in Section 2.
  The acting player is always the identity attached to the sending
  connection, resolved server-side.

**`StateUpdate`**

- `state_version` — increases with every applied action. A client can use
  this to detect and ignore a stale or duplicate broadcast.
- `encoded_view` — opaque bytes: a per-recipient view of state. This lets a
  game with hidden information (e.g. a Go Fish hand) send each player only
  what they're allowed to see, rather than the full authoritative state.
- `active_player_id` — whose turn it is now.
- `player_order` — the full turn order for the match. Needed for games with
  more than two players (e.g. Go Fish), where "next turn" isn't just "the
  other player."

**`ErrorResponse`**

- `code` — see `ErrorCode` below.
- `message` — human-readable detail, for logging/display only; clients
  should branch on `code`, not on this string.
- `match_id` — set if the error is scoped to a specific match/action; empty
  for connection- or lobby-level errors.

**`ErrorCode`**
| Code | Meaning |
|---|---|
| `ERROR_CODE_INVALID_PROTOCOL_VERSION` | Client's `protocol_version` is incompatible with the server. |
| `ERROR_CODE_AUTH_FAILED` | Unknown `player_id`, or `reconnect_token` didn't match. |
| `ERROR_CODE_UNKNOWN_GAME` | `game_id` has no registered ruleset. |
| `ERROR_CODE_LOBBY_FULL` | Join attempted on a lobby already at `max_players` (should be rare — full lobbies auto-start). |
| `ERROR_CODE_LOBBY_NOT_FOUND` | `lobby_id` does not exist. |
| `ERROR_CODE_LOBBY_ALREADY_STARTED` | Join attempted on a lobby whose status is already `STARTING` or `CLOSED`. |
| `ERROR_CODE_NOT_LOBBY_HOST` | `StartGameRequest` sent by someone other than the host. |
| `ERROR_CODE_LOBBY_NOT_READY` | `StartGameRequest` sent before `min_players` was met. |
| `ERROR_CODE_MATCH_NOT_FOUND` | `match_id` does not exist. |
| `ERROR_CODE_OUT_OF_TURN` | Action submitted by a player who is not `active_player_id`. |
| `ERROR_CODE_ILLEGAL_ACTION` | Action rejected by the ruleset's legality check. |
| `ERROR_CODE_MALFORMED_MESSAGE` | `encoded_action` could not be parsed by the active ruleset. |

**`MatchResult`**

- `winner_player_ids` — empty if `draw` is `true`; can hold more than one id
  if a game or ruleset supports tied or shared outcomes.
- `summaries` — per-player counts of accepted vs. rejected actions during the
  match, pulled from the persisted action log (useful for the demo/testing
  requirement to show illegal actions were actually rejected).

## Design notes

- **`encoded_action` / `encoded_view` stay opaque at the core.** This is what
  keeps the networking, lobby, and persistence layers game-agnostic — only
  the registered `Ruleset` for a match ever deserializes these bytes into a
  concrete move or state type.
- **`game_id` is a string, not an enum.** Adding a new game already requires
  a server-side code change (registering a new `Ruleset`); keeping `game_id`
  as a string avoids also needing a schema/proto change (and every client's
  regenerated bindings) at the same time.
- **Lobby min/max players are server-declared, not client-declared.** A
  client can display them, but cannot set or override them — this is what
  lets the server enforce "chess is exactly 2, Go Fish is 2–4" without that
  rule leaking into the protocol itself.
- **Identity is never self-asserted.** `player_id` is either freshly issued
  by the server or resumed by proving ownership via `reconnect_token`; no
  message after `AuthRequest` lets a client claim to be a specific player —
  the server always attributes actions to the identity bound to the
  connection. This is the core anti-impersonation guarantee and should be
  enforced in the session/connection layer, not re-checked per message.
