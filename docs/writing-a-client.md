# Writing a client

A client connects to a RAMServer, signs in, joins a session of a game, and plays by exchanging protobuf
messages. The server enforces every rule; the client's job is to send well-formed requests, keep an
accurate picture of the session from what the server sends, and present it to the player.

Read these first; this guide builds on them:

- [Message framing](framing.md): how messages are delimited.
- [WebSocket transport](websocket.md): connecting to the hosted server.
- [Protocol reference](../proto/README.md): every message and field, and the full session flow.

## Building blocks

- **Protobuf:** generate message code from `proto/ramserver/v1/ramserver.proto`. For C, the common
  choices are protobuf-c (`protoc --c_out`) and nanopb. Both encode and decode the `ClientMessage` and
  `ServerMessage` envelopes; everything you send or receive is one of those two.
- **Transport:** plain TCP for a local server in TCP mode, and WebSocket over TLS (`wss://`) for the
  hosted server. Hide both behind one "send frame / receive frame" interface so the rest of the client
  doesn't care which is in use.
- **An event loop:** the server sends messages at any time (other players' moves, heartbeats), not only in
  reply to you. Wait on both the connection and user input at once (for example with `poll`/`select`, or
  a library's event loop), and keep a buffer for partial frames, since one read may contain part of a
  frame or several frames.

## Connection timing

These are the server's defaults. A client that breaks them is disconnected.

| Rule | Default |
| --- | --- |
| Send your first frame (`ConnectRequest`) within | 5 seconds of connecting |
| Send *some* frame (any message or heartbeat) at least every | 30 seconds |
| The server sends you a heartbeat every | 10 seconds |
| Messages the server will queue for you before dropping a client that isn't reading | 32 |

- **Reply to every heartbeat** (a zero-length frame) with a heartbeat. That alone keeps you within the
  30-second limit, even while the player is thinking.
- **Keep reading** the connection at all times. A client that stops reading (for example while blocked
  waiting for keyboard input) is disconnected once 32 messages are waiting.
- Heartbeats are transport frames: never pass them to the protobuf decoder.

## The conversation, step by step

### 1. Connect and sign in

1. Send `ConnectRequest` with `protocol_version` `"1.2"` and a `client_name` for your client. A server with
   a different major version replies with `ERROR_CODE_INVALID_PROTOCOL_VERSION` and closes the
   connection.
2. Send `AuthRequest` with a `display_name`. The `AuthResponse` gives you a `player_id`, your identity for
   the rest of the connection, and a secret `reconnect_token`.

Until `AuthResponse` arrives, every other message is rejected with `ERROR_CODE_INVALID_STATE`.

### 2. Sequence numbers and replies

- Give every message you send a `sequence`, starting at 1 and increasing by one.
- Every request gets exactly one reply whose `in_reply_to` equals that request's `sequence`.
- Messages with `in_reply_to` 0 were not caused by your request: other players' changes, match results,
  and so on. Handle them whenever they arrive.
- Don't assume the next message is the reply to your last request; match them by `in_reply_to`.

The table in [Request/reply pairing](../proto/README.md#envelopes-and-framing) lists which message answers
which request.

### 3. Find a game

`JoinSessionRequest` with your game's `game_id` (for example `"chess"` or `"tictactoe"`) has three forms:

| Fields | Effect |
| --- | --- |
| `session_id` empty | Quick match: join an open session for the game, or create one if there is none. |
| `create_new: true` | Always create a new session. Add `private_session: true` to keep it out of listings and quick match. |
| `session_id` set | Join that exact session, public or private. |

Combining `create_new` with `session_id`, or setting `private_session` without `create_new`, is
rejected with `ERROR_CODE_INVALID_ARGUMENT`.

- The reply is a `StateUpdate` for the session you joined. Remember its `session_id`: every later
  session message must name it.
- To let a friend into a private session, show the player its `session_id` to share.
- `ListSessionsRequest` (optionally with `game_id`) returns the public sessions that have members, with
  their players and whether they are `joinable`. Use it for a lobby browser.
- You can be in only one session at a time. Joining another first requires leaving.

### 4. Lobby and readiness

- `StateUpdate.players` lists members in join order, each with `display_name`, `role`, `ready`, and
  `connected`. `owner_player_id` is the earliest member still present.
- Send `SetReadyRequest` with `ready: true` when the player is ready. The game decides when a match can
  start (for a two-player game, typically when both players are ready). When it starts, every member receives a
  `StateUpdate` with `status: SESSION_STATUS_IN_PROGRESS`.

### 5. Playing

Every in-progress `StateUpdate` carries:

- `encoded_view`: the game state as you may see it. Its format is defined by the game's ruleset README,
  for example `server/rulesets/tictactoe/README.md`; decode it to draw the board.
- `active_player_id`: whose turn it is.
- `player_order`: the order players were seated in.
- `state_version`: increases with every change. Keep the latest one.
- `annotations`: optional facts the game provides, such as move counts. The update after the move that
  ends a match still carries them, so a game can say how it ended (for example the winning line).

To move, send `ActionRequest` with:

- `session_id`,
- `encoded_action` in the format the ruleset's README defines,
- `expected_state_version`: either the `state_version` of the state the player was looking at, or 0
  to skip the check. With a version set, a move made against an out-of-date view is rejected with
  `ERROR_CODE_STALE_STATE` instead of being applied.

A successful move produces a new `StateUpdate` for every member; your copy has `in_reply_to` set to the
move's `sequence`. A rejected move gets an `ErrorResponse` and changes nothing:

| Code | Meaning |
| --- | --- |
| `ERROR_CODE_OUT_OF_TURN` | It isn't your turn. |
| `ERROR_CODE_ILLEGAL_ACTION` | The game refused the move; `message` says why. Also sent for actions before the match starts. |
| `ERROR_CODE_MALFORMED_MESSAGE` | The game couldn't decode your `encoded_action`. |
| `ERROR_CODE_STALE_STATE` | `expected_state_version` didn't match. Wait for the latest state and let the player try again. |
| `ERROR_CODE_NOT_IN_SESSION` | The `session_id` isn't your current session. |

`LegalActionsRequest` returns the actions you may take now, each as the exact `payload` to send back
as `encoded_action`, with an optional `description`. It is empty when it isn't your turn.

### 6. The end of a match

- Every member receives a `MatchResult`: `winner_player_ids` (empty and `draw: true` for a draw),
  `termination_reason` (the game's own reason, as documented by the ruleset), and `turns`.
- Then a `StateUpdate` shows the lobby waiting again, with **everyone's ready flag cleared**. Send
  `SetReadyRequest` again for a rematch.
- `MatchHistoryRequest` returns every finished match in your current session.

### 7. Leaving and disconnecting

- To leave a session, send `LeaveSessionRequest` with its `session_id`; the reply is
  `LeaveSessionResponse`. You stay signed in and can join another session.
- To quit, leave your session first, then close the connection.
- Store the `player_id` and `reconnect_token` if you want to offer resuming after a dropped
  connection. The protocol describes resuming with `AuthRequest` (`player_id` and `reconnect_token`) in
  **Resuming player** and **Disconnects** in the [protocol reference](../proto/README.md).
  **Server status:** resuming is not implemented yet. The server rejects an `AuthRequest` that carries
  a `player_id` with `ERROR_CODE_AUTH_FAILED`, and a player whose connection drops keeps their seat in the
  session. A client should handle that rejection by signing in as a new player.

## Robustness

- Treat the server as the source of truth. Update the screen from `StateUpdate`s, not from what you
  expect a move to do.
- Handle every `ErrorResponse` by showing its `message`; never crash or hang on an unexpected reply.
- Validate frame lengths before allocating (see [framing](framing.md)), and treat a truncated frame,
  a decode error, or a closed connection as the end of the connection, with a clear message to the player.
- Messages you don't recognize, or fields you don't use, should be ignored rather than treated as errors.

## Testing a client

- Unit-test the parts that don't need a server: framing (including partial and combined reads), building
  each request, decoding views, and updating your session state from a sequence of server messages.
- Feed recorded or hand-built `ServerMessage` sequences through your state handling to test lobbies,
  moves, errors, and match ends without a network.
- Then test against a real server, local and hosted; see the [README](../README.md) for running a
  local server and the hosted server's address.
