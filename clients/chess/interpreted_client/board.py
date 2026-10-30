from __future__ import annotations
import io
import re
from pathlib import Path
from typing import TYPE_CHECKING

import pygame
from pygame import Surface, Rect
from pygame.draw import rect

from piece import Piece, PieceType, PlayerColor, get_sprite_file
from styles import BoardStyle

if TYPE_CHECKING:
    from move import Move  # move imports board, so only for type hints

START_FEN = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"

PIECE_SCALE = 0.9  # piece size as a fraction of the square size
SELECTED_COLOR = (255, 230, 0, 90)  # tint on the selected piece's square
TARGET_COLOR = (60, 160, 60, 170)  # markers on the squares the selected piece can move to
DIM_COLOR = (0, 0, 0, 120)  # darkens the board behind the promotion picker
PICKER_COLOR = (235, 235, 235)  # background of the promotion picker's squares
PROMOTION_ORDER = (PieceType.QUEEN, PieceType.KNIGHT, PieceType.ROOK, PieceType.BISHOP)


def load_svg(path: Path, size: int) -> Surface:
    """Load an SVG as a size x size sprite.

    The SVGs declare width/height 128 with a 45 unit viewBox, and SDL_image versions disagree about
    which one to use, so the native load size varies between machines. Rewriting width/height to the
    target size keeps the result sharp where they are honored; the final scale makes it exact everywhere.
    """
    svg = path.read_text(encoding="utf-8")
    svg = re.sub(r'(<svg\b[^>]*?\s)width="[^"]*"', rf'\g<1>width="{size}"', svg, count=1)
    svg = re.sub(r'(<svg\b[^>]*?\s)height="[^"]*"', rf'\g<1>height="{size}"', svg, count=1)
    sprite = pygame.image.load(io.BytesIO(svg.encode()), path.name)
    if sprite.get_size() != (size, size):
        sprite = pygame.transform.smoothscale(sprite, (size, size))
    return sprite.convert_alpha() if pygame.display.get_surface() is not None else sprite


