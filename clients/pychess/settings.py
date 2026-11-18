from __future__ import annotations

from dataclasses import asdict, dataclass, field
import json
import os
from pathlib import Path
import tempfile
from typing import Any


SETTINGS_VERSION = 1
CUSTOM_COLOR_KEYS = ("light", "dark", "background", "border")
UNSAFE_MODES = ("off", "selected", "always")
SPEED_RANGE = (0.25, 4.0)


def default_custom_colors() -> dict[str, list[int]]:
    return {
        "light": [252, 246, 199],
        "dark": [54, 38, 20],
        "background": [68, 60, 52],
        "border": [40, 40, 40],
    }


def settings_path() -> Path:
    """Where settings live: RAMCHESS_CONFIG_DIR if set, otherwise ~/.ramchess."""
    base = os.environ.get("RAMCHESS_CONFIG_DIR")
    return (Path(base) if base else Path.home() / ".ramchess") / "chess_settings.json"


@dataclass
class Settings:
    # Connection
    server: str = "127.0.0.1:8080"
    display_name: str = "Python-Chess"
    # Appearance
    style: str = "STANDARD"  # a preset name from styles.PRESETS, or "CUSTOM"
    custom_colors: dict[str, list[int]] = field(default_factory=default_custom_colors)
    show_coordinates: bool = False
    show_last_move: bool = True
    show_legal_moves: bool = True
    # Animation
    animations_enabled: bool = True
    animation_speed: float = 1.0
    # Check display
    show_check_outline: bool = True
    show_checkers: bool = True
    show_unsafe_squares: str = "selected"  # one of UNSAFE_MODES

    @classmethod
    def load(cls, path: Path | None = None) -> Settings:
        """Read settings, with default values for missing or invalid."""
        path = path or settings_path()
        try:
            raw = json.loads(path.read_text(encoding="utf-8"))
        except (OSError, ValueError):
            return cls()
        return cls.from_dict(raw) if isinstance(raw, dict) else cls()
 
    @classmethod
    def from_dict(cls, raw: dict[str, Any]) -> Settings:
        result = cls()
        for name, default in asdict(result).items():
            if name not in raw:
                continue
            value = raw[name]
            if name == "custom_colors":
                result.custom_colors = _clean_colors(value)
            elif name == "show_unsafe_squares":
                if value in UNSAFE_MODES:
                    result.show_unsafe_squares = value
            elif name == "animation_speed":
                if isinstance(value, (int, float)) and not isinstance(value, bool):
                    result.animation_speed = min(max(float(value), SPEED_RANGE[0]), SPEED_RANGE[1])
            elif isinstance(default, bool):
                if isinstance(value, bool):
                    setattr(result, name, value)
            elif isinstance(default, str):
                if isinstance(value, str) and value:
                    setattr(result, name, value)
        if "server" not in raw and isinstance(raw.get("host"), str) and raw["host"]:
            # Settings saved before servers could be WebSocket URLs had a host and a port
            port = raw.get("port")
            valid = isinstance(port, int) and not isinstance(port, bool) and 1 <= port <= 65535
            result.server = f"{raw['host']}:{port if valid else 8080}"
        return result
 
    def save(self, path: Path | None = None) -> bool:
        """Write the settings atomically."""
        path = path or settings_path()
        data = {"version": SETTINGS_VERSION, **asdict(self)}
        try:
            path.parent.mkdir(parents=True, exist_ok=True)
            with tempfile.NamedTemporaryFile("w", encoding="utf-8", dir=path.parent, delete=False, suffix=".tmp") as handle:
                json.dump(data, handle, indent=2)
                temp = Path(handle.name)
            os.replace(temp, path)
            return True
        except OSError:
            return False
 
    def reset_appearance(self) -> None:
        """Restore every setting except the connection details."""
        defaults = Settings()
        for name in asdict(defaults):
            if name not in ("server", "display_name"):
                setattr(self, name, getattr(defaults, name))
 
 
def _clean_colors(value: Any) -> dict[str, list[int]]:
    result = default_custom_colors()
    if not isinstance(value, dict):
        return result
    for key in CUSTOM_COLOR_KEYS:
        rgb = value.get(key)
        if isinstance(rgb, list) and len(rgb) == 3 and all(isinstance(c, int) and not isinstance(c, bool) for c in rgb):
            result[key] = [min(max(c, 0), 255) for c in rgb]
    return result
 
