package gofish

import (
	"bytes"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/stevenlagoy/ramserver-core/server/game"
)

const Name = "go fish"

type Game struct{}

var _ game.Game = (*Game)(nil) // Compile-time check that Go Fish implements Game
var _ game.Annotator = (*Game)(nil)

const RoleDealer = "Dealer"

func (g *Game) Roles() []game.RoleSpec {
	return []game.RoleSpec{
		{Name: game.RolePlayer, Min: 2, Max: 5},
		{Name: RoleDealer, Min: 1, Max: 1},         // Dealers act like players most of the time
		{Name: game.RoleSpectator, Min: 0, Max: 0}, // Optional, Unlimited
	}
}

func (g *Game) OwnerRole() game.Role {
	return RoleDealer
}

func (g *Game) Annotations(s game.GameState, memberID string) map[string]string {
	state := s.(State)
	view, err := state.viewFor(memberID)
	if err != nil {
		return nil
	}
	hand := make([]string, 0, len(view.self.hand))
	for _, card := range view.self.hand {
		hand = append(hand, card.String())
	}
	if len(hand) == 0 {
		hand = append(hand, "none")
	}
	books := make([]string, 0, len(view.self.books))
	for _, book := range view.self.books {
		books = append(books, book.rank.symbol)
	}
	if len(books) == 0 {
		books = append(books, "none")
	}
	annotations := map[string]string{
		"hand":      strings.Join(hand, ","),
		"books":     strings.Join(books, ","),
		"pool_size": strconv.Itoa(view.poolSize),
		"turn":      strconv.Itoa(view.turnNumber),
	}
	for i, player := range view.players {
		prefix := fmt.Sprintf("player_%d_", i+1)
		annotations[prefix+"hand_size"] = strconv.Itoa(player.handSize)
		playerBooks := make([]string, 0, len(player.books))
		for _, book := range player.books {
			playerBooks = append(playerBooks, book.rank.symbol)
		}
		if len(playerBooks) == 0 {
			playerBooks = append(playerBooks, "none")
		}
		annotations[prefix+"books"] = strings.Join(playerBooks, ",")
	}
	return annotations
}

func (g *Game) NewState(order []string, roles map[string]game.Role) game.GameState {
	players := make([]Player, 0, len(order))
	nextOrder := 0
	for _, id := range order {
		if roles[id] != game.RoleSpectator && roles[id] != RoleDealer {
			players = append(players, Player{PlayerOutsideView{id, 0, []Book{}, nextOrder}, []*Card{}})
			nextOrder++
		}
	}
	for _, id := range order {
		if roles[id] == RoleDealer {
			players = append(players, Player{PlayerOutsideView{id, 0, []Book{}, nextOrder}, []*Card{}})
			nextOrder++
		}
	}
	if len(players) < 3 || len(players) > 6 {
		panic(fmt.Sprintf("gofish: NewState needs 2 to 5 players and one dealer, got %d participants", len(players)))
	}
	cards := makeCards()
	return State{
		players,
		0,
		cards,
		nil,
	}
}

func (g *Game) Validate(s game.GameState, a game.Action) error {
	state := s.(State)
	normalized, err := state.decodeClientAction(a.ActorID, a.Payload)
	if err != nil {
		return err
	}
	for _, legal := range g.LegalActions(state, a.ActorID) {
		if bytes.Equal(legal.Payload, normalized.Encode()) {
			return nil
		}
	}
	return fmt.Errorf("could not validate an action")
}

