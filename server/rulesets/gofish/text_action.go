package gofish

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type textFieldParser func(string) (any, error)

type textActionPattern struct {
	regexp   *regexp.Regexp
	captures map[string]int
}

type textActionGrammar struct {
	name         string
	patterns     []textActionPattern
	fieldNames   []string
	fieldParsers []textFieldParser
	build        func(State, *Player, map[string]any) (Action, error)
}

func ParseTextAction(state State, actorID string, payload []byte) (Action, error) {
	actor, ok := state.playerByID(actorID)
	if !ok {
		return nil, fmt.Errorf("unknown action actor %q", actorID)
	}
	text := strings.TrimSpace(string(payload))
	if text == "" {
		return nil, fmt.Errorf("empty text action")
	}
	for _, grammar := range textActionGrammars {
		for _, pattern := range grammar.patterns {
			match := pattern.regexp.FindStringSubmatch(text)
			if match == nil {
				continue
			}
			fields := make(map[string]any, len(grammar.fieldNames))
			valid := true
			for i, name := range grammar.fieldNames {
				valueIndex, exists := pattern.captures[name]
				if !exists {
					valueIndex = i + 1
				}
				if valueIndex < 1 || valueIndex >= len(match) {
					valid = false
					break
				}
				value, err := grammar.fieldParsers[i](strings.TrimSpace(match[valueIndex]))
				if err != nil {
					return nil, fmt.Errorf("%s: %w", grammar.name, err)
				}
				fields[name] = value
			}
			if valid {
				return grammar.build(state, actor, fields)
			}
		}
	}
	return nil, fmt.Errorf("unrecognized text action %q", text)
}

