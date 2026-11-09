import pygame
import pytest

import clients.pychess.ui as ui
from clients.pychess.board import Board
from clients.pychess.end_screen import EndScreen
from clients.pychess.piece import Piece, PieceType, PlayerColor
from clients.pychess.ui import TextInput, draw_text


@pytest.fixture(autouse=True)
def pygame_ready():
    pygame.init()
    pygame.display.set_mode((400, 300))
    yield


def key(k, mods=0, char=""):
    return pygame.event.Event(pygame.KEYDOWN, key=k, mod=mods, unicode=char)


def type_text(field, text):
    for character in text:
        field.handle_event(key(ord(character), 0, character))


def field(text="", **kwargs):
    f = TextInput("Server", text, **kwargs)
    f.rect = pygame.Rect(10, 10, 400, 40)
    f.focused = True
    return f


def click(f, x, y=30, button=1):
    return f.handle_event(pygame.event.Event(pygame.MOUSEBUTTONDOWN, pos=(x, y), button=button))


@pytest.fixture
def clipboard(monkeypatch):
    store = {"text": ""}
    monkeypatch.setattr(ui, "clipboard_put", lambda text: store.update(text=text) or True)
    monkeypatch.setattr(ui, "clipboard_get", lambda: store["text"])
    return store


class TestEditing:
    def test_typing_and_caret_movement(self):
        f = field()
        type_text(f, "abcd")
        f.handle_event(key(pygame.K_LEFT))
        f.handle_event(key(pygame.K_LEFT))
        type_text(f, "X")
        assert f.text == "abXcd" and f.cursor == 3
        f.handle_event(key(pygame.K_HOME))
        type_text(f, "<")
        f.handle_event(key(pygame.K_END))
        type_text(f, ">")
        assert f.text == "<abXcd>"

    def test_backspace_and_delete(self):
        f = field("abcd")
        f.handle_event(key(pygame.K_BACKSPACE))
        f.handle_event(key(pygame.K_HOME))
        f.handle_event(key(pygame.K_DELETE))
        assert f.text == "bc"
        f.handle_event(key(pygame.K_HOME))
        f.handle_event(key(pygame.K_BACKSPACE))  # nothing before the caret
        assert f.text == "bc"

    def test_limits(self):
        f = field("ab", max_len=3)
        type_text(f, "cdef")
        assert f.text == "abc"
        digits = field(digits_only=True)
        type_text(digits, "9a8-7")
        assert digits.text == "987"

    def test_words(self):
        f = field("one two three")
        f.handle_event(key(pygame.K_BACKSPACE, pygame.KMOD_CTRL))
        assert f.text == "one two "
        f.handle_event(key(pygame.K_LEFT, pygame.KMOD_CTRL))
        assert f.cursor == 4
        f.handle_event(key(pygame.K_DELETE, pygame.KMOD_CTRL))
        assert f.text == "one  "  # the word goes, the spaces around it stay

    def test_setting_text_puts_the_caret_at_the_end(self):
        f = field("abc")
        f.handle_event(key(pygame.K_HOME))
        f.text = "wss://example.org/connect"
        assert f.cursor == f.anchor == len(f.text)

    def test_enter_submits_and_unrelated_keys_are_not_used(self):
        submitted = []
        f = field("x", on_submit=lambda: submitted.append(1))
        assert f.handle_event(key(pygame.K_RETURN, 0, "\r"))
        assert submitted == [1]
        assert not f.handle_event(key(pygame.K_TAB, 0, "\t"))
        assert not f.handle_event(key(pygame.K_ESCAPE, 0, "\x1b"))
        f.focused = False
        assert not f.handle_event(key(pygame.K_a, 0, "a")) and f.text == "x"


