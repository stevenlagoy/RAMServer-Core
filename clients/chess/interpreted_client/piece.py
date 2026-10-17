from dataclasses import dataclass
from enum import Enum, StrEnum
from pygame import Surface
from pathlib import Path

ASSETS = Path.cwd() / "assets"
SPRITE_EXTENSION = "svg"


class PieceType(StrEnum):
    PAWN = "p"
    ROOK = "r"
    KNIGHT = "n"
    BISHOP = "b"
    QUEEN = "q"
    KING = "k"

class PlayerColor(Enum):
    WHITE = 1
    BLACK = 2

@dataclass
class Piece:
    rank: int
    file: int
    color: PlayerColor
    piece_type: PieceType
    sprite: Surface
    x: float | None = None
    y: float | None = None

def get_sprite_file(color: PlayerColor, piece_type: PieceType) -> Path:
    if color is PlayerColor.WHITE:
        return ASSETS / f"{piece_type.name.lower()}-w.{SPRITE_EXTENSION}"
    else:
        return ASSETS / f"{piece_type.name.lower()}-b.{SPRITE_EXTENSION}"
