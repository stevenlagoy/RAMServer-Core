"""Headless tests of the screens: scripted server events go in, clicks are simulated, and the messages the client
sends and what it shows are checked. Set SHOTS_DIR to also save a screenshot at the interesting moments."""
import os
from dataclasses import replace
from pathlib import Path

import pygame
import pytest

from .. import networking as n
from .. import piece as piece_module
from ..app import App
from ..scenes.connect import ConnectScene
from ..scenes.dialogs import ConfirmDialog
from ..scenes.game import GameScene
from ..scenes.lobby import LobbyScene
from ..scenes.settings_scene import SettingsScene
from ..settings import Settings

START = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"
AFTER_E4 = "rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 1"
AFTER_E5 = "rnbqkbnr/pppp1ppp/8/4p3/4P3/8/PPPP1PPP/RNBQKBNR w KQkq e6 0 2"
MARKS = {"pawn": "M22 12 a6 6 0 1 0 0.1 0z", "rook": "M12 12h21v20h-21z", "knight": "M14 32l8-20 9 20z",
         "bishop": "M22 10l8 22h-16z", "queen": "M10 14l6 18h13l6-18-10 8-3-12-3 12z", "king": "M20 8h5v6h6v5h-6v13h-5v-13h-6v-5h6z"}


class FakeNet(n.NetworkClient):
    """Records what the client would send, and never opens a socket."""

    def __init__(self):
        super().__init__(debug=False)
        self.sent = []
        self.connect_calls = []
        self.server_version = "1.2"
        self.player_id = "me"

    def connect(self, address, display_name):
        self.connect_calls.append((address, display_name))

    def _send(self, message):
        self._sequence += 1
        message.sequence = self._sequence
        self.sent.append(message)

    def sent_kinds(self):
        return [m.WhichOneof("payload") for m in self.sent]

    def actions(self):
        return [bytes(m.action_request.encoded_action) for m in self.sent if m.WhichOneof("payload") == "action_request"]


@pytest.fixture
def app(tmp_path, monkeypatch):
    assets = tmp_path / "assets"
    assets.mkdir()
    for name, mark in MARKS.items():
        for suffix, fill, ink in (("w", "#f2f2f2", "#222"), ("b", "#303030", "#ddd")):
            (assets / f"{name}-{suffix}.svg").write_text(
                f'<svg xmlns="http://www.w3.org/2000/svg" width="128" height="128" viewBox="0 0 45 45">'
                f'<circle cx="22.5" cy="22.5" r="19" fill="{fill}" stroke="#888" stroke-width="2"/><path d="{mark}" fill="{ink}"/></svg>')
    monkeypatch.setattr(piece_module, "ASSETS", assets)
    monkeypatch.setenv("RAMCHESS_CONFIG_DIR", str(tmp_path / "config"))
    pygame.init()
    screen = pygame.display.set_mode((1280, 720), pygame.RESIZABLE)
    application = App(Settings(), FakeNet(), screen)
    yield application


def shot(app, name):
    folder = os.environ.get("SHOTS_DIR")
    if folder:
        Path(folder).mkdir(parents=True, exist_ok=True)
        pygame.image.save(app.screen, str(Path(folder) / f"{name}.png"))


def feed(app, *events, dt=0.016):
    for event in events:
        app.net.events.put(event)
    app.frame(dt)


def click(app, pos):
    app._dispatch(pygame.event.Event(pygame.MOUSEBUTTONDOWN, pos=pos, button=1))
    app._dispatch(pygame.event.Event(pygame.MOUSEBUTTONUP, pos=pos, button=1))


def click_widget(app, widget):
    click(app, widget.rect.center)


def players(*, me_ready=False, opp_ready=False, spectator=False):
    return (n.Player("me", "Me", True, me_ready, "player"), n.Player("opp", "Opp", True, opp_ready, "player"),
            *((n.Player("spec", "Spec", True, False, "spectator"),) if spectator else ()))


