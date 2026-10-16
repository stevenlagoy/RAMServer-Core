package chess

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/stevenlagoy/ramserver-core/server/game"
)

const (
	whiteID  = "player-white"
	blackID  = "player-black"
	startFEN = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"
)

// Helpers

func mustState(t *testing.T, fen string) State {
	t.Helper()
	state, err := StateFromFEN(fen, whiteID, blackID)
	if err != nil {
		t.Fatalf("mustState failed to build game state: %v", err)
	}
	return *state
}

// Returns the player's ID whose turn it is
func activeID(s State) string {
	if s.turn == White {
		return s.whitePlayer
	}
	return s.blackPlayer
}

func legalUCI(s State) []string {
	specs := s.legalActionSpecs(activeID(s))
	out := make([]string, 0, len(specs))
	for _, spec := range specs {
		out = append(out, string(spec.Payload))
	}
	sort.Strings(out)
	return out
}

// Filter moves down to those starting on the given square
func movesFrom(moves []string, square string) []string {
	var out []string
	for _, m := range moves {
		if strings.HasPrefix(m, square) {
			out = append(out, m)
		}
	}
	return out
}

func hasMove(moves []string, uci string) bool {
	for _, m := range moves {
		if m == uci {
			return true
		}
	}
	return false
}

// Builds UCI moves from one origin square
func targets(from string, tos ...string) []string {
	out := make([]string, 0, len(tos))
	for _, to := range tos {
		out = append(out, from+to)
	}
	return out
}

func concat(lists ...[]string) []string {
	var out []string
	for _, l := range lists {
		out = append(out, l...)
	}
	return out
}

func assertMoves(t *testing.T, got, want []string) {
	t.Helper()
	g := append([]string(nil), got...)
	w := append([]string(nil), want...)
	sort.Strings(g)
	sort.Strings(w)
	if strings.Join(g, " ") == strings.Join(w, " ") {
		return
	}
	var missing, extra []string
	for _, m := range w {
		if !hasMove(g, m) {
			missing = append(missing, m)
		}
	}
	for _, m := range g {
		if !hasMove(w, m) {
			extra = append(extra, m)
		}
	}
	t.Errorf(
		"move mismatch:\n\tgot (%d): %v\n\twant (%d): %v\n\tmissing: %v\n\textra: %v",
		len(g), g, len(w), w, missing, extra,
	)
}

// Validates and applies a UCI move for the side to move
func play(t *testing.T, s State, uci string) State {
	t.Helper()
	g := &Game{}
	a := game.Action{ActorID: activeID(s), Payload: []byte(uci)}
	if err := g.Validate(s, a); err != nil {
		t.Fatalf("Validate(%q) from %q returned error: %v", uci, s.ToFEN(), err)
	}
	return g.Apply(s, a).(State)
}

func placement(fen string) string {
	return strings.Fields(fen)[0]
}

func outcomeOf(t *testing.T, s State) (ChessResult, bool) {
	t.Helper()
	g := &Game{}
	res, done := g.Outcome(s)
	if !done {
		return ChessResult{}, false
	}
	cr, ok := res.(ChessResult)
	if !ok {
		t.Fatalf("Outcome returned %T, want ChessResult", res)
	}
	return cr, true
}

// Count legal move sequences to fixed depths
func perft(s State, depth int) uint64 {
	if depth == 0 {
		return 1
	}
	g := &Game{}
	id := activeID(s)
	specs := s.legalActionSpecs(id)
	if depth == 1 {
		return uint64(len(specs))
	}
	var total uint64
	for _, spec := range specs {
		next := g.Apply(s, game.Action{ActorID: id, Payload: spec.Payload}).(State)
		total += perft(next, depth-1)
	}
	return total
}

// Reports node count under each root move
func perftDivide(s State, depth int) map[string]uint64 {
	g := &Game{}
	id := activeID(s)
	out := make(map[string]uint64)
	for _, spec := range s.legalActionSpecs(id) {
		next := g.Apply(s, game.Action{ActorID: id, Payload: spec.Payload}).(State)
		out[string(spec.Description)] = perft(next, depth-1)
	}
	return out
}

func formatDivide(d map[string]uint64) string {
	keys := make([]string, 0, len(d))
	for k := range d {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "\t%s: %d\n", k, d[k])
	}
	return b.String()
}

// https://chessprogramming.org/Perft_Results
var perftPositions = []struct {
	name   string
	fen    string
	counts []uint64
}{
	{"start position", startFEN, []uint64{20, 400, 8902, 197281}},
	{"kiwipete", "r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1", []uint64{48, 2039, 97862, 4085603}},
	{"position 3 (en passant pins, endgame)", "8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - - 0 1", []uint64{14, 191, 2812, 43238}},
	{"position 4 (promotions, castling through check)", "r3k2r/Pppp1ppp/1b3nbN/nP6/BBP1P3/q4N2/Pp1P2PP/R2Q1RK1 w kq - 0 1", []uint64{6, 264, 9467, 422333}},
	{"position 4 mirrored", "r2q1rk1/pP1p2pp/Q4n2/bbp1p3/Np6/1B3NBn/pPPP1PPP/R3K2R b KQ - 0 1", []uint64{6, 264, 9467, 422333}},
	{"position 5 (promotion with capture)", "rnbq1k1r/pp1Pbppp/2p5/8/2B5/8/PPP1NnPP/RNBQK2R w KQ - 1 8", []uint64{44, 1486, 62379, 2103487}},
	{"position 6", "r4rk1/1pp1qppp/p1np1n2/2b1p1B1/2B1P1b1/P1NP1N2/1PP1QPPP/R4RK1 w - - 0 10", []uint64{46, 2079, 89890, 3894594}},
}

func TestPerft(t *testing.T) {
	for _, pos := range perftPositions {
		for i, want := range pos.counts {
			depth := i + 1
			t.Run(fmt.Sprintf("%s/depth%d", pos.name, depth), func(t *testing.T) {
				if depth >= 4 && os.Getenv("RAMSERVER_PERFT_DEEP") == "" {
					t.Skip("set RAMSERVER_PERFT_DEEP=1 to run perft at depth 4 and above")
				}
				if depth == 3 && testing.Short() {
					t.Skip("skipping depth 3 in perft in -short mode")
				}
				s := mustState(t, pos.fen)
				got := perft(s, depth)
				if got != want {
					t.Fatalf(
						"perft(%d) = %d, want %d\nper-move counts:\n%s",
						depth, got, want, formatDivide(perftDivide(s, depth)),
					)
				}
			})
		}
	}
}

func allSquares() []string {
	var out []string
	for _, f := range "abcdefgh" {
		for r := 1; r <= 8; r++ {
			out = append(out, fmt.Sprintf("%c%d", f, r))
		}
	}
	return out
}

func TestLegalActionsAreUniqueAndValidate(t *testing.T) {
	g := &Game{}
	for _, pos := range perftPositions {
		t.Run(pos.name, func(t *testing.T) {
			s := mustState(t, pos.fen)
			moves := legalUCI(s)
			seen := map[string]bool{}
			for _, m := range moves {
				if seen[m] {
					t.Errorf("duplicate legal move %q", m)
				}
				seen[m] = true
				if err := g.Validate(s, game.Action{ActorID: activeID(s), Payload: []byte(m)}); err != nil {
					t.Errorf("legal move %q rejected by Validate: %v", m, err)
				}
			}
		})
	}
}

