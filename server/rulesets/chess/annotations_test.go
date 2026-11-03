package chess

import "testing"

func annotationsOf(s State) map[string]string {
	return (&Game{}).Annotations(s, whiteID)
}

func assertAnnotations(t *testing.T, got map[string]string, want map[string]string) {
	t.Helper()
	all := []string{AnnotationPlies, AnnotationLastMove, AnnotationCheck, AnnotationCheckers, AnnotationKingUnsafe, AnnotationDrawOffer}
	for _, key := range all {
		value, present := got[key]
		wantValue, wanted := want[key]
		if present != wanted || value != wantValue {
			t.Errorf("annotation %s = %q (present=%v), want %q (present=%v)", key, value, present, wantValue, wanted)
		}
	}
}

func TestAnnotationsAtTheStart(t *testing.T) {
	// Every square next to the king is held by its own pieces, so none is unsafe
	assertAnnotations(t, annotationsOf(mustState(t, startFEN)), map[string]string{AnnotationPlies: "0"})
}

func TestAnnotationsTrackTheLastMoveAndPlies(t *testing.T) {
	s := play(t, mustState(t, startFEN), "e2e4")
	assertAnnotations(t, annotationsOf(s), map[string]string{AnnotationPlies: "1", AnnotationLastMove: "e2e4"})
	s = play(t, s, "e7e5")
	assertAnnotations(t, annotationsOf(s), map[string]string{AnnotationPlies: "2", AnnotationLastMove: "e7e5"})
}

func TestAnnotationsDrawOfferNamesTheOfferer(t *testing.T) {
	s := play(t, mustState(t, startFEN), "e2e4(=)")
	// The draw offer mark is not part of the last move
	assertAnnotations(t, annotationsOf(s), map[string]string{
		AnnotationPlies: "1", AnnotationLastMove: "e2e4", AnnotationDrawOffer: whiteID,
	})
	s = play(t, s, "e7e5") // Moving declines the offer
	assertAnnotations(t, annotationsOf(s), map[string]string{AnnotationPlies: "2", AnnotationLastMove: "e7e5"})
}

func TestAnnotationsLastMoveKeepsThePromotionPiece(t *testing.T) {
	s := play(t, mustState(t, "4k3/P7/8/8/8/8/8/4K3 w - - 0 1"), "a7a8q")
	if got := annotationsOf(s)[AnnotationLastMove]; got != "a7a8q" {
		t.Errorf("last_move = %q, want a7a8q", got)
	}
}

func TestAnnotationsReportCheckAndCheckers(t *testing.T) {
	// The new queen on a8 checks the king along the eighth rank
	s := play(t, mustState(t, "4k3/P7/8/8/8/8/8/4K3 w - - 0 1"), "a7a8q")
	assertAnnotations(t, annotationsOf(s), map[string]string{
		AnnotationPlies: "1", AnnotationLastMove: "a7a8q", AnnotationCheck: "black", AnnotationCheckers: "a8",
		// f8 is attacked through the king's own square, which only counts when the king is taken off the board
		AnnotationKingUnsafe: "d8,f8",
	})
}

func TestAnnotationsReportEveryChecker(t *testing.T) {
	// Double check from the bishop on b4 and the knight on d3
	s := mustState(t, "4k3/8/8/8/1b6/3n4/8/R3K3 w - - 0 1")
	assertAnnotations(t, annotationsOf(s), map[string]string{
		AnnotationPlies: "0", AnnotationCheck: "white", AnnotationCheckers: "b4,d3", AnnotationKingUnsafe: "d2,f2",
	})
}

func TestAnnotationsAtCheckmate(t *testing.T) {
	s := mustState(t, "rnb1kbnr/pppp1ppp/8/4p3/6Pq/5P2/PPPPP2P/RNBQKBNR w KQkq - 1 3")
	assertAnnotations(t, annotationsOf(s), map[string]string{
		AnnotationPlies: "0", AnnotationCheck: "white", AnnotationCheckers: "h4", AnnotationKingUnsafe: "f2",
	})
}

func TestKingUnsafeIncludesDefendedPiecesOnly(t *testing.T) {
	// The pawn on d2 is defended by the bishop on f4, so the king may not capture it. The pawn also guards e1,
	// and the bishop guards e3.
	defended := mustState(t, "7k/8/8/8/5b2/8/3pK3/8 w - - 0 1")
	assertAnnotations(t, annotationsOf(defended), map[string]string{AnnotationPlies: "0", AnnotationKingUnsafe: "d2,e1,e3"})

	// Without the bishop the pawn is undefended and may be captured, so only e1 is unsafe
	undefended := mustState(t, "7k/8/8/8/8/8/3pK3/8 w - - 0 1")
	assertAnnotations(t, annotationsOf(undefended), map[string]string{AnnotationPlies: "0", AnnotationKingUnsafe: "e1"})
}
