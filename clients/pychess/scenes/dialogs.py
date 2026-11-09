from typing import Callable

from pygame import Rect, Surface
import pygame

from .base import Scene
from ..ui import TEXT_COLOR, Button, draw_panel, draw_text


class ConfirmDialog(Scene):
    overlay = True

    def __init__(self, app, message: str, confirm_label: str, on_confirm: Callable[[], None], *, danger: bool = True) -> None:
        super().__init__(app)
        self.message = message
        self.on_confirm = on_confirm
        self.yes = Button(confirm_label, self._confirm, kind="danger" if danger else "primary")
        self.no = Button("Cancel", self._cancel, kind="neutral")
        self.panel = Rect(0, 0, 0, 0)
 
    def _confirm(self) -> None:
        self.app.pop()
        self.on_confirm()
 
    def _cancel(self) -> None:
        self.app.pop()
 
    def layout(self) -> None:
        width, height = self.app.screen.get_size()
        unit = max(10, min(width, height) // 20)
        self.unit = unit
        self.panel = Rect(0, 0, unit * 12, unit * 6)
        self.panel.center = (width // 2, height // 2)
        row = unit * 3 // 2
        self.yes.rect = Rect(self.panel.x + unit // 2, self.panel.bottom - row - unit // 2, self.panel.width // 2 - unit, row)
        self.no.rect = Rect(self.panel.centerx + unit // 2, self.yes.rect.y, self.yes.rect.width, row)
 
    def handle_event(self, event: pygame.event.Event) -> None:
        if event.type == pygame.KEYDOWN and event.key == pygame.K_ESCAPE:
            self._cancel()
        elif event.type == pygame.KEYDOWN and event.key in (pygame.K_RETURN, pygame.K_KP_ENTER):
            self._confirm()
        else:
            self.yes.handle_event(event) or self.no.handle_event(event) # type: ignore
 
    def draw(self, screen: Surface) -> None:
        dim = Surface(screen.get_size(), pygame.SRCALPHA)
        dim.fill((0, 0, 0, 150))
        screen.blit(dim, (0, 0))
        draw_panel(screen, self.panel, radius=self.unit // 2)
        draw_text(
            screen, self.message, self.unit, TEXT_COLOR, max_width=self.panel.width - self.unit,
            center=(self.panel.centerx, self.panel.y + self.unit * 3 // 2)
        )
        self.yes.draw(screen)
        self.no.draw(screen)
 