func TestValidateRejectsEveryNonLegalSquarePair(t *testing.T) {
	g := &Game{}
	squares := allSquares()
	for _, pos := range perftPositions[:5] {
		t.Run(pos.name, func(t *testing.T) {
			s := mustState(t, pos.fen)
			legal := legalUCI(s)
			for _, from := range squares {
				for _, to := range squares {
					pair := from + to
					isLegal := false
					for _, m := range legal {
						if strings.HasPrefix(m, pair) {
							isLegal = true
							break
						}
					}
					if isLegal {
						continue
					}
					err := g.Validate(s, game.Action{ActorID: activeID(s), Payload: []byte(pair)})
					if err == nil {
						t.Fatalf("Validate accepted %q, which is not a legal move in %q", pair, pos.fen)
					}
				}
			}
		})
	}
}

// Individual piece movement on empty board

func TestPieceMovement(t *testing.T) {
	rookD4 := targets("d4", "d1", "d2", "d3", "d5", "d6", "d7", "d8", "a4", "b4", "c4", "e4", "f4", "g4", "h4")
	bishopD4 := targets("d4", "a7", "b6", "c5", "e3", "f2", "g1", "a1", "b2", "c3", "e5", "f6", "g7", "h8")

	tests := []struct {
		name string
		fen  string
		from string
		want []string
	}{
		{"knight in the open", "8/8/7k/8/3N4/8/8/4K3 w - - 0 1", "d4",
			targets("d4", "b3", "b5", "c2", "c6", "e2", "e6", "f3", "f5")},
		{"knight in the corner", "7k/8/8/8/8/8/8/N3K3 w - - 0 1", "a1",
			targets("a1", "b3", "c2")},
		{"bishop in the open", "8/8/7k/8/3B4/8/8/4K3 w - - 0 1", "d4", bishopD4},
		{"rook in the open", "8/8/7k/8/3R4/8/8/4K3 w - - 0 1", "d4", rookD4},
		{"queen in the open", "8/8/7k/8/3Q4/8/8/4K3 w - - 0 1", "d4", concat(rookD4, bishopD4)},
		{"king in the open", "7k/8/8/8/3K4/8/8/8 w - - 0 1", "d4",
			targets("d4", "c3", "c4", "c5", "d3", "d5", "e3", "e4", "e5")},
		{"rook blocked by own piece", "7k/8/8/8/3R1N2/8/8/4K3 w - - 0 1", "d4",
			targets("d4", "d1", "d2", "d3", "d5", "d6", "d7", "d8", "a4", "b4", "c4", "e4")},
		{"rook stops on first enemy piece", "7k/8/8/8/3R1n2/8/8/4K3 w - - 0 1", "d4",
			targets("d4", "d1", "d2", "d3", "d5", "d6", "d7", "d8", "a4", "b4", "c4", "e4", "f4")},
		{"bishop blocked on one diagonal", "8/8/7k/4P3/3B4/8/8/4K3 w - - 0 1", "d4",
			targets("d4", "a7", "b6", "c5", "e3", "f2", "g1", "a1", "b2", "c3")},
		{"knight jumps over surrounding pieces", "7k/8/8/8/8/PPP5/PNP5/PPP1K3 w - - 0 1", "b2",
			targets("b2", "a4", "c4", "d1", "d3")},
		{"knights at the start", startFEN, "b1", targets("b1", "a3", "c3")},
		{"bishop boxed in at the start", startFEN, "c1", nil},
		{"black knight", "4k3/8/8/3n4/8/8/8/4K3 b - - 0 1", "d5",
			targets("d5", "b4", "b6", "c3", "c7", "e3", "e7", "f4", "f6")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := mustState(t, tt.fen)
			assertMoves(t, movesFrom(legalUCI(s), tt.from), tt.want)
		})
	}
}

func TestPawnMovement(t *testing.T) {
	tests := []struct {
		name string
		fen  string
		from string
		want []string
	}{
		{"white pawn on start rank pushes one or two", "4k3/8/8/8/8/8/4P3/4K3 w - - 0 1", "e2", targets("e2", "e3", "e4")},
		{"black pawn on start rank pushes one or two", "4k3/4p3/8/8/8/8/8/4K3 b - - 0 1", "e7", targets("e7", "e6", "e5")},
		{"white pawn off start rank pushes one", "4k3/8/8/8/4P3/8/8/4K3 w - - 0 1", "e4", targets("e4", "e5")},
		{"black pawn off start rank pushes one", "4k3/8/8/4p3/8/8/8/4K3 b - - 0 1", "e5", targets("e5", "e4")},
		{"pawn blocked directly ahead", "4k3/8/8/8/8/4p3/4P3/4K3 w - - 0 1", "e2", nil},
		{"pawn cannot double push through a piece", "4k3/8/8/8/4p3/8/4P3/4K3 w - - 0 1", "e2", targets("e2", "e3")},
		{"pawn cannot capture straight ahead", "4k3/8/8/4p3/4P3/8/8/4K3 w - - 0 1", "e4", nil},
		{"pawn captures diagonally in both directions", "4k3/8/8/8/8/3p1p2/4P3/4K3 w - - 0 1", "e2",
			targets("e2", "e3", "e4", "d3", "f3")},
		{"pawn cannot capture own piece", "4k3/8/8/8/8/3N1b2/4P3/4K3 w - - 0 1", "e2",
			targets("e2", "e3", "e4", "f3")},
		{"pawn does not move diagonally onto an empty square", "4k3/8/8/8/3P4/8/8/4K3 w - - 0 1", "d4", targets("d4", "d5")},
		{"black pawn captures downwards", "4k3/8/8/8/3p4/2P1P3/8/4K3 b - - 0 1", "d4",
			targets("d4", "d3", "c3", "e3")},
		{"edge pawn captures only toward the board", "4k3/8/8/8/8/1p6/P7/4K3 w - - 0 1", "a2",
			targets("a2", "a3", "a4", "b3")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := mustState(t, tt.fen)
			assertMoves(t, movesFrom(legalUCI(s), tt.from), tt.want)
		})
	}
}

func TestCapturesRemoveTheTarget(t *testing.T) {
	tests := []struct {
		name          string
		fen           string
		move          string
		wantPlacement string
	}{
		{"knight captures", "4k3/8/4p3/8/3N4/8/8/4K3 w - - 0 1", "d4e6", "4k3/8/4N3/8/8/8/8/4K3"},
		{"bishop captures", "4k3/8/8/4p3/3B4/8/8/4K3 w - - 0 1", "d4e5", "4k3/8/8/4B3/8/8/8/4K3"},
		{"rook captures", "7k/8/8/8/3Rp3/8/8/4K3 w - - 0 1", "d4e4", "7k/8/8/8/4R3/8/8/4K3"},
		{"queen captures", "7k/3p4/8/8/3Q4/8/8/4K3 w - - 0 1", "d4d7", "7k/3Q4/8/8/8/8/8/4K3"},
		{"king captures an undefended piece", "7k/8/8/8/8/8/3pK3/8 w - - 0 1", "e2d2", "7k/8/8/8/8/8/3K4/8"},
		{"pawn captures", "7k/8/8/8/8/3p4/4P3/4K3 w - - 0 1", "e2d3", "7k/8/8/8/8/3P4/8/4K3"},
		{"black pawn captures", "4k3/8/8/8/8/3p4/4P3/4K3 b - - 0 1", "d3e2", "4k3/8/8/8/8/8/4p3/4K3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := mustState(t, tt.fen)
			if !hasMove(legalUCI(s), tt.move) {
				t.Fatalf("%s should be legal in %q; legal moves: %v", tt.move, tt.fen, legalUCI(s))
			}
			got := placement(play(t, s, tt.move).ToFEN())
			if got != tt.wantPlacement {
				t.Errorf("placement after %s = %q, want %q", tt.move, got, tt.wantPlacement)
			}
		})
	}
}