class TestSelection:
    def test_shift_arrows_select_and_typing_replaces(self):
        f = field("hello world")
        f.handle_event(key(pygame.K_LEFT, pygame.KMOD_SHIFT))
        f.handle_event(key(pygame.K_LEFT, pygame.KMOD_SHIFT))
        assert f.selected_text() == "ld"
        type_text(f, "LD!")
        assert f.text == "hello worLD!"

    def test_select_all_then_delete(self):
        f = field("hello")
        f.handle_event(key(pygame.K_a, pygame.KMOD_CTRL))
        assert f.selected_text() == "hello"
        f.handle_event(key(pygame.K_BACKSPACE))
        assert f.text == "" and f.cursor == 0

    def test_arrow_leaves_a_selection_at_its_edge(self):
        f = field("hello")
        f.handle_event(key(pygame.K_HOME))
        f.handle_event(key(pygame.K_RIGHT, pygame.KMOD_SHIFT))
        f.handle_event(key(pygame.K_RIGHT, pygame.KMOD_SHIFT))
        f.handle_event(key(pygame.K_LEFT))
        assert f.selection == (0, 0)
        f.handle_event(key(pygame.K_RIGHT, pygame.KMOD_SHIFT))
        f.handle_event(key(pygame.K_RIGHT))
        assert f.cursor == 1 and f.anchor == 1

    def test_copy_cut_and_paste(self, clipboard):
        f = field("hello world")
        f.handle_event(key(pygame.K_HOME))
        for _ in range(5):
            f.handle_event(key(pygame.K_RIGHT, pygame.KMOD_SHIFT))
        f.handle_event(key(pygame.K_c, pygame.KMOD_CTRL))
        assert clipboard["text"] == "hello" and f.text == "hello world"
        f.handle_event(key(pygame.K_x, pygame.KMOD_CTRL))
        assert f.text == " world" and clipboard["text"] == "hello"
        f.handle_event(key(pygame.K_END))
        f.handle_event(key(pygame.K_v, pygame.KMOD_CTRL))
        assert f.text == " worldhello"

    def test_paste_keeps_only_what_the_field_accepts(self, clipboard):
        clipboard["text"] = "9a1\n2"
        f = field(digits_only=True, max_len=3)
        f.handle_event(key(pygame.K_v, pygame.KMOD_CTRL))
        assert f.text == "912"

    def test_copy_with_nothing_selected_leaves_the_clipboard_alone(self, clipboard):
        clipboard["text"] = "kept"
        field("abc").handle_event(key(pygame.K_c, pygame.KMOD_CTRL))
        assert clipboard["text"] == "kept"

    def test_clipboard_works_inside_the_app_when_the_system_has_none(self):
        # The headless video driver has no system clipboard
        assert ui.clipboard_put("lobby-123") is False
        assert ui.clipboard_get() == "lobby-123"


class TestMouse:
    def test_click_places_the_caret_nearest_the_click(self):
        f = field("abcdef")
        area = f._text_area()
        click(f, area.x + f._x_of(3) + 1)
        assert f.cursor == f.anchor == 3
        click(f, area.x + 120)  # past the end of the text, but inside the field and clear of the clear button
        assert f.cursor == 6
        f.handle_event(pygame.event.Event(pygame.MOUSEBUTTONUP, pos=(0, 0), button=1))

    def test_dragging_selects(self):
        f = field("abcdef")
        area = f._text_area()
        click(f, area.x + f._x_of(1) + 1)
        f.handle_event(pygame.event.Event(pygame.MOUSEMOTION, pos=(area.x + f._x_of(4) + 1, 30), rel=(0, 0), buttons=(1, 0, 0)))
        f.handle_event(pygame.event.Event(pygame.MOUSEBUTTONUP, pos=(0, 0), button=1))
        assert f.selected_text() == "bcd"

    def test_double_click_selects_everything(self):
        f = field("abc def")
        area = f._text_area()
        click(f, area.x + 5)
        click(f, area.x + 5)
        assert f.selected_text() == "abc def"

    def test_clicking_elsewhere_loses_focus(self):
        f = field("abc")
        assert not click(f, 900, 30)
        assert not f.focused

    def test_the_clear_button_empties_the_field_and_keeps_focus(self):
        f = field("something long")
        clear = f._clear_rect()
        assert click(f, *clear.center)
        assert f.text == "" and f.focused
        assert not f._clear_visible()  # nothing to clear

    def test_clear_button_can_be_turned_off(self):
        f = field("abc", clearable=False)
        assert not f._clear_visible()


