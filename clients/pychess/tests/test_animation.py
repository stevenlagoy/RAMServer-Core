"""Tests of movement animation planning: which pieces move, which are captured, and the timing."""
import pytest

from clients.pychess.animation import GHOST_VISIBLE_UNTIL, LIFT_SCALE, PieceAnimation, duration_for, ease_in_out, plan_move
from clients.pychess.piece import Piece, PieceType, PlayerColor

START = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"


def plan(pieces_from_fen, before, after, uci):
    return plan_move(pieces_from_fen(before), pieces_from_fen(after), uci, 0.25)


def test_simple_move(pieces_from_fen):
    p = plan(pieces_from_fen, START, "rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 1", "e2e4")
    assert len(p.animations) == 1 and not p.ghosts
    a = p.animations[0]
    assert (a.start, a.end) == ((1, 4), (3, 4)) and a.piece.piece_type == PieceType.PAWN and not a.lift
    assert p.mover_color == PlayerColor.WHITE


def test_capture_leaves_a_ghost_on_the_target(pieces_from_fen):
    p = plan(pieces_from_fen, "4k3/8/8/3p4/4P3/8/8/4K3 w - - 0 1", "4k3/8/8/3P4/8/8/8/4K3 b - - 0 1", "e4d5")
    assert [(g.rank, g.file, g.piece_type, g.color) for g in p.ghosts] == [(4, 3, PieceType.PAWN, PlayerColor.BLACK)]


def test_en_passant_ghost_is_beside_the_origin(pieces_from_fen):
    p = plan(pieces_from_fen, "4k3/8/8/3pP3/8/8/8/4K3 w - d6 0 3", "4k3/8/3P4/8/8/8/8/4K3 b - - 0 3", "e5d6")
    assert [(g.rank, g.file) for g in p.ghosts] == [(4, 3)]  # the pawn on d5, not the empty d6


@pytest.mark.parametrize("uci,after,rook_from,rook_to", [
    ("e1g1", "r3k2r/8/8/8/8/8/8/R4RK1 b kq - 1 1", (0, 7), (0, 5)),
    ("e1c1", "r3k2r/8/8/8/8/8/8/2KR3R b kq - 1 1", (0, 0), (0, 3)),
    ("e8g8", "r4rk1/8/8/8/8/8/8/R3K2R w KQ - 1 2", (7, 7), (7, 5)),
    ("e8c8", "2kr3r/8/8/8/8/8/8/R3K2R w KQ - 1 2", (7, 0), (7, 3)),
])
def test_castling_moves_the_rook_too(pieces_from_fen, uci, after, rook_from, rook_to):
    before = "r3k2r/8/8/8/8/8/8/R3K2R " + ("w" if uci.startswith("e1") else "b") + " KQkq - 0 1"
    p = plan(pieces_from_fen, before, after, uci)
    assert len(p.animations) == 2 and not p.ghosts
    rook = p.animations[1]
    assert rook.piece.piece_type == PieceType.ROOK and (rook.start, rook.end) == (rook_from, rook_to)


def test_promotion_shows_a_pawn_until_it_arrives(pieces_from_fen):
    p = plan(pieces_from_fen, "4k3/P7/8/8/8/8/8/4K3 w - - 0 1", "Q3k3/8/8/8/8/8/8/4K3 b - - 0 1", "a7a8q")
    a = p.animations[0]
    assert a.piece.piece_type == PieceType.QUEEN and a.shown_type == PieceType.PAWN


def test_promotion_with_capture(pieces_from_fen):
    p = plan(pieces_from_fen, "1n2k3/P7/8/8/8/8/8/4K3 w - - 0 1", "1Q2k3/8/8/8/8/8/8/4K3 b - - 0 1", "a7b8q")
    assert len(p.ghosts) == 1 and p.ghosts[0].piece_type == PieceType.KNIGHT


def test_knights_lift_and_other_pieces_do_not(pieces_from_fen):
    knight = plan(pieces_from_fen, START, "rnbqkbnr/pppppppp/8/8/8/5N2/PPPPPPPP/RNBQKB1R b KQkq - 1 1", "g1f3")
    assert knight.animations[0].lift
    a = knight.animations[0]
    a.elapsed = a.duration / 2
    assert a.scale == pytest.approx(1 + LIFT_SCALE)
    a.elapsed = a.duration
    assert a.scale == pytest.approx(1.0)
    pawn = plan(pieces_from_fen, START, "rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 1", "e2e4")
    assert pawn.animations[0].scale == 1.0


@pytest.mark.parametrize("uci", ["", "garbage", "(=)", "e2e4x", "a1a2", "e4e5"])
def test_positions_which_do_not_fit_the_move_give_no_plan(pieces_from_fen, uci):
    after = "rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 1"
    assert plan_move(pieces_from_fen(START), pieces_from_fen(after), uci, 0.25) is None


def test_move_by_a_piece_of_another_color_than_the_origin_gives_no_plan(pieces_from_fen):
    after = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"
    assert plan_move(pieces_from_fen(START), pieces_from_fen(after), "e2e7", 0.25) is None  # e7 holds a black pawn after


def test_timing_and_easing():
    piece = Piece(0, 0, PlayerColor.WHITE, PieceType.ROOK)
    a = PieceAnimation(piece, (0, 0), (4, 0), 0.5)
    assert a.position() == (0, 0) and not a.done
    a.elapsed = 0.25
    assert a.position() == (2.0, 0.0)
    a.elapsed = 5
    assert a.done and a.position() == (4, 0)
    assert ease_in_out(0) == 0 and ease_in_out(1) == 1 and 0 < ease_in_out(0.25) < 0.25
    assert duration_for(2.0) == pytest.approx(duration_for(1.0) / 2)
    assert duration_for(0) > 0  # a zero speed cannot divide by zero


def test_plan_activity_and_ghost_timing(pieces_from_fen):
    p = plan(pieces_from_fen, "4k3/8/8/3p4/4P3/8/8/4K3 w - - 0 1", "4k3/8/8/3P4/8/8/8/4K3 b - - 0 1", "e4d5")
    assert p.active and p.visible_ghosts()
    p.advance(0.25 * GHOST_VISIBLE_UNTIL + 0.01)
    assert p.active and not p.visible_ghosts()  # the capture has landed, the piece is still settling
    p.advance(1)
    assert not p.active