func TestKingCannotCaptureDefendedPiece(t *testing.T) {
	// The black pawn on d2 is defended by the black bishop on f4.
	s := mustState(t, "7k/8/8/8/5b2/8/3pK3/8 w - - 0 1")
	if hasMove(legalUCI(s), "e2d2") {
		t.Error("king captured a defended piece")
	}
}

func TestPromotionMoves(t *testing.T) {
	tests := []struct {
		name string
		fen  string
		from string
		want []string
	}{
		{"white push promotes to four pieces", "4k3/P7/8/8/8/8/8/4K3 w - - 0 1", "a7",
			targets("a7", "a8q", "a8r", "a8b", "a8n")},
		{"black push promotes to four pieces", "4k3/8/8/8/8/8/p7/4K3 b - - 0 1", "a2",
			targets("a2", "a1q", "a1r", "a1b", "a1n")},
		{"promotion by capture only when blocked", "rn2k3/P7/8/8/8/8/8/4K3 w - - 0 1", "a7",
			targets("a7", "b8q", "b8r", "b8b", "b8n")},
		{"promotion by push and capture", "1n2k3/P7/8/8/8/8/8/4K3 w - - 0 1", "a7",
			targets("a7", "a8q", "a8r", "a8b", "a8n", "b8q", "b8r", "b8b", "b8n")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := mustState(t, tt.fen)
			assertMoves(t, movesFrom(legalUCI(s), tt.from), tt.want)
		})
	}
}

func TestPromotionResult(t *testing.T) {
	tests := []struct {
		move          string
		wantPlacement string
	}{
		{"a7a8q", "Q3k3/8/8/8/8/8/8/4K3"},
		{"a7a8r", "R3k3/8/8/8/8/8/8/4K3"},
		{"a7a8b", "B3k3/8/8/8/8/8/8/4K3"},
		{"a7a8n", "N3k3/8/8/8/8/8/8/4K3"},
	}
	for _, tt := range tests {
		t.Run(tt.move, func(t *testing.T) {
			s := mustState(t, "4k3/P7/8/8/8/8/8/4K3 w - - 0 1")
			got := placement(play(t, s, tt.move).ToFEN())
			if got != tt.wantPlacement {
				t.Errorf("placement after %s = %q, want %q", tt.move, got, tt.wantPlacement)
			}
		})
	}
}

func TestPromotionRequiresSuffix(t *testing.T) {
	g := &Game{}
	s := mustState(t, "4k3/P7/8/8/8/8/8/4K3 w - - 0 1")
	err := g.Validate(s, game.Action{ActorID: whiteID, Payload: []byte("a7a8")})
	if err == nil {
		t.Error("a pawn reaching the last rank without a promotion piece was accepted")
	}
}

func TestPromotionSuffixRejectedOffPromotionRank(t *testing.T) {
	g := &Game{}
	s := mustState(t, "4k3/8/8/8/4P3/8/8/4K3 w - - 0 1")
	if err := g.Validate(s, game.Action{ActorID: whiteID, Payload: []byte("e4e5q")}); err == nil {
		t.Error("promotion suffix accepted for a pawn that is not promoting")
	}
}

func TestCannotPromoteToKingOrPawn(t *testing.T) {
	g := &Game{}
	s := mustState(t, "4k3/P7/8/8/8/8/8/4K3 w - - 0 1")
	for _, m := range []string{"a7a8k", "a7a8p"} {
		if err := g.Validate(s, game.Action{ActorID: whiteID, Payload: []byte(m)}); err == nil {
			t.Errorf("%s was accepted", m)
		}
	}
}

func TestCastlingAvailability(t *testing.T) {
	tests := []struct {
		name         string
		fen          string
		wantKingside bool
		wantQueen    bool
	}{
		{"both sides available", "r3k2r/8/8/8/8/8/8/R3K2R w KQkq - 0 1", true, true},
		{"no rights", "r3k2r/8/8/8/8/8/8/R3K2R w - - 0 1", false, false},
		{"kingside right only", "r3k2r/8/8/8/8/8/8/R3K2R w K - 0 1", true, false},
		{"queenside right only", "r3k2r/8/8/8/8/8/8/R3K2R w Q - 0 1", false, true},
		{"pieces between king and rooks", "r3k2r/8/8/8/8/8/8/RN2K1NR w KQkq - 0 1", false, false},
		{"only the queenside is blocked", "r3k2r/8/8/8/8/8/8/R2QK2R w KQkq - 0 1", true, false},
		{"king may not pass through an attacked square", "r3kr2/8/8/8/8/8/8/R3K2R w KQ - 0 1", false, true},
		{"rook may pass through an attacked square", "1r2k3/8/8/8/8/8/8/R3K2R w KQ - 0 1", true, true},
		{"king may not castle out of check", "4r1k1/8/8/8/8/8/8/R3K2R w KQ - 0 1", false, false},
		{"king may not castle into check", "4k1r1/8/8/8/8/8/8/R3K2R w KQ - 0 1", false, true},
		{"black can castle both sides", "r3k2r/8/8/8/8/8/8/4K3 b kq - 0 1", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := mustState(t, tt.fen)
			moves := legalUCI(s)
			kingside, queenside := "e1g1", "e1c1"
			if s.turn == Black {
				kingside, queenside = "e8g8", "e8c8"
			}
			if got := hasMove(moves, kingside); got != tt.wantKingside {
				t.Errorf("kingside castle legal = %v, want %v (moves: %v)", got, tt.wantKingside, moves)
			}
			if got := hasMove(moves, queenside); got != tt.wantQueen {
				t.Errorf("queenside castle legal = %v, want %v (moves: %v)", got, tt.wantQueen, moves)
			}
		})
	}
}

func TestCastlingMovesTheRook(t *testing.T) {
	base := "r3k2r/8/8/8/8/8/8/R3K2R w KQkq - 0 1"
	walled := "r3k2r/pppppppp/8/8/8/8/8/R3K2R w KQkq - 0 1"
	tests := []struct {
		name    string
		fen     string
		moves   []string
		wantFEN string
	}{
		{"white kingside", base, []string{"e1g1"}, "r3k2r/8/8/8/8/8/8/R4RK1 b kq - 1 1"},
		{"white queenside", base, []string{"e1c1"}, "r3k2r/8/8/8/8/8/8/2KR3R b kq - 1 1"},
		// Black pawns block the f and d files: otherwise the white rook attacks the square black's king must cross
		{"black kingside after white kingside", walled, []string{"e1g1", "e8g8"}, "r4rk1/pppppppp/8/8/8/8/8/R4RK1 w - - 2 2"},
		{"black queenside after white queenside", walled, []string{"e1c1", "e8c8"}, "2kr3r/pppppppp/8/8/8/8/8/2KR3R w - - 2 2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := mustState(t, tt.fen)
			for _, m := range tt.moves {
				s = play(t, s, m)
			}
			if got := s.ToFEN(); got != tt.wantFEN {
				t.Errorf("FEN = %q, want %q", got, tt.wantFEN)
			}
		})
	}
}

