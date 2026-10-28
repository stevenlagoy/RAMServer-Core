import re
from dataclasses import dataclass

from piece import PieceType
from board import file_ord_to_char, file_char_to_ord

UCI_PATTERN = re.compile(r"([a-h])([1-8])([a-h])([1-8])([qrbn])?")

@dataclass
class Move:
    from_rank: int
    from_file: int
    to_rank: int
    to_file: int
    promotion: PieceType | None = None

    @staticmethod
    def from_uci(uci: str) -> "Move | None":
        """Parse "e2e4", or "e7e8q" for a promotion. Returns None for anything else, such as a draw offer."""
        match = UCI_PATTERN.fullmatch(uci)
        if match is None:
            return None
        from_file, from_rank, to_file, to_rank, promotion = match.groups()
        return Move(
            int(from_rank) - 1, file_char_to_ord(from_file) - 1,
            int(to_rank) - 1, file_char_to_ord(to_file) - 1,
            PieceType(promotion) if promotion else None,
        )

    def __str__(self) -> str:
        promotion = self.promotion.value if self.promotion else ""
        return f"{file_ord_to_char(self.from_file+1)}{self.from_rank+1}{file_ord_to_char(self.to_file+1)}{self.to_rank+1}{promotion}"

    def encode(self) -> bytes:
        return self.__str__().encode("ascii")