from __future__ import annotations
from pathlib import Path
from pygame import Surface, Rect
from pygame.image import load as pyg_load
from pygame.draw import rect

from piece import Piece, PieceType, PlayerColor, get_sprite_file
from styles import BoardStyle

START_FEN = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"


class Board(Surface):
    def __init__(self, dimensions: tuple[float, float], pieces: list[Piece] | None = None, *, style: BoardStyle | None = None):
        super().__init__(dimensions)
        self.pieces: list[Piece] = pieces or []
        self.style = style or BoardStyle.STANDARD
        self.squares_per_rank: int = 8
        self.squares_per_file: int = 8

        self.border_radius: int = 10
        self.internal_width = self.get_width() - (self.border_radius * 2)
        self.internal_height = self.get_height() - (self.border_radius * 2)
        self.square_width = self.internal_width // self.squares_per_rank
        self.square_height = self.internal_height // self.squares_per_file

    def render(self) -> None:
        # Draw border
        border = Rect(0, 0, self.get_width(), self.get_height())
        rect(self, self.style.border_color, border)

        # Draw squares
        for rank in range(self.squares_per_file):
            for file in range(self.squares_per_rank):
                x, y = self.square_to_coords(rank, file)
                color = self.style.dark_color if (rank + file) % 2 == 1 else self.style.light_color
                square = Rect(x, y, self.square_width, self.square_height)
                rect(self, color, square)

        # Draw pieces
        for piece in self.pieces:
            coords = self.square_to_coords(piece.rank, piece.file)
            if piece.x is not None and piece.y is not None:
                coords = (piece.x, piece.y)
            self.blit(piece.sprite, coords)

    def square_to_coords(self, rank: int | tuple[int, int], file: int | None = None) -> tuple[float, float]:
        if isinstance(rank, tuple):
            rank, file = rank
        assert rank is not None and file is not None
        return (
            file * self.square_width + self.border_radius,
            (self.squares_per_file - rank - 1) * self.square_height + self.border_radius,
        )

    def coords_to_square(self, x: float | tuple[float, float], y: float | None = None) -> tuple[int, int]:
        if isinstance(x, tuple):
            x, y = x
        assert x is not None and y is not None
        file = int((x - self.border_radius) / self.square_width)
        rank = self.squares_per_file - int((y - self.border_radius) / self.square_height) - 1
        return rank, file

    def piece_at_coords(self, x: float | tuple[float, float], y: float | None = None) -> Piece | None:
        if isinstance(x, tuple):
            x, y = x
        assert x is not None and y is not None
        return self.piece_at_square(self.coords_to_square(x, y))

    def piece_at_square(self, rank: int | tuple[int, int], file: int | None = None) -> Piece | None:
        if isinstance(rank, tuple):
            rank, file = rank
        for piece in self.pieces:
            if piece.rank == rank and piece.file == file:
                return piece
        return None

    def from_FEN(self, FEN: str) -> None:
        ranks = FEN.split(" ")[0].split("/")
        for i, string in enumerate(ranks):
            rank = self.squares_per_rank - i - 1
            file = 0
            for symbol in string:
                is_white = symbol == symbol.capitalize()
                color = PlayerColor.WHITE if is_white else PlayerColor.BLACK
                symbol = symbol.lower()
                piece_type: PieceType | None = None
                try:
                    piece_type = PieceType(symbol)
                except ValueError as e:
                    # This is a digit marking empty space
                    skip = int(symbol)
                    file += skip
                    continue
                assert piece_type is not None
                self.pieces.append(Piece(rank, file, color, piece_type, pyg_load(get_sprite_file(color, piece_type))))
                file += 1

def file_ord_to_char(file: int) -> str:
    return chr(file + 96)

def file_char_to_ord(file: str) -> int:
    return int(file) - 96