func TestCastlingRightsAreLost(t *testing.T) {
	tests := []struct {
		name       string
		fen        string
		move       string
		wantRights string
	}{
		{"king moves", "r3k2r/8/8/8/8/8/8/R3K2R w KQkq - 0 1", "e1e2", "kq"},
		{"kingside rook moves", "r3k2r/8/8/8/8/8/8/R3K2R w KQkq - 0 1", "h1h2", "Qkq"},
		{"queenside rook moves", "r3k2r/8/8/8/8/8/8/R3K2R w KQkq - 0 1", "a1a2", "Kkq"},
		{"black king moves", "r3k2r/8/8/8/8/8/8/R3K2R b KQkq - 0 1", "e8e7", "KQ"},
		{"black queenside rook moves", "r3k2r/8/8/8/8/8/8/R3K2R b KQkq - 0 1", "a8a7", "KQk"},
		{"castling spends both rights", "r3k2r/8/8/8/8/8/8/R3K2R w KQkq - 0 1", "e1g1", "kq"},
		{"rook is captured on its home square", "r3k2r/8/8/8/8/8/6b1/R3K2R b KQkq - 0 1", "g2h1", "Qkq"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := mustState(t, tt.fen)
			if got := play(t, s, tt.move).CastlingRights(); got != tt.wantRights {
				t.Errorf("castling rights after %s = %q, want %q", tt.move, got, tt.wantRights)
			}
		})
	}
}

func TestCastlingRightsDoNotReturn(t *testing.T) {
	s := mustState(t, "r3k2r/8/8/8/8/8/8/R3K2R w KQkq - 0 1")
	// White rook leaves and returns, black shuffles a rook; white may no longer castle kingside.
	for _, m := range []string{"h1h2", "a8a7", "h2h1", "a7a8"} {
		s = play(t, s, m)
	}
	if hasMove(legalUCI(s), "e1g1") {
		t.Error("kingside castle became legal again after the rook moved away and back")
	}
}

func TestDoublePushSetsEnPassantTarget(t *testing.T) {
	s := mustState(t, startFEN)
	got := play(t, s, "e2e4").ToFEN()
	want := "rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 1"
	if got != want {
		t.Errorf("FEN = %q, want %q", got, want)
	}
}

func TestSinglePushClearsEnPassantTarget(t *testing.T) {
	s := mustState(t, startFEN)
	s = play(t, s, "e2e4")
	got := play(t, s, "e7e6").ToFEN()
	want := "rnbqkbnr/pppp1ppp/4p3/8/4P3/8/PPPP1PPP/RNBQKBNR w KQkq - 0 2"
	if got != want {
		t.Errorf("FEN = %q, want %q", got, want)
	}
}

func TestEnPassantCapture(t *testing.T) {
	tests := []struct {
		name    string
		fen     string
		move    string
		wantFEN string
	}{
		{"white captures", "4k3/8/8/3pP3/8/8/8/4K3 w - d6 0 3", "e5d6", "4k3/8/3P4/8/8/8/8/4K3 b - - 0 3"},
		{"white captures on the other side", "4k3/8/8/3Pp3/8/8/8/4K3 w - e6 0 3", "d5e6", "4k3/8/4P3/8/8/8/8/4K3 b - - 0 3"},
		{"black captures", "4k3/8/8/8/3pP3/8/8/4K3 b - e3 0 1", "d4e3", "4k3/8/8/8/8/4p3/8/4K3 w - - 0 2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := mustState(t, tt.fen)
			if !hasMove(legalUCI(s), tt.move) {
				t.Fatalf("%s should be legal; legal moves: %v", tt.move, legalUCI(s))
			}
			if got := play(t, s, tt.move).ToFEN(); got != tt.wantFEN {
				t.Errorf("FEN = %q, want %q", got, tt.wantFEN)
			}
		})
	}
}

func TestEnPassantNotAvailableWithoutTarget(t *testing.T) {
	s := mustState(t, "4k3/8/8/3pP3/8/8/8/4K3 w - - 0 3")
	if hasMove(legalUCI(s), "e5d6") {
		t.Error("en passant was legal with no en passant target square")
	}
}

func TestEnPassantExpiresAfterOneMove(t *testing.T) {
	s := mustState(t, "4k3/3p4/8/4P3/8/8/7P/4K3 b - - 0 1")
	s = play(t, s, "d7d5")
	if !hasMove(legalUCI(s), "e5d6") {
		t.Fatal("en passant should be available immediately after the double push")
	}
	s = play(t, s, "h2h3")
	s = play(t, s, "e8e7")
	if hasMove(legalUCI(s), "e5d6") {
		t.Error("en passant stayed available after a move was made elsewhere")
	}
}

func TestEnPassantIllegalWhenItExposesTheKing(t *testing.T) {
	// Capturing b5xc6 removes both pawns from the fifth rank and exposes the white king to the rook.
	s := mustState(t, "8/8/8/KPp4r/8/8/8/7k w - c6 0 1")
	moves := legalUCI(s)
	if hasMove(moves, "b5c6") {
		t.Errorf("en passant that exposes the king was allowed; moves: %v", moves)
	}
	if !hasMove(moves, "b5b6") {
		t.Errorf("ordinary pawn push is missing; moves: %v", moves)
	}
}

func TestMovesWhileInCheck(t *testing.T) {
	tests := []struct {
		name string
		fen  string
		want []string
	}{
		{"king steps aside or captures the checker", "4k3/8/8/8/8/8/4r3/R3K3 w - - 0 1",
			[]string{"e1d1", "e1f1", "e1e2"}},
		{"a piece may block the check", "4r1k1/8/8/8/8/8/3B4/4K3 w - - 0 1",
			[]string{"d2e3", "e1d1", "e1f1", "e1f2"}},
		{"a piece may capture the checker", "4k3/8/8/8/8/3n4/8/3QK3 w - - 0 1",
			[]string{"d1d3", "e1d2", "e1e2", "e1f1"}},
		{"double check allows only king moves", "4k3/8/8/8/1b6/3n4/8/R3K3 w - - 0 1",
			[]string{"e1d1", "e1e2", "e1f1"}},
		{"kings cannot stand adjacent", "8/8/8/8/8/4k3/8/4K3 w - - 0 1",
			[]string{"e1d1", "e1f1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := mustState(t, tt.fen)
			assertMoves(t, legalUCI(s), tt.want)
		})
	}
}