var textActionGrammars = []textActionGrammar{
	{
		name:     "GoFish",
		patterns: []textActionPattern{{regexp: regexp.MustCompile(`(?i)^go\s*fish!?$`), captures: map[string]int{}}},
		build: func(state State, actor *Player, _ map[string]any) (Action, error) {
			request, ok := state.recentAction.(CardRequest)
			if !ok {
				return nil, fmt.Errorf("there is no pending card request")
			}
			if request.to == nil || request.to.id != actor.id {
				return nil, fmt.Errorf("this player was not asked for cards")
			}
			return CardResponse{
				from:          actor,
				to:            request.from,
				cards:         []*Card{},
				requestedRank: request.rank,
			}, nil
		},
	},
	{
		name:         "TellGoFish",
		patterns:     []textActionPattern{{regexp: regexp.MustCompile(`(?i)^tell\s+(?:player\s+)?([Pp]?\d+)\s+to\s+go\s*fish!?$`), captures: map[string]int{"target": 1}}},
		fieldNames:   []string{"target"},
		fieldParsers: []textFieldParser{parsePlayerField},
		build: func(state State, actor *Player, fields map[string]any) (Action, error) {
			request, ok := state.recentAction.(CardRequest)
			if !ok || request.to == nil || request.to.id != actor.id {
				return nil, fmt.Errorf("there is no pending card request for this player")
			}
			order := fields["target"].(int)
			if request.from.order != order {
				return nil, fmt.Errorf("requested player does not match Player %d", order+1)
			}
			return CardResponse{
				from:          actor,
				to:            request.from,
				cards:         []*Card{},
				requestedRank: request.rank,
			}, nil
		},
	},
	{
		name:     "Deal",
		patterns: []textActionPattern{{regexp: regexp.MustCompile(`(?i)^deal(?: cards)?$`), captures: map[string]int{}}},
		build: func(state State, actor *Player, _ map[string]any) (Action, error) {
			return DealCards{dealer: actor, first: &state.players[0]}, nil
		},
	},
	{
		name:     "EndTurn",
		patterns: []textActionPattern{{regexp: regexp.MustCompile(`(?i)^(?:end(?: turn)?|end your turn|pass)$`), captures: map[string]int{}}},
		build: func(state State, actor *Player, _ map[string]any) (Action, error) {
			next := state.NextTurn()
			if next == nil {
				return nil, fmt.Errorf("there is no valid next player")
			}
			return EndTurn{from: actor, to: next, newOrder: state.turnNumber + 1}, nil
		},
	},
	{
		name:         "PlaceBook",
		patterns:     []textActionPattern{{regexp: regexp.MustCompile(`(?i)^place(?: (?:a )?book(?: of)?| a)?\s+(.+)$`), captures: map[string]int{"rank": 1}}},
		fieldNames:   []string{"rank"},
		fieldParsers: []textFieldParser{parseRankField},
		build: func(state State, actor *Player, fields map[string]any) (Action, error) {
			return PlaceBook{
				from:         actor,
				rank:         fields["rank"].(Rank),
				continueTurn: continuingCatchTurn(state.recentAction),
			}, nil
		},
	},
	{
		name:     "Draw",
		patterns: []textActionPattern{{regexp: regexp.MustCompile(`(?i)^draw$`), captures: map[string]int{}}},
		build: func(state State, actor *Player, _ map[string]any) (Action, error) {
			card := state.topOfPool()
			if card == nil {
				return nil, fmt.Errorf("there is no card in the pool")
			}
			return poolDrawAction(state, actor, card), nil
		},
	},
	{
		name:     "DrawPoolDescription",
		patterns: []textActionPattern{{regexp: regexp.MustCompile(`(?i)^draw\s+a\s+card\s+from\s+(?:the\s+)?pool$`), captures: map[string]int{}}},
		build: func(state State, actor *Player, _ map[string]any) (Action, error) {
			if _, ok := state.recentAction.(PlaceBook); ok {
				card := state.topOfPool()
				if card == nil {
					return nil, fmt.Errorf("there is no card in the pool")
				}
				return TopUpFromPool{
					from:         actor,
					card:         card,
					continueTurn: continuingCatchTurn(state.recentAction),
				}, nil
			}
			card := state.topOfPool()
			if card == nil {
				return nil, fmt.Errorf("there is no card in the pool")
			}
			return poolDrawAction(state, actor, card), nil
		},
	},
	{
		name: "DrawCard",
		patterns: []textActionPattern{
			{regexp: regexp.MustCompile(`(?i)^draw(?: (?:a|the))?\s+(.+)$`), captures: map[string]int{"card": 1}},
		},
		fieldNames:   []string{"card"},
		fieldParsers: []textFieldParser{parseCardField},
		build: func(state State, actor *Player, fields map[string]any) (Action, error) {
			return poolDrawAction(state, actor, fields["card"].(*Card)), nil
		},
	},
	{
		name:     "TopUp",
		patterns: []textActionPattern{{regexp: regexp.MustCompile(`(?i)^top\s*up$`), captures: map[string]int{}}},
		build: func(state State, actor *Player, _ map[string]any) (Action, error) {
			card := state.topOfPool()
			if card == nil {
				return nil, fmt.Errorf("there is no card in the pool")
			}
			return TopUpFromPool{
				from:         actor,
				card:         card,
				continueTurn: continuingCatchTurn(state.recentAction),
			}, nil
		},
	},
	{
		name: "TopUpCard",
		patterns: []textActionPattern{
			{regexp: regexp.MustCompile(`(?i)^top\s*up(?: with)?\s+(.+)$`), captures: map[string]int{"card": 1}},
		},
		fieldNames:   []string{"card"},
		fieldParsers: []textFieldParser{parseCardField},
		build: func(state State, actor *Player, fields map[string]any) (Action, error) {
			return TopUpFromPool{
				from:         actor,
				card:         fields["card"].(*Card),
				continueTurn: continuingCatchTurn(state.recentAction),
			}, nil
		},
	},
	{
		name: "Ask",
		patterns: []textActionPattern{
			{regexp: regexp.MustCompile(`(?i)^ask\s+(?:player\s+)?([Pp]?\d+)\s+for\s+(.+)$`), captures: map[string]int{"target": 1, "rank": 2}},
			{regexp: regexp.MustCompile(`(?i)^request\s+(?:player\s+)?([Pp]?\d+)\s+for\s+(.+)$`), captures: map[string]int{"target": 1, "rank": 2}},
			{regexp: regexp.MustCompile(`(?i)^request\s+all\s+(.+?)\s+from\s+(?:player\s+)?([Pp]?\d+)$`), captures: map[string]int{"rank": 1, "target": 2}},
			{regexp: regexp.MustCompile(`(?i)^give\s+me\s+(.+?)\s+from\s+(?:player\s+)?([Pp]?\d+)$`), captures: map[string]int{"rank": 1, "target": 2}},
			{regexp: regexp.MustCompile(`(?i)^get\s+(.+?)\s+from\s+(?:player\s+)?([Pp]?\d+)$`), captures: map[string]int{"rank": 1, "target": 2}},
		},
		fieldNames: []string{"target", "rank"},
		fieldParsers: []textFieldParser{
			parsePlayerField,
			parseRankField,
		},
		build: func(state State, actor *Player, fields map[string]any) (Action, error) {
			order := fields["target"].(int)
			to := state.playerByOrder(order)
			if to == nil {
				return nil, fmt.Errorf("unknown target player P%d", order+1)
			}
			return CardRequest{from: actor, to: to, rank: fields["rank"].(Rank)}, nil
		},
	},
	{
		name: "SendAndTopUp",
		patterns: []textActionPattern{
			{regexp: regexp.MustCompile(`(?i)^send\s+(?:player\s+)?([Pp]?\d+)\s*:\s*(.+?)\s*;\s*(?:draw|top\s*up)\s+(.+)$`), captures: map[string]int{"target": 1, "cards": 2, "drawn": 3}},
		},
		fieldNames: []string{"target", "cards", "drawn"},
		fieldParsers: []textFieldParser{
			parsePlayerField,
			parseCardListField,
			parseCardField,
		},
		build: func(state State, actor *Player, fields map[string]any) (Action, error) {
			order := fields["target"].(int)
			to := state.playerByOrder(order)
			if to == nil {
				return nil, fmt.Errorf("unknown recipient player P%d", order+1)
			}
			response := CardResponse{from: actor, to: to, cards: fields["cards"].([]*Card)}
			return CardResponseAndTopUp{
				CardResponse:  response,
				TopUpFromPool: TopUpFromPool{from: actor, card: fields["drawn"].(*Card)},
			}, nil
		},
	},
	{
		name:         "SendRequestedAndTopUp",
		patterns:     []textActionPattern{{regexp: regexp.MustCompile(`(?i)^send\s+the\s+requested\s+(.+?)\s+to\s+(?:player\s+)?([Pp]?\d+),?\s+and\s+draw\s+one\s+card\s+from\s+(?:the\s+)?pool$`), captures: map[string]int{"rank": 1, "target": 2}}},
		fieldNames:   []string{"rank", "target"},
		fieldParsers: []textFieldParser{parseRankField, parsePlayerField},
		build: func(state State, actor *Player, fields map[string]any) (Action, error) {
			order := fields["target"].(int)
			to := state.playerByOrder(order)
			if to == nil {
				return nil, fmt.Errorf("unknown recipient player P%d", order+1)
			}
			rank := fields["rank"].(Rank)
			cards := make([]*Card, 0)
			for _, card := range actor.hand {
				if card.Rank == rank {
					cards = append(cards, card)
				}
			}
			card := state.topOfPool()
			if card == nil {
				return nil, fmt.Errorf("there is no card in the pool")
			}
			return CardResponseAndTopUp{
				CardResponse:  CardResponse{from: actor, to: to, cards: cards},
				TopUpFromPool: TopUpFromPool{from: actor, card: card},
			}, nil
		},
	},
	{
		name: "SendRank",
		patterns: []textActionPattern{
			{regexp: regexp.MustCompile(`(?i)^send\s+the\s+requested\s+(.+?)\s+to\s+(?:player\s+)?([Pp]?\d+)$`), captures: map[string]int{"rank": 1, "target": 2}},
			{regexp: regexp.MustCompile(`(?i)^send\s+(.+?)\s+to\s+(?:player\s+)?([Pp]?\d+)$`), captures: map[string]int{"rank": 1, "target": 2}},
			{regexp: regexp.MustCompile(`(?i)^give\s+(.+?)\s+to\s+(?:player\s+)?([Pp]?\d+)$`), captures: map[string]int{"rank": 1, "target": 2}},
		},
		fieldNames:   []string{"rank", "target"},
		fieldParsers: []textFieldParser{parseRankField, parsePlayerField},
		build: func(state State, actor *Player, fields map[string]any) (Action, error) {
			order := fields["target"].(int)
			to := state.playerByOrder(order)
			if to == nil {
				return nil, fmt.Errorf("unknown recipient player P%d", order+1)
			}
			rank := fields["rank"].(Rank)
			cards := make([]*Card, 0)
			for _, card := range actor.hand {
				if card.Rank == rank {
					cards = append(cards, card)
				}
			}
			if len(cards) == 0 {
				return nil, fmt.Errorf("you do not hold any %s", rank.plural)
			}
			if len(actor.hand) == len(cards) {
				card := state.topOfPool()
				if card == nil {
					return nil, fmt.Errorf("there is no card in the pool")
				}
				return CardResponseAndTopUp{
					CardResponse:  CardResponse{from: actor, to: to, cards: cards},
					TopUpFromPool: TopUpFromPool{from: actor, card: card},
				}, nil
			}
			return CardResponse{from: actor, to: to, cards: cards}, nil
		},
	},
	{
		name: "Send",
		patterns: []textActionPattern{
			{regexp: regexp.MustCompile(`(?i)^send\s+(?:player\s+)?([Pp]?\d+)\s*:\s*(.+)$`), captures: map[string]int{"target": 1, "cards": 2}},
			{regexp: regexp.MustCompile(`(?i)^give\s+(?:player\s+)?([Pp]?\d+)\s*:\s*(.+)$`), captures: map[string]int{"target": 1, "cards": 2}},
			{regexp: regexp.MustCompile(`(?i)^send\s+(.+?)\s+to\s+(?:player\s+)?([Pp]?\d+)$`), captures: map[string]int{"cards": 1, "target": 2}},
		},
		fieldNames:   []string{"target", "cards"},
		fieldParsers: []textFieldParser{parsePlayerField, parseCardListField},
		build: func(state State, actor *Player, fields map[string]any) (Action, error) {
			order := fields["target"].(int)
			to := state.playerByOrder(order)
			if to == nil {
				return nil, fmt.Errorf("unknown recipient player P%d", order+1)
			}
			return CardResponse{from: actor, to: to, cards: fields["cards"].([]*Card)}, nil
		},
	},
}

