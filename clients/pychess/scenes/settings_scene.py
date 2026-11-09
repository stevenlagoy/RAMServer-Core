from __future__ import annotations

import pygame
from pygame import Rect, Surface

from .base import Scene
from ..settings import CUSTOM_COLOR_KEYS, SPEED_RANGE, UNSAFE_MODES
from ..styles import CUSTOM, style_names
from ..ui import (BUTTON_DISABLED_COLOR, MUTED_TEXT_COLOR, PANEL_ALT_COLOR, TEXT_COLOR, Button, Selector, Slider,
                Toggle, Widget, draw_panel, draw_text)

COLOR_LABELS = {"light": "Light squares", "dark": "Dark squares", "background": "Background", "border": "Border"}
UNSAFE_LABELS = {"off": "Off", "selected": "When king selected", "always": "Always"}


class SettingsScene(Scene):
    overlay = True

    def __init__(self, app) -> None:
        super().__init__(app)
        self.settings = app.settings
        self.scroll = 0
        self.editing = 0  # which custom color the sliders edit: index of CUSTOM_COLOR_KEYS
        self.rows: list[tuple[str, str | Widget | None]] = []  # ("header", text), ("widget", widget) or ("swatches", None)
        self.panel = self.content = Rect(0, 0, 0, 0)
        self.unit = 16
        self.reset_button = Button("Reset to defaults", self.reset, kind="neutral")
        self.done_button = Button("Done", self.close)
        self.build()

    def build(self) -> None:
        s = self.settings
        names = style_names()
        rows: list[tuple[str, str | Widget | None]] = [("header", "Appearance")]
        rows.append(("widget", Selector("Board style", [n.replace("_", " ").title() for n in names],
                                         names.index(s.style) if s.style in names else 0, self._set_style)))
        if s.style == CUSTOM:
            key = CUSTOM_COLOR_KEYS[self.editing]
            rows.append(("widget", Selector("Edit color", [COLOR_LABELS[k] for k in CUSTOM_COLOR_KEYS], self.editing, self._set_editing)))
            for channel, label in enumerate(("Red", "Green", "Blue")):
                rows.append(
                    ("widget", Slider(label, 0, 255, s.custom_colors[key][channel], 1,
                    lambda v, c=channel: self._set_channel(c, int(v)), lambda v: str(int(v))))
                )
        rows.append(("swatches", None))
        rows.append(("widget", Toggle("Coordinates", s.show_coordinates, lambda v: setattr(s, "show_coordinates", v))))
        rows.append(("widget", Toggle("Highlight last move", s.show_last_move, lambda v: setattr(s, "show_last_move", v))))
        rows.append(("widget", Toggle("Show legal moves", s.show_legal_moves, lambda v: setattr(s, "show_legal_moves", v))))
        rows.append(("header", "Animations"))
        self.speed = Slider(
            "Speed", SPEED_RANGE[0] * 2, 3.0, min(max(s.animation_speed, SPEED_RANGE[0] * 2), 3.0), 0.25,
            lambda v: setattr(s, "animation_speed", v), lambda v: f"{v:g}x"
        )
        self.speed.enabled = s.animations_enabled
        rows.append(("widget", Toggle("Animate moves", s.animations_enabled, self._set_animations)))
        rows.append(("widget", self.speed))
        rows.append(("header", "Check display"))
        rows.append(("widget", Toggle("Red king outline", s.show_check_outline, lambda v: setattr(s, "show_check_outline", v))))
        rows.append(("widget", Toggle("Show checking pieces", s.show_checkers, lambda v: setattr(s, "show_checkers", v))))
        rows.append(
            ("widget", Selector("Unsafe king squares", [UNSAFE_LABELS[m] for m in UNSAFE_MODES],
            UNSAFE_MODES.index(s.show_unsafe_squares), self._set_unsafe))
        )
        self.rows = rows
        self.layout()

    @property
    def widgets(self) -> list[Widget]:
        return [item for kind, item in self.rows if kind == "widget" and isinstance(item, Widget)]

    def _set_style(self, index: int) -> None:
        self.settings.style = style_names()[index]
        self.build()

    def _set_editing(self, index: int) -> None:
        self.editing = index
        self.build()

    def _set_channel(self, channel: int, value: int) -> None:
        self.settings.custom_colors[CUSTOM_COLOR_KEYS[self.editing]][channel] = value

    def _set_animations(self, value: bool) -> None:
        self.settings.animations_enabled = value
        self.speed.enabled = value

    def _set_unsafe(self, index: int) -> None:
        self.settings.show_unsafe_squares = UNSAFE_MODES[index]

    def reset(self) -> None:
        self.settings.reset_appearance()
        self.editing = 0
        self.build()

    def close(self) -> None:
        self.settings.save()
        self.app.pop()

    def _row_height(self, kind: str) -> int:
        return int(self.unit * (1.3 if kind == "header" else 1.7))

    def layout(self) -> None:
        width, height = self.app.screen.get_size()
        self.unit = unit = max(10, min(height // 24, width // 30))
        self.panel = Rect(0, 0, min(int(width * 0.94), unit * 28), int(height * 0.92))
        self.panel.center = (width // 2, height // 2)
        footer = int(unit * 2.4)
        self.content = Rect(
            self.panel.x + unit, self.panel.y + int(unit * 2.6), self.panel.width - 2 * unit,
            self.panel.height - int(unit * 2.6) - footer
        )
        total = sum(self._row_height(kind) for kind, _ in self.rows)
        self.scroll = min(max(self.scroll, 0), max(0, total - self.content.height))
        y = self.content.y - self.scroll
        for kind, item in self.rows:
            row = self._row_height(kind)
            if isinstance(item, Widget):
                item.rect = Rect(self.content.x, y, self.content.width - unit // 2, row)
            y += row
        button_height = int(unit * 1.6)
        top = self.panel.bottom - button_height - unit // 2
        self.reset_button.rect = Rect(self.panel.x + unit, top, self.panel.width // 2 - int(unit * 1.5), button_height)
        self.done_button.rect = Rect(self.panel.centerx + unit // 2, top, self.panel.width // 2 - int(unit * 1.5), button_height)

    def handle_event(self, event: pygame.event.Event) -> None:
        if event.type == pygame.KEYDOWN and event.key == pygame.K_ESCAPE:
            self.close()
            return
        if event.type == pygame.MOUSEWHEEL:
            self.scroll -= event.y * self.unit * 2
            self.layout()
            return
        if self.reset_button.handle_event(event) or self.done_button.handle_event(event):
            return
        in_content = event.type not in (pygame.MOUSEBUTTONDOWN,) or self.content.collidepoint(event.pos)
        if in_content:
            for widget in self.widgets:
                if widget.handle_event(event):
                    break

    def draw(self, screen: Surface) -> None:
        dim = Surface(screen.get_size(), pygame.SRCALPHA)
        dim.fill((0, 0, 0, 170))
        screen.blit(dim, (0, 0))
        unit = self.unit
        draw_panel(screen, self.panel, radius=unit // 2)
        draw_text(screen, "Settings", int(unit * 1.6), TEXT_COLOR, midleft=(self.panel.x + unit, self.panel.y + int(unit * 1.4)))
        draw_text(screen, "Esc to close", int(unit * 0.9), MUTED_TEXT_COLOR, midright=(self.panel.right - unit, self.panel.y + int(unit * 1.4)))

        clip = screen.get_clip()
        screen.set_clip(self.content)
        y = self.content.y - self.scroll
        for kind, item in self.rows:
            row = self._row_height(kind)
            if kind == "header":
                draw_text(screen, str(item), int(unit * 1.1), (120, 190, 140), midleft=(self.content.x + 4, y + row // 2 + unit // 4))
            elif kind == "swatches":
                self._draw_swatches(screen, Rect(self.content.x, y, self.content.width, row))
            elif isinstance(item, Widget):
                item.draw(screen)
            y += row
        screen.set_clip(clip)

        total = sum(self._row_height(kind) for kind, _ in self.rows)
        if total > self.content.height:  # scroll bar
            bar_height = max(unit, int(self.content.height * self.content.height / total))
            bar_y = self.content.y + int((self.content.height - bar_height) * self.scroll / (total - self.content.height))
            pygame.draw.rect(screen, BUTTON_DISABLED_COLOR, Rect(self.content.right - 4, bar_y, 4, bar_height), border_radius=2)
        self.reset_button.draw(screen)
        self.done_button.draw(screen)

    def _draw_swatches(self, screen: Surface, area: Rect) -> None:
        """A small preview of the current style."""
        style = self.app.style
        size = area.height - 8
        x, y = area.x + 4, area.y + 4
        pygame.draw.rect(screen, style.border_color, Rect(x - 3, y - 3, size * 2 + 6, size + 6))
        for i in range(8):
            color = style.dark_color if (i % 4 + i // 4) % 2 == 0 else style.light_color
            cell = size // 2
            pygame.draw.rect(screen, color, Rect(x + (i % 4) * cell, y + (i // 4) * cell, cell, cell))
        draw_text(screen, "Preview", int(self.unit * 0.9), MUTED_TEXT_COLOR, midleft=(x + size * 2 + 14, area.centery))
        for i, color in enumerate((style.background_color, style.border_color)):
            chip = Rect(area.right - (2 - i) * (size + 8) - 4, y, size, size)
            pygame.draw.rect(screen, color, chip, border_radius=4)
            pygame.draw.rect(screen, PANEL_ALT_COLOR, chip, 2, border_radius=4)
            