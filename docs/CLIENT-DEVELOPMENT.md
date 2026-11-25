# Client Development

RAMServer-Core clients are independent applications that communicate with a server through the documented wire protocol. A client may be written in any language with a compatible Protocol Buffers implementation and networking support.

## Start from `clients/client-base`

The maintained client branch pattern is based on the `clients/client-base` branch. Create a new client branch or repository from that base and preserve its established structure, build conventions, and documentation pattern.

Before starting a client, inspect the current `clients/client-base` branch and at least one maintained client example. Treat those as the structural reference; do not assume that a client repository should contain the server source tree.

Each client should be a relatively self-contained repository or branch for one client implementation and one supported game. It should include the client source, its own build/run instructions, dependency declarations, and tests appropriate to its language.

## Client responsibilities

A conforming client is responsible for:
- Establishing a connection to the server.
- Implementing the protocol handshake and version negotiation.
- Encoding and decoding Protocol Buffers messages.
- Applying the documented frame format.
- Responding to heartbeat messages as specified.
- Handling server errors and disconnects.
- Presenting the participant's permitted game view.
- Collecting player input and sending action requests.
- Respecting server updates and turn/readiness state.

A client is not authoritative. It must not assume that a locally calculated move is legal or that its local state is the canonical game state. The server decides whether an action is accepted.

## Protocol source of truth

The protocol definitions under `proto/` and the specification in `docs/protocol.md` are authoritative. A client must not infer protocol behavior from another client's implementation.

When the protocol changes, update the client to the supported protocol version and test compatibility against the corresponding server build. Do not silently reinterpret message fields or omit required handshake behavior.

## Client repository expectations

Each client should provide:
- A clear project name and supported game.
- A concise README with prerequisites, installation, build, and run instructions.
- A list of supported operating systems and runtime versions.
- Dependency and build configuration.
- Source code organized according to the language's conventions.
- Tests for protocol encoding/decoding and client behavior.
- A description of known limitations.
- A license and third-party notices appropriate to that client repository.

Keep game-specific presentation and input handling separate from reusable protocol/transport code where practical.

## Testing and conformance

At minimum, test:
- Connection and protocol handshake.
- Version compatibility behavior.
- Heartbeat handling.
- Session join and readiness flow.
- Valid action submission.
- Rejected action and error handling.
- State update handling.
- Completion of a full match.

Where a conformance suite is available, run it against the same server build used by other client implementations. The project values independently implemented clients because they demonstrate that the protocol is sufficiently documented and language-neutral.

## Proposing a client for the official examples

A client can be maintained independently without being featured by the official project. To propose it as an official featured client, follow [Featured Clients](FEATURED-CLIENTS.md).
