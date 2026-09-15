# Contributing

Full team policy: [docs/TEAM-POLICIES.md](docs/TEAM-POLICIES.md). This file
is the quick-reference version for day-to-day work.

## Before you start

- Every piece of work is a GitHub Issue on the project board before you touch code.
- An issue needs an owner and stated acceptance criteria before it moves to Ready.

## Branch naming

    <type>/<issue-number>-<short-kebab-description>

    feature/42-illegal-action-rejection
    fix/57-tcp-frame-boundary
    test/61-spoofed-action-suite
    docs/63-protocol-spec-v2
    chore/70-dockerfile-multistage

## Commits

- One subsystem per commit.
- Imperative mood, subject line under 72 characters, body explains reasoning.
- No `fix`, `wip`, `update`, or similarly non-descriptive messages.
- Reference the issue: `Refs #42`.

## Pull requests

- Title references the issue: `Reject out-of-turn actions and log rejection reason (#42)`.
- Description states what changed, why, how it was tested, and any follow-up left open.
- Requires approval from **every owner of the target branch** (see [branch-ownership.md](docs/branch-ownership.md)); a PR into `main` needs all four.
- **Every approval needs a written reason, minimum one sentence.** An approval with no comment is not valid under team policy.
- Any PR touching `proto/` requires the Protocol Designer as a reviewer, regardless of target branch.
- Squash-merge feature branches; delete the branch after merge.

## Before you open a PR, it must:

- [ ] Pass `gofmt` / your language's linter (CI enforces this — see §7 of team-policies.md)
- [ ] Include tests for any new behavior (a PR with no tests gets returned in review)
- [ ] Pass all existing tests locally
- [ ] Update `docs/protocol.md` in the same PR, if the change touches the wire protocol

## Testing tiers required (see §9)

1. Unit — validation logic, state transitions, protocol encoding/decoding
2. Integration — full request → validate → apply → broadcast cycle
3. Adversarial — illegal, out-of-turn, spoofed, malformed, and replayed actions
4. Cross-language conformance — both clients complete a match against the same server build

## Conflict resolution

Direct conversation → full team (majority vote, PM breaks ties) → instructor.
Full detail in §11 of team-policies.md.