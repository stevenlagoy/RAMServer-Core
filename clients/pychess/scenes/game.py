from __future__ import annotations

from dataclasses import replace

import pygame
from pygame import Rect, Surface

from ..animation import MovePlan, duration_for, plan_move
from .base import Scene
from ..board import START_FEN, Board, square_from_uci
from .dialogs import ConfirmDialog
from ..end_screen import EndScreen
from ..fen import GameView, parse_fen
from ..move import ACCEPT_DRAW, DRAW_MARK, RESIGN, Move
from ..networking import STATUS_WAITING, ErrorEvent, Event, LegalMovesEvent, MatchEnded, Snapshot, StateEvent, readable_error
from ..outcome import result_text
from ..piece import Piece, PieceType, PlayerColor
from ..ui import *


SCREEN_CAPTION = "Chess"
BOARD_FILL = 0.9 # Fraction of the window taken by the board
SPECTATOR = "spectator"


def play_or_choose(moves: list[Move]) -> tuple[Move | None, list[Move]]:
    return (moves[0], []) if len(moves) == 1 else (None, moves)


def parse_squares(text: str) -> list[tuple[int, int]]:
    return [square for part in text.split(",") if (square := square_from_uci(part)) is not None]


class GameScene(Scene):
    def __init__(self, app, snapshot: Snapshot) -> None:
        super().__init__(app)
        self.net = app.net
        pygame.display.set_caption(SCREEN_CAPTION)
        self.board = Board(0, PlayerColor.WHITE)
        self.snapshot = snapshot
        self.view: GameView | None = None
        self.own_color: PlayerColor | None = None
        self.spectating = False
        self.my_turn = False # pieces can be picked up on my turn
        self.legal_moves: list[Move] = []
        self.can_accept_draw = False
        self.held_piece: Piece | None = None # being dragged
        self.selected: Piece | None = None # picked by click or drag
        self.deselect_on_release = False # clicking again to deselect rather than drag
        self.promotions: list[Move] = []
        self.draw_armed = False # next move carries a draw offer
        self.dismissed_offer_ply = -1 # draw offer declined
        self.end_screen: EndScreen | None = None # shown when a match ends
        self.plan: MovePlan | None = None  # opponent's move during animation
        self.displayed_plies = -1
        self.player_count = len(snapshot.players)
        self.last_move_uci = ""
        self.lobby_snapshot: Snapshot | None = None
        self.landscape = True
        self.unit = 16
        
        self.draw_button = Button("Offer draw with the next move", self.toggle_draw, kind="neutral")
        self.resign_button = Button("Resign", self.ask_resign, kind="danger")
        self.settings_button = Button("Settings", self.app.open_settings, kind="neutral")
        self.accept_button = Button("Accept", self.accept_draw)
        self.decline_button = Button("Delcine", self.decline_draw, kind="neutral")
        self.panel = self.top_row = self.bottom_row = self.status_rect = self.banner_rect = Rect(0, 0, 0, 0)

        self.apply_snapshot(snapshot, animate=False)

    def player_ids(self) -> list[str]:
        roles = {p.player_id: p.role for p in self.snapshot.players}
        return [pid for pid in self.snapshot.player_order if roles.get(pid, "") != SPECTATOR]

    def color_of(self, player_id: str) -> PlayerColor | None:
        ids = self.player_ids()
        if player_id in ids[:2]:
            return PlayerColor.WHITE if ids.index(player_id) == 0 else PlayerColor.BLACK
        return None

    def player_of(self, color: PlayerColor) -> str:
        ids = self.player_ids()
        index = 0 if color == PlayerColor.WHITE else 1
        return ids[index] if len(ids) > index else ""

    def name_of(self, player_id: str, fallback: str) -> str:
        player = self.snapshot.player(player_id)
        return (player.display_name if player and player.display_name else fallback)

    @property
    def bottom_color(self) -> PlayerColor:
        return self.own_color or PlayerColor.WHITE

    @property
    def animating(self) -> bool:
        return self.plan is not None and self.plan.active

    def apply_snapshot(self, snapshot: Snapshot, *, animate: bool) -> None:
        fen = snapshot.view.decode("utf-8", "replace")
        view = parse_fen(fen)
        if view is None: return
        settings = self.app.settings
        annotations = snapshot.annotations
        self.snapshot, self.view = snapshot, view
        self.own_color = self.color_of(self.net.player_id)
        self.spectating = self.own_color is None
        self.board.view_for = self.bottom_color
        plies = int(annotations["plies"]) if annotations.get("plies", "").isdigit() else view.plies

        before = self.board.pieces
        self.board.from_FEN(fen)
        self.plan = None
        last_move = annotations.get("last_move", "")
        if animate and settings.animations_enabled and last_move and self.displayed_plies >= 0 and plies == self.displayed_plies + 1:
            plan = plan_move(before, self.board.pieces, last_move, duration_for(settings.animation_speed))
            if plan is not None and plan.mover_color != self.own_color:
                self.plan = plan # player's own move was already shown by preview()
        self.displayed_plies = plies
        self.last_move_uci = last_move

        self.my_turn = not self.spectating and snapshot.active_player_id == self.net.player_id
        self.legal_moves, self.can_accept_draw = [], False
        self.held_piece = self.selected = None
        self.promotions = []
        if not self.my_turn:
            self.draw_armed = False
        elif snapshot.in_progress:
            self.net.request_legal_moves()

    def on_net_event(self, event: Event) -> None:
        if isinstance(event, StateEvent):
            self._on_state(event.snapshot)
        elif isinstance(event, LegalMovesEvent):
            self.legal_moves = [m for payload in event.payloads if (m := Move.from_uci(payload)) is not None] # Yay walrus operator
            self.can_accept_draw = DRAW_MARK in event.payloads
        elif isinstance(event, MatchEnded):
            self._on_match_end(event)
        elif isinstance(event, ErrorEvent):
            self.app.toast(readable_error(event), error=True)
            if self.end_screen is None:
                self.apply_snapshot(self.snapshot, animate=False) # reject restores last good state
        else:
            super().on_net_event(event)

    def _on_state(self, snapshot: Snapshot) -> None:
        if len(snapshot.players) < self.player_count:
            self.app.toast("A player left the lobby")
        self.player_count = len(snapshot.players)
        if snapshot.in_progress:
            if self.end_screen is not None: # Rematch started
                self.end_screen = None
                self.displayed_plies = -1
                self.board.from_FEN(START_FEN)
            self.apply_snapshot(snapshot, animate=True)
            return
        self.lobby_snapshot = snapshot
        if self.end_screen is not None:
            me = self.net.player_id
            self.end_screen.opponent_ready = any(p.ready and p.player_id != me and p.role != SPECTATOR for p in snapshot.players)

    def _on_match_end(self, event: MatchEnded) -> None:
        names = {p.player_id: p.display_name or "Player" for p in self.snapshot.players}
        title, subtitle = result_text(
            event.summary.winners, event.summary.draw, event.summary.reason,
            self.net.player_id, names, self.spectating
        )
        self.end_screen = EndScreen(title, subtitle, can_rematch=not self.spectating)
        self.held_piece = self.selected = None
        self.promotions = []
        self.my_turn = False
        # lobby after this match has same roster, resets readiness, no board
        self.lobby_snapshot = replace(
            self.snapshot, status=STATUS_WAITING, view=b"", annotations={}, active_player_id="",
            players=tuple(replace(p, ready=False) for p in self.snapshot.players),
        )

    def back_to_lobby(self) -> None:
        from .lobby import LobbyScene
        self.app.replace(LobbyScene(self.app, self.lobby_snapshot or self.snapshot))

    @property
    def draw_offerer(self) -> str:
        return self.snapshot.annotations.get("draw_offer", "")

    @property
    def offer_banner_visible(self) -> bool:
        offerer = self.draw_offerer
        return (
            bool(offerer) and offerer != self.net.player_id and not self.spectating and self.end_screen is None
            and self.dismissed_offer_ply != self.displayed_plies and (self.my_turn or self.can_accept_draw)
        )

    def toggle_draw(self) -> None:
        self.draw_armed = not self.draw_armed

    def accept_draw(self) -> None:
        self.net.send_action(ACCEPT_DRAW, self.snapshot.version)
        self.my_turn = False

    def decline_draw(self) -> None:
        self.dismissed_offer_ply = self.displayed_plies # no special message

    def ask_resign(self) -> None:
        self.app.push(ConfirmDialog(self.app, "Resign this game?", "Resign", self.resign))

    def resign(self) -> None:
        self.net.send_action(RESIGN, 0) # Resign is always valid

    def legal_moves_to(self, piece: Piece, square: tuple[int, int]) -> list[Move]:
        return [m for m in self.legal_moves if (m.from_rank, m.from_file) == (piece.rank, piece.file) and (m.to_rank, m.to_file) == square]

    def submit_move(self, move: Move) -> None:
        move = replace(move, offer_draw=self.draw_armed)
        self.net.send_action(move.encode(), self.snapshot.version)
        self.board.preview(move)
        self.last_move_uci = move.squares()
        self.my_turn = False
        self.draw_armed = False
        self.legal_moves = []
        self.selected = self.held_piece = None

    def handle_event(self, event: pygame.event.Event) -> None:
        if self.end_screen is not None:
            if event.type == pygame.MOUSEBUTTONDOWN and event.button == 1:
                button = self.end_screen.button_at(event.pos)
                if button == EndScreen.AGAIN:
                    self.net.set_ready(True) # Rematch starts when both players ready
                    self.end_screen.waiting = True
                elif button == EndScreen.LOBBY:
                    self.back_to_lobby()
            elif event.type == pygame.KEYDOWN and event.key == pygame.K_ESCAPE:
                self.app.open_settings()
            return
        if event.type == pygame.KEYDOWN and event.key == pygame.K_ESCAPE:
            if self.promotions or self.selected is not None:
                self.promotions, self.selected = [], None
            else: self.app.open_settings()
            return
        panel_buttons = [self.draw_button, self.resign_button, self.settings_button]
        if self.offer_banner_visible:
            panel_buttons += [self.accept_button, self.decline_button]
        for button in panel_buttons:
            if button.handle_event(event): return
        self._board_event(event)

    def _board_event(self, event: pygame.event.Event) -> None:
        board = self.board
        move: Move | None = None
        can_pick_up = self.my_turn and not self.animating and not self.spectating
        if event.type == pygame.MOUSEBUTTONDOWN and event.button == 1 and self.promotions:
            choice = board.promotion_choice_at(board.screen_to_board(event.pos))
            move = next((m for m in self.promotions if m.promotion == choice), None)
            self.promotions, self.selected = [], None
        elif event.type == pygame.MOUSEBUTTONDOWN and event.button == 1:
            pos = board.screen_to_board(event.pos)
            square = board.square_at(pos)
            piece = board.piece_at_square(*square) if square is not None else None
            if self.selected is not None and square is not None and (moves := self.legal_moves_to(self.selected, square)):
                move, self.promotions = play_or_choose(moves)
                self.selected = None
            # own pieces can be picked up on own turn
            elif can_pick_up and piece is not None and piece.color == self.own_color:
                self.deselect_on_release = piece is self.selected
                self.selected = self.held_piece = piece
                piece.x, piece.y = pos
            else: self.selected = None
        elif event.type == pygame.MOUSEMOTION and self.held_piece is not None:
            self.held_piece.x, self.held_piece.y = board.screen_to_board(event.pos)
        elif event.type == pygame.MOUSEBUTTONUP and event.button == 1 and self.held_piece is not None:
            held = self.held_piece
            held.x = held.y = None
            square = board.square_at(board.screen_to_board(event.pos))
            if square is not None and (moves := self.legal_moves_to(held, square)):
                move, self.promotions = play_or_choose(moves)
                self.selected = None
            elif square == (held.rank, held.file) and self.deselect_on_release:
                self.selected = None
            self.held_piece = None
        if move is not None: self.submit_move(move)

    def layout(self) -> None:
        width, height = self.app.screen.get_size()
        margin = max(6, min(width, height) // 60)
        self.landscape = width >= height * 1.2
        if self.landscape:
            panel_width = min(max(int(width * 0.27), 200), 380)
            area = Rect(0, 0, width - panel_width - margin, height)
            self.panel = Rect(area.right, margin, panel_width, height - 2)
        else:
            panel_height = min(max(int(height * 0.3), 150), 280)
            area = Rect(0, 0, width, height - panel_height)
            self.pane = Rect(margin, area.bottom, width - 2 * margin, panel_height - margin)
        self.board.resize(max(32, int(min(area.width, area.height) * BOARD_FILL)))
        self.board.center_on(area.center)
        self._layout_panel()

    def _layout_panel(self) -> None:
        panel, pad = self.panel, max(6, self.panel.width // 40)
        if self.landscape:
            u = self.unit = max(10, min(panel.width // 11, panel.height // 20))
            gap = max(4, u // 3)
            x, w, y = panel.x + pad, panel.width - 2 * pad, panel.y + pad
            self.top_row = Rect(x, y, w, int(u * 1.9)); y = self.top_row.bottom + gap
            self.status_rect = Rect(x, y, w, int(u * 3.0)); y = self.status_rect.bottom + gap
            self.bottom_row = Rect(x, y, w, int(u * 1.9)); y = self.bottom_row.bottom + gap
            self.banner_rect = Rect(x, y, w, int(u * 3.8)); y = self.banner_rect.bottom + gap
            button_height = int(u * 1.8)
            for button in (self.draw_button, self.resign_button, self.settings_button):
                button.rect = Rect(x, y, w, button_height)
                y += button_height + gap
        else:
            u = self.unit = max(10, min(panel.width // 24, panel.height // 8))
            gap = max(3, u // 4)
            left_width = int((panel.width - 3 * pad) * 0.58)
            x, y = panel.x + pad, panel.y + pad
            self.top_row = Rect(x, y, left_width, int(u * 1.7)); y = self.top_row.bottom + gap
            self.status_rect = Rect(x, y, left_width, int(u * 3.4)); y = self.status_rect.bottom + gap
            self.bottom_row = Rect(x, y, left_width, int(u * 1.7))
            self.banner_rect = self.status_rect # banner replaces status while shown
            right_x = x + left_width + pad
            right_width = panel.right - pad - right_x
            button_height = int(u * 1.9)
            y = panel.y + pad
            for button in (self.draw_button, self.resign_button, self.settings_button):
                button.rect = Rect(right_x, y, right_width, button_height)
                y += button_height + gap
        half = (self.banner_rect.width - 3 * (self.banner_rect.width // 24)) // 2
        inset = self.banner_rect.width // 24
        button_height = int(u * 1.5)
        self.accept_button.rect = Rect(self.banner_rect.x + inset, self.banner_rect.bottom - button_height - inset // 2, half, button_height)
        self.decline_button.rect = Rect(self.accept_button.rect.right + inset, self.accept_button.rect.y, half, button_height)

    def update(self, dt: float) -> None:
        if self.plan is not None:
            self.plan.advance(dt)
            if not self.plan.active:
                self.plan = None
        self._update_buttons()
        self._sync_board()

    def _update_buttons(self) -> None:
        playing = self.snapshot.in_progress and self.end_screen is None and not self.spectating
        self.draw_button.enabled = playing and self.my_turn
        self.draw_button.label = "Cancel draw offer" if self.draw_armed else "Offer draw with next move"
        self.draw_button.kind = "primary" if self.draw_armed else "neutral"
        self.resign_button.enabled = playing and self.net.supports_lobby_features

    def _sync_board(self) -> None:
        settings, board, annotations = self.app.settings, self.board, self.snapshot.annotations
        board.style = self.app.style
        board.show_coordinates = settings.show_coordinates
        board.animations = self.plan.animations if self.animating and self.plan else []
        board.ghosts = self.plan.visible_ghosts() if self.animating and self.plan else []
        board.overlays_visible = not self.animating
        board.last_move = None
        if settings.show_last_move:
            squares = parse_squares(self.last_move_uci[:2]) + parse_squares(self.last_move_uci[2:4])
            board.last_move = (squares[0], squares[1]) if len(squares) == 2 else None

        board.check_square, board.checkers, board.unsafe = None, [], []
        board.check_outline = settings.show_check_outline
        in_check = annotations.get("check")
        if in_check:
            board.check_square = board.king_square(PlayerColor.WHITE if in_check == "white" else PlayerColor.BLACK)
            if settings.show_checkers:
                board.checkers = parse_squares(annotations.get("checkers", ""))
        unsafe = parse_squares(annotations.get("king_unsafe", ""))
        king_selected = self.selected is not None and self.selected.piece_type == PieceType.KING and self.my_turn
        if settings.show_unsafe_squares == "always" or (settings.show_unsafe_squares == "selected" and king_selected):
            board.unsafe = unsafe

        board.selected = (self.selected.rank, self.selected.file) if self.selected is not None else None
        board.targets = set()
        if self.selected is not None and settings.show_legal_moves:
            board.targets = {(m.to_rank, m.to_file) for m in self.legal_moves if (m.from_rank, m.from_file) == (self.selected.rank, self.selected.file)}
        board.promotion = (((self.promotions[0].to_rank, self.promotions[0].to_file), self.own_color)
                           if self.promotions and self.own_color else None)
        board.turn_marker = None
        if self.view is not None and self.end_screen is None:
            to_move = PlayerColor.WHITE if self.view.white_to_move else PlayerColor.BLACK
            board.turn_marker = "bottom" if to_move == self.bottom_color else "top"

    def draw(self, screen: Surface) -> None:
        self.board.draw(screen)
        self._draw_panel(screen)
        if self.end_screen is not None:
            self.end_screen.draw(screen)

    def _draw_panel(self, screen: Surface) -> None:
        u = self.unit
        draw_panel(screen, self.panel, radius=u // 2)
        if self.view is None: return
        top_color = PlayerColor.BLACK if self.bottom_color == PlayerColor.WHITE else PlayerColor.WHITE
        self._draw_player_row(screen, self.top_row, top_color)
        self._draw_player_row(screen, self.bottom_row, self.bottom_color)

        banner = self.offer_banner_visible
        if banner:
            self._draw_banner(screen)
        if not banner or self.landscape: # banner takes status's place
            self._draw_status(screen)
        for button in (self.draw_button, self.resign_button, self.settings_button):
            button.draw(screen)

    def _draw_player_row(self, screen: Surface, rect: Rect, color: PlayerColor) -> None:
        assert self.view is not None
        u = self.unit
        style = self.app.style
        player_id = self.player_of(color)
        fallback = "White" if color == PlayerColor.WHITE else "Black"
        name = self.name_of(player_id, fallback)
        if player_id and player_id == self.net.player_id:
            name += " (you)"
        on_turn = self.end_screen is None and self.view.white_to_move == (color == PlayerColor.WHITE)
        pygame.draw.rect(screen, PANEL_ALT_COLOR, rect, border_radius=u // 3)
        if on_turn:
            pygame.draw.rect(screen, ACCENT_COLOR, rect, 2, border_radius=u // 3)
        chip = Rect(0, 0, int(u * 0.9), int(u * 0.9))
        chip.midleft = (rect.x + u // 2, rect.centery)
        pygame.draw.rect(screen, style.white_color if color == PlayerColor.WHITE else style.black_color, chip, border_radius=3)
        pygame.draw.rect(screen, MUTED_TEXT_COLOR, chip, 1, border_radius=3)
        right = rect.right - u // 2
        if on_turn:
            marker = (right - u // 3, rect.centery)
            pygame.draw.circle(screen, ACCENT_COLOR, marker, u // 3)
            right = marker[0] - u // 3 - 6
            if rect.width > u * 9:
                right = draw_text(screen, "To move", int(u * 0.85), ACCENT_COLOR, midright=(right, rect.centery)).left - 6
        draw_text(screen, name, int(u * 1.05), TEXT_COLOR, max_width=max(20, right - chip.right - u), midleft=(chip.right + u // 2, rect.centery))

    def _draw_status(self, screen: Surface) -> None:
        assert self.view is not None
        u, rect = self.unit, self.status_rect
        plies = self.displayed_plies if self.displayed_plies >= 0 else self.view.plies
        draw_text(
            screen, f"Move {self.view.fullmove}", int(u * 1.45), TEXT_COLOR, midleft=(rect.x + u // 3, rect.y + int(u * 0.9)),
            midright=(rect.right - u // 3, rect.y + int(u * 0.95))
        )
        text, color = self._status_text()
        draw_text(screen, text, int(u * 1.05), color, max_width=rect.width - u // 2, midleft=(rect.x + u // 3, rect.y + int(u * 2.25)))

    def _status_text(self) -> tuple[str, tuple[int, int, int]]:
        annotations = self.snapshot.annotations
        if self.end_screen is not None: return "Game over", MUTED_TEXT_COLOR
        in_check = annotations.get("check")
        to_move = PlayerColor.WHITE if self.view and self.view.white_to_move else PlayerColor.BLACK
        mover = "White" if to_move == PlayerColor.WHITE else "Black"
        if self.draw_offerer == self.net.player_id and not self.spectating: return "Draw offer sent", WARNING_COLOR
        if in_check:
            who = "You are" if to_move == self.own_color else f"{mover} is"
            return f"{who} in check", ERROR_COLOR
        if self.spectating: return f"{mover} to move", TEXT_COLOR
        return ("Your turn", TEXT_COLOR) if self.my_turn else ("Opponent's turn", MUTED_TEXT_COLOR)

    def _draw_banner(self, screen: Surface) -> None:
        u, rect = self.unit, self.banner_rect
        pygame.draw.rect(screen, (74, 62, 28), rect, border_radius=u // 3)
        pygame.draw.rect(screen, WARNING_COLOR, rect, 2, border_radius=u // 3)
        opponent = self.name_of(self.draw_offerer, "Your opponent")
        draw_text(
            screen, f"{opponent} offers a draw", int(u * 1.0), TEXT_COLOR,
            max_width=rect.width - u, center=(rect.centerx, rect.y + int(u * 0.75))
        )
        self.accept_button.draw(screen)
        self.decline_button.draw(screen)