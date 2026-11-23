package tictactoe

import (
	"errors"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/stevenlagoy/ramserver-core/server/game"
)

const (
	alice   = "alice" // joins first: X
	bob     = "bob"   // joins second: O
	watcher = "watcher"
)

var g = &Game{}

func roster() ([]string, map[string]game.Role) {
	return []string{alice, bob, watcher}, map[string]game.Role{
		alice: game.RolePlayer, bob: game.RolePlayer, watcher: game.RoleSpectator,
	}
}

func newState(t *testing.T) game.GameState {
	t.Helper()
	order, roles := roster()
	return g.NewState(order, roles)
}

func act(actor, payload string) game.Action {
	return game.Action{ActorID: actor, Payload: []byte(payload)}
}

// play validates and applies moves alternately from alice (X) and bob (O), failing on any rejection.
func play(t *testing.T, cells ...string) game.GameState {
	t.Helper()
	st := newState(t)
	players := []string{alice, bob}
	for i, c := range cells {
		a := act(players[i%2], c)
		if err := g.Validate(st, a); err != nil {
			t.Fatalf("move %d (%q by %s) on %v rejected: %v", i+1, c, a.ActorID, st, err)
		}
		st = g.Apply(st, a)
	}
	return st
}

func mustEnd(t *testing.T, st game.GameState, reason game.TerminationReason, winners ...string) {
	t.Helper()
	res, ended := g.Outcome(st)
	if !ended {
		t.Fatalf("game on %v not over, want %s", st, reason)
	}
	if res.TerminationReason() != reason {
		t.Errorf("reason = %q, want %q", res.TerminationReason(), reason)
	}
	if got := res.GetWinnerIDs(); !slices.Equal(got, winners) {
		t.Errorf("winners = %v, want %v", got, winners)
	}
	if active := g.ActiveTurn(st); active == nil || len(active) != 0 {
		t.Errorf("ActiveTurn after the end = %#v, want empty non-nil", active)
	}
	for _, m := range []string{alice, bob, watcher} {
		if legal := g.LegalActions(st, m); len(legal) != 0 {
			t.Errorf("LegalActions(%s) after the end = %v, want none", m, legal)
		}
	}
}

func mustContinue(t *testing.T, st game.GameState) {
	t.Helper()
	if res, ended := g.Outcome(st); ended {
		t.Fatalf("game on %v ended early: %v", st, res)
	}
}

