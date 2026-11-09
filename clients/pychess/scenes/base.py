from __future__ import annotations

from typing import TYPE_CHECKING

import pygame

from ..networking import Disconnected, ErrorEvent, Event, readable_error

if TYPE_CHECKING:
    from ..app import App


class Scene:
    overlay = False

    def __init__(self, app: App) -> None:
        self.app = app
        self._layout_size: tuple[int, int] = (0, 0)

    def on_enter(self) -> None: pass

    def on_resume(self) -> None: pass

    def layout(self) -> None: pass

    def on_resize(self, size: tuple[int, int]) -> None:
        self.layout()
        self._layout_size = size
 
    def ensure_layout(self, screen: pygame.Surface) -> None:
        if screen.get_size() != self._layout_size:
            self._layout_size = screen.get_size()
            self.layout()

    def handle_event(self, event: pygame.event.Event) -> None: pass

    def update(self, dt: float) -> None: pass

    def draw(self, screen: pygame.Surface) -> None: pass

    def on_net_event(self, event: Event) -> None:
        if isinstance(event, ErrorEvent):
            self.app.toast(readable_error(event), error=True)
        elif isinstance(event, Disconnected):
            self.app.show_connect(event.reason)