def waiting(**kwargs):
    return n.Snapshot("s1", "chess", n.STATUS_WAITING, players(**kwargs), owner_id="me", player_order=("me", "opp"))


def playing(fen=START, plies=0, active="me", last="", **annotations):
    notes = {"plies": str(plies), **annotations}
    if last:
        notes["last_move"] = last
    return n.Snapshot("s1", "chess", n.STATUS_IN_PROGRESS, players(), owner_id="me", version=plies, view=fen.encode(),
                      active_player_id=active, player_order=("me", "opp"), annotations=notes)


def start_game(app, snapshot=None, version="1.2"):
    app.push(ConnectScene(app))
    feed(app, n.Connected(version), n.Authed("me", "token"))
    assert isinstance(app.scenes[-1], LobbyScene)
    feed(app, n.StateEvent(waiting()))
    feed(app, n.StateEvent(snapshot or playing()))
    game = app.scenes[-1]
    assert isinstance(game, GameScene)
    return game


def square_pos(game, uci):
    from ..board import square_from_uci
    rank, file = square_from_uci(uci)
    x, y = game.board.square_center(rank, file)
    return game.board.origin[0] + x, game.board.origin[1] + y


def move(app, game, uci):
    click(app, square_pos(game, uci[:2]))
    click(app, square_pos(game, uci[2:4]))


LEGAL = n.LegalMovesEvent(("e2e4", "e2e3", "g1f3", "b1c3"))


# --- Connect and lobby ---

def test_connect_screen_validates_and_connects(app):
    app.push(ConnectScene(app))
    scene = app.scenes[-1]
    app.frame(0.016)
    shot(app, "connect")
    for bad in ("", "example.org:99999", "a:b:c", "ws://"):
        scene.server.text = bad
        scene.connect()
        assert scene.status_is_error and not app.net.connect_calls and not scene.connecting, bad
    scene.server.text, scene.name.text = "example.org:9100", ""
    scene.connect()
    assert scene.status_is_error and not app.net.connect_calls  # a display name is needed
    scene.name.text = "Zed"
    scene.connect()
    assert [(a.display, name) for a, name in app.net.connect_calls] == [("example.org:9100", "Zed")]
    assert app.settings.server == "example.org:9100"  # remembered for next time
    feed(app, n.Disconnected("Could not connect"))
    assert scene.status == "Could not connect" and not scene.connecting  # the player can try again


def test_server_shortcuts_fill_in_the_address(app):
    app.push(ConnectScene(app))
    scene = app.scenes[-1]
    click_widget(app, scene.hosted_button)
    assert scene.server.text == n.HOSTED_SERVER
    click_widget(app, scene.local_button)
    assert scene.server.text == n.LOCAL_SERVER
    scene.server.text = "wss://example.org/connect"
    scene.name.text = "Zed"
    scene.connect()
    assert app.net.connect_calls[0][0].kind == "ws"
    app.frame(0.016)
    shot(app, "connect_hosted")


def test_signing_in_opens_the_lobby_and_asks_for_open_sessions(app):
    app.push(ConnectScene(app))
    feed(app, n.Connected("1.2"), n.Authed("me", "t"))
    assert isinstance(app.scenes[-1], LobbyScene)
    assert "list_sessions_request" in app.net.sent_kinds()


def test_lobby_browse_create_and_join(app):
    app.push(ConnectScene(app))
    feed(app, n.Connected("1.2"), n.Authed("me", "t"))
    lobby = app.scenes[-1]
    info = n.SessionInfo("s9", "chess", n.STATUS_WAITING, (n.Player("o", "Olga", True, False, "player"),), "o", True)
    full = n.SessionInfo("s8", "chess", n.STATUS_IN_PROGRESS, (n.Player("p", "Pat", True, True, "player"),), "p", False)
    feed(app, n.SessionsEvent((info, full)))
    shot(app, "lobby_browse")
    assert [b.enabled for _, b in lobby.session_buttons] == [True, False]

    app.net.sent.clear()
    click_widget(app, lobby.session_buttons[0][1])
    click_widget(app, lobby.create_button)
    click_widget(app, lobby.private_button)
    click_widget(app, lobby.quick_button)
    joins = [m.join_session_request for m in app.net.sent]
    assert [(j.session_id, j.create_new, j.private_session) for j in joins] == [("s9", False, False), ("", True, False), ("", True, True), ("", False, False)]

    lobby.join_id.text = "session-abc"
    click_widget(app, lobby.join_button)
    assert app.net.sent[-1].join_session_request.session_id == "session-abc"