func (g *Game) Apply(s game.GameState, a game.Action) game.GameState {
	state := s.(State)
	decoded, err := state.decodeClientAction(a.ActorID, a.Payload)
	if err != nil {
		return state
	}
	next := state.clone()
	decoded = next.bindAction(decoded)
	switch action := decoded.(type) {
	case CardRequest:
		next.recentAction = action
	case DealCards:
		next.deal()
		next.recentAction = action
	case CardResponse:
		next.transfer(action.from, action.to, action.cards)
		next.recentAction = action
	case CardResponseAndTopUp:
		next.transfer(action.CardResponse.from, action.CardResponse.to, action.cards)
		next.draw(action.TopUpFromPool.from, action.card)
		next.recentAction = action
	case DrawFromPool:
		next.draw(action.from, action.card)
		next.recentAction = action
	case TopUpFromPool:
		next.draw(action.from, action.card)
		next.recentAction = action
	case PlaceBook:
		action.from.hand = slices.DeleteFunc(
			action.from.hand,
			func(c *Card) bool {
				return c.Rank == action.rank
			},
		)
		action.from.handSize = len(action.from.hand)
		action.from.books = append(action.from.books, Book{action.rank, action.from.PlayerOutsideView})
		next.recentAction = action
	case EndTurn:
		next.turnNumber++
		next.recentAction = action
	}
	return next
}

func (s State) decodeClientAction(actorID string, payload []byte) (Action, error) {
	decoded, consumed, err := DecodeAction(payload)
	if err == nil && consumed == len(payload) {
		return decoded, nil
	}
	return ParseTextAction(s, actorID, payload)
}

func (s State) clone() State {
	players := append([]Player(nil), s.players...)
	for i := range players {
		players[i].hand = append([]*Card(nil), s.players[i].hand...)
		players[i].books = append([]Book(nil), s.players[i].books...)
	}
	return State{
		players:      players,
		turnNumber:   s.turnNumber,
		cards:        append([]Card(nil), s.cards...),
		recentAction: s.recentAction,
	}
}

func (s State) bindAction(action Action) Action {
	player := func(order int) *Player {
		for i := range s.players {
			if s.players[i].order == order {
				return &s.players[i]
			}
		}
		return &Player{PlayerOutsideView: PlayerOutsideView{order: order}}
	}
	switch a := action.(type) {
	case CardRequest:
		a.from, a.to = player(a.from.order), player(a.to.order)
		return a
	case CardResponse:
		a.from, a.to = player(a.from.order), player(a.to.order)
		return a
	case CardResponseAndTopUp:
		a.CardResponse.from, a.CardResponse.to = player(a.CardResponse.from.order), player(a.CardResponse.to.order)
		a.TopUpFromPool.from = a.CardResponse.from
		return a
	case PlaceBook:
		a.from = player(a.from.order)
		return a
	case DrawFromPool:
		a.from = player(a.from.order)
		return a
	case TopUpFromPool:
		a.from = player(a.from.order)
		return a
	case EndTurn:
		a.from, a.to = player(a.from.order), player(a.to.order)
		return a
	case DealCards:
		a.dealer, a.first = player(a.dealer.order), player(a.first.order)
		return a
	default:
		return action
	}
}

func (s *State) deal() {
	players := s.players
	perPlayer := 5
	if len(players) <= 4 {
		perPlayer = 7
	}
	cardIndex := 0
	for round := 0; round < perPlayer; round++ {
		for i := range players {
			card := &s.cards[cardIndex]
			card.Holder = &players[i]
			players[i].hand = append(players[i].hand, card)
			players[i].handSize = len(players[i].hand)
			cardIndex++
		}
	}
}

func (s *State) transfer(from, to *Player, requested []*Card) {
	for _, wanted := range requested {
		for i, held := range from.hand {
			if held.symbol() == wanted.symbol() {
				from.hand = append(from.hand[:i], from.hand[i+1:]...)
				to.hand = append(to.hand, held)
				break
			}
		}
	}
	from.handSize, to.handSize = len(from.hand), len(to.hand)
}

func (s *State) draw(player *Player, wanted *Card) {
	for i := range s.cards {
		if s.cards[i].Holder == nil && s.cards[i].symbol() == wanted.symbol() {
			s.cards[i].Holder = player
			player.hand = append(player.hand, &s.cards[i])
			player.handSize = len(player.hand)
			return
		}
	}
}