class TestHeldKeys:
    @pytest.fixture
    def clock(self, monkeypatch):
        now = {"t": 100.0}
        monkeypatch.setattr(ui.time, "monotonic", lambda: now["t"])
        return now

    def held(self, monkeypatch, *keys):
        pressed = {k: True for k in keys}

        class Keys:
            def __getitem__(self, k):
                return pressed.get(k, False)

        monkeypatch.setattr(pygame.key, "get_pressed", lambda: Keys())
        return pressed

    def test_backspace_repeats_after_a_delay_while_held(self, monkeypatch, clock):
        f = field("x" * 30)
        pressed = self.held(monkeypatch, pygame.K_BACKSPACE)
        surface = pygame.Surface((420, 60))
        f.handle_event(key(pygame.K_BACKSPACE))
        assert len(f.text) == 29
        clock["t"] += 0.2
        f.draw(surface)
        assert len(f.text) == 29  # still inside the initial delay
        clock["t"] += 0.3
        f.draw(surface)
        after_delay = len(f.text)
        assert after_delay < 29
        clock["t"] += 0.2
        f.draw(surface)
        assert len(f.text) < after_delay  # and it keeps going
        pressed[pygame.K_BACKSPACE] = False  # released without a KEYUP reaching the field
        before = len(f.text)
        clock["t"] += 1
        f.draw(surface)
        assert len(f.text) == before

    def test_keyup_stops_the_repeat(self, monkeypatch, clock):
        f = field("x" * 30)
        self.held(monkeypatch, pygame.K_BACKSPACE)
        f.handle_event(key(pygame.K_BACKSPACE))
        f.handle_event(pygame.event.Event(pygame.KEYUP, key=pygame.K_BACKSPACE, mod=0))
        clock["t"] += 2
        f.draw(pygame.Surface((420, 60)))
        assert len(f.text) == 29

    def test_held_character_repeats(self, monkeypatch, clock):
        f = field()
        self.held(monkeypatch, ord("a"))
        f.handle_event(key(ord("a"), 0, "a"))
        clock["t"] += 0.4 + 0.035 * 3
        f.draw(pygame.Surface((420, 60)))
        assert f.text.startswith("aaaa") and set(f.text) == {"a"}

    def test_a_long_pause_does_not_run_away(self, monkeypatch, clock):
        f = field("x" * 100, max_len=200)
        self.held(monkeypatch, pygame.K_BACKSPACE)
        f.handle_event(key(pygame.K_BACKSPACE))
        clock["t"] += 60
        f.draw(pygame.Surface((420, 60)))
        assert len(f.text) >= 90  # a few repeats, not a hundred

    def test_losing_focus_stops_the_repeat(self, monkeypatch, clock):
        f = field("x" * 30)
        self.held(monkeypatch, pygame.K_BACKSPACE)
        f.handle_event(key(pygame.K_BACKSPACE))
        f.focused = False
        clock["t"] += 2
        f.draw(pygame.Surface((420, 60)))
        assert len(f.text) == 29

    def test_enter_does_not_repeat(self, monkeypatch, clock):
        submitted = []
        f = field("x", on_submit=lambda: submitted.append(1))
        self.held(monkeypatch, pygame.K_RETURN)
        f.handle_event(key(pygame.K_RETURN, 0, "\r"))
        clock["t"] += 2
        f.draw(pygame.Surface((420, 60)))
        assert submitted == [1]


class TestDrawing:
    def render(self, f, blink_on, monkeypatch):
        monkeypatch.setattr(ui.time, "time", lambda: 0.0 if blink_on else 0.5)
        surface = pygame.Surface((420, 60))
        surface.fill((0, 0, 0))
        f.draw(surface)
        return surface

    @pytest.mark.parametrize("text", ["", "short", "wss://ramserver-core-80752473398.us-east5.run.app/connect"])
    def test_the_blinking_caret_changes_only_the_caret(self, text, monkeypatch):
        f = field(text)
        on, off = self.render(f, True, monkeypatch), self.render(f, False, monkeypatch)
        columns = {x for x in range(on.get_width()) for y in range(on.get_height()) if on.get_at((x, y)) != off.get_at((x, y))}
        assert columns and max(columns) - min(columns) <= 2  # the caret is 2 pixels wide, and nothing else moves

    def test_text_size_does_not_depend_on_its_length(self):
        f = field("a")
        short = f._font()
        area = f._text_area()
        f.text = "a" * 60
        assert f._font() is short and f._text_area() == area

    def test_the_caret_stays_in_view_in_long_text(self):
        f = field("0123456789" * 15)
        area = f._text_area()
        f.draw(pygame.Surface((420, 60)))
        assert 0 <= f._x_of(f.cursor) - f._scroll <= area.width
        f.handle_event(key(pygame.K_HOME))
        f.draw(pygame.Surface((420, 60)))
        assert f._scroll == 0

    def test_the_text_area_does_not_move_with_the_mouse(self, monkeypatch):
        f = field("abc")
        before = f._text_area()
        monkeypatch.setattr(pygame.mouse, "get_pos", lambda: f._clear_rect().center)
        f.draw(pygame.Surface((420, 60)))
        assert f._text_area() == before


