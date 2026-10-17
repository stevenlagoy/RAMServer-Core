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

BoardStyle.STANDARD = BoardStyle(
    (255, 255, 255), (0, 0, 0),
    (252, 246, 199), (54, 38, 20),
    (68, 60, 52), (40, 40, 40),
)