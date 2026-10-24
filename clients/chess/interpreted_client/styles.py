from __future__ import annotations
from dataclasses import dataclass
from typing import ClassVar

type Color = tuple[int, int, int]

@dataclass
class BoardStyle:
    white_color: Color
    black_color: Color
    light_color: Color 
    dark_color: Color
    background_color: Color
    border_color: Color

    STANDARD: ClassVar['BoardStyle']
    COLORFUL: ClassVar['BoardStyle']

BoardStyle.STANDARD = BoardStyle(
    (255, 255, 255), (0, 0, 0),
    (252, 246, 199), (54, 38, 20),
    (68, 60, 52), (40, 40, 40),
)
BoardStyle.COLORFUL = BoardStyle(
    (200, 200, 255), (64, 0, 0),
    (200, 255, 200), (0, 65, 0),
    (10, 10, 10), (64, 64, 64)
)