func TestPinnedPieces(t *testing.T) {
	tests := []struct {
		name string
		fen  string
		from string
		want []string
	}{
		{"pinned knight cannot move", "4r1k1/8/8/8/8/8/4N3/4K3 w - - 0 1", "e2", nil},
		{"pinned rook may slide along the pin", "4r1k1/8/8/8/8/8/4R3/4K3 w - - 0 1", "e2",
			targets("e2", "e3", "e4", "e5", "e6", "e7", "e8")},
		{"pinned bishop cannot leave a rook pin", "4r1k1/8/8/8/8/8/4B3/4K3 w - - 0 1", "e2", nil},
		{"diagonally pinned pawn may only capture the pinner", "6k1/8/8/8/8/2b5/3P4/4K3 w - - 0 1", "d2", targets("d2", "c3")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := mustState(t, tt.fen)
			assertMoves(t, movesFrom(legalUCI(s), tt.from), tt.want)
		})
	}
}

func TestOngoingGameHasNoOutcome(t *testing.T) {
	for _, fen := range []string{startFEN, "4k3/8/8/8/8/8/8/R3K3 w - - 0 1"} {
		if _, done := outcomeOf(t, mustState(t, fen)); done {
			t.Errorf("game reported finished for %q", fen)
		}
	}
}

func TestCheckmate(t *testing.T) {
	t.Run("fool's mate position", func(t *testing.T) {
		s := mustState(t, "rnb1kbnr/pppp1ppp/8/4p3/6Pq/5P2/PPPPP2P/RNBQKBNR w KQkq - 1 3")
		if moves := legalUCI(s); len(moves) != 0 {
			t.Fatalf("checkmated side has legal moves: %v", moves)
		}
		res, done := outcomeOf(t, s)
		if !done {
			t.Fatal("checkmate was not reported as game over")
		}
		if got := res.Result.TerminationReason(); got != ReasonCheckmate {
			t.Errorf("reason = %q, want %q", got, ReasonCheckmate)
		}
		if len(res.Result.WinnerIDs) != 1 || res.Result.WinnerIDs[0] != blackID {
			t.Errorf("winners = %v, want [%s]", res.Result.WinnerIDs, blackID)
		}
		if res.loser == nil || *res.loser != whiteID {
			t.Errorf("loser = %v, want %s", res.loser, whiteID)
		}
	})
	t.Run("back rank mate after the final move", func(t *testing.T) {
		s := mustState(t, "6k1/5ppp/8/8/8/8/8/R3K3 w - - 0 1")
		s = play(t, s, "a1a8")
		res, done := outcomeOf(t, s)
		if !done {
			t.Fatal("back rank checkmate was not reported as game over")
		}
		if got := res.Result.TerminationReason(); got != ReasonCheckmate {
			t.Errorf("reason = %q, want %q", got, ReasonCheckmate)
		}
		if len(res.Result.WinnerIDs) != 1 || res.Result.WinnerIDs[0] != whiteID {
			t.Errorf("winners = %v, want [%s]", res.Result.WinnerIDs, whiteID)
		}
	})
}

func TestCheckIsNotCheckmateWhenEscapeExists(t *testing.T) {
	// White rook gives check, but the king can step to f7.
	s := mustState(t, "4k3/8/8/8/8/8/8/R3K2R w - - 0 1")
	s = play(t, s, "a1a8")
	if _, done := outcomeOf(t, s); done {
		t.Error("check with an escape square was reported as game over")
	}
}

func TestStalemate(t *testing.T) {
	s := mustState(t, "7k/5Q2/6K1/8/8/8/8/8 b - - 0 1")
	if moves := legalUCI(s); len(moves) != 0 {
		t.Fatalf("stalemated side has legal moves: %v", moves)
	}
	res, done := outcomeOf(t, s)
	if !done {
		t.Fatal("stalemate was not reported as game over")
	}
	if got := res.Result.TerminationReason(); got != ReasonStalemate {
		t.Errorf("reason = %q, want %q", got, ReasonStalemate)
	}
	if len(res.Result.WinnerIDs) != 0 {
		t.Errorf("stalemate produced winners: %v", res.Result.WinnerIDs)
	}
}

func TestInsufficientMaterial(t *testing.T) {
	tests := []struct {
		name     string
		fen      string
		wantDraw bool
	}{
		{"king versus king", "8/8/8/4k3/8/8/8/4K3 w - - 0 1", true},
		{"king and bishop versus king", "8/8/8/4k3/8/8/3B4/4K3 w - - 0 1", true},
		{"king and knight versus king", "8/8/8/4k3/8/8/3N4/4K3 w - - 0 1", true},
		{"king and bishop versus king and bishop on same color", "5b2/8/8/4k3/8/8/8/2B1K3 w - - 0 1", true},
		{"king and bishop versus king and bishop on opposite colors", "2b1k3/8/8/8/8/8/8/2B1K3 w - - 0 1", false},
		{"king and rook versus king", "8/8/8/4k3/8/8/8/R3K3 w - - 0 1", false},
		{"king and pawn versus king", "8/8/8/4k3/8/8/4P3/4K3 w - - 0 1", false},
		{"king and queen versus king", "8/8/8/4k3/8/8/8/Q3K3 w - - 0 1", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, done := outcomeOf(t, mustState(t, tt.fen))
			if done != tt.wantDraw {
				t.Fatalf("game over = %v, want %v", done, tt.wantDraw)
			}
			if done {
				if got := res.Result.TerminationReason(); got != ReasonInsufficientMaterial {
					t.Errorf("reason = %q, want %q", got, ReasonInsufficientMaterial)
				}
				if len(res.Result.WinnerIDs) != 0 {
					t.Errorf("draw produced winners: %v", res.Result.WinnerIDs)
				}
			}
		})
	}
}

func TestSeventyFiveMoveRule(t *testing.T) {
	// The 75-move rule is 150 halfmoves without a pawn move or capture.
	if _, done := outcomeOf(t, mustState(t, "4k3/8/8/8/8/8/8/R3K3 w - - 149 100")); done {
		t.Error("draw declared at 149 halfmoves")
	}
	res, done := outcomeOf(t, mustState(t, "4k3/8/8/8/8/8/8/R3K3 w - - 150 100"))
	if !done {
		t.Fatal("draw not declared at 150 halfmoves")
	}
	if got := res.Result.TerminationReason(); got != ReasonAutomaticDraw {
		t.Errorf("reason = %q, want %q", got, ReasonAutomaticDraw)
	}
}

// TODO: threefold should allow claim to end the game, fivefold is automatic
func TestThreefoldRepetitionEndsTheGame(t *testing.T) {
	s := mustState(t, startFEN)
	shuffle := []string{"g1f3", "g8f6", "f3g1", "f6g8"}
	for round := 0; round < 2; round++ {
		for _, m := range shuffle {
			s = play(t, s, m)
		}
	}
	res, done := outcomeOf(t, s)
	if !done {
		t.Fatal("game continued after the start position occurred three times")
	}
	if len(res.Result.WinnerIDs) != 0 {
		t.Errorf("repetition produced winners: %v", res.Result.WinnerIDs)
	}
}

func TestMoveCounters(t *testing.T) {
	s := mustState(t, startFEN)
	steps := []struct {
		move    string
		wantFEN string
	}{
		{"e2e4", "rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 1"},
		{"g8f6", "rnbqkb1r/pppppppp/5n2/8/4P3/8/PPPP1PPP/RNBQKBNR w KQkq - 1 2"},
		{"b1c3", "rnbqkb1r/pppppppp/5n2/8/4P3/2N5/PPPP1PPP/R1BQKBNR b KQkq - 2 2"},
		{"f6e4", "rnbqkb1r/pppppppp/8/8/4n3/2N5/PPPP1PPP/R1BQKBNR w KQkq - 0 3"}, // capture resets halfmove
	}
	for _, step := range steps {
		s = play(t, s, step.move)
		if got := s.ToFEN(); got != step.wantFEN {
			t.Fatalf("after %s: FEN = %q, want %q", step.move, got, step.wantFEN)
		}
	}
}

