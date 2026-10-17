from dataclasses import dataclass

from piece import PieceType
from board import file_ord_to_char

@dataclass
class Move:
    from_rank: int
    from_file: int
    to_rank: int
    to_file: int
    promotion: PieceType | None = None

    def encode(self) -> str:
        return f"{file_ord_to_char(self.from_file+1)}{self.from_rank+1}{file_ord_to_char(self.to_file+1)}{self.to_rank+1}"