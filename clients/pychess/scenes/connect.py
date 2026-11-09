from __future__ import annotations

import pygame
from pygame import Rect, Surface

from ..networking import (
    HOSTED_SERVER, LOCAL_SERVER, Authed, Connected,
    Disconnected, ErrorEvent, Event,
    parse_server_address, readable_error
)
from .base import Scene
from ..ui import ERROR_COLOR, MUTED_TEXT_COLOR, TEXT_COLOR, Button, TextInput, draw_text

MAX_NAME_LENGTH = 24


class ConnectScene(Scene):
    def __init__(self, app, status: str = "") -> None:
        super().__init__(app)
        s = app.settings
        self.server = TextInput("Server", s.server, max_len=200, on_submit=self.connect)
        self.name = TextInput("Display name", s.display_name, max_len=MAX_NAME_LENGTH, on_submit=self.connect)
        self.fields = [self.server, self.name]
        for field in self.fields:
            field.LABEL_SHARE = 0.24  # a long address needs the room more than the label does
        self.server.focused = True
        self.local_button = Button("Local server", lambda: self.use_server(LOCAL_SERVER), kind="neutral")
        self.hosted_button = Button("Hosted server", lambda: self.use_server(HOSTED_SERVER), kind="neutral")
        self.connect_button = Button("Connect", self.connect)
        self.settings_button = Button("Settings", self.app.open_settings, kind="neutral")
        self.status = status
        self.status_is_error = bool(status)
        self.connecting = False
        self.unit = 16
        self.title_pos = (0, 0)
        self.status_pos = (0, 0)

    def on_enter(self) -> None:
        if self.app.auto and not self.status:
            self.connect()

    def use_server(self, address: str) -> None:
        """Fill in a known server. The player can still edit it before connecting."""
        self.server.text = address
        self.status, self.status_is_error = "", False

    def layout(self) -> None:
        width, height = self.app.screen.get_size()
        self.unit = unit = max(10, min(height // 18, width // 24))
        column = min(int(width * 0.92), unit * 24)
        x = (width - column) // 2
        row, gap = int(unit * 1.7), unit // 2
        top = max(unit, (height - (row * 5 + gap * 7 + unit * 4)) // 2)
        self.title_pos = (width // 2, top + unit)
        y = top + unit * 3
        self.server.rect = Rect(x, y, column, row)
        y += row + gap
        half = (column - gap) // 2
        self.local_button.rect = Rect(x, y, half, int(row * 0.85))
        self.hosted_button.rect = Rect(x + half + gap, y, column - half - gap, int(row * 0.85))
        y += int(row * 0.85) + gap * 2
        self.name.rect = Rect(x, y, column, row)
        y += row + gap
        self.connect_button.rect = Rect(x, y + gap, column, row)
        self.status_pos = (width // 2, self.connect_button.rect.bottom + unit)
        self.settings_button.rect = Rect(width - unit - unit * 6, unit // 2, unit * 6, int(unit * 1.5))

    def connect(self) -> None:
        if self.connecting:
            return
        name = self.name.text.strip()
        try:
            address = parse_server_address(self.server.text)
        except ValueError as error:
            return self._fail(str(error))
        if not name:
            return self._fail("Enter a display name")
        s = self.app.settings
        s.server, s.display_name = self.server.text.strip(), name
        s.save()
        self.connecting = True
        self.status, self.status_is_error = f"Connecting to {address.display}...", False
        self.app.net.connect(address, name)

    def _fail(self, text: str) -> None:
        self.status, self.status_is_error = text, True

    def on_net_event(self, event: Event) -> None:
        if isinstance(event, Connected):
            self.status = "Signing in..."
        elif isinstance(event, Authed):
            from .lobby import LobbyScene
            self.app.replace(LobbyScene(self.app))
        elif isinstance(event, ErrorEvent):
            self.connecting = False
            self._fail(readable_error(event) + (f" ({event.message})" if event.message else ""))
        elif isinstance(event, Disconnected):
            self.connecting = False
            self._fail(event.reason)

    def handle_event(self, event: pygame.event.Event) -> None:
        if event.type == pygame.KEYDOWN and event.key == pygame.K_TAB:
            focused = next((i for i, f in enumerate(self.fields) if f.focused), -1)
            step = -1 if event.mod & pygame.KMOD_SHIFT else 1
            for field in self.fields:
                field.focused = False
            self.fields[(focused + step) % len(self.fields)].focused = True
            return
        for button in (self.connect_button, self.local_button, self.hosted_button, self.settings_button):
            if button.handle_event(event):
                return
        for field in self.fields:
            field.handle_event(event)

    def draw(self, screen: Surface) -> None:
        draw_text(screen, "Chess", self.unit * 3, TEXT_COLOR, center=self.title_pos)
        for field in self.fields:
            field.draw(screen)
        self.local_button.draw(screen)
        self.hosted_button.draw(screen)
        self.connect_button.enabled = not self.connecting
        self.connect_button.draw(screen)
        self.settings_button.draw(screen)
        color = ERROR_COLOR if self.status_is_error else MUTED_TEXT_COLOR
        draw_text(screen, self.status, int(self.unit * 1.1), color, max_width=screen.get_width() - self.unit * 2, center=self.status_pos)