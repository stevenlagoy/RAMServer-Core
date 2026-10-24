package chess

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stevenlagoy/ramserver-core/server/game"
)

// Draw offers and claims

// A move followed by (=) offers a draw. (=) alone accepts the offer made with the opponent's last move.
// Any other move declines it. Claims: an offer made after 100 halfmoves, or while the position has occurred three
// times, ends the game at once. 150 halfmoves or five repetitions end it with no offer.

func hasPayload(specs []game.ActionSpec, payload string) bool {
	for _, spec := range specs {
		if string(spec.Payload) == payload {
			return true
		}
	}
	return false
}

// Expects Validate to reject the payload, and returns the error
func rejected(t *testing.T, s State, actor string, payload string) error {
	t.Helper()
	err := (&Game{}).Validate(s, game.Action{ActorID: actor, Payload: []byte(payload)})
	if err == nil {
		t.Fatalf("Validate(%q) by %s accepted a payload which should be rejected", payload, actor)
	}
	return err
}

func TestDecodeDrawOffers(t *testing.T) {
	tests := []struct {
		payload   string
		wantPiece bool // Whether the payload also carries a move
	}{
		{"e2e4(=)", true},
		{"e7e8q(=)", true},
		{"(=)", false},
		{"e2e4", true},
	}
	for _, tt := range tests {
		m, err := DecodeMove([]byte(tt.payload))
		if err != nil {
			t.Errorf("DecodeMove(%q): %v", tt.payload, err)
			continue
		}
		if (m.From != nil) != tt.wantPiece || m.offeringDraw != strings.HasSuffix(tt.payload, "(=)") {
			t.Errorf("DecodeMove(%q) = %+v", tt.payload, m)
		}
		if string(m.Encode()) != tt.payload {
			t.Errorf("Encode(DecodeMove(%q)) = %q", tt.payload, m.Encode())
		}
	}
	for _, payload := range []string{"abc", "e2e", "e2e4abc", "e2e4(=", "e2e4(==)", "(=)e2e4", "(==)", "e2e4q(x)"} {
		if _, err := DecodeMove([]byte(payload)); !errors.Is(err, game.ErrMalformedAction) {
			t.Errorf("DecodeMove(%q) error = %v, want game.ErrMalformedAction", payload, err)
		}
	}
}

func TestDrawResponseHasNoSquares(t *testing.T) {
	m := Move{offeringDraw: true}
	if got := m.toUCI(); got != "(=)" {
		t.Errorf("toUCI() = %q, want (=)", got)
	}
	if san, err := m.toSAN(mustState(t, startFEN)); err != nil || san != "(=)" {
		t.Errorf("toSAN() = %q, %v, want (=)", san, err)
	}
}

func TestDrawOfferSAN(t *testing.T) {
	if got := sanOf(t, mustState(t, startFEN), "e2e4(=)"); got != "e4 (=)" {
		t.Errorf("SAN = %q, want %q", got, "e4 (=)")
	}
}

func TestOfferIsRecordedWithoutChangingThePosition(t *testing.T) {
	before := mustState(t, startFEN)
	s := play(t, before, "e2e4(=)")
	if !s.activeDrawOffer {
		t.Fatal("the offer was not recorded")
	}
	if want := "rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 1"; s.ToFEN() != want {
		t.Errorf("FEN = %q, want %q", s.ToFEN(), want)
	}
	if before.activeDrawOffer {
		t.Error("offering changed the previous state")
	}
	if _, done := outcomeOf(t, s); done {
		t.Error("an unanswered offer ended the game")
	}
}

func TestAcceptActionIsListedOnlyWhilePending(t *testing.T) {
	g := &Game{}
	s := mustState(t, startFEN)
	if hasPayload(g.LegalActions(s, whiteID), "(=)") {
		t.Error("accept listed with no offer pending")
	}
	s = play(t, s, "e2e4(=)")
	black := g.LegalActions(s, blackID)
	if !hasPayload(black, "(=)") {
		t.Error("accept not listed for the player who was offered a draw")
	}
	if len(black) != 21 {
		t.Errorf("black has %d actions, want 20 moves and the accept", len(black))
	}
	if hasPayload(g.LegalActions(s, whiteID), "(=)") {
		t.Error("accept listed for the player who made the offer")
	}
	s = play(t, s, "e7e5")
	if hasPayload(g.LegalActions(s, whiteID), "(=)") {
		t.Error("accept still listed after the offer was declined")
	}
}

func TestAcceptingEndsTheMatchAsADraw(t *testing.T) {
	s := play(t, mustState(t, startFEN), "e2e4(=)")
	s = play(t, s, "(=)")
	res, done := outcomeOf(t, s)
	if !done {
		t.Fatal("accepting the offer did not end the game")
	}
	if res.Result.TerminationReason() != ReasonDrawOffer || len(res.Result.WinnerIDs) != 0 || res.toSAN() != "1/2-1/2" {
		t.Errorf("result = %v %v %q, want a drawn MUTUAL_DRAW_OFFER", res.Result.TerminationReason(), res.Result.WinnerIDs, res.toSAN())
	}
}

func TestMovingDeclinesTheOffer(t *testing.T) {
	s := play(t, mustState(t, startFEN), "e2e4(=)")
	s = play(t, s, "e7e5")
	if s.activeDrawOffer {
		t.Error("the offer survived the opponent's move")
	}
	if err := rejected(t, s, whiteID, "(=)"); !errors.Is(err, game.ErrIllegalAction) {
		t.Errorf("accepting a declined offer: error = %v, want game.ErrIllegalAction", err)
	}
	if _, done := outcomeOf(t, s); done {
		t.Error("a declined offer ended the game")
	}
}

