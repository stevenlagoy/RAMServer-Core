# RAMServer Message Framing

This document defines how to read and write RAMServer messages over a TCP
connection. It is intended for developers implementing clients, servers, or
other tools that communicate with the server core.

Framing identifies the boundary and size of each message in TCP's byte
stream. It does not define the contents of a non-empty payload. After reading
a complete frame, pass its payload to the message parser defined by the
[wire protocol](protocol.md).

## Frame format

Every frame is a four-byte length prefix followed by the payload:

```
+----------------------------+-------------------------------+
|   length prefix (4 bytes)  |     payload (length bytes)    |
|   uint32, big-endian       |       message bytes           |
+----------------------------+-------------------------------+
```

- The prefix is an unsigned 32-bit integer encoded in big-endian (network)
  byte order.
- The prefix value is the number of payload bytes that follow. It does not
  include the four-byte prefix.
- Payloads are variable-sized, from zero through the **maximum payload size**
  (see [Maximum payload size](#maximum-payload-size)). The default maximum is
  65,536 bytes (64 KiB), so a maximum-size frame occupies 65,540 bytes on the
  wire.
- Each frame carries one payload. Do not combine separate messages into one
  frame or split one message across multiple frames.

## Maximum payload size

The maximum payload size is set by the `RAMSERVER_MAX_FRAME_BYTES`
environment variable, in bytes. It limits the payload only; the four-byte
prefix is not counted.

| Variable                    | Default           | Allowed values                     |
| --------------------------- | ----------------- | ---------------------------------- |
| `RAMSERVER_MAX_FRAME_BYTES` | `65536` (64 KiB)  | Integer from `1` to `4294967295`   |

- If the variable is unset or empty, the default is used.
- If it is set to a value that is not an integer in the allowed range, the
  server must refuse to start rather than fall back to the default.
- The value must be the same on both ends of a connection. A client must not
  send payloads larger than the server's maximum, and should be configured
  with the same value so it accepts every frame the server may send. See
  [`.env.example`](../.env.example).

## Reading frames

TCP is a byte stream: one read may return part of a prefix, part of a
payload, exactly one frame, or bytes spanning multiple frames. Do not treat
individual reads as frame boundaries. Read exactly the requested number of
bytes, preserving any bytes buffered for the next frame.

For each frame, a reader should:

1. Read exactly four bytes for the prefix. If the stream ends before the
   prefix is complete, report a truncated frame/connection error.
2. Decode those bytes as an unsigned big-endian 32-bit integer.
3. Reject lengths greater than the maximum payload size **before allocating
   or reading the payload**. Do not attempt to recover by treating payload bytes as another
   prefix; close or otherwise fail the connection because the stream cannot
   safely continue under this framing contract.
4. Read exactly the declared number of payload bytes. If the stream ends
   before all bytes arrive, report a truncated frame/connection error.
5. Handle a zero-length payload as a heartbeat. Do not pass it to the
   application message parser. For a non-empty payload, pass exactly those
   bytes to the parser for the expected protocol message.
6. Continue reading the next prefix from the same stream.

An exact-read helper is appropriate (for example, Go's `io.ReadFull`). If
using an API that may return short reads, loop until the requested byte
count is satisfied, an error occurs, or the connection closes.

## Writing frames

To write a frame:

1. Check that the payload is no larger than the maximum payload size.
2. Encode the payload length as a four-byte unsigned big-endian integer.
3. Write the complete prefix followed immediately by the payload.
4. Ensure every byte is written. If the write API can return a short write,
   continue writing the remaining bytes; stop and report an error if it
   cannot make progress or returns an error.

Serialize writes from a connection so bytes from concurrent frames cannot
interleave. On a write failure, treat the frame/connection as failed rather
than silently dropping bytes and continuing with a potentially corrupted
stream.

To send a heartbeat, write a frame with a zero length and no payload. A
zero-length frame is reserved for heartbeats and must not be used to encode
an empty application message.

## Heartbeats

A zero-length frame is a heartbeat. On receipt, consume it as a transport
control frame, not as an application message. A client should reply to a
server heartbeat with a zero-length frame so the server can observe that the
connection is still responsive. A server should treat an inbound client
heartbeat as liveness traffic and should not immediately echo it; it sends
its own heartbeats on its configured schedule. This avoids heartbeat
echo-loops. Heartbeats do not replace the need to read and write application
frames, and they do not change the framing of subsequent messages.

## Example

The payload `hello` contains five bytes (`68 65 6c 6c 6f` in hexadecimal),
so its frame is:

```
Length:       5
Prefix:       00 00 00 05
Payload:      68 65 6c 6c 6f
Full frame:   00 00 00 05 68 65 6c 6c 6f
```

A heartbeat has no payload and is encoded as:

```
00 00 00 00
```

## Protocol parsing

Framing only returns payload bytes; it does not determine which message type
they represent, validate fields, or provide message sequencing. Use the
protocol schema and message lifecycle in [protocol.md](protocol.md) to
serialize outgoing messages and parse incoming non-empty payloads. A
successfully read frame is not necessarily a valid protocol message: report
payload decoding or validation failures through the connection's normal
protocol-error handling.
