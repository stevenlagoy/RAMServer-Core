# WebSocket transport

A RAMServer can accept connections over WebSocket as well as plain TCP. The hosted server only accepts
WebSocket, because its hosting platform only forwards HTTP traffic. Everything above the connection
itself is identical: the same [frames](framing.md) carrying the same [protocol](../proto/README.md) messages,
in the same order, with the same rules.

## Endpoints

| Server                  | Address                                                         |
| ----------------------- | --------------------------------------------------------------- |
| Hosted                  | `wss://ramserver-core-80752473398.us-east5.run.app/connect`     |
| Local, WebSocket mode   | `ws://127.0.0.1:<PORT>/connect`, with the server started with `PORT` set (see the [README](../README.md)) |
| Local, TCP mode         | `127.0.0.1:8080`, plain TCP, no WebSocket                       |

- `/connect` is the only WebSocket endpoint. Any other path is not a WebSocket endpoint.
- `GET /` on the same host returns HTTP 200 with the text `RAMServer-Core is running`. Use it to check the
  server is up before debugging a connection.
- `wss://` is WebSocket over TLS on port 443. The client must perform a TLS handshake with SNI set to the
  host name before the WebSocket handshake.

## Opening the connection

Open the connection with a standard WebSocket handshake (RFC 6455): an HTTP/1.1 `GET /connect` with
`Upgrade: websocket`, `Connection: Upgrade`, `Sec-WebSocket-Key`, and `Sec-WebSocket-Version: 13`.

- No subprotocol (`Sec-WebSocket-Protocol`) is needed.
- **Do not send an `Origin` header**, or send one whose host matches the server's host. The server rejects
  the handshake (HTTP 403) when an `Origin` header names a different host. Native clients normally send no
  `Origin` header.
- After the handshake succeeds (HTTP 101), start the protocol exactly as over TCP: send `ConnectRequest`
  as your first frame.

The hosted server may take a few seconds to accept the first connection after a quiet period, while
it starts up. Allow for this in connection timeouts and in any UI feedback.

## Carrying frames in WebSocket messages

- **Use binary messages only** (opcode `0x2`). A text message makes the server close the connection.
- The bytes of binary messages form the same byte stream as a TCP connection: a four-byte big-endian
  length prefix followed by the payload, as described in [framing](framing.md).
- **The server sends exactly one frame per message**: each binary message from the server contains one
  complete length prefix and its payload, nothing more.
- **Send exactly one frame per message** as well. The server reads binary messages as one continuous
  stream, so a frame split across messages is accepted, but one frame per message is simpler and is what
  every existing client does.
- A single message must not exceed 65,540 bytes (a maximum-size frame). Larger messages close the
  connection.
- **Heartbeats** are zero-length frames, so a heartbeat is a binary message containing exactly four zero
  bytes (`00 00 00 00`). They work exactly as over TCP; see [Connection timing](writing-a-client.md#connection-timing).
- WebSocket ping/pong control frames are separate from heartbeats. Most WebSocket libraries answer pings
  automatically; the server does not rely on them, so they do not replace replying to heartbeats.

## Closing

- To leave cleanly, send `LeaveSessionRequest` first if you are in a session, then a WebSocket close
  frame, then close the socket.
- When the server ends the connection, it sends a close frame. Treat a close frame, an error, or the
  socket closing the same way as a dropped TCP connection.
- The hosting platform may end long-lived connections after a time limit. Treat this like any other
  dropped connection.

## Differences from TCP

There are none at the protocol level. Two practical ones:

- `ConnectResponse.connection_id` is an address on the hosting platform's network, not your client's
  address. Use it only for logging.
- Connection limits may differ from a local server's. A client should handle being refused or
  disconnected and report it clearly.