func poolDrawAction(state State, actor *Player, card *Card) Action {
	if response, ok := state.recentAction.(CardResponse); ok &&
		len(response.cards) == 0 &&
		response.requestedRank == card.Rank &&
		actor.order == state.CurrentTurn().order {
		return MakeCatchFromPool{from: actor, card: card}
	}
	return DrawFromPool{from: actor, card: card}
}

func continuingCatchTurn(action Action) bool {
	switch a := action.(type) {
	case MakeCatchFromPool:
		return true
	case PlaceBook:
		return a.continueTurn
	case TopUpFromPool:
		return a.continueTurn
	default:
		return false
	}
}

func parsePlayerField(text string) (any, error) {
	text = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(text), "p"))
	number, err := strconv.Atoi(text)
	if err != nil || number < 1 {
		return nil, fmt.Errorf("invalid player reference %q", text)
	}
	return number - 1, nil
}

func parseRankField(text string) (any, error) {
	normalized := strings.ToLower(strings.TrimSpace(text))
	normalized = strings.TrimSuffix(normalized, "s")
	for _, rank := range Ranks {
		if normalized == strings.ToLower(rank.name) ||
			normalized == strings.ToLower(rank.plural) ||
			normalized == strings.ToLower(rank.symbol) {
			return rank, nil
		}
	}
	return nil, fmt.Errorf("invalid rank %q", text)
}