func TestApplyDoesNotMutateThePreviousState(t *testing.T) {
	s := mustState(t, startFEN)
	before := s.ToFEN()
	historyBefore := len(s.stateHistory)
	_ = play(t, s, "e2e4")
	if after := s.ToFEN(); after != before {
		t.Errorf("Apply mutated its input: %q became %q", before, after)
	}
	if len(s.stateHistory) != historyBefore {
		t.Errorf("Apply changed the input's history length from %d to %d", historyBefore, len(s.stateHistory))
	}
}

func TestApplyRecordsHistory(t *testing.T) {
	s := mustState(t, startFEN)
	next := play(t, s, "e2e4")
	if len(next.moveHistory) != len(s.moveHistory)+1 {
		t.Errorf("moveHistory length = %d, want %d", len(next.moveHistory), len(s.moveHistory)+1)
	}
	if len(next.stateHistory) != len(s.stateHistory)+1 {
		t.Errorf("stateHistory length = %d, want %d", len(next.stateHistory), len(s.stateHistory)+1)
	}
}

func TestNewStateProducesStartFEN(t *testing.T) {
	if got := newState(whiteID, blackID).ToFEN(); got != startFEN {
		t.Errorf("newState FEN = %q, want %q", got, startFEN)
	}
}

func TestFENRoundTrip(t *testing.T) {
	fens := []string{
		startFEN,
		"rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 1",
		"r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1",
		"8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - - 0 1",
		"4k3/8/8/8/8/8/8/R3K3 w - - 149 100",
		"r3k2r/8/8/8/8/8/8/R3K2R w Kq - 12 34",
	}
	for _, fen := range fens {
		t.Run(fen, func(t *testing.T) {
			if got := mustState(t, fen).ToFEN(); got != fen {
				t.Errorf("round trip changed the FEN\n  got  %q\n  want %q", got, fen)
			}
		})
	}
}

func TestStateFromFENBindsPlayers(t *testing.T) {
	s := mustState(t, startFEN)
	if s.whitePlayer != whiteID || s.blackPlayer != blackID {
		t.Errorf("players = (%q, %q), want (%q, %q)", s.whitePlayer, s.blackPlayer, whiteID, blackID)
	}
	if s.turn != White {
		t.Errorf("turn = %v, want white", s.turn)
	}
	if black := mustState(t, "4k3/8/8/8/8/8/8/4K3 b - - 0 1"); black.turn != Black {
		t.Errorf("turn = %v, want black", black.turn)
	}
}

func TestCastlingRightsString(t *testing.T) {
	tests := map[string]string{
		"r3k2r/8/8/8/8/8/8/R3K2R w KQkq - 0 1": "KQkq",
		"r3k2r/8/8/8/8/8/8/R3K2R w Kq - 0 1":   "Kq",
		"r3k2r/8/8/8/8/8/8/R3K2R w - - 0 1":    "-",
	}
	for fen, want := range tests {
		if got := mustState(t, fen).CastlingRights(); got != want {
			t.Errorf("CastlingRights for %q = %q, want %q", fen, got, want)
		}
	}
}

func TestSquareToUCI(t *testing.T) {
	tests := []struct {
		square Square
		want   string
	}{
		{Square{rank: 1, file: 1}, "a1"},
		{Square{rank: 1, file: 5}, "e1"},
		{Square{rank: 8, file: 8}, "h8"},
		{Square{rank: 4, file: 4}, "d4"},
	}
	for _, tt := range tests {
		if got := tt.square.toUCI(); got != tt.want {
			t.Errorf("Square{rank:%d, file:%d}.toUCI() = %q, want %q", tt.square.rank, tt.square.file, got, tt.want)
		}
	}
}

func TestDecodeMoveRoundTrip(t *testing.T) {
	for _, uci := range []string{"e2e4", "g1f3", "a7a8q", "h2h1n", "e1g1"} {
		t.Run(uci, func(t *testing.T) {
			m, err := DecodeMove([]byte(uci))
			if err != nil {
				t.Fatalf("DecodeMove(%q) error: %v", uci, err)
			}
			if got := m.toUCI(); got != uci {
				t.Errorf("toUCI() = %q, want %q", got, uci)
			}
			if got := string(m.Encode()); got != uci {
				t.Errorf("Encode() = %q, want %q", got, uci)
			}
		})
	}
}

func TestDecodeMoveRejectsMalformedPayloads(t *testing.T) {
	for _, payload := range []string{"", "e2", "e2e", "e9e4", "i2e4", "e2e0", "e7e8x", "e2e4qq", "E2E4", "e2-e4", "hello"} {
		t.Run(fmt.Sprintf("%q", payload), func(t *testing.T) {
			_, err := DecodeMove([]byte(payload))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !errors.Is(err, game.ErrMalformedAction) {
				t.Errorf("error %v does not wrap game.ErrMalformedAction", err)
			}
		})
	}
}

func TestValidateErrorKinds(t *testing.T) {
	g := &Game{}
	s := mustState(t, startFEN)

	err := g.Validate(s, game.Action{ActorID: whiteID, Payload: []byte("e2e5")})
	if !errors.Is(err, game.ErrIllegalAction) {
		t.Errorf("illegal move error = %v, want game.ErrIllegalAction", err)
	}
	err = g.Validate(s, game.Action{ActorID: whiteID, Payload: []byte("not a move")})
	if !errors.Is(err, game.ErrMalformedAction) {
		t.Errorf("malformed payload error = %v, want game.ErrMalformedAction", err)
	}
	// It is white's turn, so a black move is illegal in this position.
	err = g.Validate(s, game.Action{ActorID: whiteID, Payload: []byte("e7e5")})
	if !errors.Is(err, game.ErrIllegalAction) {
		t.Errorf("moving an opponent's piece error = %v, want game.ErrIllegalAction", err)
	}
}

func TestActiveTurnReturnsPlayerIDs(t *testing.T) {
	g := &Game{}
	s := mustState(t, startFEN)
	if got := g.ActiveTurn(s); len(got) != 1 || got[0] != whiteID {
		t.Errorf("ActiveTurn at the start = %v, want [%s]", got, whiteID)
	}
	s = play(t, s, "e2e4")
	if got := g.ActiveTurn(s); len(got) != 1 || got[0] != blackID {
		t.Errorf("ActiveTurn after white moves = %v, want [%s]", got, blackID)
	}
}

func TestNewStateAssignsColorsByJoinOrder(t *testing.T) {
	g := &Game{}
	roles := map[string]game.Role{"alice": game.RolePlayer, "watcher": game.RoleSpectator, "bob": game.RolePlayer}
	s := g.NewState([]string{"alice", "watcher", "bob"}, roles).(State)
	if s.whitePlayer != "alice" || s.blackPlayer != "bob" {
		t.Errorf("players = (%q, %q), want (alice, bob)", s.whitePlayer, s.blackPlayer)
	}
	if got := s.ToFEN(); got != startFEN {
		t.Errorf("initial FEN = %q, want %q", got, startFEN)
	}
}