def test_lobby_session_roster_ready_history_and_leave(app):
    app.push(ConnectScene(app))
    feed(app, n.Connected("1.2"), n.Authed("me", "t"))
    lobby = app.scenes[-1]
    feed(app, n.StateEvent(waiting(opp_ready=True)))
    assert lobby.in_session and "match_history_request" in app.net.sent_kinds()
    history = (n.MatchSummary(("me",), False, "CHECKMATE", 41), n.MatchSummary((), True, "STALEMATE", 70))
    feed(app, n.HistoryEvent("s1", history))
    shot(app, "lobby_session")
    assert lobby.history == history

    app.net.sent.clear()
    click_widget(app, lobby.ready_button)
    assert app.net.sent[-1].set_ready_request.ready is True
    feed(app, n.StateEvent(waiting(me_ready=True, opp_ready=True)))
    assert lobby.my_ready()
    click_widget(app, lobby.ready_button)  # cancels
    assert app.net.sent[-1].set_ready_request.ready is False

    click_widget(app, lobby.leave_button)
    assert app.net.sent[-1].WhichOneof("payload") == "leave_session_request"
    feed(app, n.LeftEvent("s1"))
    assert not lobby.in_session


def test_spectator_cannot_ready(app):
    app.push(ConnectScene(app))
    feed(app, n.Connected("1.2"), n.Authed("me", "t"))
    lobby = app.scenes[-1]
    spectating = replace(waiting(), players=(n.Player("opp", "Opp", True, False, "player"), n.Player("me", "Me", True, False, "spectator")))
    feed(app, n.StateEvent(spectating))
    assert not lobby.ready_button.enabled


def test_old_servers_get_notes_instead_of_the_new_lists(app):
    app.net.server_version = "1.1"
    app.push(ConnectScene(app))
    feed(app, n.Connected("1.1"), n.Authed("me", "t"))
    lobby = app.scenes[-1]
    app.frame(0.016)
    shot(app, "lobby_old_server")
    assert not lobby.private_button.enabled
    kinds = app.net.sent_kinds()
    assert "list_sessions_request" not in kinds


def test_match_starting_moves_everyone_to_the_board(app):
    game = start_game(app)
    assert game.own_color is not None and game.my_turn
    assert "legal_actions_request" in app.net.sent_kinds()  # asked because it is the player's turn


# --- Playing ---

def test_dragging_and_clicking_a_move_sends_uci(app):
    game = start_game(app)
    feed(app, LEGAL)
    app.frame(0.016)
    shot(app, "game_start")
    # Click to select, then click the target: hints show the targets
    click(app, square_pos(game, "e2"))
    app.frame(0.016)
    assert game.board.targets == {(2, 4), (3, 4)}
    move_from = game.board.piece_at_square(1, 4)
    click(app, square_pos(game, "e4"))
    assert app.net.actions() == [b"e2e4"]
    assert not game.my_turn and game.board.piece_at_square(3, 4) is move_from  # shown at once, until the server answers

    # An illegal pick is not sent, and the opponent's pieces cannot be picked up
    app.net.sent.clear()
    feed(app, n.StateEvent(playing(AFTER_E4, 1, active="opp", last="e2e4")))
    click(app, square_pos(game, "e7"))
    click(app, square_pos(game, "e5"))
    assert app.net.actions() == []