func parseCardListField(text string) (any, error) {
	parts := strings.FieldsFunc(strings.TrimSpace(text), func(r rune) bool {
		return r == ',' || r == ' '
	})
	if len(parts) == 0 {
		return nil, fmt.Errorf("card list is empty")
	}
	cards := make([]*Card, 0, len(parts))
	for _, part := range parts {
		value, err := parseCardField(part)
		if err != nil {
			return nil, err
		}
		cards = append(cards, value.(*Card))
	}
	return cards, nil
}

func parseCardField(text string) (any, error) {
	text = strings.TrimSpace(text)
	runes := []rune(text)
	if len(runes) < 2 {
		return nil, fmt.Errorf("invalid card %q", text)
	}
	rankText := string(runes[:len(runes)-1])
	suitText := strings.ToLower(string(runes[len(runes)-1]))
	rankValue, err := parseRankField(rankText)
	if err != nil {
		return nil, err
	}
	suits := map[string]Suit{
		"s": Spades, "♠": Spades,
		"h": Hearts, "♥": Hearts,
		"d": Diamonds, "♦": Diamonds,
		"c": Clubs, "♣": Clubs,
		"spades": Spades, "hearts": Hearts, "diamonds": Diamonds, "clubs": Clubs,
	}
	suit, ok := suits[suitText]
	if !ok {
		return nil, fmt.Errorf("invalid card suit %q", suitText)
	}
	card := &Card{Suit: suit, Rank: rankValue.(Rank)}
	return card, nil
}