class TestTruncation:
    def test_text_which_cannot_fit_is_cut_not_overlapped(self):
        surface = pygame.Surface((300, 40))
        long = "Extremely-Long-Display-Name-That-Cannot-Fit"
        rect = draw_text(surface, long, 24, max_width=80, topleft=(0, 0))
        assert rect.width <= 80
        full = draw_text(surface, long, 24, topleft=(0, 0))
        assert full.width > 80

    def test_text_which_fits_is_unchanged(self):
        surface = pygame.Surface((300, 40))
        assert draw_text(surface, "Host", 24, max_width=200, topleft=(0, 0)).width == draw_text(surface, "Host", 24, topleft=(0, 0)).width

    def test_helper_edge_cases(self):
        font = ui.get_font(20)
        assert ui._truncate(font, "anything", 0) == ""
        assert ui._truncate(font, "anything", 2) == ""
        assert ui._truncate(font, "short", 500) .endswith("...")  # callers only truncate text which does not fit
        assert font.size(ui._truncate(font, "x" * 200, 120))[0] <= 120


class TestButtons:
    def test_neutral_buttons_do_not_turn_green_on_hover(self):
        assert ui.NEUTRAL_HOVER_COLOR != ui.ACCENT_COLOR

    def test_clicks_are_logged_in_debug_mode(self, monkeypatch, capsys):
        clicked = []
        button = ui.Button("Ready", lambda: clicked.append(1))
        button.rect = pygame.Rect(0, 0, 50, 20)
        event = pygame.event.Event(pygame.MOUSEBUTTONDOWN, pos=(5, 5), button=1)
        monkeypatch.delenv("RAMCHESS_DEBUG", raising=False)
        button.handle_event(event)
        assert capsys.readouterr().out == "" and clicked == [1]
        monkeypatch.setenv("RAMCHESS_DEBUG", "1")
        button.handle_event(event)
        assert "clicked 'Ready' at (5, 5)" in capsys.readouterr().out


class TestEndScreenLayout:
    @pytest.mark.parametrize("size", [(1280, 720), (720, 1000), (320, 320), (200, 200)])
    def test_the_buttons_clear_the_text_and_stay_in_the_panel(self, size):
        screen = pygame.Surface(size)
        end = EndScreen("You win!", "Checkmate")
        end.opponent_ready = True
        end.draw(screen)
        unit = max(10, min(size) // 20)
        panel = pygame.Rect(0, 0, unit * 11, unit * 9)
        panel.center = (size[0] // 2, size[1] // 2)
        note_bottom = panel.top + unit * 4 + unit // 2  # the last line of text
        assert end._again.top > note_bottom
        assert end._again.bottom < end._lobby.top
        assert panel.contains(end._again) and panel.contains(end._lobby)

    def test_buttons_work(self):
        end = EndScreen("Draw", "Stalemate")
        end.draw(pygame.Surface((800, 600)))
        assert end.button_at(end._again.center) == EndScreen.AGAIN
        assert end.button_at(end._lobby.center) == EndScreen.LOBBY
        end.waiting = True
        assert end.button_at(end._again.center) is None
        assert EndScreen("x", "y", can_rematch=False).button_at((0, 0)) is None


class TestBoardSprites:
    def test_a_piece_in_motion_can_be_drawn_as_another_type(self, monkeypatch):
        board = Board(160, PlayerColor.WHITE)
        asked = []
        monkeypatch.setattr(board, "sprite", lambda color, kind: asked.append((color, kind)) or pygame.Surface((10, 10)))
        queen = Piece(7, 0, PlayerColor.WHITE, PieceType.QUEEN)
        surface = pygame.Surface((160, 160))
        board._draw_piece(surface, queen, (50, 50))
        board._draw_piece(surface, queen, (50, 50), PieceType.PAWN)  # a promoting pawn is shown as a pawn while it slides
        assert asked == [(PlayerColor.WHITE, PieceType.QUEEN), (PlayerColor.WHITE, PieceType.PAWN)]
 
