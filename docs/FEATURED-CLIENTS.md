# Featured Clients

Featured clients are selected client implementations maintained or highlighted by the RAMServer-Core project as examples of how to build applications against its language-neutral protocol.

The goal is to provide several useful, independently implemented examples across languages, frameworks, and game interfaces. Featured status is curated and may change as clients become outdated or unsupported.

## What featured status means

A featured client:
- Is linked from the official repository's client directory or README.
- Has been reviewed against the project's client expectations.
- Has clear build and run instructions.
- Identifies the game and protocol version it supports.
- Has a maintainer or clear maintenance status.
- Is presented as an example, not as the only correct way to implement a client.

Featured status does not mean that the RAMServer maintainers warrant the client, provide support for every platform, or guarantee compatibility with future server releases.

## Eligibility criteria

A candidate should:
1. Be based on the structure and conventions established by `clients/client-base`.
2. Implement the documented protocol without depending on another client's source.
3. Clearly identify its language, runtime, supported game, and protocol version.
4. Include reproducible build and run instructions.
5. Include tests for protocol handling and important client behavior.
6. Complete a representative match against a supported server build.
7. Handle server rejections and state updates correctly.
8. Include a license and required third-party notices.
9. Document limitations and known issues.
10. Have an identified maintainer or explicitly state that it is an archived example.

## Nomination process

1. Open a pull request or client nomination issue.
2. Link the client branch or repository and identify its source revision.
3. Provide build/test instructions and conformance evidence.
4. Identify the supported game and protocol version.
5. Explain what the example demonstrates (language, framework, UI style, or other useful distinction).
6. Respond to review feedback.

Maintainers may accept, request changes, defer, or decline a nomination. The project may feature multiple clients for the same game when they demonstrate meaningfully different implementation approaches.

## Featured client listing

The official README should link to each selected client and include:

| Field      | Description                             |
|------------|-----------------------------------------|
| Client     | Project name and repository/branch link |
| Game       | Game ruleset supported                  |
| Language   | Primary implementation language         |
| Runtime    | Required runtime or framework           |
| Protocol   | Supported protocol version              |
| Status     | Maintained, experimental, or archived   |
| Maintainer | Public project handle or contact route  |

Do not label an unreviewed or unmaintained client as officially supported. Reassess featured entries when the protocol changes or the client no longer builds.
