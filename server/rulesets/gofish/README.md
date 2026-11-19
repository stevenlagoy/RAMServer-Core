# Go Fish

The Go Fish ruleset for RAMServer, implemented as a `game.Game` plugin in
package `gofish`. The server core owns connections, lobbies, and persistence;
this package owns the cards, players, actions, and match outcome.

## Status

The ruleset is under development. The card/deck model, role declarations,
readiness checks, hand/book helpers, outcome scoring, and action encodings are
implemented and tested. Action decoding and the complete deal/turn workflow are
not yet implemented, so Go Fish is not ready for production matches.

## Roles and players

| Role | Count | Notes |
|---|---:|---|
| player | 2 or more | Players take turns requesting cards and placing books |
| Dealer | exactly 1 | Participates in the game like a player |
| spectator | unlimited | Spectators do not need to be ready |

A match may start when both players and the dealer are ready. `NewState`
requires at least three non-spectator participants and preserves their join
order. Spectators are excluded from the player list.

## Rules model

- The deck contains one standard 52-card deck.
- Cards are encoded as one byte, with unique values for each suit/rank pair.
- A book is four cards of the same rank.
- A player may request cards only for a rank in their hand.
- A player with four cards of one rank may place that book.
- The outcome scores each player by the number of books; the highest score wins.
- A match ends when no cards remain in the pool and no player holds cards.

## Actions

The ruleset defines encodings for:

- card requests and responses;
- card responses which also draw from the pool;
- placing a book;
- drawing or topping up from the pool; and
- ending a turn.

Actions use a one-byte type tag followed by the fields described above.
`DecodeAction` validates and decodes these payloads, and `Apply` resolves the
encoded player orders against the current state before applying an action.

## Views and hidden information

`ViewFor` is intended to expose the requesting player's hand while exposing
only hand size and completed books for other players. The wire representation
is defined in `state.go`; an initial state has no recent action.

## Code layout

| File | Contents |
|---|---|
| `go_fish.go` | `Game` implementation, roles, turns, legal actions, and outcomes |
| `state.go` | Players, state, pool helpers, and view encoding |
| `card.go` | Suits, ranks, cards, and deck construction |
| `book.go` | Completed-book representation |
| `action.go` | Action types and wire encodings |
| `outcome.go` | Book scoring and termination reason |
| `gofish_test.go` | Ruleset, card, action, and outcome tests |

## Testing

Run from the `server/` directory:

```bash
go test ./rulesets/gofish
```

The tests cover role declarations, readiness, initial state construction,
deck uniqueness and card round trips, invalid card payloads, hand/book
calculation, action encodings, and outcome scoring. Add a regression test for
each newly implemented rule before extending the gameplay flow.