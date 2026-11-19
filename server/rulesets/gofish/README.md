# Go Fish

The Go Fish ruleset for RAMServer, implemented as a `game.Game` plugin in
package `gofish`. The server core owns connections, lobbies, and persistence;
this package owns the cards, players, actions, and match outcome.

## Status

The card/deck model, role declarations, readiness checks, dealing, turn
workflow, hand/book helpers, outcome scoring, binary action decoding, and text
action parsing are implemented and tested.

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
- The dealer participates as a player and receives cards in the deal.
- Two or three playing participants receive seven cards; four or five receive five.
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

### Legacy binary payloads

The existing number-tagged payloads remain accepted:

| Action | Payload |
|---|---|
| Ask | `[1, fromOrder, toOrder, rank]` |
| Send | `[2, fromOrder, toOrder, cardCount, ...cardBytes]` |
| Send and top up | `[3, fromOrder, toOrder, drawnCard, cardCount, ...cardBytes]` |
| Place book | `[4, playerOrder, rank]` |
| Draw | `[5, playerOrder, card]` |
| Top up | `[6, playerOrder, card]` |
| End turn | `[7, fromOrder, toOrder, turnNumber]` |
| Deal | `[8, dealerOrder, firstPlayerOrder]` |

Ranks are `1` through `13` from ace to king. Cards are `1` through `52`: spades
`1–13`, hearts `14–26`, diamonds `27–39`, and clubs `40–52`.

### Human-readable text payloads

Clients may submit UTF-8 text actions. Text is interpreted by named, ordered
entries in `text_action.go`. Each entry owns its accepted regular-expression
aliases, named capture fields, field parsers, and action builder. Captures are
normalized into the same internal action and legacy payload used for validation
and state application.

| Action | Examples |
|---|---|
| Ask | `ask player 2 for kings`, `request P2 for K`, `get Kings from P2`, `give me queens from player 1` |
| Send | `send P1: K♠, Q♥`, `give P1: A♠`, `send K♠, Q♥ to P1` |
| Send and top up | `send P1: K♠, Q♥; draw 3♦`, `send P1: K♠; top up 3♦` |
| Place book | `place a book of aces`, `place kings`, `place book K` |
| Draw | `draw 3♦`, `draw K♠` |
| Top up | `top up 3♦`, `top up A♣` |
| End turn | `end`, `end turn`, `pass` |
| Deal | `deal`, `deal cards` |

Player references use one-based display numbers such as `P1` or `player 2`;
the server converts them to zero-based player orders. The authenticated
session identity supplies the actor, so text payloads do not include `from`.
Server-determined values such as the next player and deal participants are also
derived by the server.

Card text accepts ASCII rank and suit forms such as `KS`, `10D`, `A♣`, and
`QH`, Unicode suit symbols `♠`, `♥`, `♦`, and `♣`, and rank names or symbols
such as `kings`, `K`, and `ace`.

Text parsing is strict: a command must match a registered grammar, all captures
must parse, and the normalized action must pass normal turn and legality
validation.

## Views and hidden information

`ViewFor` exposes the requesting player's hand while exposing only hand size
and completed books for other players. The 4-byte public player view is followed
by one byte for each private hand card. The state view contains the player
count, public player views, turn number, pool size, and encoded most recent
action. The most recent action includes transferred cards, so catches remain
visible to clients for verification. `View.Decode` decodes the complete view,
including recent actions.

## Code layout

| File | Contents |
|---|---|
| `go_fish.go` | `Game` implementation, roles, turns, legal actions, and outcomes |
| `state.go` | Players, state, pool helpers, and view encoding |
| `card.go` | Suits, ranks, cards, and deck construction |
| `book.go` | Completed-book representation |
| `action.go` | Action types and wire encodings |
| `text_action.go` | Data-driven text action grammars and field parsers |
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