## Summary

Describe what this pull request changes.

## Motivation

Why is this change needed? Reference one or more related issue(s).

## Change type

- [ ] Core feature or improvement
- [ ] Bug fix
- [ ] New game ruleset
- [ ] Client or featured-client change
- [ ] Protocol change
- [ ] Documentation
- [ ] Build, test, or infrastructure

## Implementation

Describe the important implementation decisions and affected components.

## Testing

List commands run and their results.

- [ ] Unit tests
- [ ] Integration tests
- [ ] Adversarial tests, where applicable
- [ ] Cross-language conformance, where applicable
- [ ] Formatting and linting
- [ ] Buf lint/breaking checks, where applicable

## Compatibility and operational impact

- [ ] No wire-protocol change
- [ ] `docs/protocol.md` updated if protocol behavior changed
- [ ] Persistence/configuration/deployment impacts documented
- [ ] Migration or upgrade steps included, if needed

Describe any impact not covered above.

## Ruleset contribution checklist

Complete this section for a new ruleset.

- [ ] New package under `server/rulesets/<game-name>/`
- [ ] Implements the current `game.Game` interface
- [ ] Includes ruleset README and tests
- [ ] Does not modify `server/internal/`
- [ ] Uses the supported registration point
- [ ] Includes integration evidence

## Client contribution checklist

Complete this section for a client or featured-client proposal.

- [ ] Based on `clients/client-base`
- [ ] Implements the published protocol independently
- [ ] Includes build/run documentation and tests
- [ ] Identifies game and protocol version
- [ ] Includes conformance evidence, where available

## Follow-up work and known limitations

List anything intentionally left incomplete.

## Issue

Refs #