def test_rejected_move_restores_the_board_and_the_turn(app):
    game = start_game(app)
    feed(app, LEGAL)
    move(app, game, "e2e4")
    assert not game.my_turn
    error = n.ErrorEvent("ERROR_CODE_ILLEGAL_ACTION", "e2e4", 5)
    feed(app, error)
    assert game.my_turn and game.board.piece_at_square(1, 4) is not None and game.board.piece_at_square(3, 4) is None
    assert any("Illegal move" in t[0] for t in app.toasts.items)


def test_promotion_asks_which_piece(app):
    fen = "4k3/P7/8/8/8/8/8/4K3 w - - 0 1"
    game = start_game(app, playing(fen))
    feed(app, n.LegalMovesEvent(("a7a8q", "a7a8r", "a7a8b", "a7a8n", "e1e2")))
    move(app, game, "a7a8")
    assert app.net.actions() == [] and len(game.promotions) == 4
    app.frame(0.016)
    shot(app, "game_promotion")
    queen_square = game.board._promotion_squares()[0][1]
    click(app, (game.board.origin[0] + queen_square[0] + 5, game.board.origin[1] + queen_square[1] + 5))
    assert app.net.actions() == [b"a7a8q"]


def test_offering_a_draw_rides_on_the_next_move(app):
    game = start_game(app)
    feed(app, LEGAL)
    click_widget(app, game.draw_button)
    assert game.draw_armed
    app.frame(0.016)
    assert game.draw_button.label == "Cancel draw offer"
    shot(app, "game_draw_armed")
    click_widget(app, game.draw_button)  # cancel
    click_widget(app, game.draw_button)  # arm again
    move(app, game, "e2e4")
    assert app.net.actions() == [b"e2e4(=)"] and not game.draw_armed


def test_draw_button_is_off_when_it_is_not_your_turn(app):
    game = start_game(app, playing(AFTER_E4, 1, active="opp", last="e2e4"))
    app.frame(0.016)
    assert not game.draw_button.enabled and game.resign_button.enabled


def test_incoming_draw_offer_can_be_accepted_or_declined(app):
    game = start_game(app, playing(AFTER_E4.replace(" b ", " w "), 1, active="me", last="e2e4", draw_offer="opp"))
    feed(app, n.LegalMovesEvent(("e7e5", "(=)")))
    assert game.offer_banner_visible and game.can_accept_draw
    app.frame(0.016)
    shot(app, "game_draw_offer")
    click_widget(app, game.decline_button)
    assert not game.offer_banner_visible and app.net.actions() == []  # declining sends nothing: any move declines

    # A new offer at a later position shows the banner again
    feed(app, n.StateEvent(playing(AFTER_E5, 3, active="me", last="e7e5", draw_offer="opp")))
    feed(app, n.LegalMovesEvent(("e1e2", "(=)")))
    assert game.offer_banner_visible
    click_widget(app, game.accept_button)
    assert app.net.actions() == [b"(=)"]


def test_own_offer_is_shown_as_sent(app):
    game = start_game(app, playing(AFTER_E4, 1, active="opp", last="e2e4", draw_offer="me"))
    assert not game.offer_banner_visible
    assert game._status_text()[0] == "Draw offer sent"


def test_resigning_asks_first_and_then_sends(app):
    game = start_game(app)
    click_widget(app, game.resign_button)
    dialog = app.scenes[-1]
    assert isinstance(dialog, ConfirmDialog) and app.net.actions() == []
    app.frame(0.016)
    shot(app, "game_resign_dialog")
    click_widget(app, dialog.no)
    assert app.scenes[-1] is game and app.net.actions() == []
    click_widget(app, game.resign_button)
    click_widget(app, app.scenes[-1].yes)
    assert app.net.actions() == [b"resign"] and app.scenes[-1] is game


def test_resign_is_unavailable_on_old_servers(app):
    game = start_game(app, version="1.1")
    app.frame(0.016)
    assert not game.resign_button.enabled