func TestRolesAndStart(t *testing.T) {
	specs := g.Roles()
	want := []game.RoleSpec{{Name: game.RolePlayer, Min: 2, Max: 2}, {Name: game.RoleSpectator}}
	if !reflect.DeepEqual(specs, want) {
		t.Fatalf("Roles() = %v, want %v", specs, want)
	}
	two := map[string]game.Role{alice: game.RolePlayer, bob: game.RolePlayer}
	withWatcher := map[string]game.Role{alice: game.RolePlayer, bob: game.RolePlayer, watcher: game.RoleSpectator}
	tests := []struct {
		name   string
		roster map[string]game.Role
		ready  map[string]bool
		want   bool
	}{
		{"empty", map[string]game.Role{}, map[string]bool{}, false},
		{"one ready player", map[string]game.Role{alice: game.RolePlayer}, map[string]bool{alice: true}, false},
		{"two players, none ready", two, map[string]bool{}, false},
		{"two players, one ready", two, map[string]bool{alice: true}, false},
		{"two players, both ready", two, map[string]bool{alice: true, bob: true}, true},
		{"spectator need not be ready", withWatcher, map[string]bool{alice: true, bob: true}, true},
		{"ready spectator doesn't replace a player", withWatcher, map[string]bool{alice: true, watcher: true}, false},
		{"spectators alone", map[string]game.Role{watcher: game.RoleSpectator}, map[string]bool{watcher: true}, false},
	}
	for _, tt := range tests {
		if got := g.ReadyToStart(tt.roster, tt.ready); got != tt.want {
			t.Errorf("%s: ReadyToStart = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestSeatsFollowJoinOrder(t *testing.T) {
	// A spectator who joined between the players (possible after a player leaves and rejoins) is skipped.
	order := []string{"w", "p2", "p1"}
	roles := map[string]game.Role{"w": game.RoleSpectator, "p1": game.RolePlayer, "p2": game.RolePlayer}
	st := g.NewState(order, roles)
	if got := g.ActiveTurn(st); !slices.Equal(got, []string{"p2"}) {
		t.Fatalf("ActiveTurn = %v, want the first player in join order", got)
	}
	ann := g.Annotations(st, "w")
	if ann[AnnotationXPlayer] != "p2" || ann[AnnotationOPlayer] != "p1" {
		t.Fatalf("annotations = %v, want p2 as X and p1 as O", ann)
	}
}

func TestNotEnoughPlayers(t *testing.T) {
	st := g.NewState([]string{alice}, map[string]game.Role{alice: game.RolePlayer})
	res, ended := g.Outcome(st)
	if !ended || res.TerminationReason() != ReasonNotEnoughPlayers || len(res.GetWinnerIDs()) != 0 {
		t.Fatalf("Outcome = %v, %v; want NOT_ENOUGH_PLAYERS with no winner", res, ended)
	}
	if v := string(g.ViewFor(st, alice)); v != ".........-X" {
		t.Fatalf("view = %q", v)
	}
}

func TestTurnOrder(t *testing.T) {
	st := newState(t)
	if got := g.ActiveTurn(st); !slices.Equal(got, []string{alice}) {
		t.Fatalf("X (alice) should move first, ActiveTurn = %v", got)
	}
	st = g.Apply(st, act(alice, "4"))
	if got := g.ActiveTurn(st); !slices.Equal(got, []string{bob}) {
		t.Fatalf("after X, ActiveTurn = %v, want bob", got)
	}
	st = g.Apply(st, act(bob, "0"))
	if got := g.ActiveTurn(st); !slices.Equal(got, []string{alice}) {
		t.Fatalf("after O, ActiveTurn = %v, want alice", got)
	}
	if v := string(g.ViewFor(st, alice)); v != "O...X....XX" {
		t.Fatalf("view = %q", v)
	}
}

// Every one of the eight lines wins, for X and for O.
func TestEveryLineWins(t *testing.T) {
	for _, l := range lines {
		var others []int
		for c := range numCells {
			if !slices.Contains(l[:], c) {
				others = append(others, c)
			}
		}
		d := func(c int) string { return string(rune('0' + c)) }

		// X wins: X plays the line, O plays two other cells (two marks can't make a line).
		st := play(t, d(l[0]), d(others[0]), d(l[1]), d(others[1]), d(l[2]))
		mustEnd(t, st, ReasonThreeInARow, alice)
		if got := g.Annotations(st, watcher)[AnnotationWinningLine]; got != lineString(l) {
			t.Errorf("X on %v: winning_line = %q, want %q", l, got, lineString(l))
		}
		if v := g.ViewFor(st, watcher); v[9] != 'x' {
			t.Errorf("X on %v: status byte %q, want 'x'", l, v[9])
		}

		// O wins: X plays three cells off the line that don't form a line themselves.
		xs := nonLine(others)
		if xs == nil {
			t.Fatalf("no three non-winning cells off line %v", l)
		}
		st = play(t, d(xs[0]), d(l[0]), d(xs[1]), d(l[1]), d(xs[2]), d(l[2]))
		mustEnd(t, st, ReasonThreeInARow, bob)
		if got := g.Annotations(st, watcher)[AnnotationWinningLine]; got != lineString(l) {
			t.Errorf("O on %v: winning_line = %q, want %q", l, got, lineString(l))
		}
		if v := g.ViewFor(st, watcher); v[9] != 'o' {
			t.Errorf("O on %v: status byte %q, want 'o'", l, v[9])
		}
	}
}

func lineString(l [3]int) string {
	return string([]byte{byte('0' + l[0]), ',', byte('0' + l[1]), ',', byte('0' + l[2])})
}

// nonLine picks three cells from cells that don't form a line.
func nonLine(cells []int) []int {
	for i := 0; i < len(cells); i++ {
		for j := i + 1; j < len(cells); j++ {
			for k := j + 1; k < len(cells); k++ {
				var b [numCells]byte
				for c := range b {
					b[c] = cellEmpty
				}
				b[cells[i]], b[cells[j]], b[cells[k]] = markX, markX, markX
				if _, won := winningLine(b); !won {
					return []int{cells[i], cells[j], cells[k]}
				}
			}
		}
	}
	return nil
}

func TestWinOnLastMoveIsAWinNotADraw(t *testing.T) {
	// Nine marks with no line: X 1,2,3,4,8 / O 0,5,6,7 is a draw.
	st := play(t, "4", "0", "2", "6", "3", "5", "1", "7", "8")
	mustEnd(t, st, ReasonDraw)

	// X's ninth mark fills the board and completes 2,4,6: a win, not a draw.
	st = play(t, "1", "0", "2", "3", "8", "5", "4", "7", "6")
	mustEnd(t, st, ReasonThreeInARow, alice)
	if got := g.Annotations(st, alice)[AnnotationWinningLine]; got != "2,4,6" {
		t.Fatalf("winning_line = %q", got)
	}
}

func TestDraw(t *testing.T) {
	// X O X / X O O / O X X
	st := play(t, "0", "1", "2", "4", "3", "5", "7", "6")
	mustContinue(t, st)
	st = g.Apply(st, act(alice, "8"))
	mustEnd(t, st, ReasonDraw)
	if v := string(g.ViewFor(st, bob)); v != "XOXXOOOXXDO" {
		t.Fatalf("view = %q", v)
	}
	ann := g.Annotations(st, bob)
	if _, has := ann[AnnotationWinningLine]; has {
		t.Errorf("a draw has a winning_line: %v", ann)
	}
	if ann[AnnotationMoves] != "9" || ann[AnnotationLastMove] != "8" {
		t.Errorf("annotations = %v", ann)
	}
}

func TestResignation(t *testing.T) {
	tests := []struct {
		name     string
		moves    []string
		resigner string
		winner   string
		mark     string
	}{
		{"X resigns before any move", nil, alice, bob, "X"},
		{"O resigns before any move (out of turn)", nil, bob, alice, "O"},
		{"X resigns on O's turn", []string{"4"}, alice, bob, "X"},
		{"O resigns on its own turn", []string{"4"}, bob, alice, "O"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := play(t, tt.moves...)
			a := act(tt.resigner, ResignPayload)
			if !g.AllowsOutOfTurn(st, a) {
				t.Fatal("resign is not allowed out of turn")
			}
			if err := g.Validate(st, a); err != nil {
				t.Fatalf("Validate(resign) = %v", err)
			}
			st = g.Apply(st, a)
			mustEnd(t, st, ReasonResignation, tt.winner)
			if got := g.Annotations(st, watcher)[AnnotationResigned]; got != tt.mark {
				t.Errorf("resigned = %q, want %q", got, tt.mark)
			}
		})
	}
}

func TestOnlyResignIsAllowedOutOfTurn(t *testing.T) {
	st := newState(t)
	for _, p := range []string{"0", "8", "resign ", "RESIGN", ""} {
		if g.AllowsOutOfTurn(st, act(bob, p)) {
			t.Errorf("AllowsOutOfTurn(%q) = true", p)
		}
	}
}

func TestLegalActions(t *testing.T) {
	st := play(t, "4")
	payloads := func(m string) []string {
		var out []string
		for _, s := range g.LegalActions(st, m) {
			out = append(out, string(s.Payload))
		}
		return out
	}
	if got, want := payloads(bob), []string{"0", "1", "2", "3", "5", "6", "7", "8", "resign"}; !slices.Equal(got, want) {
		t.Errorf("player to move: %v, want %v", got, want)
	}
	if got, want := payloads(alice), []string{"resign"}; !slices.Equal(got, want) {
		t.Errorf("waiting player: %v, want %v", got, want)
	}
	if got := payloads(watcher); len(got) != 0 {
		t.Errorf("spectator: %v, want none", got)
	}
	if got := payloads("stranger"); len(got) != 0 {
		t.Errorf("non-member: %v, want none", got)
	}
	if d := g.LegalActions(st, bob)[0].Description; d != "top-left" {
		t.Errorf("description of 0 = %q", d)
	}
}

func TestMalformedActions(t *testing.T) {
	st := newState(t)
	for _, p := range []string{"", "9", "a", "X", "00", "4\n", " 4", "-1", "resign\n", "Resign", "resig", "\x00", "\x04",
		strings.Repeat("4", 70<<10)} {
		err := g.Validate(st, act(alice, p))
		if !errors.Is(err, game.ErrMalformedAction) {
			t.Errorf("Validate(%q) = %v, want ErrMalformedAction", p, err)
		}
	}
	if err := g.Validate(st, game.Action{ActorID: alice}); !errors.Is(err, game.ErrMalformedAction) {
		t.Errorf("nil payload: %v", err)
	}
}

func TestIllegalActions(t *testing.T) {
	st := play(t, "4")
	over := play(t, "0", "3", "1", "4", "2")
	tests := []struct {
		name  string
		state game.GameState
		a     game.Action
		msg   string
	}{
		{"cell taken", st, act(bob, "4"), "illegal action: cell 4 is already taken"},
		{"not your turn", st, act(alice, "0"), "illegal action: it is not your turn"},
		{"spectator moves", st, act(watcher, "0"), "illegal action: spectators cannot play"},
		{"spectator resigns", st, act(watcher, "resign"), "illegal action: spectators cannot play"},
		{"non-member moves", st, act("stranger", "0"), "illegal action: spectators cannot play"},
		{"move after the end", over, act(bob, "8"), "illegal action: the game is over"},
		{"resign after the end", over, act(bob, "resign"), "illegal action: the game is over"},
		{"foreign state", "not a state", act(alice, "0"), "illegal action: no game in progress"},
	}
	for _, tt := range tests {
		err := g.Validate(tt.state, tt.a)
		if !errors.Is(err, game.ErrIllegalAction) || err.Error() != tt.msg {
			t.Errorf("%s: Validate = %v, want %q", tt.name, err, tt.msg)
		}
	}
	if err := g.Validate(st, act(alice, "9")); err == nil ||
		err.Error() != `malformed action: send one digit "0" to "8" for a cell, or "resign"` {
		t.Errorf("malformed message = %v", err)
	}
}

// snapshot copies everything observable about a state, so a later comparison catches any modification.
func snapshot(st game.GameState) any {
	type obs struct {
		Value   State
		Views   map[string]string
		Legal   map[string][]game.ActionSpec
		Ann     map[string]map[string]string
		Active  []string
		Outcome game.MatchResult
	}
	o := obs{Views: map[string]string{}, Legal: map[string][]game.ActionSpec{}, Ann: map[string]map[string]string{}}
	o.Value, _ = stateOf(st)
	for _, m := range []string{alice, bob, watcher} {
		o.Views[m] = string(g.ViewFor(st, m))
		o.Legal[m] = g.LegalActions(st, m)
		o.Ann[m] = maps.Clone(g.Annotations(st, m))
	}
	o.Active = g.ActiveTurn(st)
	o.Outcome, _ = g.Outcome(st)
	return o
}

func TestApplyAndValidateDoNotModifyInput(t *testing.T) {
	sequences := [][]string{
		{"4", "0", "8", "2", "1", "7", "6", "3", "5"}, // a draw
		{"0", "3", "1", "4", "2"},                     // X wins
	}
	for _, seq := range sequences {
		st := newState(t)
		players := []string{alice, bob}
		for i, c := range seq {
			before := snapshot(st)
			for _, a := range []game.Action{act(players[i%2], c), act(players[(i+1)%2], ResignPayload), act(watcher, c), act(alice, "zz")} {
				_ = g.Validate(st, a)
			}
			resigned := g.Apply(st, act(players[(i+1)%2], ResignPayload))
			next := g.Apply(st, act(players[i%2], c))
			if after := snapshot(st); !reflect.DeepEqual(before, after) {
				t.Fatalf("move %d modified its input state:\nbefore %#v\nafter  %#v", i+1, before, after)
			}
			if again := g.Apply(st, act(players[i%2], c)); !reflect.DeepEqual(again, next) {
				t.Fatalf("Apply is not deterministic at move %d", i+1)
			}
			if _, ended := g.Outcome(resigned); !ended {
				t.Fatalf("resigning at move %d didn't end the game", i+1)
			}
			st = next
		}
	}
}

func TestPointerStateIsAccepted(t *testing.T) {
	s, _ := stateOf(newState(t))
	p := &s
	next := g.Apply(p, act(alice, "4"))
	if s.Board[4] != cellEmpty {
		t.Fatal("Apply modified the state behind a pointer")
	}
	if v := string(g.ViewFor(next, bob)); v != "....X....OO" {
		t.Fatalf("view = %q", v)
	}
}

func TestForeignStateNeverPanics(t *testing.T) {
	for _, st := range []game.GameState{nil, 42, (*State)(nil)} {
		_ = g.ActiveTurn(st)
		_ = g.LegalActions(st, alice)
		_ = g.Validate(st, act(alice, "0"))
		_ = g.Apply(st, act(alice, "0"))
		_, _ = g.Outcome(st)
		_ = g.ViewFor(st, alice)
		_ = g.Annotations(st, alice)
		if got := g.ActiveTurn(st); got == nil {
			t.Errorf("ActiveTurn(%v) = nil", st)
		}
	}
}

// TestReadmeViewExamples keeps the encoded_view examples in README.md true.
func TestReadmeViewExamples(t *testing.T) {
	tests := []struct {
		moves         []string
		x, o, watcher string
	}{
		{nil, ".........XX", ".........XO", ".........XS"},
		{[]string{"4"}, "....X....OX", "....X....OO", "....X....OS"},
		{[]string{"4", "0"}, "O...X....XX", "O...X....XO", "O...X....XS"},
		{[]string{"0", "3", "1", "4", "2"}, "XXXOO....xX", "XXXOO....xO", "XXXOO....xS"},
		{[]string{"0", "1", "2", "4", "3", "5", "7", "6", "8"}, "XOXXOOOXXDX", "XOXXOOOXXDO", "XOXXOOOXXDS"},
	}
	for _, tt := range tests {
		st := play(t, tt.moves...)
		for m, want := range map[string]string{alice: tt.x, bob: tt.o, watcher: tt.watcher} {
			if got := string(g.ViewFor(st, m)); got != want || len(got) != ViewLen {
				t.Errorf("after %v, view for %s = %q, want %q", tt.moves, m, got, want)
			}
		}
	}
	// O wins by resignation of X: lowercase o.
	st := g.Apply(play(t, "4"), act(alice, ResignPayload))
	if got := string(g.ViewFor(st, alice)); got != "....X....oX" {
		t.Errorf("after X resigns, view = %q", got)
	}
}
