# leanclient

`leanclient` is a small manual-testing client for RAMServer's current TCP framing layer. It performs the placeholder handshake, replies to zero-length heartbeat frames, lets you send arbitrary frame payloads at runtime, and labels frames received from the server.

Run these commands from the `server` directory.

## Interactive mode

```bash
go run ./cmd/leanclient
```

Type one payload per line and press Enter. Each line is sent as one length-prefixed frame. Empty lines are skipped because a zero-length frame is reserved for heartbeats.

Example:

```text
Connected to 127.0.0.1:9000
[2006-01-02 15:04:05] -> HELLO     leanclient
[2006-01-02 15:04:05] <- WELCOME   welcome leanclient
Enter frame payloads on separate lines. Exit with Ctrl+C or EOF.
[2006-01-02 15:04:05] -> FRAME     move
[2006-01-02 15:04:05] <- REJECT    reject 0: transport: DecodeClientMessage not implemented
```

## Send one frame

Use `-send` to send one frame after the handshake, print the server's next non-heartbeat response, and exit:

```bash
go run ./cmd/leanclient -send "move"
```

## Options

```text
-addr string
      server address (default: RAMSERVER_ADDR, otherwise 127.0.0.1:9000)
-hello string
      handshake frame payload (default "leanclient")
-send string
      send one frame after the handshake, print one response, then exit
```

Go's standard `-h` / `--help` output is also available:

```bash
go run ./cmd/leanclient -h
```

When the server is running through the repository's Docker Compose setup, connect to its published port with:

```bash
go run ./cmd/leanclient -addr 127.0.0.1:9000
```

No application-level encoding is assumed yet. Payloads are sent exactly as entered, wrapped only in the existing 4-byte big-endian length prefix used by the transport layer.