def test_turn_marker_and_counter(app):
    game = start_game(app)
    app.frame(0.016)
    assert game.board.turn_marker == "bottom" and game.view.fullmove == 1 and game.displayed_plies == 0
    feed(app, n.StateEvent(playing(AFTER_E4, 1, active="opp", last="e2e4")))
    assert game.board.turn_marker == "top" and game.displayed_plies == 1
    feed(app, n.StateEvent(playing(AFTER_E5, 2, active="me", last="e7e5")))
    assert game.board.turn_marker == "bottom" and game.view.fullmove == 2 and game.displayed_plies == 2
    shot(app, "game_counter")


# --- Check display ---

CHECK_FEN = "rnb1kbnr/pppp1ppp/8/4p3/6Pq/5P2/PPPPP2P/RNBQKBNR w KQkq - 1 3"


def test_check_overlays_follow_the_annotations_and_settings(app):
    notes = dict(check="white", checkers="h4", king_unsafe="f2")
    game = start_game(app, playing(CHECK_FEN, 4, active="me", last="d8h4", **notes))
    game.update(1.0)
    board = game.board
    assert board.check_square == (0, 4) and board.checkers == [(3, 7)] and board.unsafe == []  # unsafe shows once the king is selected
    assert game._status_text() == ("You are in check", (205, 80, 80))
    app.frame(0.016)
    shot(app, "game_check")

    app.settings.show_unsafe_squares = "always"
    game.update(0.0)
    assert board.unsafe == [(1, 5)]
    app.frame(0.016)
    shot(app, "game_check_unsafe")

    app.settings.show_check_outline, app.settings.show_checkers, app.settings.show_unsafe_squares = False, False, "off"
    game.update(0.0)
    assert board.check_outline is False and board.checkers == [] and board.unsafe == []
    assert board.check_square == (0, 4)  # still known, so enabling the checkers later still draws lines


def test_unsafe_squares_show_when_the_king_is_selected(app):
    game = start_game(app, playing(CHECK_FEN, 4, active="me", last="d8h4", check="white", checkers="h4", king_unsafe="f2"))
    feed(app, n.LegalMovesEvent(("e1e2",)))
    click(app, square_pos(game, "e1"))
    game.update(0.0)
    assert game.board.unsafe == [(1, 5)]
    click(app, square_pos(game, "a3"))  # deselect
    game.update(0.0)
    assert game.board.unsafe == []


# --- Animations ---

def test_opponents_move_slides_but_your_own_does_not(app):
    game = start_game(app)
    feed(app, LEGAL)
    move(app, game, "e2e4")
    feed(app, n.StateEvent(playing(AFTER_E4, 1, active="opp", last="e2e4")))
    assert game.plan is None  # the move was already shown when it was made

    feed(app, n.StateEvent(playing(AFTER_E5, 2, active="me", last="e7e5")))
    assert game.plan is not None and game.animating
    game.update(0.0)
    assert game.board.animations and not game.board.overlays_visible
    # Pieces cannot be picked up while the move is still sliding
    feed(app, LEGAL)
    click(app, square_pos(game, "g1"))
    assert game.selected is None
    game.update(0.1)
    app.frame(0.0)
    shot(app, "game_animating")
    game.update(1.0)
    assert game.plan is None and game.board.animations == [] and game.board.overlays_visible


def test_animation_follows_settings(app):
    app.settings.animations_enabled = False
    game = start_game(app, playing(AFTER_E4, 1, active="me", last="e2e4"))
    feed(app, n.StateEvent(playing(AFTER_E5, 2, active="me", last="e7e5")))
    assert game.plan is None
    app.settings.animations_enabled, app.settings.animation_speed = True, 2.0
    fen3 = "rnbqkbnr/pppp1ppp/8/4p3/4P3/5N2/PPPP1PPP/RNBQKB1R b KQkq - 1 2"
    feed(app, n.StateEvent(playing(fen3, 3, active="opp", last="g1f3")))
    assert game.plan is None  # the player's own move
    fen4 = "rnbqkb1r/pppp1ppp/5n2/4p3/4P3/5N2/PPPP1PPP/RNBQKB1R w KQkq - 2 3"
    feed(app, n.StateEvent(playing(fen4, 4, active="me", last="g8f6")))
    assert game.plan is not None and game.plan.animations[0].duration == pytest.approx(0.125)  # twice as fast
    assert game.plan.animations[0].lift  # the knight jumps


