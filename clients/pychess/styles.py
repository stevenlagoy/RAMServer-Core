from __future__ import annotations
from dataclasses import dataclass
from typing import ClassVar, Mapping, Sequence

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


PRESETS: dict[str, BoardStyle] = {
    "STANDARD": BoardStyle.STANDARD,
    "COLORFUL": BoardStyle.COLORFUL,
    "OCEAN": BoardStyle((235, 245, 255), (10, 25, 50), (222, 227, 230), (80, 125, 160), (28, 40, 54), (18, 28, 40)),
    "TOURNAMENT": BoardStyle((255, 255, 255), (20, 20, 20), (238, 238, 210), (118, 150, 86), (49, 46, 43), (30, 30, 30)),
    "HIGH_CONTRAST": BoardStyle((255, 255, 255), (0, 0, 0), (255, 255, 255), (90, 90, 90), (0, 0, 0), (255, 255, 255)),
}
CUSTOM = "CUSTOM"
 
 
def style_names() -> list[str]:
    return [*PRESETS, CUSTOM]
 
 
def resolve_style(name: str, custom_colors: Mapping[str, Sequence[int]]) -> BoardStyle:
    """The style for a settings name. Unknown names give the standard style."""
    if name == CUSTOM:
        base = PRESETS["STANDARD"]
        pick = lambda key, fallback: tuple(custom_colors.get(key, fallback))  # noqa: E731
        return BoardStyle(
            base.white_color,
            base.black_color,
            pick("light", base.light_color), # type: ignore
            pick("dark", base.dark_color), # type: ignore
            pick("background", base.background_color), # type: ignore
            pick("border", base.border_color), # type: ignore
        )
    return PRESETS.get(name.upper(), PRESETS["STANDARD"])
 