func TestReadyToStart(t *testing.T) {
	g := &Game{}
	tests := []struct {
		name   string
		roster map[string]game.Role
		ready  map[string]bool
		want   bool
	}{
		{"two ready players",
			map[string]game.Role{"a": game.RolePlayer, "b": game.RolePlayer},
			map[string]bool{"a": true, "b": true}, true},
		{"one player not ready",
			map[string]game.Role{"a": game.RolePlayer, "b": game.RolePlayer},
			map[string]bool{"a": true}, false},
		{"only one player",
			map[string]game.Role{"a": game.RolePlayer},
			map[string]bool{"a": true}, false},
		{"unready spectators do not block the start",
			map[string]game.Role{"a": game.RolePlayer, "b": game.RolePlayer, "s": game.RoleSpectator},
			map[string]bool{"a": true, "b": true}, true},
		{"nobody present", map[string]game.Role{}, map[string]bool{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := g.ReadyToStart(tt.roster, tt.ready); got != tt.want {
				t.Errorf("ReadyToStart = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRolesDeclareTwoPlayersAndUnlimitedSpectators(t *testing.T) {
	roles := (&Game{}).Roles()
	byName := map[game.Role]game.RoleSpec{}
	for _, r := range roles {
		byName[r.Name] = r
	}
	if p := byName[game.RolePlayer]; p.Min != 2 || p.Max != 2 {
		t.Errorf("player role = %+v, want Min 2 Max 2", p)
	}
	if sp, ok := byName[game.RoleSpectator]; !ok || sp.Max != 0 {
		t.Errorf("spectator role = %+v (present=%v), want unlimited (Max 0)", sp, ok)
	}
}

func TestViewIsTheFullBoard(t *testing.T) {
	g := &Game{}
	s := mustState(t, startFEN)
	for _, id := range []string{whiteID, blackID, "spectator"} {
		v, ok := g.ViewFor(s, id).(State)
		if !ok {
			t.Fatalf("ViewFor(%q) returned %T, want State", id, g.ViewFor(s, id))
		}
		if v.ToFEN() != startFEN {
			t.Errorf("ViewFor(%q) FEN = %q, want %q", id, v.ToFEN(), startFEN)
		}
	}
}

// SAN output

// Resolves a UCI move against a state and writes it in SAN
func sanOf(t *testing.T, s State, uci string) string {
	t.Helper()
	m, err := s.moveFromPayload([]byte(uci))
	if err != nil {
		t.Fatalf("moveFromPayload(%q): %v", uci, err)
	}
	san, err := m.toSAN(s)
	if err != nil {
		t.Fatalf("toSAN(%q): %v", uci, err)
	}
	return san
}

func TestMoveToSAN(t *testing.T) {
	tests := []struct {
		name string
		fen  string
		move string
		want string
	}{
		{"pawn push has no symbol or file", startFEN, "e2e4", "e4"},
		{"knight move", startFEN, "g1f3", "Nf3"},
		{"pawn capture names its file", "4k3/8/8/3p4/4P3/8/8/4K3 w - - 0 1", "e4d5", "exd5"},
		{"piece capture", "4k3/8/8/3p4/8/8/8/3RK3 w - - 0 1", "d1d5", "Rxd5"},
		{"en passant is written as a capture to the target square", "4k3/8/8/3pP3/8/8/8/4K3 w - d6 0 3", "e5d6", "exd6"},
		{"promotion", "8/P6k/8/8/8/8/8/4K3 w - - 0 1", "a7a8q", "a8=Q"},
		{"promotion with capture and check", "1n2k3/P7/8/8/8/8/8/4K3 w - - 0 1", "a7b8q", "axb8=Q+"},
		{"check", "4k3/8/8/8/8/8/8/R3K3 w - - 0 1", "a1a8", "Ra8+"},
		{"checkmate", "6k1/5ppp/8/8/8/8/8/R3K3 w - - 0 1", "a1a8", "Ra8#"},
		{"kingside castle", "r3k2r/8/8/8/8/8/8/R3K2R w KQkq - 0 1", "e1g1", "0-0"},
		{"queenside castle", "r3k2r/8/8/8/8/8/8/R3K2R w KQkq - 0 1", "e1c1", "0-0-0"},
		{"file disambiguates rooks on one rank", "4k3/8/8/8/8/8/4K3/R6R w - - 0 1", "a1d1", "Rad1"},
		{"rank disambiguates rooks on one file", "4k3/R7/8/8/8/8/8/R3K3 w - - 0 1", "a1a4", "R1a4"},
		{"no disambiguation when the other rook is pinned", "b3k3/8/8/3R4/8/8/8/R6K w - - 0 1", "a1d1", "Rd1"},
		{"both file and rank when needed", "4k3/8/8/8/8/Q7/8/Q1Q1K3 w - - 0 1", "a1c3", "Qa1c3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanOf(t, mustState(t, tt.fen), tt.move); got != tt.want {
				t.Errorf("SAN = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestToSANRejectsUnresolvedMove(t *testing.T) {
	m, err := DecodeMove([]byte("e2e4"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.toSAN(mustState(t, startFEN)); err == nil {
		t.Error("toSAN accepted a move which was never resolved against a state")
	}
}

func TestResultToSAN(t *testing.T) {
	s := mustState(t, startFEN)
	loser := whiteID
	tests := []struct {
		name   string
		result ChessResult
		want   string
	}{
		{"white wins", ChessResult{game.Result{WinnerIDs: []string{whiteID}}, &loser, s}, "1-0"},
		{"black wins", ChessResult{game.Result{WinnerIDs: []string{blackID}}, &loser, s}, "0-1"},
		{"stalemate is a draw", ChessResult{game.Result{Reason: ReasonStalemate}, nil, s}, "1/2-1/2"},
		{"unknown winner", ChessResult{game.Result{WinnerIDs: []string{"someone else"}}, nil, s}, "*"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.result.toSAN(); got != tt.want {
				t.Errorf("toSAN() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Other State and Game behavior

func TestPlayersInCheck(t *testing.T) {
	white, black := mustState(t, "4k3/8/8/8/8/8/8/R3K3 w - - 0 1").PlayersInCheck()
	if white || black {
		t.Errorf("PlayersInCheck = (%v, %v), want (false, false)", white, black)
	}
	white, black = mustState(t, "R3k3/8/8/8/8/8/8/4K3 b - - 0 1").PlayersInCheck()
	if white || !black {
		t.Errorf("PlayersInCheck = (%v, %v), want (false, true)", white, black)
	}
}

func TestOnlyThePlayerToMoveHasLegalActions(t *testing.T) {
	g := &Game{}
	s := mustState(t, startFEN)
	if got := len(g.LegalActions(s, whiteID)); got != 20 {
		t.Errorf("white has %d actions at the start, want 20", got)
	}
	if got := len(g.LegalActions(s, blackID)); got != 0 {
		t.Errorf("black has %d actions on white's turn, want 0", got)
	}
	if got := len(g.LegalActions(s, "spectator")); got != 0 {
		t.Errorf("a spectator has %d actions, want 0", got)
	}
}

func TestValidateRejectsActionByWrongPlayer(t *testing.T) {
	g := &Game{}
	s := mustState(t, startFEN)
	// A legal move for white, submitted by black
	err := g.Validate(s, game.Action{ActorID: blackID, Payload: []byte("e2e4")})
	if !errors.Is(err, game.ErrIllegalAction) {
		t.Errorf("out of turn action error = %v, want game.ErrIllegalAction", err)
	}
}

func TestApplyLeavesStateUnchangedForUndecodablePayload(t *testing.T) {
	g := &Game{}
	s := mustState(t, startFEN)
	got := g.Apply(s, game.Action{ActorID: whiteID, Payload: []byte("garbage")}).(State)
	if got.ToFEN() != startFEN {
		t.Errorf("Apply changed the state for an undecodable payload: %q", got.ToFEN())
	}
}

func TestNewStatePanicsWithoutTwoPlayers(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("NewState did not panic with a single player")
		}
	}()
	(&Game{}).NewState([]string{"alone"}, map[string]game.Role{"alone": game.RolePlayer})
}

func TestStateFromFENRejectsInvalidInput(t *testing.T) {
	tests := []struct{ name, fen string }{
		{"too few fields", "8/8/8/8/8/8/8/8 w - -"},
		{"too few ranks", "8/8/8/8/8/8/8 w - - 0 1"},
		{"short rank", "4k3/8/8/8/8/8/8/3K3 w - - 0 1"},
		{"long rank", "4k4/8/8/8/8/8/8/4K3 w - - 0 1"},
		{"unknown piece", "4k3/8/8/8/8/8/8/4X3 w - - 0 1"},
		{"missing king", "4k3/8/8/8/8/8/8/8 w - - 0 1"},
		{"two white kings", "4k3/8/8/8/8/8/8/K3K3 w - - 0 1"},
		{"bad active color", "4k3/8/8/8/8/8/8/4K3 x - - 0 1"},
		{"bad castling rights", "4k3/8/8/8/8/8/8/4K3 w KX - 0 1"},
		{"repeated castling right", "4k3/8/8/8/8/8/8/4K3 w KK - 0 1"},
		{"bad en passant square", "4k3/8/8/8/8/8/8/4K3 w - e9 0 1"},
		{"en passant on an impossible rank", "4k3/8/8/8/8/8/8/4K3 w - e4 0 1"},
		{"bad halfmove clock", "4k3/8/8/8/8/8/8/4K3 w - - x 1"},
		{"bad fullmove number", "4k3/8/8/8/8/8/8/4K3 w - - 0 0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := StateFromFEN(tt.fen, whiteID, blackID); err == nil {
				t.Errorf("StateFromFEN accepted %q", tt.fen)
			}
		})
	}
}

func TestSquareShade(t *testing.T) {
	// a1 is dark, h1 is light, and shades alternate along ranks and files
	tests := map[string]string{"a1": "dark", "h1": "light", "a2": "light", "b2": "dark", "h8": "dark", "d4": "dark", "e4": "light"}
	for uci, want := range tests {
		sq, err := squareFromUCI(uci)
		if err != nil {
			t.Fatal(err)
		}
		if got := sq.Shade(); got != want {
			t.Errorf("%s shade = %q, want %q", uci, got, want)
		}
	}
}

func TestSquareFromUCIRejectsInvalidSquares(t *testing.T) {
	for _, uci := range []string{"", "e", "e44", "i1", "a0", "a9", "A1", "1a"} {
		if _, err := squareFromUCI(uci); err == nil {
			t.Errorf("squareFromUCI(%q) succeeded", uci)
		}
	}
}

func TestInsufficientMaterialIgnoresFurtherPieces(t *testing.T) {
	// Two knights can still (with help from the defender) deliver mate, so the game must go on
	if _, done := outcomeOf(t, mustState(t, "8/8/8/4k3/8/8/2NN4/4K3 w - - 0 1")); done {
		t.Error("king and two knights against king was declared a draw")
	}
}

func TestCheckmateTakesPriorityOverSeventyFiveMoveRule(t *testing.T) {
	res, done := outcomeOf(t, mustState(t, "rnb1kbnr/pppp1ppp/8/4p3/6Pq/5P2/PPPPP2P/RNBQKBNR w KQkq - 150 80"))
	if !done || res.Result.TerminationReason() != ReasonCheckmate {
		t.Errorf("outcome = %v (done=%v), want checkmate", res.Result.TerminationReason(), done)
	}
}

func TestPiecesCannotMoveOntoOrThroughTheirOwnPieces(t *testing.T) {
	// Every piece of the starting position is boxed in except knights and pawns
	moves := legalUCI(mustState(t, startFEN))
	for _, bad := range []string{"a1a2", "a1b1", "c1b2", "d1d2", "e1e2", "e1d1", "h1g1"} {
		if hasMove(moves, bad) {
			t.Errorf("%s was legal at the start", bad)
		}
	}
}

func TestSlidersMoveInEveryDirection(t *testing.T) {
	// Regression: movement patterns once subtracted unsigned coordinates, so moves toward a1 failed
	s := mustState(t, "7k/8/8/8/3Q4/8/8/4K3 w - - 0 1")
	for _, m := range []string{"d4a1", "d4g1", "d4a7", "d4h8", "d4a4", "d4d1"} {
		if !hasMove(legalUCI(s), m) {
			t.Errorf("queen move %s is missing", m)
		}
	}
}

func TestFoolsMateThroughTheGameInterface(t *testing.T) {
	g := &Game{}
	var state game.GameState = g.NewState(
		[]string{whiteID, blackID},
		map[string]game.Role{whiteID: game.RolePlayer, blackID: game.RolePlayer},
	)
	for _, uci := range []string{"f2f3", "e7e5", "g2g4", "d8h4"} {
		actor := g.ActiveTurn(state)[0] // The same lookup Match.ApplyAction does before validating
		action := game.Action{ActorID: actor, Payload: []byte(uci)}
		if err := g.Validate(state, action); err != nil {
			t.Fatalf("Validate(%s) by %s: %v", uci, actor, err)
		}
		if _, done := g.Outcome(state); done {
			t.Fatalf("game ended before %s", uci)
		}
		state = g.Apply(state, action)
	}
	result, done := g.Outcome(state)
	if !done {
		t.Fatal("fool's mate did not end the game")
	}
	cr := result.(ChessResult)
	if cr.Result.TerminationReason() != ReasonCheckmate || cr.toSAN() != "0-1" {
		t.Errorf("result = %v %q, want checkmate 0-1", cr.Result.TerminationReason(), cr.toSAN())
	}
	if got := len(state.(State).moveHistory); got != 4 {
		t.Errorf("move history has %d moves, want 4", got)
	}
}

func BenchmarkLegalActions(b *testing.B) {
	for _, pos := range perftPositions[:2] {
		s, _ := StateFromFEN(pos.fen, whiteID, blackID)
		b.Run(pos.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				s.legalActionSpecs(whiteID)
			}
		})
	}
}

func TestOnlyPawnsCanPromote(t *testing.T) {
	for _, pt := range allPieceTypes {
		if want := pt.name == pawnType.name; pt.canPromote != want {
			t.Errorf("%s canPromote = %v, want %v", pt.name, pt.canPromote, want)
		}
	}
}
