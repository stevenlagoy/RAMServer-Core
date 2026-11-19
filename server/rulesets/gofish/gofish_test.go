package gofish

import (
	"bytes"
	"fmt"
	"sort"
	"testing"

	"github.com/stevenlagoy/ramserver-core/server/game"
)

func TestRolesDeclarePlayersDealerAndSpectators(t *testing.T) {
	got := (&Game{}).Roles()
	want := []game.RoleSpec{
		{Name: game.RolePlayer, Min: 2, Max: 5},
		{Name: RoleDealer, Min: 1, Max: 1},
		{Name: game.RoleSpectator, Min: 0, Max: 0},
	}
	if len(got) != len(want) {
		t.Fatalf("Roles() returned %d roles, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Roles()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestReadyToStartRequiresTwoReadyPlayersAndDealer(t *testing.T) {
	g := &Game{}
	roster := map[string]game.Role{
		"alice":   game.RolePlayer,
		"bob":     game.RolePlayer,
		"dealer":  RoleDealer,
		"watcher": game.RoleSpectator,
	}
	tests := []struct {
		name  string
		ready map[string]bool
		want  bool
	}{
		{"all required members ready", map[string]bool{"alice": true, "bob": true, "dealer": true}, true},
		{"player not ready", map[string]bool{"alice": true, "dealer": true}, false},
		{"dealer not ready", map[string]bool{"alice": true, "bob": true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := g.ReadyToStart(roster, tt.ready); got != tt.want {
				t.Errorf("ReadyToStart() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReadyToStartSupportsEveryValidPlayerCount(t *testing.T) {
	for players := 1; players <= 6; players++ {
		roster := make(map[string]game.Role, players+1)
		ready := make(map[string]bool, players+1)
		for i := 0; i < players; i++ {
			id := fmt.Sprintf("player-%d", i)
			roster[id] = game.RolePlayer
			ready[id] = true
		}
		roster["dealer"] = RoleDealer
		ready["dealer"] = true
		want := players >= 2 && players <= 5
		if got := (&Game{}).ReadyToStart(roster, ready); got != want {
			t.Errorf("%d players: ReadyToStart() = %v, want %v", players, got, want)
		}
	}
}

func TestAnnotationsDescribeSelfAndPublicPlayerState(t *testing.T) {
	state := testState(
		[]*Card{{Suit: Spades, Rank: Ace}, {Suit: Hearts, Rank: King}},
		[]*Card{{Suit: Diamonds, Rank: Three}},
		nil,
		[]*Card{{Suit: Clubs, Rank: Queen}},
	)
	state.turnNumber = 2
	state.players[0].books = []Book{{rank: Ace, player: state.players[0].PlayerOutsideView}}
	state.players[1].handSize = len(state.players[1].hand)

	got := (&Game{}).Annotations(state, "alice")
	want := map[string]string{
		"hand":               "A♠,K♥",
		"books":              "A",
		"pool_size":          "1",
		"turn":               "2",
		"player_1_hand_size": "2",
		"player_1_books":     "A",
		"player_2_hand_size": "1",
		"player_2_books":     "none",
		"player_3_hand_size": "0",
		"player_3_books":     "none",
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("Annotations()[%q] = %q, want %q", key, got[key], value)
		}
	}
}

func TestNewStateCreatesOneDeckAndExcludesSpectators(t *testing.T) {
	g := &Game{}
	state := g.NewState(
		[]string{"alice", "watcher", "bob", "dealer"},
		map[string]game.Role{
			"alice":   game.RolePlayer,
			"watcher": game.RoleSpectator,
			"bob":     game.RolePlayer,
			"dealer":  RoleDealer,
		},
	).(State)
	if len(state.players) != 3 {
		t.Fatalf("NewState created %d players, want 3", len(state.players))
	}
	if state.players[0].id != "alice" || state.players[1].id != "bob" || state.players[2].id != "dealer" {
		t.Errorf("players = %q, %q, %q; want join order excluding spectator",
			state.players[0].id, state.players[1].id, state.players[2].id)
	}
	if len(state.cards) != 52 {
		t.Errorf("NewState created %d cards, want 52", len(state.cards))
	}
	if len(state.cardsInPool()) != 52 || state.topOfPool() == nil {
		t.Errorf("initial pool = %d cards, want 52 and a top card", len(state.cardsInPool()))
	}
	for _, player := range state.players {
		if len(player.hand) != 0 {
			t.Errorf("%s has %d cards before dealing, want 0", player.id, len(player.hand))
		}
	}
	if got := (&Game{}).ActiveTurn(state); len(got) != 1 || got[0] != "dealer" {
		t.Errorf("initial ActiveTurn() = %v, want [dealer]", got)
	}
	if legal := (&Game{}).LegalActions(state, "dealer"); len(legal) != 1 {
		t.Errorf("dealer has %d initial actions, want one deal action", len(legal))
	}
}

func TestNextTurnWrapsAndSkipsEmptyHands(t *testing.T) {
	state := testState(
		[]*Card{{Suit: Spades, Rank: Ace}},
		nil,
		[]*Card{{Suit: Hearts, Rank: King}},
		nil,
	)
	state.turnNumber = 2

	next := state.NextTurn()
	if next == nil || next.id != "alice" {
		t.Fatalf("NextTurn() = %v, want alice after wrapping from dealer", next)
	}

	state.players[0].hand = nil
	state.players[0].handSize = 0
	state.players[1].hand = nil
	state.players[1].handSize = 0
	state.turnNumber = 2
	if next = state.NextTurn(); next == nil || next.id != "dealer" {
		t.Fatalf("NextTurn() = %v, want dealer when bob and alice are empty", next)
	}
}

func TestEndTurnTextActionsWrapToFirstPlayer(t *testing.T) {
	state := testState(
		[]*Card{{Suit: Spades, Rank: Ace}},
		[]*Card{{Suit: Hearts, Rank: King}},
		[]*Card{{Suit: Diamonds, Rank: Queen}},
		nil,
	)
	state.turnNumber = 2

	for _, text := range []string{"end", "end turn", "end your turn", "pass"} {
		action, err := ParseTextAction(state, "dealer", []byte(text))
		if err != nil {
			t.Errorf("ParseTextAction(%q): %v", text, err)
			continue
		}
		end := action.(EndTurn)
		if end.to.id != "alice" {
			t.Errorf("ParseTextAction(%q) targets %q, want alice", text, end.to.id)
		}
	}
}

func TestDealCardsSupportsEveryValidPlayerCount(t *testing.T) {
	for players := 2; players <= 5; players++ {
		t.Run(fmt.Sprintf("%d players", players), func(t *testing.T) {
			order := make([]string, 0, players+1)
			roles := make(map[string]game.Role, players+1)
			for i := 0; i < players; i++ {
				id := fmt.Sprintf("player-%d", i)
				order = append(order, id)
				roles[id] = game.RolePlayer
			}
			order = append(order, "dealer")
			roles["dealer"] = RoleDealer
			state := (&Game{}).NewState(order, roles).(State)
			deal := (&Game{}).LegalActions(state, "dealer")[0]
			if err := (&Game{}).Validate(state, game.Action{ActorID: "dealer", Payload: deal.Payload}); err != nil {
				t.Fatalf("Validate(deal) returned %v", err)
			}
			next := (&Game{}).Apply(state, game.Action{ActorID: "dealer", Payload: deal.Payload}).(State)
			wantHandSize := 5
			if players <= 3 {
				wantHandSize = 7
			}
			for i := range next.players {
				if got := len(next.players[i].hand); got != wantHandSize {
					t.Errorf("%s has %d cards, want %d", next.players[i].id, got, wantHandSize)
				}
				for round := 0; round < wantHandSize; round++ {
					dealt := next.players[i].hand[round]
					expected := next.cards[round*len(next.players)+i]
					if dealt.symbol() != expected.symbol() {
						t.Errorf("%s round %d received %s, want %s", next.players[i].id, round, dealt.symbol(), expected.symbol())
					}
				}
			}
			if got, want := len(next.cardsInPool()), 52-len(next.players)*wantHandSize; got != want {
				t.Errorf("pool has %d cards, want %d", got, want)
			}
			if got := (&Game{}).ActiveTurn(next); len(got) != 1 || got[0] != "player-0" {
				t.Errorf("turn after deal = %v, want [player-0]", got)
			}
		})
	}
}

func TestDeckCardsRoundTripThroughEncoding(t *testing.T) {
	cards := makeCards()
	seen := make(map[uint8]bool, len(cards))
	for _, card := range cards {
		encoded := card.Encode()
		if seen[encoded] {
			t.Fatalf("duplicate card encoding %d", encoded)
		}
		seen[encoded] = true
		decoded, err := DecodeCard([]byte{encoded})
		if err != nil {
			t.Fatalf("DecodeCard(%d): %v", encoded, err)
		}
		if decoded.Suit != card.Suit || decoded.Rank != card.Rank {
			t.Errorf("DecodeCard(%d) = %+v, want %+v", encoded, decoded, card)
		}
	}
}

func TestDecodeCardRejectsInvalidPayloads(t *testing.T) {
	for _, payload := range [][]byte{nil, {}, {0}, {53}, {1, 2}} {
		if _, err := DecodeCard(payload); err == nil {
			t.Errorf("DecodeCard(%v) accepted invalid payload", payload)
		}
	}
}

func TestPlayerRanksAndBooks(t *testing.T) {
	player := Player{PlayerOutsideView: PlayerOutsideView{id: "alice"}}
	player.hand = []*Card{
		{Suit: Spades, Rank: Ace},
		{Suit: Hearts, Rank: Ace},
		{Suit: Diamonds, Rank: Ace},
		{Suit: Clubs, Rank: Ace},
		{Suit: Spades, Rank: King},
	}
	ranks := player.HeldRanks()
	sort.Slice(ranks, func(i, j int) bool { return ranks[i].value < ranks[j].value })
	if len(ranks) != 2 || ranks[0] != Ace || ranks[1] != King {
		t.Errorf("HeldRanks() = %+v, want ace and king", ranks)
	}
	if books := player.ValidHeldBooks(); len(books) != 1 || books[0] != Ace {
		t.Errorf("ValidHeldBooks() = %+v, want ace", books)
	}
}

func TestActionEncodings(t *testing.T) {
	from := &Player{PlayerOutsideView: PlayerOutsideView{order: 1}}
	to := &Player{PlayerOutsideView: PlayerOutsideView{order: 2}}
	card := &Card{Suit: Hearts, Rank: Queen}
	tests := []struct {
		name string
		got  []byte
		want []byte
	}{
		{"request", (CardRequest{from: from, to: to, rank: Queen}).Encode(), []byte{actionCardRequest, 1, 2, 12}},
		{"response", (CardResponse{from: from, to: to, cards: []*Card{card}}).Encode(), []byte{actionCardResponse, 1, 2, 1, card.Encode()}},
		{"place book", (PlaceBook{from: from, rank: Queen}).Encode(), []byte{actionPlaceBook, 1, 12}},
		{"draw", (DrawFromPool{from: from, card: card}).Encode(), []byte{actionDrawFromPool, 1, card.Encode()}},
		{"end turn", (EndTurn{from: from, to: to, newOrder: 3}).Encode(), []byte{actionEndTurn, 1, 2, 3}},
	}
	for _, tt := range tests {
		if !bytes.Equal(tt.got, tt.want) {
			t.Errorf("%s encoding = %v, want %v", tt.name, tt.got, tt.want)
		}
	}
}

func TestDecodeActionRoundTripsEveryActionType(t *testing.T) {
	from := &Player{PlayerOutsideView: PlayerOutsideView{order: 1}}
	to := &Player{PlayerOutsideView: PlayerOutsideView{order: 2}}
	card := &Card{Suit: Hearts, Rank: Queen}
	actions := []Action{
		CardRequest{from: from, to: to, rank: Queen},
		CardResponse{from: from, to: to, cards: []*Card{card}},
		CardResponseAndTopUp{
			CardResponse:  CardResponse{from: from, to: to, cards: []*Card{card}},
			TopUpFromPool: TopUpFromPool{from: from, card: card},
		},
		PlaceBook{from: from, rank: Queen},
		DrawFromPool{from: from, card: card},
		TopUpFromPool{from: from, card: card},
		EndTurn{from: from, to: to, newOrder: 3},
		DealCards{dealer: from, first: to},
	}
	for _, want := range actions {
		got, consumed, err := DecodeAction(want.Encode())
		if err != nil {
			t.Fatalf("DecodeAction(%T): %v", want, err)
		}
		if consumed != len(want.Encode()) {
			t.Errorf("DecodeAction(%T) consumed %d bytes, want %d", want, consumed, len(want.Encode()))
		}
		if string(got.Encode()) != string(want.Encode()) {
			t.Errorf("round trip for %T = %v, want %v", want, got.Encode(), want.Encode())
		}
	}
}

func TestTextActionGrammarRoundTripsToLegacyPayloads(t *testing.T) {
	state := testState(
		[]*Card{{Suit: Spades, Rank: Ace}},
		[]*Card{{Suit: Hearts, Rank: King}},
		[]*Card{{Suit: Diamonds, Rank: Queen}},
		[]*Card{{Suit: Hearts, Rank: Three}},
	)
	tests := []struct {
		name  string
		actor string
		text  string
		want  []byte
	}{
		{"ask canonical", "alice", "ask player 2 for kings", []byte{actionCardRequest, 0, 1, byte(King.value)}},
		{"ask alias", "alice", "get Kings from P2", []byte{actionCardRequest, 0, 1, byte(King.value)}},
		{"place", "alice", "place a book of aces", []byte{actionPlaceBook, 0, byte(Ace.value)}},
		{"place description", "alice", "Place a book of aces", []byte{actionPlaceBook, 0, byte(Ace.value)}},
		{"draw ASCII", "alice", "draw 10♦", []byte{actionDrawFromPool, 0, Card{Suit: Diamonds, Rank: Ten}.Encode()}},
		{"draw symbol alias", "alice", "draw K♠", []byte{actionDrawFromPool, 0, Card{Suit: Spades, Rank: King}.Encode()}},
		{"draw bare", "alice", "draw", []byte{actionDrawFromPool, 0, Card{Suit: Hearts, Rank: Three}.Encode()}},
		{"draw description", "alice", "Draw a card from the pool", []byte{actionDrawFromPool, 0, Card{Suit: Hearts, Rank: Three}.Encode()}},
		{"top up", "alice", "top up Q♥", []byte{actionTopUpFromPool, 0, Card{Suit: Hearts, Rank: Queen}.Encode()}},
		{"top up bare", "alice", "top up", []byte{actionTopUpFromPool, 0, Card{Suit: Hearts, Rank: Three}.Encode()}},
		{"send", "bob", "send P1: K♠, Q♥", nil},
		{"send and top up", "bob", "send P1: K♠; draw 3♦", nil},
		{"end turn", "alice", "pass", nil},
		{"end turn description", "alice", "End your turn", nil},
		{"deal", "dealer", "deal cards", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action, err := ParseTextAction(state, tt.actor, []byte(tt.text))
			if err != nil {
				t.Fatalf("ParseTextAction(%q): %v", tt.text, err)
			}
			if tt.want != nil && !bytes.Equal(action.Encode(), tt.want) {
				t.Errorf("encoded %q = %v, want %v", tt.text, action.Encode(), tt.want)
			}
			if _, consumed, err := DecodeAction(action.Encode()); err != nil || consumed != len(action.Encode()) {
				t.Errorf("normalized action %v did not round trip: consumed=%d err=%v", action.Encode(), consumed, err)
			}
		})
	}
}

func TestTextGoFishAndSendRankActions(t *testing.T) {
	state := testState(
		[]*Card{{Suit: Spades, Rank: Ace}},
		[]*Card{{Suit: Hearts, Rank: Three}, {Suit: Clubs, Rank: Three}},
		nil,
		[]*Card{{Suit: Diamonds, Rank: Seven}},
	)
	requester := &state.players[0]
	requestee := &state.players[1]
	state.recentAction = CardRequest{from: requester, to: requestee, rank: Three}

	goFish, err := ParseTextAction(state, "bob", []byte("go fish"))
	if err != nil {
		t.Fatalf("ParseTextAction(go fish): %v", err)
	}
	response := goFish.(CardResponse)
	if len(response.cards) != 0 || response.from.id != "bob" || response.to.id != "alice" {
		t.Errorf("go fish action = %+v, want empty response from bob to alice", response)
	}

	state.recentAction = nil
	send, err := ParseTextAction(state, "bob", []byte("send threes to player 1"))
	if err != nil {
		t.Fatalf("ParseTextAction(send threes): %v", err)
	}
	combined, ok := send.(CardResponseAndTopUp)
	if !ok {
		t.Fatalf("send threes action = %T, want CardResponseAndTopUp", send)
	}
	response = combined.CardResponse
	if len(response.cards) != 2 || response.from.id != "bob" || response.to.id != "alice" {
		t.Errorf("send threes action = %+v, want two threes from bob to alice", response)
	}

	state.recentAction = CardRequest{from: requester, to: requestee, rank: Three}
	tellGoFish, err := ParseTextAction(state, "bob", []byte("Tell Player 1 to Go Fish!"))
	if err == nil {
		request := tellGoFish.(CardResponse)
		if len(request.cards) != 0 {
			t.Errorf("Tell Player 1 to Go Fish! returned cards: %+v", request.cards)
		}
	}

	sendRequested, err := ParseTextAction(state, "bob", []byte("Send the requested threes to Player 1"))
	if err != nil {
		t.Errorf("ParseTextAction(Send the requested threes): %v", err)
	} else if _, ok := sendRequested.(CardResponseAndTopUp); !ok {
		t.Errorf("Send the requested threes = %T, want CardResponseAndTopUp", sendRequested)
	}

	sendAndDraw, err := ParseTextAction(state, "bob", []byte("Send the requested threes to Player 1, and draw one card from the pool"))
	if err != nil {
		t.Errorf("ParseTextAction(Send the requested threes and draw): %v", err)
	} else if _, ok := sendAndDraw.(CardResponseAndTopUp); !ok {
		t.Errorf("Send the requested threes and draw = %T, want CardResponseAndTopUp", sendAndDraw)
	}
}

func TestLegalActionDescriptionsCanBeSubmittedAsText(t *testing.T) {
	state := testState(
		[]*Card{{Suit: Spades, Rank: Seven}},
		[]*Card{{Suit: Hearts, Rank: Ace}},
		nil,
		[]*Card{{Suit: Hearts, Rank: Three}},
	)
	state.recentAction = DealCards{
		dealer: &state.players[2],
		first:  &state.players[0],
	}

	for _, spec := range (&Game{}).LegalActions(state, "alice") {
		action, err := ParseTextAction(state, "alice", []byte(spec.Description))
		if err != nil {
			t.Errorf("could not parse request description %q: %v", spec.Description, err)
			continue
		}
		if err := (&Game{}).Validate(state, game.Action{ActorID: "alice", Payload: []byte(spec.Description)}); err != nil {
			t.Errorf("could not validate request description %q: %v", spec.Description, err)
		}
		if action.Encode() == nil {
			t.Errorf("request description %q normalized to no action", spec.Description)
		}
	}

	state.recentAction = CardResponse{from: &state.players[0], to: &state.players[1], cards: []*Card{}}
	drawLegals := (&Game{}).LegalActions(state, "bob")
	if len(drawLegals) != 1 {
		t.Fatalf("draw phase has %d legals, want 1", len(drawLegals))
	}
	if drawLegals[0].Description != "Draw a card from the pool" {
		t.Errorf("draw description = %q, want %q", drawLegals[0].Description, "Draw a card from the pool")
	}
	if _, err := ParseTextAction(state, "bob", []byte(drawLegals[0].Description)); err != nil {
		t.Errorf("could not parse draw description: %v", err)
	}
}

func TestTextActionRejectsUnknownAndMalformedCommands(t *testing.T) {
	state := testState(
		[]*Card{{Suit: Spades, Rank: Ace}},
		nil,
		nil,
		nil,
	)
	for _, text := range []string{"", "ask anybody for aces", "place book of zebras", "draw ZZ", "send P99: A♠"} {
		if _, err := ParseTextAction(state, "alice", []byte(text)); err == nil {
			t.Errorf("ParseTextAction(%q) unexpectedly accepted text", text)
		}
	}
}

func TestViewForDecodesRecentTransferredCards(t *testing.T) {
	state := testState(
		[]*Card{{Suit: Spades, Rank: Ace}, {Suit: Hearts, Rank: Ace}},
		nil,
		nil,
		nil,
	)
	state.recentAction = CardResponse{
		from:  &state.players[1],
		to:    &state.players[0],
		cards: []*Card{{Suit: Spades, Rank: Ace}, {Suit: Hearts, Rank: Ace}},
	}
	viewBytes := (&Game{}).ViewFor(state, "alice")
	view, consumed, err := (View{}).Decode(viewBytes)
	if err != nil {
		t.Fatalf("Decode(view): %v", err)
	}
	if consumed != len(viewBytes) {
		t.Fatalf("Decode(view) consumed %d bytes, want %d", consumed, len(viewBytes))
	}
	response, ok := view.recentAction.(CardResponse)
	if !ok {
		t.Fatalf("recent action = %T, want CardResponse", view.recentAction)
	}
	if len(response.cards) != 2 {
		t.Errorf("recent response contains %d cards, want 2", len(response.cards))
	}
}

func TestActiveTurnAndLegalActions(t *testing.T) {
	state := testState(
		[]*Card{{Suit: Spades, Rank: Ace}},
		[]*Card{{Suit: Hearts, Rank: Ace}},
		[]*Card{{Suit: Diamonds, Rank: King}},
		nil,
	)
	state.recentAction = DealCards{dealer: &state.players[2], first: &state.players[0]}
	g := &Game{}
	if got := g.ActiveTurn(state); len(got) != 1 || got[0] != "alice" {
		t.Errorf("initial ActiveTurn() = %v, want [alice]", got)
	}
	legal := g.LegalActions(state, "alice")
	if len(legal) != 2 {
		t.Fatalf("alice has %d initial actions, want two ace requests", len(legal))
	}
	if len(g.LegalActions(state, "bob")) != 0 {
		t.Error("off-turn player received legal actions")
	}
	if err := g.Validate(state, game.Action{ActorID: "bob", Payload: legal[0].Payload}); err == nil {
		t.Error("Validate accepted an action from the wrong player")
	}
	if err := g.Validate(state, game.Action{ActorID: "alice", Payload: []byte{actionCardRequest, 0}}); err == nil {
		t.Error("Validate accepted a malformed or non-legal action")
	}
}

func TestApplyDoesNotMutatePreviousState(t *testing.T) {
	state := testState(
		[]*Card{{Suit: Spades, Rank: Ace}},
		[]*Card{{Suit: Hearts, Rank: Ace}},
		[]*Card{{Suit: Diamonds, Rank: King}},
		nil,
	)
	state.recentAction = DealCards{dealer: &state.players[2], first: &state.players[0]}
	beforeAction := state.recentAction
	action := CardRequest{from: &state.players[0], to: &state.players[1], rank: Ace}
	next := (&Game{}).Apply(state, game.Action{ActorID: "alice", Payload: action.Encode()}).(State)
	if state.recentAction != beforeAction {
		t.Error("Apply mutated the previous state's recent action")
	}
	if next.recentAction == nil || (&Game{}).ActiveTurn(next)[0] != "bob" {
		t.Error("Apply did not record the request or pass control to the requestee")
	}
	if len(state.players[0].hand) != 1 || len(next.players[0].hand) != 1 {
		t.Error("request unexpectedly changed a hand")
	}
}

func TestViewForEncodesPrivateHandAndPublicPlayerData(t *testing.T) {
	state := testState(
		[]*Card{{Suit: Spades, Rank: Ace}},
		[]*Card{{Suit: Hearts, Rank: King}},
		[]*Card{{Suit: Diamonds, Rank: Queen}},
		nil,
	)
	view := (&Game{}).ViewFor(state, "alice")
	if len(view) == 0 {
		t.Fatal("ViewFor returned an empty view")
	}
	decodedView, consumed, err := (View{}).Decode(view)
	if err != nil {
		t.Fatalf("Decode(view): %v", err)
	}
	if consumed != len(view) {
		t.Errorf("Decode(view) consumed %d bytes, want %d", consumed, len(view))
	}
	if decodedView.self.handSize != 1 || len(decodedView.self.hand) != 1 {
		t.Errorf("self hand = %d cards, want private size and one card", len(decodedView.self.hand))
	}
	if _, err := state.viewFor("missing"); err == nil {
		t.Error("viewFor accepted an unknown player")
	}
}

func TestActualGameCanPlaceABookAndFinish(t *testing.T) {
	aces := []*Card{
		{Suit: Spades, Rank: Ace},
		{Suit: Hearts, Rank: Ace},
		{Suit: Diamonds, Rank: Ace},
		{Suit: Clubs, Rank: Ace},
	}
	state := testState(aces, nil, nil, nil)
	state.recentAction = DrawFromPool{from: &state.players[0], card: aces[3]}
	g := &Game{}
	legal := g.LegalActions(state, "alice")
	if len(legal) != 1 {
		t.Fatalf("book-ready player has %d legal actions, want one", len(legal))
	}
	if err := g.Validate(state, game.Action{ActorID: "alice", Payload: legal[0].Payload}); err != nil {
		t.Fatalf("Validate(book) returned %v", err)
	}
	finished := g.Apply(state, game.Action{ActorID: "alice", Payload: legal[0].Payload}).(State)
	result, complete := g.Outcome(finished)
	if !complete {
		t.Fatal("placing the final book did not finish the game")
	}
	outcome := result.(Outcome)
	if got := outcome.GetWinnerIDs(); len(got) != 1 || got[0] != "alice" {
		t.Errorf("winner IDs = %v, want [alice]", got)
	}
}

func TestOutcomeCoversOngoingWinAndTieCases(t *testing.T) {
	tests := []struct {
		name      string
		books     [][]Rank
		hands     [][]*Card
		pool      []*Card
		complete  bool
		winnerIDs []string
	}{
		{"ongoing pool", [][]Rank{{Ace}, {}}, [][]*Card{{}, {}}, []*Card{{Suit: Spades, Rank: Two}}, false, nil},
		{"single winner", [][]Rank{{Ace, King}, {Two}}, [][]*Card{{}, {}}, nil, true, []string{"alice"}},
		{"tie", [][]Rank{{Ace}, {Two}}, [][]*Card{{}, {}}, nil, true, []string{"alice", "bob"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := testState(tt.hands[0], tt.hands[1], nil, tt.pool)
			for i, ranks := range tt.books {
				for _, rank := range ranks {
					state.players[i].books = append(state.players[i].books, Book{rank: rank})
				}
			}
			result, complete := (&Game{}).Outcome(state)
			if complete != tt.complete {
				t.Fatalf("complete = %v, want %v", complete, tt.complete)
			}
			if !complete {
				return
			}
			got := result.(Outcome).GetWinnerIDs()
			sort.Strings(got)
			sort.Strings(tt.winnerIDs)
			if len(got) != len(tt.winnerIDs) {
				t.Fatalf("winner IDs = %v, want %v", got, tt.winnerIDs)
			}
			for i := range got {
				if got[i] != tt.winnerIDs[i] {
					t.Errorf("winner IDs = %v, want %v", got, tt.winnerIDs)
				}
			}
		})
	}
}

func testState(alice, bob, dealer, pool []*Card) State {
	players := []Player{
		{PlayerOutsideView: PlayerOutsideView{id: "alice", order: 0}},
		{PlayerOutsideView: PlayerOutsideView{id: "bob", order: 1}},
		{PlayerOutsideView: PlayerOutsideView{id: "dealer", order: 2}},
	}
	hands := [][]*Card{alice, bob, dealer}
	for i, hand := range hands {
		players[i].hand = hand
		players[i].handSize = len(hand)
		for _, card := range hand {
			card.Holder = &players[i]
		}
	}
	cards := append([]Card(nil), dereferenceCards(alice)...)
	cards = append(cards, dereferenceCards(bob)...)
	cards = append(cards, dereferenceCards(dealer)...)
	cards = append(cards, dereferenceCards(pool)...)
	return State{players: players, cards: cards}
}

func dereferenceCards(cards []*Card) []Card {
	out := make([]Card, 0, len(cards))
	for _, card := range cards {
		out = append(out, *card)
	}
	return out
}

func TestOutcomeReportsHighestBookCount(t *testing.T) {
	alice := Player{PlayerOutsideView: PlayerOutsideView{id: "alice"}}
	bob := Player{PlayerOutsideView: PlayerOutsideView{id: "bob"}}
	state := State{players: []Player{
		alice,
		bob,
	}, cards: []Card{}}
	state.players[0].books = []Book{{rank: Ace}, {rank: King}}
	state.players[1].books = []Book{{rank: Two}}
	result, complete := (&Game{}).Outcome(state)
	if !complete {
		t.Fatal("Outcome reported an incomplete match with no cards in play")
	}
	outcome, ok := result.(Outcome)
	if !ok {
		t.Fatalf("Outcome returned %T, want gofish.Outcome", result)
	}
	if got := outcome.GetWinnerIDs(); len(got) != 1 || got[0] != "alice" {
		t.Errorf("GetWinnerIDs() = %v, want [alice]", got)
	}
	if outcome.TerminationReason() != "No cards were left in play" {
		t.Errorf("TerminationReason() = %q, want no-cards reason", outcome.TerminationReason())
	}
}