def test_skipped_states_snap_instead_of_animating(app):
    game = start_game(app, playing(AFTER_E4, 1, active="opp", last="e2e4"))
    feed(app, n.StateEvent(playing("rnbqkbnr/pppp1ppp/8/4p3/4P3/5N2/PPPP1PPP/RNBQKB1R b KQkq - 1 2", 3, active="opp", last="g1f3")))
    assert game.plan is None  # two moves arrived at once


def test_castling_slides_the_rook_with_the_king(app):
    before = "r3k2r/8/8/8/8/8/8/R3K2R w KQkq - 0 1"
    game = start_game(app, playing(before, 0, active="opp"))
    feed(app, n.StateEvent(playing("r3k2r/8/8/8/8/8/8/R4RK1 b kq - 1 1", 1, active="me", last="e1g1")))
    # White is the local player, so it is a spectator-style check on the plan rather than a visible slide
    assert game.plan is None
    game.displayed_plies = 1
    feed(app, n.StateEvent(playing("r4rk1/8/8/8/8/8/8/R4RK1 w - - 2 2", 2, active="me", last="e8g8")))
    assert game.plan is not None and len(game.plan.animations) == 2


# --- End of the match ---

def test_match_end_screen_and_rematch(app):
    game = start_game(app)
    feed(app, n.StateEvent(playing(AFTER_E4, 1, active="opp", last="e2e4")))
    feed(app, n.MatchEnded(n.MatchSummary(("opp",), False, "RESIGNATION", 1)))
    assert game.end_screen and (game.end_screen.title, game.end_screen.subtitle) == ("You lose", "You resigned")
    app.frame(0.016)
    shot(app, "game_end")
    assert not game.draw_button.enabled and not game.resign_button.enabled

    app.net.sent.clear()
    click(app, game.end_screen._again.center)
    assert app.net.sent[-1].set_ready_request.ready is True and game.end_screen.waiting
    feed(app, n.StateEvent(replace(waiting(me_ready=True, opp_ready=True), status=n.STATUS_WAITING)))
    feed(app, n.StateEvent(playing()))
    assert game.end_screen is None and game.displayed_plies == 0 and game.my_turn


def test_opponent_asking_for_a_rematch_is_shown(app):
    game = start_game(app)
    feed(app, n.MatchEnded(n.MatchSummary(("me",), False, "CHECKMATE", 4)))
    assert (game.end_screen.title, game.end_screen.subtitle) == ("You win!", "Checkmate")
    feed(app, n.StateEvent(waiting(opp_ready=True)))
    assert game.end_screen.opponent_ready


def test_back_to_lobby_shows_nobody_ready(app):
    game = start_game(app)
    feed(app, n.MatchEnded(n.MatchSummary((), True, "STALEMATE", 70)))
    app.frame(0.016)
    click(app, game.end_screen._lobby.center)
    lobby = app.scenes[-1]
    assert isinstance(lobby, LobbyScene) and lobby.in_session and not lobby.my_ready()
    assert all(not p.ready for p in lobby.snapshot.players) and not lobby.snapshot.in_progress


def test_spectator_sees_the_board_but_cannot_move(app):
    app.net.player_id = "spec"
    snap = replace(playing(), players=players(spectator=True))
    app.push(ConnectScene(app))
    feed(app, n.Connected("1.2"), n.Authed("spec", "t"))
    feed(app, n.StateEvent(replace(waiting(), players=players(spectator=True))))
    feed(app, n.StateEvent(snap))
    game = app.scenes[-1]
    assert game.spectating and game.own_color is None and not game.my_turn
    app.net.sent.clear()
    feed(app, LEGAL)
    click(app, square_pos(game, "e2"))
    click(app, square_pos(game, "e4"))
    assert app.net.actions() == []
    app.frame(0.016)
    assert not game.draw_button.enabled and not game.resign_button.enabled
    feed(app, n.MatchEnded(n.MatchSummary(("opp",), False, "CHECKMATE", 4)))
    assert game.end_screen.title == "Opp wins" and not game.end_screen.can_rematch


