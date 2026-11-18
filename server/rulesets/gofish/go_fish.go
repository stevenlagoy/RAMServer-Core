package gofish

import (
	"fmt"
	"slices"

	"github.com/stevenlagoy/ramserver-core/server/game"
)

const Name = "go fish"

type Game struct{}

var _ game.Game = (*Game)(nil) // Compile-time check that Go Fish implements Game

const RoleDealer = "Dealer"

func (g *Game) Roles() []game.RoleSpec {
	return []game.RoleSpec{
		{Name: game.RolePlayer, Min: 2, Max: 0},    // Unlimited
		{Name: RoleDealer, Min: 1, Max: 1},         // Dealers act like players most of the time
		{Name: game.RoleSpectator, Min: 0, Max: 0}, // Optional, Unlimited
	}
}

func (g *Game) NewState(order []string, roles map[string]game.Role) game.GameState {
	players := make([]Player, len(order))
	for order, id := range order {
		if roles[id] != game.RoleSpectator {
			players = append(players, Player{PlayerOutsideView{id, 0, []Book{}, order}, []*Card{}})
		}
	}
	if len(players) < 3 {
		panic(fmt.Sprintf("gofish: NewState needs 3 players, got %d", len(players)))
	}
	return State{
		players,
		0,
		makeCards(),
		nil,
	}
}

func (g *Game) Validate(s game.GameState, a game.Action) error {
	state := s.(State)
	// Require that action is present in LegalActions
	for _, legal := range g.LegalActions(state, a.ActorID) {
		for i, b := range a.Payload {
			if legal.Payload[i] != b {
				return fmt.Errorf("could not validate an action")
			}
		}
	}
	return nil
}

func (g *Game) Apply(s game.GameState, a game.Action) game.GameState {
	state := s.(State)
	decoded, _, err := DecodeAction(a.Payload)
	if err != nil {
		return State{
			state.players,
			state.turnNumber,
			state.cards,
			state.recentAction,
		}
	}
	switch action := decoded.(type) {
	case CardRequest:
		// No changes
		return State{
			state.players,
			state.turnNumber,
			state.cards,
			action,
		}
	case CardResponse:
		for _, card := range action.cards {
			// Remove the cards from the requestee's hand
			action.from.hand = slices.DeleteFunc(
				action.from.hand,
				func(c *Card) bool {
					return c.symbol() == card.symbol()
				},
			)
			// Add the cards to the requester's hand
			action.to.hand = append(action.to.hand, card)
		}
		return State{
			state.players,
			state.turnNumber,
			state.cards,
			action,
		}
	case CardResponseAndTopUp:
		// Remove the pool card from the pool
		// Add the pool card to the requestee's hand

		for _, card := range action.cards {
			// Remove the cards from the requestee's hand
			action.From().hand = slices.DeleteFunc(
				action.From().hand,
				func(c *Card) bool {
					return c.symbol() == card.symbol()
				},
			)
			// Add the cards to the requester's hand
			action.to.hand = append(action.to.hand, card)
		}
		return State{
			state.players,
			state.turnNumber,
			state.cards,
			action,
		}
	case DrawFromPool:
		// Remove the card from the pool
		// Add the card to the drawer's hand
		action.From().hand = append(action.From().hand, action.card)
		return State{
			state.players,
			state.turnNumber,
			state.cards,
			action,
		}
	case PlaceBook:
		// Remove the cards from the placer's hand
		action.from.hand = slices.DeleteFunc(
			action.from.hand,
			func(c *Card) bool {
				return c.Rank == action.rank
			},
		)
		// Add the book to the placer's books
		action.from.books = append(action.from.books, Book{action.rank, action.from.PlayerOutsideView})
		return State{
			state.players,
			state.turnNumber,
			state.cards,
			action,
		}
	case EndTurn:
		// Increment turnNumber
		return State{
			state.players,
			state.turnNumber + 1,
			state.cards,
			action,
		}
	}
	// Unreachable
	return State{
		state.players,
		state.turnNumber,
		state.cards,
		state.recentAction,
	}
}

func (g *Game) AllowsOutOfTurn(_ game.GameState, a game.Action) bool {
	return false // No actions are allowed out-of-turn
}

func (g *Game) ActiveTurn(s game.GameState) []string {
	return []string{s.(State).recentAction.To().id}
}

func (g *Game) ReadyToStart(roster map[string]game.Role, ready map[string]bool) bool {
	players := 0
	for id, role := range roster {
		if role == game.RoleSpectator {
			continue // spectators do not block starting the game
		} // Role is Player or Dealer
		if !ready[id] {
			return false
		}
		players++
	}
	return players == 2
}

func (g *Game) Outcome(s game.GameState) (result game.MatchResult, complete bool) {
	state := s.(State)
	// Play continues as long as any players hold cards or there are cards in the pool
	if len(state.cardsInPool()) > 0 {
		return nil, false // Cards left to draw
	}
	for _, player := range state.players {
		if len(player.hand) > 0 {
			return nil, false // Players still hold cards
		}
	} // No players have any cards, the game is over
	// Score players by their number of books
	books := make(map[*Player]int, len(state.players))
	for _, player := range state.players {
		books[&player] = len(player.books)
	}
	return Outcome{books}, true
}

func (g *Game) ViewFor(s game.GameState, playerID string) []byte {
	view, err := s.(State).viewFor(playerID)
	if err != nil {
		panic(err)
	}
	return view.Encode()
}

