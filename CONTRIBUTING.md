# Contributing to RAMServer-Core

Thank you for considering a contribution. This document explains how to propose, implement, and submit changes to the official RAMServer-Core repository.

The project accepts several kinds of contributions, but they do not all follow the same process:

1. Core changes to the server, protocol, persistence, or infrastructure.
2. New game rulesets.
3. Client implementations and featured client examples.
4. Feature proposals, bug reports, and documentation improvements.

For substantial work, open an issue before changing code. An issue should explain the problem or opportunity, identify the expected outcome, and provide acceptance criteria.

## Before you start

- Search existing issues and pull requests to avoid duplicate work.
- Open an issue using the appropriate template.
- Wait for maintainer feedback before beginning work that could affect architecture, protocol compatibility, or project scope.
- Do not include credentials, private user data, or production secrets in issues, commits, or test fixtures.
- Contributions to the official repository are reviewed; opening a pull request does not guarantee acceptance or deployment to the project-hosted server.

## Contribution paths

### Core features and bug fixes

Use a feature request or bug report issue. Core changes should be scoped to the smallest coherent change and include tests and documentation where appropriate.

Core pull requests must explain:
- What changed.
- Why the change is needed.
- How it was tested.
- Whether it changes the protocol, persistence format, deployment requirements, or public API.
- Any known limitations or follow-up work.

Changes to `proto/` require particular care. Update `docs/protocol.md` in the same pull request, run Buf lint and breaking-change checks, and obtain review from the Protocol Designer or designated protocol maintainer.

### New game rulesets

A game ruleset is an implementation of the public `game.Game` interface. Developers may add rulesets to their own fork without submitting them upstream.

To propose a ruleset for the official repository or the project-hosted server:
1. Open a ruleset contribution pull request using the repository's pull request template.
2. Add the implementation in a new, game-specific directory under `server/rulesets/`.
3. Include the ruleset's README, tests, and any required fixtures.
4. Register the ruleset only through the approved registration point.
5. Do not modify the server's internal transport, connection, session, match, or persistence implementation as part of a ruleset-only contribution.
6. Include evidence that the ruleset works through the normal server request/validation/application/broadcast path.

The maintainers may request changes, decline the contribution, or defer it to a later release. Acceptance into the source repository does not automatically guarantee immediate inclusion in a hosted deployment.

Full requirements: [Game Contribution](docs/GAME-CONTRIBUTION.md).

### Client implementations

Clients are separate programs that implement the wire protocol. They should be developed from the `clients/client-base` branch and follow the established client repository pattern. A client should implement the protocol independently; it must not rely on another client's source code to interpret server behavior.

A client may be proposed as a featured project example. Featured status is a curated designation, not an automatic consequence of submitting a client. See [Client Development](docs/CLIENT-DEVELOPMENT.md) and [Featured Clients](docs/FEATURED-CLIENTS.md).

### Feature suggestions

Use the **Feature request** issue template. A useful proposal describes:
- The desired behavior, from a user's or developer's perspective.
- The problem or motivation.
- Who benefits and how.
- Expected impact on the server, protocol, clients, persistence, security, and compatibility.
- Alternatives considered, if any.
- A reasonable definition of success or acceptance criteria.

A proposal is a request for discussion, not a commitment to implement the feature.

## Development workflow

The project uses issue-based development and pull-request review.

1. Create or select an issue.
2. Agree on scope and acceptance criteria.
3. Create a branch using the repository's branch naming convention.
4. Make a focused change.
5. Add or update tests and documentation.
6. Run the relevant local checks.
7. Open a pull request referencing the issue.
8. Address review feedback.
9. Merge only after required approvals and checks pass.

For the original team workflow, branch ownership and approval requirements are documented in [Team Policies](docs/TEAM-POLICIES.md). Repository maintainers may adapt those internal policies for external contributors; the active branch protection and pull request checks are authoritative.

## Branch naming

Use:

```text
<type>/<issue-number>-<short-kebab-description>
```

Examples:

```text
feature/42-illegal-action-rejection
fix/57-tcp-frame-boundary
test/61-spoofed-action-suite
docs/63-protocol-spec-v2
chore/70-dockerfile-multistage
```

## Commit messages

- Keep each commit focused on one subsystem or coherent change.
- Use the imperative mood.
- Keep the subject line under 72 characters.
- Explain reasoning in the body when it is not obvious.
- Reference the issue, for example `Refs #42`.
- Avoid vague messages such as `fix`, `wip`, or `update`.

## Pull request requirements

A pull request should:
- Reference the issue it addresses.
- State what changed and why.
- Describe how it was tested.
- Identify protocol, API, persistence, configuration, or compatibility impacts.
- List known limitations and follow-up work.
- Include tests for new behavior and regression tests for bug fixes.
- Update relevant documentation.
- Avoid unrelated refactoring.

For a ruleset contribution, also satisfy [Game Contribution](docs/GAME-CONTRIBUTION.md). For a client proposal, satisfy [Client Development](docs/CLIENT-DEVELOPMENT.md).

## Testing expectations

Run formatting, linting, and tests relevant to the changed components. At minimum, server changes should be checked with:

```sh
cd server
go test -race ./...
```

If applicable, also run:

```sh
buf lint
buf breaking --against <configured-baseline>
```

Use the repository's configured Buf baseline and CI workflow; do not substitute an arbitrary comparison target. Include the actual commands and results in the pull request.

Tests should cover behavior, not merely implementation details. Depending on the change, consider:
- Unit tests for validation and state transitions.
- Integration tests for the full client request through server validation and broadcast.
- Adversarial tests for malformed, illegal, spoofed, out-of-turn, or replayed actions.
- Cross-language conformance tests for client implementations.

## Review and acceptance

Maintainers review contributions for correctness, maintainability, scope, documentation, compatibility, and project direction. They may ask for revisions, additional tests, or a narrower change.

Do not assume that a contribution is accepted until it is merged. Do not advertise an unmerged ruleset or client as part of an official release.

## Code of conduct

All contributors are expected to communicate respectfully and constructively. See [Code of Conduct](CODE_OF_CONDUCT.md).

## Questions

For project questions, use GitHub Discussions if enabled, or open an issue with the appropriate label. Do not use private contact details from historical course documentation as the default support channel.