func (g *Game) AllowsOutOfTurn(_ game.GameState, a game.Action) bool {
	return false // No actions are allowed out-of-turn
}

func (g *Game) ActiveTurn(s game.GameState) []string {
	state := s.(State)
	if state.recentAction == nil {
		return []string{state.players[len(state.players)-1].id}
	}
	return []string{state.recentAction.To().id}
}

func (g *Game) ReadyToStart(roster map[string]game.Role, ready map[string]bool) bool {
	players := 0
	dealers := 0
	for id, role := range roster {
		if role == game.RoleSpectator {
			continue // spectators do not block starting the game
		}
		if !ready[id] {
			return false
		}
		switch role {
		case game.RolePlayer:
			players++
		case RoleDealer:
			dealers++
		}
	}
	return players >= 2 && players <= 5 && dealers == 1
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
	if state.recentAction == nil {
		dealer := state.players[len(state.players)-1]
		if dealer.id != playerID {
			return []game.ActionSpec{}
		}
		return []game.ActionSpec{{
			Payload:     (DealCards{dealer: &dealer, first: &state.players[0]}).Encode(),
			Description: "Deal cards",
		}}
	}
	player := state.CurrentTurn()
	if state.recentAction != nil {
		player = state.recentAction.To()
	}
	if player.id != playerID {
		return []game.ActionSpec{} // No actions are allowed out-of-turn
	}
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
			return []game.ActionSpec{{
				Payload:     CardResponse{player, action.from, []*Card{}}.Encode(),
				Description: fmt.Sprintf("Tell Player %d to Go Fish!", action.from.order+1),
			}}
		} // We have a requested card
		if len(player.hand) == len(cards) { // There would be no cards left after responding to the request
			return []game.ActionSpec{{
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
		return []game.ActionSpec{{
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
			legals := make([]game.ActionSpec, 0, len(possibleRequests))
			for _, request := range possibleRequests {
				legals = append(legals, game.ActionSpec{
					Payload:     request.Encode(),
					Description: fmt.Sprintf("Request all %s from Player %d", request.rank.plural, request.to.order+1),
				})
			}
			return legals
		} else { // Go Fish!
			// Draw from pool
			return []game.ActionSpec{{
				Payload:     DrawFromPool{player, state.topOfPool()}.Encode(),
				Description: "Draw a card from the pool",
			}}
		}
	case DrawFromPool:
		heldBooks := player.ValidHeldBooks()
		if len(heldBooks) > 0 { // Any valid held books?
			// Must place a book
			return []game.ActionSpec{{
				Payload:     PlaceBook{player, heldBooks[0]}.Encode(),
				Description: "Place a book of " + heldBooks[0].plural,
			}}
		} // No valid held books
		// End turn
		return []game.ActionSpec{{
			Payload:     EndTurn{player, state.NextTurn(), state.turnNumber + 1}.Encode(),
			Description: "End your turn",
		}}
	case PlaceBook:
		if len(player.hand) == 0 { // Is our hand empty?
			// Top up from pool
			return []game.ActionSpec{{
				Payload:     TopUpFromPool{player, state.topOfPool()}.Encode(),
				Description: "Draw a card from the pool",
			}}
		} // We have cards in hand
		heldBooks := player.ValidHeldBooks()
		if len(heldBooks) > 0 { // Any valid held books?
			// Must place a book
			return []game.ActionSpec{{
				Payload:     PlaceBook{player, heldBooks[0]}.Encode(),
				Description: "Place a book of " + heldBooks[0].plural,
			}}
		} // No valid held books
		// End turn
		return []game.ActionSpec{{
			Payload:     EndTurn{player, state.NextTurn(), state.turnNumber + 1}.Encode(),
			Description: "End your turn",
		}}
	case DealCards, EndTurn: // True turn after dealing, or the start of a new turn
		// Must request cards from another player with cards
		possibleRequests := state.AllValidRequestsBy(player)
		legals := make([]game.ActionSpec, 0, len(possibleRequests))
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
