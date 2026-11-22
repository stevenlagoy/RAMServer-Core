package chess

import (
	"errors"
	"testing"

	"github.com/stevenlagoy/ramserver-core/server/game"
)

func resignAction(actor string) game.Action {
	return game.Action{ActorID: actor, Payload: []byte("resign")}
}

func TestResignEndsTheMatchForEitherPlayer(t *testing.T) {
	g := &Game{}
	tests := []struct {
		name     string
		resigner string
		winner   string
		wantSAN  string
	}{
		{"player on turn resigns", whiteID, blackID, "0-1"},
		{"player off turn resigns", blackID, whiteID, "1-0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := mustState(t, startFEN) // White is on turn
			a := resignAction(tt.resigner)
			if !g.AllowsOutOfTurn(s, a) {
				t.Fatal("resigning must be allowed out of turn")
			}
			if err := g.Validate(s, a); err != nil {
				t.Fatalf("Validate rejected a resignation: %v", err)
			}
			res, done := outcomeOf(t, g.Apply(s, a).(State))
			if !done {
				t.Fatal("resigning did not end the game")
			}
			if res.Result.TerminationReason() != ReasonResignation || len(res.Result.WinnerIDs) != 1 || res.Result.WinnerIDs[0] != tt.winner {
				t.Errorf("result = %v %v, want %s to win by resignation", res.Result.TerminationReason(), res.Result.WinnerIDs, tt.winner)
			}
			if res.loser == nil || *res.loser != tt.resigner {
				t.Errorf("loser = %v, want %s", res.loser, tt.resigner)
			}
			if got := res.toSAN(); got != tt.wantSAN {
				t.Errorf("toSAN() = %q, want %q", got, tt.wantSAN)
			}
		})
	}
}

func TestResignDoesNotChangeTheBoardOrThePreviousState(t *testing.T) {
	g := &Game{}
	s := mustState(t, startFEN)
	next := g.Apply(s, resignAction(whiteID)).(State)
	if next.ToFEN() != startFEN {
		t.Errorf("FEN after resigning = %q, want it unchanged", next.ToFEN())
	}
	if _, done := outcomeOf(t, s); done {
		t.Error("resigning ended the game in the state it was applied to")
	}
}

func TestOnlyPlayersCanResign(t *testing.T) {
	g := &Game{}
	s := mustState(t, startFEN)
	err := g.Validate(s, resignAction("spectator"))
	if !errors.Is(err, game.ErrIllegalAction) {
		t.Errorf("a spectator resigning: error = %v, want game.ErrIllegalAction", err)
	}
}

func TestResignPayloadIsExact(t *testing.T) {
	g := &Game{}
	s := mustState(t, startFEN)
	for _, payload := range []string{"Resign", "resign ", "resig", "resign(=)"} {
		if g.AllowsOutOfTurn(s, game.Action{ActorID: blackID, Payload: []byte(payload)}) {
			t.Errorf("%q was allowed out of turn", payload)
		}
		err := g.Validate(s, game.Action{ActorID: whiteID, Payload: []byte(payload)})
		if !errors.Is(err, game.ErrMalformedAction) {
			t.Errorf("Validate(%q) error = %v, want game.ErrMalformedAction", payload, err)
		}
	}
}

func TestOtherActionsAreNotAllowedOutOfTurn(t *testing.T) {
	g := &Game{}
	s := mustState(t, startFEN)
	for _, payload := range []string{"e7e5", "e2e4", "(=)", "e7e5(=)"} {
		if g.AllowsOutOfTurn(s, game.Action{ActorID: blackID, Payload: []byte(payload)}) {
			t.Errorf("%q was allowed out of turn", payload)
		}
	}
}

func TestResigningWithADrawOfferPendingIsAResignation(t *testing.T) {
	g := &Game{}
	s := play(t, mustState(t, startFEN), "e2e4(=)") // Black is on turn and has been offered a draw
	res, done := outcomeOf(t, g.Apply(s, resignAction(whiteID)).(State))
	if !done || res.Result.TerminationReason() != ReasonResignation || res.Result.WinnerIDs[0] != blackID {
		t.Errorf("outcome = %v (done=%v), want black to win by resignation", res.Result.TerminationReason(), done)
	}
}
