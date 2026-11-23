# Writing a ruleset

A ruleset teaches the server one game. It is a Go package that implements the `game.Game` interface from
`server/game`. The server core is game-agnostic: it never looks inside your game's states,
actions, or views. It only calls your methods, at the moments described below, and passes the bytes you
produce to clients.

This guide describes the contract the server holds your ruleset to. It is written for external
developers: you never need, and should never try, to look at the server's own code.

## The package

- Import the interface as `github.com/stevenlagoy/ramserver-core/server/game`.
- Put the ruleset in its own package under `server/rulesets/`, for example `server/rulesets/tictactoe`
  with `package tictactoe`, and
  export a type implementing `game.Game`, conventionally `Game`, plus a `Name` constant for its game id:

  ```go
  const Name = "tictactoe" // the game_id clients send in JoinSessionRequest

  type Game struct{}

  var _ game.Game = (*Game)(nil) // compile-time check
  ```

- Register the game in `server/cmd/ramserver/main.go`, next to the other games:

  ```go
  games.Register(tictactoe.Name, func() game.Game { return &tictactoe.Game{} })
  ```

  Clients then select it with `game_id: "tictactoe"`.

## Rules every method must follow

- **A Game holds no per-match data.** One Game value may serve many sessions at once, from different
  goroutines. Everything about a match lives in the `GameState` value you return. A stateless struct is
  the normal choice.
- **States are immutable.** `GameState` is `any`, so Go won't enforce this: never modify a state you were
  given. `Apply` returns a new state. If your state is a struct containing slices, arrays behind
  pointers, or maps, copy them before changing anything. The server may keep earlier states.
- **Be deterministic.** Given the same state and action, return the same result. Don't read the clock or
  random numbers inside `Validate`/`Apply`/`Outcome` (do it in `NewState` if a game needs randomness,
  and store the result in the state).
- **Don't panic.** The server recovers from a panic, but the request that caused it fails with
  `ERROR_CODE_INTERNAL`. Return errors instead.
- **Use member IDs only as opaque strings.** They are the server-issued `player_id`s. Don't parse them.

## The interface, method by method

### `Roles() []RoleSpec`

Lists the roles your game supports and how many members each can hold:

```go
func (g *Game) Roles() []game.RoleSpec {
	return []game.RoleSpec{
		{Name: game.RolePlayer, Min: 2, Max: 2},
		{Name: game.RoleSpectator, Min: 0, Max: 0}, // Max 0 means unlimited
	}
}
```

- When a member joins, the server gives them **the first role in this list that still has room**. Order
  the list accordingly: players first, then anything like spectators.
- If no role has room, the join is refused with `ERROR_CODE_SESSION_NOT_JOINABLE` (lobby full).
- A member keeps their role for as long as they stay in the session.
- `Min` is not enforced by the server. Enforce minimums in `ReadyToStart`.
- `RolePlayer` (`"player"`) and `RoleSpectator` (`"spectator"`) are predefined. Games may define others.
- Clients see each member's role in `SessionPlayer.role`.

### `ReadyToStart(roster map[string]Role, ready map[string]bool) bool`

Decides when a waiting lobby starts its match.

- Called whenever a member changes their ready flag, and whenever a member leaves a waiting lobby.
- `roster` maps every member to their role. `ready` maps every member to whether they have sent
  `SetReadyRequest` with `ready: true`.
- Return true only when a match can really start: for example, exactly two players present and both
  ready. Decide whether spectators need to be ready (usually not).
- When it returns true, the match starts immediately.

### `NewState(rosterOrder []string, roles map[string]Role) GameState`

Creates the state at the start of a match.

- `rosterOrder` lists **every** member of the session, in the order they joined, including spectators.
  Filter by `roles` to find the players.
- Assign seats here, for example the first player is X and moves first. Store everything you will need
  later, including the player IDs, in the state.
- Called again for each new match in the same session (a rematch), with the current roster.

### `ActiveTurn(state GameState) []string`

Returns the member IDs who may act right now.

- The server rejects an action from anyone not in this list with `ERROR_CODE_OUT_OF_TURN`, without
  calling `Validate`, unless the game allows that action out of turn (see `OutOfTurnActions` below).
- Return an empty slice, not `nil`, when nobody may act.
- When exactly one member is returned, clients see them as `StateUpdate.active_player_id`.

### `LegalActions(state GameState, memberID string) []ActionSpec`

Lists the actions `memberID` may take right now. The server uses it to answer `LegalActionsRequest`.

- `Payload` must be **exactly** the bytes a client should send back as `ActionRequest.encoded_action`.
- `Description` is an optional human-readable label, such as `"top-left"`.
- Return an empty slice for members who may not act (spectators, the player waiting for their turn).
- Every payload you list must pass `Validate`; the compliance tests check this.

### `Validate(state GameState, action Action) error`

Decides whether an action is legal. Must not change `state`.

- `action.ActorID` is the sender's ID, set by the server. It can be trusted.
- `action.Payload` is the client's `encoded_action`, exactly as sent. You define its format.
- `action.ID` is the client's request `sequence`. You won't normally need it.
- Return `nil` to accept. Errors map to protocol error codes:

  | Return | Client receives |
  | --- | --- |
  | an error wrapping `game.ErrMalformedAction` | `ERROR_CODE_MALFORMED_MESSAGE` |
  | any other error, usually wrapping `game.ErrIllegalAction` | `ERROR_CODE_ILLEGAL_ACTION` |

  Wrap with `fmt.Errorf("%w: square 4 is taken", game.ErrIllegalAction)`. The error text becomes
  `ErrorResponse.message`, including the `illegal action: ` or `malformed action: ` prefix from the wrapped
  error, so make it useful to a player.
