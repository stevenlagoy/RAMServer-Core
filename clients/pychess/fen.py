from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True)
class GameView:
    placement: str
    turn: str  # "w" or "b"
    castling: str
    en_passant: str
    halfmove: int
    fullmove: int

    @property
    def white_to_move(self) -> bool:
        return self.turn == "w"

    @property
    def plies(self) -> int:
        return (self.fullmove - 1) * 2 + (0 if self.white_to_move else 1)


def parse_fen(fen: str) -> GameView | None:
    """Parse a full FEN string. Returns None for anything which is not one."""
    fields = fen.split()
    if len(fields) != 6:
        return None
    placement, turn, castling, en_passant, halfmove, fullmove = fields
    if turn not in ("w", "b") or len(placement.split("/")) != 8:
        return None
    try:
        halfmove_clock, fullmove_number = int(halfmove), int(fullmove)
    except ValueError:
        return None
    if halfmove_clock < 0 or fullmove_number < 1:
        return None
    return GameView(placement, turn, castling, en_passant, halfmove_clock, fullmove_number)