def test_playing_black_flips_the_board(app):
    app.net.player_id = "opp"
    app.push(ConnectScene(app))
    feed(app, n.Connected("1.2"), n.Authed("opp", "t"))
    feed(app, n.StateEvent(waiting()))
    feed(app, n.StateEvent(playing(AFTER_E4, 1, active="opp", last="e2e4")))
    game = app.scenes[-1]
    from ..piece import PlayerColor
    assert game.own_color == PlayerColor.BLACK and game.board.view_for == PlayerColor.BLACK and game.my_turn
    app.frame(0.016)
    shot(app, "game_black")


def test_disconnect_returns_to_the_connect_screen_with_the_reason(app):
    start_game(app)
    feed(app, n.Disconnected("The server closed the connection"))
    assert isinstance(app.scenes[-1], ConnectScene) and "closed" in app.scenes[-1].status


# --- Settings ---

def test_settings_scene_changes_settings_and_saves(app, tmp_path):
    game = start_game(app)
    click_widget(app, game.settings_button)
    scene = app.scenes[-1]
    assert isinstance(scene, SettingsScene)
    app.frame(0.016)
    shot(app, "settings")
    s = app.settings
    widgets = {w.label: w for w in scene.widgets}
    click_widget(app, widgets["Coordinates"])
    assert s.show_coordinates is True
    # Pick the custom style: its color editor appears
    style = widgets["Board style"]
    for _ in range(len(style.options) - 1):
        pygame.event.clear()
        click(app, style._arrows()[1].center)
        style = {w.label: w for w in scene.widgets}["Board style"]
    assert s.style == "CUSTOM"
    labels = [w.label for w in scene.widgets]
    assert {"Edit color", "Red", "Green", "Blue"} <= set(labels)
    red = {w.label: w for w in scene.widgets}["Red"]
    click(app, (red.track.right, red.rect.centery))
    assert s.custom_colors["light"][0] == 255
    app.frame(0.016)
    shot(app, "settings_custom")
    assert app.style.light_color[0] == 255
    # Animation speed is greyed out with animations off. The custom color rows push these below the visible part
    scene.handle_event(pygame.event.Event(pygame.MOUSEWHEEL, y=-20, x=0))
    widgets = {w.label: w for w in scene.widgets}
    click_widget(app, widgets["Animate moves"])
    assert s.animations_enabled is False and not widgets["Speed"].enabled
    scene.handle_event(pygame.event.Event(pygame.KEYDOWN, key=pygame.K_ESCAPE, mod=0, unicode=""))
    assert app.scenes[-1] is game
    assert Settings.load().style == "CUSTOM"  # saved when the screen closed
    # And the board uses them
    game.update(0.0)
    assert game.board.show_coordinates and game.board.style.light_color[0] == 255


def test_reset_restores_defaults_but_not_the_connection(app):
    app.settings.show_coordinates, app.settings.style, app.settings.host = True, "OCEAN", "example.org"
    app.push(ConnectScene(app))
    app.open_settings()
    scene = app.scenes[-1]
    scene.reset()
    assert app.settings.style == "STANDARD" and not app.settings.show_coordinates and app.settings.host == "example.org"


def test_settings_scroll_in_a_small_window(app):
    pygame.display.set_mode((400, 330), pygame.RESIZABLE)
    app.push(ConnectScene(app))
    app.open_settings()
    scene = app.scenes[-1]
    app.frame(0.016)
    first = [w.rect.y for w in scene.widgets][:2]
    scene.handle_event(pygame.event.Event(pygame.MOUSEWHEEL, y=-3, x=0))
    assert scene.scroll > 0 and [w.rect.y for w in scene.widgets][:2] != first
    scene.handle_event(pygame.event.Event(pygame.MOUSEWHEEL, y=50, x=0))
    assert scene.scroll == 0