class Board:
    """The chessboard: draws itself onto a target surface and converts between screen positions and squares.
    Call resize() whenever the window changes size."""

    def __init__(self, size: int, view_for: PlayerColor, pieces: list[Piece] | None = None, *, style: BoardStyle | None = None):
        self.view_for: PlayerColor = view_for
        self.pieces: list[Piece] = pieces or []
        self.style = style or BoardStyle.STANDARD
        self.squares_per_rank: int = 8
        self.squares_per_file: int = 8
        self.origin: tuple[int, int] = (0, 0)  # top-left corner on the screen
        self.selected: tuple[int, int] | None = None  # (rank, file) of the selected piece
        self.targets: set[tuple[int, int]] = set()  # (rank, file) squares the selected piece can move to
        self.promotion: tuple[tuple[int, int], PlayerColor] | None = None  # (rank, file) and color of a promotion being chosen
        self._sprites: dict[tuple[PlayerColor, PieceType], Surface] = {}  # scaled for the current square size
        self.resize(size)

    def resize(self, size: int) -> None:
        """Fit the board, border included, into a size x size area."""
        self.border_width = max(2, size // 60)
        self.square_size = max(1, (size - 2 * self.border_width) // self.squares_per_rank)
        self.size = self.square_size * self.squares_per_rank + 2 * self.border_width  # snapped so squares fill exactly
        self.surface = Surface((self.size, self.size))
        self._sprites.clear()

    def center_on(self, center: tuple[int, int]) -> None:
        self.origin = (center[0] - self.size // 2, center[1] - self.size // 2)

    def sprite(self, color: PlayerColor, piece_type: PieceType) -> Surface:
        key = (color, piece_type)
        if key not in self._sprites:
            self._sprites[key] = load_svg(get_sprite_file(color, piece_type), int(self.square_size * PIECE_SCALE))
        return self._sprites[key]

    def draw(self, target: Surface) -> None:
        surface = self.surface
        rect(surface, self.style.border_color, Rect(0, 0, self.size, self.size))

        for rank in range(self.squares_per_file):
            for file in range(self.squares_per_rank):
                x, y = self.square_to_coords(rank, file)
                color = self.style.dark_color if (rank + file) % 2 == 0 else self.style.light_color
                rect(surface, color, Rect(x, y, self.square_size, self.square_size))

        if self.selected is not None:
            tint = Surface((self.square_size, self.square_size), pygame.SRCALPHA)
            tint.fill(SELECTED_COLOR)
            surface.blit(tint, self.square_to_coords(*self.selected))

        # Pieces centered in their squares; a dragged piece is centered on the cursor and drawn last, on top
        dragged = [p for p in self.pieces if p.x is not None and p.y is not None]
        for piece in self.pieces:
            if piece not in dragged:
                x, y = self.square_to_coords(piece.rank, piece.file)
                self._draw_piece(surface, piece, (x + self.square_size / 2, y + self.square_size / 2))

        # Target markers go over the pieces, so a capture shows as a ring around the piece it takes
        markers = Surface((self.size, self.size), pygame.SRCALPHA)
        for rank, file in self.targets:
            x, y = self.square_to_coords(rank, file)
            center = (x + self.square_size // 2, y + self.square_size // 2)
            if self.piece_at_square(rank, file) is None:
                pygame.draw.circle(markers, TARGET_COLOR, center, self.square_size // 6)
            else:
                pygame.draw.circle(markers, TARGET_COLOR, center, self.square_size // 2, max(2, self.square_size // 12))
        surface.blit(markers, (0, 0))

        for piece in dragged:
            assert piece.x is not None and piece.y is not None
            self._draw_piece(surface, piece, (piece.x, piece.y))

        if self.promotion is not None:
            dim = Surface((self.size, self.size), pygame.SRCALPHA)
            dim.fill(DIM_COLOR)
            surface.blit(dim, (0, 0))
            _, color = self.promotion
            for piece_type, (x, y) in self._promotion_squares():
                rect(surface, PICKER_COLOR, Rect(x, y, self.square_size, self.square_size))
                sprite = self.sprite(color, piece_type)
                surface.blit(sprite, sprite.get_rect(center=(x + self.square_size // 2, y + self.square_size // 2)))

        target.blit(surface, self.origin)

    def _promotion_squares(self) -> list[tuple[PieceType, tuple[int, int]]]:
        """Each promotion choice with the top-left corner of its picker square. The picker starts on the
        promotion square and runs toward the middle of the board, so it always fits."""
        if self.promotion is None:
            return []
        (rank, file), _ = self.promotion
        x, y = self.square_to_coords(rank, file)
        step = self.square_size if y < self.size // 2 else -self.square_size
        return [(piece_type, (x, y + i * step)) for i, piece_type in enumerate(PROMOTION_ORDER)]

    def promotion_choice_at(self, pos: tuple[float, float]) -> PieceType | None:
        """The promotion piece under a board-relative position, if the picker is showing one there."""
        for piece_type, (x, y) in self._promotion_squares():
            if x <= pos[0] < x + self.square_size and y <= pos[1] < y + self.square_size:
                return piece_type
        return None

    def _draw_piece(self, surface: Surface, piece: Piece, center: tuple[float, float]) -> None:
        sprite = self.sprite(piece.color, piece.piece_type)
        surface.blit(sprite, sprite.get_rect(center=(round(center[0]), round(center[1]))))

    def preview(self, move: Move) -> None:
        """Show move on the board until the server's next state replaces it."""
        self.pieces = [p for p in self.pieces if (p.rank, p.file) != (move.to_rank, move.to_file)]  # a captured piece
        piece = self.piece_at_square(move.from_rank, move.from_file)
        if piece is not None:
            piece.rank, piece.file = move.to_rank, move.to_file
            piece.piece_type = move.promotion or piece.piece_type

    def square_to_coords(self, rank: int, file: int) -> tuple[int, int]:
        """Top-left corner of a square, relative to the board."""
        if self.view_for == PlayerColor.BLACK:
            rank = self.squares_per_file - rank - 1
            file = self.squares_per_rank - file - 1
        return (
            file * self.square_size + self.border_width,
            (self.squares_per_file - rank - 1) * self.square_size + self.border_width,
        )

    def screen_to_board(self, pos: tuple[float, float]) -> tuple[float, float]:
        return pos[0] - self.origin[0], pos[1] - self.origin[1]

    def square_at(self, pos: tuple[float, float]) -> tuple[int, int] | None:
        """The (rank, file) under a board-relative position, or None if it is outside the squares."""
        col = (pos[0] - self.border_width) // self.square_size
        row = (pos[1] - self.border_width) // self.square_size
        if not (0 <= col < self.squares_per_rank and 0 <= row < self.squares_per_file):
            return None
        rank, file = self.squares_per_file - int(row) - 1, int(col)
        if self.view_for == PlayerColor.BLACK:
            rank = self.squares_per_file - rank - 1
            file = self.squares_per_rank - file - 1
        return rank, file

    def piece_at_square(self, rank: int, file: int) -> Piece | None:
        for piece in self.pieces:
            if piece.rank == rank and piece.file == file:
                return piece
        return None

    def piece_at(self, pos: tuple[float, float]) -> Piece | None:
        square = self.square_at(pos)
        return self.piece_at_square(*square) if square is not None else None

    def from_FEN(self, FEN: str) -> None:
        self.pieces = []
        ranks = FEN.split(" ")[0].split("/")
        for i, string in enumerate(ranks):
            rank = self.squares_per_rank - i - 1
            file = 0
            for symbol in string:
                if symbol.isdigit():  # a run of empty squares
                    file += int(symbol)
                    continue
                color = PlayerColor.WHITE if symbol.isupper() else PlayerColor.BLACK
                self.pieces.append(Piece(rank, file, color, PieceType(symbol.lower())))
                file += 1


def file_ord_to_char(file: int) -> str:
    return chr(file + 96)


def file_char_to_ord(file: str) -> int:
    return ord(file) - 96
