# Contributing a Game Ruleset

This document defines the requirements for proposing a new game ruleset for inclusion in the official RAMServer-Core repository and possible use by the project-hosted server.

A ruleset accepted into your own fork is your responsibility. A ruleset submitted upstream is subject to maintainer review and must meet the boundaries below.

## Required location and scope

The contribution must add a new, game-specific directory:

```text
server/rulesets/<game-name>/
```

The directory should contain the implementation, tests, and a `README.md` describing the game and its ruleset behavior.

A ruleset-only pull request must not modify:
- `server/internal/` implementation code;
- transport or wire framing;
- session or match orchestration;
- persistence implementation or schema;
- unrelated existing rulesets.

The ruleset must implement the public `game.Game` interface and use the supported game registration mechanism. If the proposed game genuinely requires a change to the core API, submit that as a separate feature proposal first. Do not bundle core redesign into a ruleset contribution.

## Submission checklist

A pull request should include:

- [ ] A new directory under `server/rulesets/<game-name>/`.
- [ ] A complete implementation of the current `game.Game` interface.
- [ ] A ruleset README covering objective, roles, setup, turn order, actions, legal/illegal behavior, and end conditions.
- [ ] Unit tests for rules, state transitions, turn order, and outcomes.
- [ ] Tests for invalid, out-of-turn, and malformed game actions.
- [ ] Tests for participant-specific views and hidden information, if applicable.
- [ ] Deterministic handling of randomness, where applicable.
- [ ] Registration through the existing supported registration point.
- [ ] An integration test or clear evidence of a complete match through the server.
- [ ] No changes to `server/internal/` or unrelated core packages.
- [ ] Updated root documentation or game listings, if required.

## Review process

Maintainers review:
1. Interface correctness and compatibility.
2. Whether game logic is isolated in the new ruleset package.
3. Correctness and completeness of validation and state transitions.
4. Protection of server authority and participant-specific information.
5. Test quality and reproducibility.
6. Documentation quality and maintainability.
7. Whether the game is suitable for the project's current release and hosted environment.

Maintainers may request changes, ask for a separate core feature issue, defer the game to a later release, or decline it. A pull request being open does not mean the game is approved.

## Hosted-server inclusion

Merging a ruleset into the repository and enabling it on a project-hosted server are separate decisions. The maintainers may choose when and whether to deploy an accepted ruleset. A merge does not guarantee a deployment date, ongoing hosting, or support for the ruleset.

## Suggested ruleset README

Use this outline in `server/rulesets/<game-name>/README.md`:

```markdown
# <Game Name>

## Overview
## Players and Roles
## Setup
## Turn Order
## Actions
## Validation Rules
## End Conditions
## State Visibility
## Randomness
## Testing
## Known Limitations
```
