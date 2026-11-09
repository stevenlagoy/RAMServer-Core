import pygame
from pygame import Rect, Surface

from .base import Scene
from ..networking import STATUS_NAMES, HistoryEvent, LeftEvent, MatchSummary, SessionInfo, SessionsEvent, Snapshot, StateEvent, Event
from ..outcome import history_text
from ..ui import ACCENT_COLOR, MUTED_TEXT_COLOR, PANEL_ALT_COLOR, TEXT_COLOR, WARNING_COLOR, Button, TextInput, Widget, clipboard_put, draw_panel, draw_text


REFRESH_SECONDS = 3.0
SPECTATOR = "spectator"


class LobbyScene(Scene):
    def __init__(self, app, snapshot: Snapshot | None = None) -> None:
        super().__init__(app)
        self.net = app.net
        self.snapshot = snapshot
        self.local_ready = False
        self.sessions: tuple[SessionInfo, ...] = ()
        self.history: tuple[MatchSummary, ...] = ()
        self.refresh_timer = 0.0
        self.auto_ready_sent = False

        self.quick_button = Button("Quick match", lambda: self.net.join_session(""))
        self.create_button = Button("Create lobby", lambda: self.net.create_session(False))
        self.private_button = Button("Create private lobby", lambda: self.net.create_session(True), kind="neutral")
        self.join_id = TextInput("Lobby ID", "", max_len=80, on_submit=self.join_by_id)
        self.join_button = Button("Join lobby by ID", self.join_by_id, kind="neutral")
        self.refresh_button = Button("Refresh list", self.net.request_sessions, kind="neutral")
        self.session_buttons: list[tuple[SessionInfo, Button]] = []

        self.copy_button = Button("Copy ID", self.copy_id, kind="neutral")
        self.ready_button = Button("Ready", self.toggle_ready)
        self.leave_button = Button("Leave lobby", self.leave, kind="danger")

        self.settings_button = Button("Settings", self.app.open_settings, kind="neutral")
        self.disconnect_button = Button("Disconnect", lambda: self.app.show_connect(""), kind="neutral")

        self.unit = 16
        self.row = self.gap = 0
        self.left = self.right = self.roster_area = Rect(0, 0, 0, 0)

    @property
    def in_session(self) -> bool:
        return self.snapshot is not None

    def on_enter(self) -> None:
        self.net.request_sessions()
        if self.in_session:
            self.net.request_history()
        elif self.app.auto:
            self.net.join_session(self.app.auto_session or "")

    def join_by_id(self) -> None:
        session_id = self.join_id.text.strip()
        if not session_id:
            self.app.toast("Type the ID of the lobby to join", error=True)
            return
        self.net.join_session(session_id)

    def my_player(self):
        return self.snapshot.player(self.net.player_id) if self.snapshot else None

    def my_ready(self) -> bool:
        me = self.my_player()
        return me.ready if me and self.net.supports_lobby_features else self.local_ready

    def toggle_ready(self) -> None:
        self.local_ready = not self.my_ready()
        self.net.set_ready(self.local_ready)

    def leave(self) -> None:
        self.net.leave_session()

    def copy_id(self) -> None:
        if self.snapshot is None: return
        session_id = self.snapshot.session_id
        if clipboard_put(session_id):
            self.app.toast("Lobby ID copied")
        else:
            self.app.toast(f"Lobby ID: {session_id}")

    def on_net_event(self, event: Event) -> None:
        if isinstance(event, StateEvent):
            snapshot = event.snapshot
            if snapshot.in_progress:
                from .game import GameScene
                self.app.replace(GameScene(self.app, snapshot))
                return
            first = self.snapshot is None
            self.snapshot = snapshot
            if first:
                self.local_ready = False
                self.net.request_history()
                self.layout()
            if self.app.auto and not self.auto_ready_sent:
                self.auto_ready_sent = True
                self.local_ready = True
                self.net.set_ready(True)
        elif isinstance(event, LeftEvent):
            self.snapshot, self.history, self.local_ready = None, (), False
            self.net.request_sessions()
            self.layout()
        elif isinstance(event, SessionsEvent):
            self.sessions = event.sessions
            self.layout()
        elif isinstance(event, HistoryEvent):
            if event.session_id == self.net.session_id:
                self.history = event.matches
        else:
            super().on_net_event(event)

    def update(self, dt: float) -> None:
        self.refresh_timer += dt
        if self.refresh_timer >= REFRESH_SECONDS:
            self.refresh_timer = 0.0
            if not self.in_session:
                self.net.request_sessions()

    def layout(self) -> None:
        width, height = self.app.screen.get_size()
        self.unit = u = max(10, min(height // 24, width // 40))
        top = int(u * 3.6)
        left_width = int((width - 3 * u) * 0.56)
        self.left = Rect(u, top, left_width, height - top - u)
        self.left = Rect(self.left.right + u, top, width - 3 * u - left_width, height - top - u)
        self.row, self.gap = int(u * 1.7), u // 3
        row, gap = self.row, self.gap

        self.settings_button.rect = Rect(width - u - u * 6, u // 2, u * 6, int(u * 1.6))
        self.disconnect_button.rect = Rect(self.settings_button.rect.x - gap - u * 4, u // 2, u * 7, int(u * 1.6))

        pad = u // 2
        inner = Rect(self.left.x + pad, self.left.y + pad, self.left.width - 2 * pad, self.left.height - 2 * pad)
        y = inner.y + int(u * 1.6)
        for button in (self.quick_button, self.create_button, self.private_button):
            button.rect = Rect(inner.x, y, inner.width, row)
            y += row + gap
        y += u
        self.join_id.rect = Rect(inner.x, y, inner.width, row)
        self.join_button.rect = Rect(inner.x, y + row + gap, inner.width, row)
        self.private_button.enabled = self.net.supports_lobby_features
        self.copy_button.rect = Rect(inner.right - u * 6, inner.y, u * 6, row)
        bottom = inner.bottom - row
        ready_width = int(inner.width * 0.58)
        self.ready_button.rect = Rect(inner.x, bottom, ready_width, row)
        self.leave_button.rect = Rect(inner.x + ready_width + gap, bottom, inner.width - ready_width - gap, row)
        roster_top = inner.y + row + gap * 2
        self.roster_area = Rect(inner.x, roster_top, inner.width, bottom - gap * 3 - u - roster_top)
        list_row = int(u * 2.5)
        self.refresh_button.rect = Rect(self.right.x + pad, self.right.bottom - pad - row, self.right.width - 2 * pad, row)
        y = self.right.y + pad + int(u * 1.6)
        limit = self.refresh_button.rect.y - gap
        self.session_buttons = []
        self._list_row = list_row
        self._list_top = y
        for info in self.sessions:
            if y + list_row > limit: break
            button = Button("Join", lambda sid=info.session_id: self.net.join_session(sid))
            button.rect = Rect(self.right.right - pad - u * 4, y + (list_row - row) // 2, u * 4, row)
            button.enabled = info.joinable
            self.session_buttons.append((info, button))
            y += list_row + gap

    def _active_widgets(self) -> list[Widget]:
        if self.in_session:
            return [self.copy_button, self.ready_button, self.leave_button, self.settings_button, self.disconnect_button]
        return [
            self.quick_button, self.create_button, self.private_button, self.join_button, self.join_id,
            self.refresh_button, *(b for _, b in self.session_buttons), self.settings_button, self.disconnect_button
        ]

    def handle_event(self, event: pygame.event.Event) -> None:
        for widget in self._active_widgets():
            if widget.handle_event(event): break

    def draw(self, screen: Surface) -> None:
        u = self.unit
        draw_text(screen, "Chess lobby", int(u * 2.2), TEXT_COLOR, midleft=(u, int(u * 1.7)))
        draw_text(screen, f"Signed in as {self.app.settings.display_name}", int(u * 0.95), MUTED_TEXT_COLOR, midleft=(u + 2, int(u * 3.0)))
        draw_panel(screen, self.left)
        draw_panel(screen, self.right)
        if self.in_session: self._draw_session(screen)
        else: self._draw_browse(screen)
        self.settings_button.draw(screen)
        self.disconnect_button.draw(screen)

    def _draw_browse(self, screen: Surface) -> None:
        u, pad = self.unit, self.unit // 2
        draw_text(screen, "Find a match", int(u * 1.3), TEXT_COLOR, topleft=(self.left.x + pad, self.left.y + pad))
        for widget in (self.quick_button, self.create_button, self.private_button, self.join_id, self.join_button):
            widget.draw(screen)
        draw_text(screen, "Open lobbies", int(u * 1.3), TEXT_COLOR, topleft=(self.right.x + pad, self.right.y + pad))
        if not self.net.supports_lobby_features:
            draw_text(
                screen, "Browsing lobbies needs a server with protocol 1.2+", int(u * 0.95), WARNING_COLOR,
                max_width=self.right.width - 2 * pad, center=self.right.center
            )
            draw_text(
                screen, "Use Quick match, or join a lobby by its ID.", int(u * 0.95), MUTED_TEXT_COLOR,
                max_width=self.right.width - 2 * pad, center=(self.right.centerx, self.right.centery + int(u * 1.4))
            )
        elif not self.sessions:
            draw_text(screen, "No open lobbies right now", int(u * 1.0), MUTED_TEXT_COLOR, center=self.right.center)
        for info, button in self.session_buttons:
            row = Rect(self.right.x + pad, button.rect.y - (self._list_row - self.row) // 2, self.right.width - 2 * pad, self._list_row)
            pygame.draw.rect(screen, PANEL_ALT_COLOR, row, border_radius=u // 3)
            owner = next((p.display_name for p in info.players if p.player_id == info.owner_id), "") or "Someone"
            draw_text(screen, f"{owner}'s lobby", int(u * 1.1), TEXT_COLOR, max_width=row.width - u * 5, midleft=(row.x + u // 2, row.y + row.height // 3))
            names = ", ".join(p.display_name for p in info.players)
            detail = f"{STATUS_NAMES.get(info.status, '?')}: {names}"
            draw_text(screen, detail, int(u * 0.9), MUTED_TEXT_COLOR, max_width=row.width - u * 5, midleft=(row.x + u // 2, row.y + row.height * 2 // 3))
            button.draw(screen)
        self.refresh_button.draw(screen)

    def _draw_session(self, screen: Surface) -> None:
        assert self.snapshot is not None
        u, pad = self.unit, self.unit // 2
        snapshot = self.snapshot
        draw_text(
            screen, f"Lobby {snapshot.session_id}", int(u * 1.1), TEXT_COLOR,
            max_width=self.copy_button.rect.x - self.left.x - u, midleft=(self.left.x + pad, self.copy_button.rect.centery)
        )
        self.copy_button.draw(screen)

        area = self.roster_area
        row_height = min(int(u * 2.0), max(u, area.height // max(1, len(snapshot.players))))
        for index, player in enumerate(snapshot.players):
            row = Rect(area.x, area.y + index * (row_height + 3), area.width, row_height)
            if row.bottom > area.bottom: break
            pygame.draw.rect(screen, PANEL_ALT_COLOR, row, border_radius=u // 3)
            you = player.player_id == self.net.player_id
            name = (player.display_name or "Player") + (" (you)" if you else "")
            right = row.right - u // 2
            if player.role != SPECTATOR and snapshot.player(player.player_id):
                ready = self.my_ready() if you else player.ready
                if self.net.supports_lobby_features or you:
                    label, color = ("Ready", ACCENT_COLOR) if ready else ("Not ready", MUTED_TEXT_COLOR)
                    right = draw_text(screen, label, int(u * 0.95), color, midright=(right, row.centery)).left - u
            if player.role:
                right = draw_text(screen, player.role.capitalize(), int(u * 0.9), MUTED_TEXT_COLOR, midright=(right, row.centery)).left - u
            if player.player_id == snapshot.owner_id:
                right = draw_text(screen, "Host", int(u * 0.9), (230, 200, 90), midright=(right, row.centery)).left - u
                draw_text(screen, name, int(u * 1.1), TEXT_COLOR, max_width=right + u // 2 - (row.x + u // 2), midleft=(row.x + u // 2, row.centery))

        me = self.my_player()
        spectating = bool(me and me.role == SPECTATOR)
        players = [p for p in snapshot.players if p.role != SPECTATOR]
        if spectating: status = "You are spectating: the match starts when the players are ready"
        elif len(players) < 2: status = "Waiting for an opponent to join"
        else: status = "Waiting for both players to be ready"
        draw_text(screen, status, int(u * 0.95), MUTED_TEXT_COLOR, max_width=self.left.width - u, midleft=(self.left.x + pad, self.ready_button.rect.y - u))
        self.ready_button.label = "Cancel ready" if self.my_ready() else "Ready"
        self.ready_button.enabled = not spectating
        self.ready_button.draw(screen)
        self.leave_button.draw(screen)

        draw_text(screen, "Match history", int(u * 1.3), TEXT_COLOR, topleft=(self.right.x + pad, self.right.y + pad))
        if not self.net.supports_lobby_features:
            draw_text(screen, "Match history needs a server with protocol 1.2+", int(u * 0.95), WARNING_COLOR, max_width=self.right.width - 2 * pad, center=self.right.center)
        elif not self.history:
            draw_text(screen, "No matches played here yet", int(u * 1.0), MUTED_TEXT_COLOR, center=self.right.center)
        else:
            y = self.right.y + pad + int(u * 1.8)
            for number in range(len(self.history), 0, -1): # newest first
                match = self.history[number - 1]
                if y + u * 2 > self.right.bottom - pad: break
                text = f"{number}. " + history_text(match.winners, match.draw, match.reason, match.turns, self.net.player_id)
                row = Rect(self.right.x + pad, y, self.right.width - 2 * pad, int(u * 1.9))
                pygame.draw.rect(screen, PANEL_ALT_COLOR, row, border_radius=u // 3)
                draw_text(screen, text, int(u * 1.0), TEXT_COLOR, max_width=row.width - u, midleft=(row.x + u // 2, row.centery))
                y += row.height + 3