/*
A player may:

	If they've been requested for cards:
		If they hold any cards of the requested rank:
			Send all cards of the requested rank
		If they hold no cards of the requested rank:
			Say "Go Fish"
		If they hold no cards:
			Draw a card from the pool
		Yield to the requester
	If it is their true turn and they hold no cards:
		Draw a card from the pool, and restart their own turn (may send a request)
	If it is their true turn, and they hold cards:
		Request cards of a held rank from an opponent who has cards, and yield to the requestee
		If their request has received a response with cards:
			Restart their own turn (may send another request to any opponent)
		If their request has received a "Go Fish" response:
			Draw a card from the pool
		If hold 4 cards of any rank: place all complete books
		If hold no cards:
			Draw a card from the pool
		Pass to the next player in order
*/
func (g *Game) LegalActions(s game.GameState, playerID string) []game.ActionSpec {
	state := s.(State)
	if state.CurrentTurn().id != playerID {
		return []game.ActionSpec{} // No actions are allowed out-of-turn
	} // It is this player's active turn
	player := state.CurrentTurn()
	switch action := state.recentAction.(type) {
	case CardRequest: // Have been requested for cards
		// Determine if player holds any requested cards
		cards := []*Card{}
		for _, card := range player.hand {
			if card.Rank == action.rank {
				cards = append(cards, card)
			}
		}
		if len(cards) == 0 { // Do we have any requested cards?
			// Go fish!
			return []game.ActionSpec{game.ActionSpec{
				Payload:     CardResponse{player, action.from, []*Card{}}.Encode(),
				Description: fmt.Sprintf("Tell Player %d to Go Fish!", action.from.order+1),
			}}
		} // We have a requested card
		if len(player.hand) == len(cards) { // There would be no cards left after responding to the request
			return []game.ActionSpec{game.ActionSpec{
				Payload: CardResponseAndTopUp{
					CardResponse{player, action.from, cards},
					TopUpFromPool{player, state.topOfPool()},
				}.Encode(),
				Description: fmt.Sprintf(
					"Send the requested %s to Player %d, and draw one card from the pool",
					action.rank.plural, action.from.order+1,
				),
			}}
		} // Have more cards than were requested
		return []game.ActionSpec{game.ActionSpec{
			Payload: CardResponse{player, action.from, cards}.Encode(),
			Description: fmt.Sprintf(
				"Send the requested %s to Player %d",
				action.rank.plural, action.from.order+1,
			),
		}}
	case CardResponse, CardResponseAndTopUp:
		var resp CardResponse
		switch v := action.(type) {
		case CardResponse:
			resp = v
		case CardResponseAndTopUp:
			resp = v.CardResponse
		}
		if len(resp.cards) > 0 { // The response contains cards
			// Must request cards again from another player
			possibleRequests := state.AllValidRequestsBy(player)
			legals := make([]game.ActionSpec, len(possibleRequests))
			for _, request := range possibleRequests {
				legals = append(legals, game.ActionSpec{
					Payload:     request.Encode(),
					Description: fmt.Sprintf("Request all %s from Player %d", request.rank.plural, request.to.order+1),
				})
			}
			return legals
		} else { // Go Fish!
			// Draw from pool
			return []game.ActionSpec{game.ActionSpec{
				Payload:     DrawFromPool{player, state.topOfPool()}.Encode(),
				Description: "Draw a card from the pool",
			}}
		}
	case DrawFromPool:
		heldBooks := player.ValidHeldBooks()
		if len(heldBooks) > 0 { // Any valid held books?
			// Must place a book
			return []game.ActionSpec{game.ActionSpec{
				Payload:     PlaceBook{player, heldBooks[0]}.Encode(),
				Description: "Place a book of " + heldBooks[0].plural,
			}}
		} // No valid held books
		// End turn
		return []game.ActionSpec{game.ActionSpec{
			Payload:     EndTurn{player, state.NextTurn(), state.turnNumber + 1}.Encode(),
			Description: "End your turn",
		}}
	case PlaceBook:
		if len(player.hand) == 0 { // Is our hand empty?
			// Top up from pool
			return []game.ActionSpec{game.ActionSpec{
				Payload:     TopUpFromPool{player, state.topOfPool()}.Encode(),
				Description: "Draw a card from the pool",
			}}
		} // We have cards in hand
		heldBooks := player.ValidHeldBooks()
		if len(heldBooks) > 0 { // Any valid held books?
			// Must place a book
			return []game.ActionSpec{game.ActionSpec{
				Payload:     PlaceBook{player, heldBooks[0]}.Encode(),
				Description: "Place a book of " + heldBooks[0].plural,
			}}
		} // No valid held books
		// End turn
		return []game.ActionSpec{game.ActionSpec{
			Payload:     EndTurn{player, state.NextTurn(), state.turnNumber + 1}.Encode(),
			Description: "End your turn",
		}}
	case EndTurn, nil: // True turn, or the start of the game
		// Must request cards from another player with cards
		possibleRequests := state.AllValidRequestsBy(player)
		legals := make([]game.ActionSpec, len(possibleRequests))
		for _, request := range possibleRequests {
			legals = append(legals, game.ActionSpec{
				Payload:     request.Encode(),
				Description: fmt.Sprintf("Request all %s from Player %d", request.rank.plural, request.to.order+1),
			})
		}
		return legals
	}
	panic("the most recent action was of an invalid type; cannot generate legal actions")
}