- A rejected action changes nothing and produces no broadcast.

### `Apply(state GameState, action Action) GameState`

Returns the state after a validated action.

- Only called after `Validate` returned `nil` for the same state and action. Don't validate again.
- Must not change the state it was given (see the rules above).
- After every applied action the server broadcasts a fresh `StateUpdate` to every member, then calls
  `Outcome`.

### `Outcome(state GameState) (MatchResult, bool)`

Reports whether the match is over.

- Return `(nil, false)` while play continues.
- When it ends, return a result and `true`. Use `game.Result`:

  ```go
  return game.Result{WinnerIDs: []string{winner}, Reason: "THREE_IN_A_ROW"}, true
  return game.Result{WinnerIDs: nil, Reason: "DRAW"}, true // no winners means a draw
  ```

  Clients receive a `MatchResult` with `winner_player_ids`, `draw` (true when there are no winners),
  `termination_reason` (your `Reason` string), and `turns` (the number of applied actions).
  `Result.Scores` is not currently sent to clients.
- After a match ends, the session returns to waiting, **every member's ready flag is cleared**, and a new
  match starts through `ReadyToStart` and `NewState` once players ready up again.
- If you want a series of games (best of three, and so on), keep it inside one match: track the series
  in your state and only report an outcome when the series is decided.

### `ViewFor(state GameState, memberID string) []byte`

Encodes what `memberID` may see. The bytes become `StateUpdate.encoded_view`.

- Called for every member on every broadcast while a match is in progress. While the lobby is waiting,
  `encoded_view` is empty.
- You define the encoding. Choose one a client can decode easily, document it precisely in your README,
  and keep it stable. For a game with no hidden information, every member gets the same bytes.

## Optional interfaces

Implement these as well if your game needs them; the server detects them automatically.

### `OutOfTurnActions`

```go
AllowsOutOfTurn(state GameState, action Action) bool
```

Return true for actions any member may send at any time, such as resigning. The server skips its turn
check for them; `Validate` is still called and still decides who may really take the action.

### `Annotator`

```go
Annotations(state GameState, memberID string) map[string]string
```

Small key/value facts sent with each in-progress `StateUpdate` as `annotations`, so clients don't have
to decode the view for them: for example `"moves": "5"` or `"winning_line": "0,4,8"`. Document your
keys. Clients ignore keys they don't know.

## The match lifecycle, end to end

1. Members join. Each gets a role from `Roles()`.
2. Members send `SetReadyRequest`. After each change, `ReadyToStart` decides whether to start.
3. Start: `NewState(rosterOrder, roles)`, then every member receives an in-progress `StateUpdate`
   built with `ViewFor` (and `Annotations`, if implemented).
4. A member sends `ActionRequest`. The server checks they are a member, then `ActiveTurn` (unless
   `AllowsOutOfTurn`), then `Validate`. On success: `Apply`, broadcast, `Outcome`.
5. When `Outcome` reports an end, every member receives `MatchResult`, then the lobby is waiting again
   with ready flags cleared. Step 2 starts a rematch.

Membership while a match runs:

- Members cannot join a session whose match has started.
- If a player leaves during a match, the server removes them from the session but does not tell your
  game, and the match is not ended for you. If it may be their turn, the match can get stuck. Giving
  players a way to end a match themselves, such as a resign action through `OutOfTurnActions`, avoids
  this.

## Testing

- Write unit tests for your rules: legal and illegal moves, every way a game can end, and that `Apply`
  never changes its input.
- Run the compliance tests in `server/game/gametest` against your game:

  ```go
  import "github.com/stevenlagoy/ramserver-core/server/game/gametest"

  func TestCompliance(t *testing.T) {
  	gametest.Run(t, &Game{}, gametest.Config{
  		Members: []string{"p1", "p2", "watcher"}, // join order; roles are assigned like the server does
  	})
  }
  ```

  They play complete random matches through the same calls the server makes, and check:

  - `Roles()` is sensible, and `Config.Members` can all be seated.
  - `ReadyToStart` starts a full, ready roster, but not before anyone is ready, not with an empty
    roster, and not below a role's `Min`.
  - Every action `LegalActions` offers passes `Validate`, and is offered only to a member whose turn it
    is (or one the game allows out of turn).
  - `Validate` rejects payloads `LegalActions` doesn't offer that member, and empty or oversized
    payloads. Its errors wrap `game.ErrIllegalAction` or `game.ErrMalformedAction`.
  - `Validate` and `Apply` never change the state they are given, and `Apply` is deterministic.
  - `ActiveTurn` never returns `nil`, and is empty once the match has ended.
  - Every match ends within `Config.MaxActions` actions, with a result whose winners are members and
    whose `TerminationReason` is set.
  - No method panics.

  Runs are reproducible: the same `Config` plays the same matches. Change `Config.Seed` to explore
  others. A failure names the match and the number of actions before it, so add a unit test for that
  position once you've found the bug.
- Test with real clients: once it is registered, run the server and connect two clients, for example
  two instances of leanclient (`server/cmd/leanclient`), and play a match.

## Document your game

Your README is the only description client developers get. Include:

- the `game_id`,
- the roles, player count, and start condition,
- the exact `encoded_action` and `encoded_view` formats, with examples,
- your annotation keys,
- every `termination_reason` you produce,
- the error messages a player can trigger.