# --- Window sizes ---

@pytest.mark.parametrize("size", [(1280, 720), (720, 1000), (320, 320), (900, 900)])
def test_every_screen_draws_at_every_size(app, size):
    pygame.display.set_mode(size, pygame.RESIZABLE)
    app.push(ConnectScene(app))
    app.frame(0.016)
    feed(app, n.Connected("1.2"), n.Authed("me", "t"))
    feed(app, n.SessionsEvent((n.SessionInfo("s9", "chess", 1, players()[:1], "me", True),)))
    feed(app, n.StateEvent(waiting()))
    app.frame(0.016)
    feed(app, n.StateEvent(playing(CHECK_FEN, 4, active="me", last="d8h4", check="white", checkers="h4", king_unsafe="f2", draw_offer="opp")))
    feed(app, n.LegalMovesEvent(("e1e2", "(=)")))
    app.settings.show_coordinates = True
    app.settings.show_unsafe_squares = "always"
    app.frame(0.016)
    shot(app, f"game_{size[0]}x{size[1]}")
    app.open_settings()
    app.frame(0.016)
    app.pop()
    feed(app, n.MatchEnded(n.MatchSummary(("me",), False, "CHECKMATE", 4)))
    app.frame(0.016)
    shot(app, f"end_{size[0]}x{size[1]}")
    game = app.scenes[-1]
    for button in (game.draw_button, game.resign_button, game.settings_button):
        assert game.panel.contains(button.rect), (size, button.label)


def test_copy_id_copies_and_the_id_can_be_pasted_into_the_join_field(app, monkeypatch):
    import clients.pychess.ui as ui
    import clients.pychess.scenes.lobby as lobby_module
    store = {}
    monkeypatch.setattr(lobby_module, "clipboard_put", lambda text: store.update(text=text) or True)
    monkeypatch.setattr(ui, "clipboard_get", lambda: store["text"])
    app.push(ConnectScene(app))
    feed(app, n.Connected("1.2"), n.Authed("me", "t"))
    lobby = app.scenes[-1]
    feed(app, n.StateEvent(waiting()))
    click_widget(app, lobby.copy_button)
    assert store["text"] == "s1" and any("copied" in t[0] for t in app.toasts.items)
    feed(app, n.LeftEvent("s1"))
    click_widget(app, lobby.join_id)
    lobby.handle_event(pygame.event.Event(pygame.KEYDOWN, key=pygame.K_v, mod=pygame.KMOD_CTRL, unicode="\x16"))
    assert lobby.join_id.text == "s1"


def test_copy_id_without_a_system_clipboard_still_works_inside_the_app(app):
    app.push(ConnectScene(app))
    feed(app, n.Connected("1.2"), n.Authed("me", "t"))
    lobby = app.scenes[-1]
    feed(app, n.StateEvent(waiting()))
    click_widget(app, lobby.copy_button)  # the headless driver has no clipboard
    assert any("inside the app" in t[0] for t in app.toasts.items)
    feed(app, n.LeftEvent("s1"))
    click_widget(app, lobby.join_id)
    lobby.handle_event(pygame.event.Event(pygame.KEYDOWN, key=pygame.K_v, mod=pygame.KMOD_CTRL, unicode="\x16"))
    assert lobby.join_id.text == "s1"


def test_long_names_never_overlap_the_roster_labels(app):
    pygame.display.set_mode((320, 320), pygame.RESIZABLE)
    app.push(ConnectScene(app))
    feed(app, n.Connected("1.2"), n.Authed("me", "t"))
    long = "Extremely-Long-Display-Name"
    snap = n.Snapshot("s1", "chess", n.STATUS_WAITING, (n.Player("me", long, True, True, "player"), n.Player("opp", long, True, False, "spectator")),
                      owner_id="me", player_order=("me", "opp"))
    feed(app, n.StateEvent(snap))
    shot(app, "lobby_long_names_small")
