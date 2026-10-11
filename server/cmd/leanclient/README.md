# leanclient

`leanclient` is a small manual-testing client for RAMServer. It speaks the protobuf protocol described in [docs/protocol.md](../../../docs/protocol.md): it performs the `ConnectRequest` / `AuthRequest` handshake, replies to zero-length heartbeat frames, sends protocol messages from typed commands, and prints every message the server sends, decoded.

Run these commands from the `server` directory, with a server running (`go run ./cmd/ramserver` or Docker Compose).

## Interactive mode

```bash
go run ./cmd/leanclient -name Alice
```

The client connects, completes the handshake, then reads one command per line. Messages you send are marked `->`, and messages from the server are marked `<-`. Type `help` to list the commands.

Example:

```text
Connected to 127.0.0.1:9000
[2026-10-11 00:32:22] -> CONNECT   sequence:1 connect_request:{protocol_version:"1.0" client_name:"leanclient"}
[2026-10-11 00:32:22] <- CONNECTED in_reply_to:1 connect_response:{connection_id:"127.0.0.1:34686" server_protocol_version:"1.0"}
[2026-10-11 00:32:22] -> AUTH      sequence:2 auth_request:{display_name:"Alice"}
[2026-10-11 00:32:22] <- AUTHED    in_reply_to:2 auth_response:{player_id:"69e0ff70..." reconnect_token:"8b5d0494..."}
Type a command, or help for the list. Exit with Ctrl+C or EOF.
create stub
[2026-10-11 00:32:25] -> JOIN      sequence:3 join_session_request:{game_id:"stub"}
[2026-10-11 00:32:25] <- STATE     in_reply_to:3 state_update:{session_id:"session-b3d4c64e..." game_id:"stub" status:SESSION_STATUS_WAITING players:{player_id:"69e0ff70..." display_name:"Alice" connected:true role:"player"}}
raw hello
[2026-10-11 00:32:28] -> RAW       "hello"
[2026-10-11 00:32:28] <- ERROR     error_response:{code:ERROR_CODE_MALFORMED_MESSAGE message:"proto: cannot parse invalid wire-format data"}
```

## Commands

| Command                                 | Sends                 | Note                                                                                     |
|-----------------------------------------|-----------------------|------------------------------------------------------------------------------------------|
| `connect [protocol_version]`            | `ConnectRequest`      | Only needed with `-handshake=false`.                                                     |
| `auth <display_name> [player_id token]` | `AuthRequest`         | Only needed with `-handshake=false`.                                                     |
| `create <game_id> [role]`               | `JoinSessionRequest`  | Does not include `session_id`, so a new session will be created.                         |
| `join <game_id> <session_id> [role]`    | `JoinSessionRequest`  | `session_id` should match an existing session.                                           |
| `leave [session_id]`                    | `LeaveSessionRequest` | Defaults to the current session.                                                         |
| `action <payload>`                      | `ActionRequest`       | Targets the current session. The rest of the line is sent as the `encoded_action` bytes. |
| `raw <text>`                            | arbitrary frame       | Sends the text as an arbitary frame, not protobuf. Useful for testing malformed input.   |
| `help`                                  | n/a                   | Lists the commands.                                                                      |

The client numbers each message's `sequence` automatically. It remembers the current session from the last `StateUpdate` it received, and forgets it after a `LeaveSessionResponse`.

Matches cannot be started over the protocol yet, so `action` is currently dropped by the server after joining a session.

## Run one command

Use `-send` to run one command after the handshake, print the server's next message, and exit:

```bash
go run ./cmd/leanclient -send "create stub"
```

## Testing the handshake

Use `-handshake=false` to skip the automatic handshake and send `connect` and `auth` yourself, for example to check that a message sent before authenticating is rejected with `ERROR_CODE_INVALID_STATE`.

Use `-version` to send a different protocol version. A different major version is rejected and the connection is closed:

```bash
go run ./cmd/leanclient -version 2.0
```

## Options

```text
-addr string
      server address (default: RAMSERVER_ADDR, otherwise 127.0.0.1:9000)
-handshake
      send ConnectRequest and AuthRequest automatically on connect (default true)
-name string
      display name sent in AuthRequest (default "leanclient")
-send string
      run one command after the handshake, print one response, then exit
-version string
      protocol version sent in ConnectRequest (default "1.0")
```

Go's standard `-h` / `--help` output is also available:

```bash
go run ./cmd/leanclient -h
```

When the server is running through the repository's Docker Compose setup, connect to its published port with:

```bash
go run ./cmd/leanclient -addr 127.0.0.1:9000
```