func TestCounterOfferReplacesTheOffer(t *testing.T) {
	s := play(t, mustState(t, startFEN), "e2e4(=)")
	s = play(t, s, "e7e5(=)")
	s = play(t, s, "(=)") // White accepts Black's offer
	if _, done := outcomeOf(t, s); !done {
		t.Error("accepting the counter-offer did not end the game")
	}
}

func TestAcceptingWithoutAnOfferIsIllegal(t *testing.T) {
	s := mustState(t, startFEN)
	if err := rejected(t, s, whiteID, "(=)"); !errors.Is(err, game.ErrIllegalAction) {
		t.Errorf("error = %v, want game.ErrIllegalAction", err)
	}
	if got := (&Game{}).Apply(s, game.Action{ActorID: whiteID, Payload: []byte("(=)")}).(State); got.drawAccepted {
		t.Error("Apply accepted a draw nobody offered")
	}
}

func TestOnlyTheOfferedPlayerCanAccept(t *testing.T) {
	s := play(t, mustState(t, startFEN), "e2e4(=)")
	rejected(t, s, whiteID, "(=)") // The offerer is not on turn, and could not accept their own offer
}

func TestIllegalMoveCannotRideOnAnOffer(t *testing.T) {
	// A move which is illegal stays illegal when it carries (=), whether or not an offer is pending
	start := mustState(t, startFEN)
	rejected(t, start, whiteID, "e2e5(=)")
	pending := play(t, start, "e2e4(=)")
	for _, payload := range []string{"a8a5(=)", "e7e2(=)", "d8d1(=)", "e8e7(=)"} {
		if err := rejected(t, pending, blackID, payload); !errors.Is(err, game.ErrIllegalAction) {
			t.Errorf("%s: error = %v, want game.ErrIllegalAction", payload, err)
		}
	}
}

func TestOfferOnCheckmateStillWins(t *testing.T) {
	s := play(t, mustState(t, "6k1/5ppp/8/8/8/8/8/R3K3 w - - 0 1"), "a1a8(=)")
	res, done := outcomeOf(t, s)
	if !done || res.Result.TerminationReason() != ReasonCheckmate || len(res.Result.WinnerIDs) != 1 || res.Result.WinnerIDs[0] != whiteID {
		t.Errorf("outcome = %v (done=%v), want white wins by checkmate", res.Result.TerminationReason(), done)
	}
}

func TestFiftyMoveClaim(t *testing.T) {
	const rookEndgame = "4k3/8/8/8/8/8/8/R3K3 w - - %d 60"
	tests := []struct {
		name      string
		halfmove  int
		move      string
		wantDraw  bool
		wantNoWin bool
	}{
		{"offer at 100 halfmoves is a claim", 99, "a1a2(=)", true, true},
		{"offer before 100 is only an offer", 98, "a1a2(=)", false, true},
		{"no offer at 100 plays on", 99, "a1a2", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := play(t, mustState(t, fmt.Sprintf(rookEndgame, tt.halfmove)), tt.move)
			res, done := outcomeOf(t, s)
			if done != tt.wantDraw {
				t.Fatalf("done = %v, want %v (FEN %s)", done, tt.wantDraw, s.ToFEN())
			}
			if done && (res.Result.TerminationReason() != ReasonDrawOffer || len(res.Result.WinnerIDs) != 0) {
				t.Errorf("result = %v %v, want a draw by claim", res.Result.TerminationReason(), res.Result.WinnerIDs)
			}
		})
	}
}

func TestThreefoldRepetitionNeedsAClaim(t *testing.T) {
	shuffle := []string{"g1f3", "g8f6", "f3g1", "f6g8"}
	s := mustState(t, startFEN)
	for round := 0; round < 2; round++ {
		for i, m := range shuffle {
			if round == 1 && i == len(shuffle)-1 {
				break // The last move of the second round is played below, with and without a claim
			}
			s = play(t, s, m)
		}
	}
	// The start position occurs a third time after black's last move
	if _, done := outcomeOf(t, play(t, s, "f6g8")); done {
		t.Error("threefold repetition ended the game without a claim")
	}
	res, done := outcomeOf(t, play(t, s, "f6g8(=)"))
	if !done || res.Result.TerminationReason() != ReasonDrawOffer || len(res.Result.WinnerIDs) != 0 {
		t.Errorf("claim on the third repetition: done=%v reason=%v", done, res.Result.TerminationReason())
	}
	// An offer on the second occurrence is only an offer
	early := mustState(t, startFEN)
	for _, m := range []string{"g1f3", "g8f6", "f3g1"} {
		early = play(t, early, m)
	}
	if _, done := outcomeOf(t, play(t, early, "f6g8(=)")); done {
		t.Error("an offer on the second repetition ended the game")
	}
}

func TestFivefoldRepetitionIsAutomatic(t *testing.T) {
	shuffle := []string{"g1f3", "g8f6", "f3g1", "f6g8"}
	s := mustState(t, startFEN)
	for round := 1; round <= 4; round++ {
		for _, m := range shuffle {
			s = play(t, s, m)
		}
		// The start position has now occurred round+1 times
		res, done := outcomeOf(t, s)
		if want := round == 4; done != want {
			t.Fatalf("after %d rounds done = %v, want %v", round, done, want)
		}
		if done && res.Result.TerminationReason() != ReasonAutomaticDraw {
			t.Errorf("reason = %v, want AUTOMATIC_DRAW", res.Result.TerminationReason())
		}
	}
}
