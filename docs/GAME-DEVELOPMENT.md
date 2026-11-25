# Game Development

This guide explains how to implement a game ruleset for a RAMServer-Core server that you operate or maintain.

## The Game Interface

Every game ruleset must implement the `game.Game` interface, defined in `server/game/game.go`. This interface separates game-specific rules from the server's networking, session management, and persistence systems.

A `Game` implementation is a stateless rulebook. The server may use the same registered `Game` instance for multiple concurrent matches, so implementations must not store match-specific data in the `Game` itself. All match-specific data belongs in the `GameState` value passed to its methods.

### GameState

`GameState` is defined as `any`, allowing each ruleset to choose its own representation. It might contain a board, player hands, scores, turn information, or any other data needed to represent a match.

The server core does not inspect or interpret `GameState`; it passes the value between the game implementation and the rest of the server.

Go does not enforce immutability. However, implementations must follow the convention that game states are not modified in place. In particular:

- Methods that inspect a state must not modify it.
- `Apply` must return a new state rather than modifying the supplied state.
- Implementations using pointers or mutable structures should copy the relevant data before changing it.
- Match-specific information must not be stored in fields on the `Game` implementation.

### Required Methods

Every `Game` implementation must provide the following methods:

| Method                          | Responsibility                                                                                                                                        |
|---------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------|
| `Roles()`                       | Defines the roles available in the game and the permitted number of participants for each role.                                                       |
| `NewState(rosterOrder, roles)`  | Creates the initial `GameState` for a new match using the participant order and assigned roles.                                                       |
| `LegalActions(state, memberID)` | Returns the actions a participant may currently take.                                                                                                 |
| `Validate(state, action)`       | Determines whether a submitted action is legal in the current state. Must not modify the state.                                                       |
| `Apply(state, action)`          | Returns the state resulting from an action. The caller is responsible for validation before applying it. Must not modify the supplied state in place. |
| `ActiveTurn(state)`             | Returns the participant IDs currently permitted to submit actions. Should return an empty, non-nil slice when nobody may act.                         |
| `Outcome(state)`                | Determines whether the match has ended and, if so, returns its result.                                                                                |
| `ReadyToStart(roster, ready)`   | Determines whether the current roster and readiness flags satisfy the game's requirements to begin a match.                                           |
| `ViewFor(state, memberID)`      | Returns the serialized state visible to a particular participant, allowing games to withhold hidden information.                                      |

The server uses these methods to manage the match lifecycle without needing to understand the rules of a particular game.

### Actions and Results

A submitted `game.Action` contains:

- `ID`: A client-assigned sequence number.
- `ActorID`: The participant submitting the action. The server associates actions with the authenticated participant.
- `Payload`: Game-defined bytes describing the requested action.

Each game defines its own action encoding. The same encoding is used in `ActionSpec.Payload` to describe available actions and in submitted actions for validation and application.

An `ActionSpec` contains:
- `Payload`: The encoded action a participant can submit.
- `Description`: A human-readable description of the action.

`Outcome` returns a `MatchResult` when a match finishes. A result provides the winning participant IDs and a `TerminationReason`. The generic `game.Result` implementation also supports per-participant scores and an `IsDraw()` helper. Games may implement `MatchResult` themselves when they need additional result information.

### Optional Interfaces

A game may implement additional interfaces to enable behavior beyond the required `Game` methods. These interfaces are detected by the server when supported; they are not required for every ruleset.

#### OutOfTurnActions

Implement `game.OutOfTurnActions` when certain actions may be submitted by participants who are not currently in `ActiveTurn`.

Its `AllowsOutOfTurn(state, action)` method tells the server whether turn checking should be skipped for a particular action. The server still calls `Validate`, so the game remains responsible for determining whether the action is legal.

This is useful for actions such as resignation or other game-defined actions that should be available regardless of whose turn it is.

#### Annotator

Implement `game.Annotator` when the game can provide additional state information through `StateUpdate.annotations`.

Its `Annotations(state, memberID)` method returns a map of annotation names to values. Annotations must depend only on the supplied state and participant ID.

This can expose useful information to clients without requiring them to interpret the game's internal state representation.

### Implementation Guidance

A typical ruleset implementation should:

1. Define its game-specific state and action encodings.
2. Implement all nine required `Game` methods.
3. Keep all match-specific data in `GameState`.
4. Return independent state values from `Apply`.
5. Implement `OutOfTurnActions` or `Annotator` only when the game needs those capabilities.
6. Register the ruleset with the server's game registry.

See the existing rulesets and `StubGame` for implementation examples.

## Implementing a ruleset

1. Read the `game.Game` interface and related types in `server/game/`.
2. Study the Chess ruleset as a concrete reference.
3. Create a new package under `server/rulesets/<game-name>/`.
4. Define game state and action types appropriate to the game.
5. Implement every method required by `game.Game`.
6. Ensure validation and application enforce the game's rules.
7. Ensure `ViewFor` does not disclose information a participant is not permitted to see.
8. Add deterministic tests for valid actions, invalid actions, turn handling, terminal outcomes, and participant views.
9. Register the ruleset through the server's supported registration point.
10. Run the server's tests and verify a complete match through the normal protocol.

## Ruleset design requirements

A ruleset should:

- Keep game-specific behavior inside its own package.
- Avoid importing or depending on `server/internal/` packages.
- Never trust a client-supplied player identity as authority.
- Reject invalid actions without corrupting or partially changing state.
- Enforce turn and role rules.
- Correctly report match outcomes.
- Return only the participant-specific state view allowed by the game.
- Handle edge cases and malformed game actions safely.
- Make random behavior deterministic when a seed is supplied or required by the API.
- Document game-specific actions, roles, setup, and special behavior.

## Testing

At minimum, include tests for:
- Initial state and role assignment.
- Legal and illegal actions.
- Out-of-turn actions and invalid actors.
- State transitions and invariants.
- Win, loss, draw, and other terminal conditions.
- Readiness and start conditions.
- Participant-specific views, including hidden information if applicable.
- Any game-specific special actions.

Where feasible, add an integration test that starts a session and completes a match through the server protocol.

## Registering the game

The design specification identifies `cmd/ramserver` as the executable registration point. Follow the current registration pattern in the release you are using. Registration should be the only server executable change needed for a new ruleset; do not modify internal server packages to accommodate game-specific logic.

For an upstream contribution, follow [Game Contribution](GAME-CONTRIBUTION.md) rather than treating this guide as approval to change the official repository.
