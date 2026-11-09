from functools import lru_cache
import os
import time
from typing import Callable, Sequence

import pygame
from pygame import Rect, Surface


BACKGROUND = (30, 30, 30)
PANEL_COLOR = (40, 40, 40)
PANEL_ALT_COLOR = (54, 54, 54)
TEXT_COLOR = (240, 240, 240)
MUTED_TEXT_COLOR = (165, 165, 165)
BUTTON_COLOR = (70, 140, 70)
BUTTON_HOVER_COLOR = (90, 170, 90)
BUTTON_DISABLED_COLOR = (78, 78, 78)
BUTTON_DISABLED_TEXT_COLOR = (130, 130, 130)
DANGER_COLOR = (160, 60, 60)
DANGER_HOVER_COLOR = (195, 80, 80)
NEUTRAL_COLOR = (80, 84, 92)
NEUTRAL_HOVER_COLOR = (104, 110, 120)
ACCENT_COLOR = (80, 200, 120)
FIELD_COLOR = (24, 24, 24)
FIELD_FOCUS_COLOR = (70, 120, 170)
ERROR_COLOR = (205, 80, 80)
WARNING_COLOR = (230, 180, 60)


@lru_cache(maxsize=64)
def get_font(size: int) -> pygame.font.Font:
    return pygame.font.Font(None, max(8, size))


def fit_font(text: str, max_width: int, size: int) -> pygame.font.Font:
    while size > 10 and get_font(size).size(text)[0] > max_width:
        size -= 1
    return get_font(size)


def draw_text(surface: Surface, text: str, size: int, color: tuple[int, int, int] = TEXT_COLOR, *, max_width: int | None = None, **anchor: tuple[int, int]) -> Rect:
    font = fit_font(text, max_width, size) if max_width else get_font(size)
    if max_width is not None and font.size(text)[0] > max_width:
        text = _truncate(font, text, max_width)
    image = font.render(text, True, color)
    rect = image.get_rect(**anchor)
    surface.blit(image, rect)
    return rect


def draw_panel(surface: Surface, rect: Rect, color: tuple[int, int, int] = PANEL_COLOR, radius: int = 8) -> None:
    pygame.draw.rect(surface, color, rect, border_radius=radius)


_internal_clipboard = ""
_system_clipboard_works = False


def clipboard_put(text: str) -> bool:
    global _internal_clipboard, _system_clipboard_works
    _internal_clipboard = text
    try:
        pygame.scrap.init()
        if hasattr(pygame.scrap, "put_text"):
            pygame.scrap.put_text(text)
        else:
            pygame.scrap.put(pygame.SCRAP_TEXT, text.encode("utf-8"))
        _system_clipboard_works = True
    except Exception: # No reachable clipboard
        _system_clipboard_works = False
    return _system_clipboard_works


def clipboard_get() -> str:
    if not _system_clipboard_works:
        try:
            pygame.scrap.init()
            data = pygame.scrap.get_text() if hasattr(pygame.scrap, "get_text") else pygame.scrap.get(pygame.SCRAP_TEXT)
        except Exception:
            return _internal_clipboard
        if not data:
            return _internal_clipboard
        return (data if isinstance(data, str) else data.decode("utf-8", "ignore")).strip("\x00\r\r")


def debug_enabled() -> bool:
    return bool(os.environ.get("RAMCHESS_DEBUG"))


def _truncate(font: pygame.font.Font, text: str, max_width: int) -> str:
    if max_width <= 0: return ""
    ellipsis = "..."
    while text and font.size(text + ellipsis)[0] > max_width:
        text = text[:-1]
    return (text.rstrip() + ellipsis) if text else (ellipsis if font.size(ellipsis)[0] <= max_width else "")


class Widget:
    def __init__(self) -> None:
        self.rect = Rect(0, 0, 0, 0)
        self.enabled = True
        self.visible = True

    def handle_event(self, event: pygame.event.Event) -> bool:
        if self._clicked(event):
            if debug_enabled():
                print(f"[ui] clicked {self.label!r} at {event.pos}")
            self.on_click()
            return True
        return False

    def draw(self, surface: Surface) -> None: pass

    def _clicked(self, event: pygame.event.Event) -> bool:
        return (self.enabled and self.visible and event.type == pygame.MOUSEBUTTONDOWN and event.button == 1 and self.rect.collidepoint(event.pos))


class Button(Widget):
    def __init__(self, label: str, on_click: Callable[[], None], *, kind: str = "primary") -> None:
        super().__init__()
        self.label = label
        self.on_click = on_click
        self.kind = kind

    def handle_event(self, event: pygame.event.Event) -> bool:
        if self._clicked(event):
            self.on_click()
            return True
        return False

    def draw(self, surface: Surface) -> None:
        if not self.visible: return
        hovered = self.enabled and self.rect.collidepoint(pygame.mouse.get_pos())
        base, hover = {"danger": (DANGER_COLOR, DANGER_HOVER_COLOR), "neutral": (NEUTRAL_COLOR, NEUTRAL_HOVER_COLOR)}.get(self.kind, (BUTTON_COLOR, BUTTON_HOVER_COLOR))
        color = BUTTON_DISABLED_COLOR if not self.enabled else (hover if hovered else base)
        pygame.draw.rect(surface, color, self.rect, border_radius=max(4, self.rect.height // 5))
        draw_text(
            surface, self.label, int(self.rect.height * 0.55), TEXT_COLOR if self.enabled else BUTTON_DISABLED_TEXT_COLOR,
            max_width=self.rect.width - 12, center=self.rect.center
        )


class _LabeledWidget(Widget):
    LABEL_SHARE = 0.42

    def __init__(self, label: str) -> None:
        super().__init__()
        self.label = label

    @property
    def control_rect(self) -> Rect:
        label_width = int(self.rect.width * self.LABEL_SHARE)
        return Rect(self.rect.x + label_width, self.rect.y, self.rect.width - label_width, self.rect.height)
 
    def _draw_label(self, surface: Surface) -> None:
        color = TEXT_COLOR if self.enabled else BUTTON_DISABLED_TEXT_COLOR
        draw_text(surface, self.label, int(self.rect.height * 0.55), color,
                  max_width=int(self.rect.width * self.LABEL_SHARE) - 8, midleft=(self.rect.x + 4, self.rect.centery))


class Toggle(_LabeledWidget):
    def __init__(self, label: str, value: bool, on_change: Callable[[bool], None]) -> None:
        super().__init__(label)
        self.value = value
        self.on_change = on_change
 
    def handle_event(self, event: pygame.event.Event) -> bool:
        if self._clicked(event):
            self.value = not self.value
            self.on_change(self.value)
            return True
        return False
 
    def draw(self, surface: Surface) -> None:
        self._draw_label(surface)
        height = int(self.rect.height * 0.62)
        track = Rect(0, 0, height * 2, height)
        track.midleft = (self.control_rect.x, self.rect.centery)
        pygame.draw.rect(surface, ACCENT_COLOR if self.value else NEUTRAL_COLOR, track, border_radius=height // 2)
        knob_x = track.right - height // 2 if self.value else track.x + height // 2
        pygame.draw.circle(surface, TEXT_COLOR, (knob_x, track.centery), height // 2 - 3)
        draw_text(surface, "On" if self.value else "Off", int(self.rect.height * 0.5), MUTED_TEXT_COLOR, midleft=(track.right + 10, self.rect.centery))


class Slider(_LabeledWidget):
    def __init__(
        self, label: str, minimum: float, maximum: float, value: float, step: float,
        on_change: Callable[[float], None], fmt: Callable[[float], str] | None = None
    ) -> None:
        super().__init__(label)
        self.minimum, self.maximum, self.step = minimum, maximum, step
        self.value = value
        self.on_change = on_change
        self.fmt = fmt or (lambda v: f"{v:g}")
        self.dragging = False
 
    @property
    def track(self) -> Rect:
        control = self.control_rect
        return Rect(control.x, self.rect.centery - 3, max(1, control.width - int(self.rect.height * 2.2)), 6)
 
    def _set_from_x(self, x: int) -> None:
        track = self.track
        fraction = min(max((x - track.x) / track.width, 0.0), 1.0)
        raw = self.minimum + fraction * (self.maximum - self.minimum)
        value = round(raw / self.step) * self.step
        value = round(min(max(value, self.minimum), self.maximum), 6)
        if value != self.value:
            self.value = value
            self.on_change(value)
 
    def handle_event(self, event: pygame.event.Event) -> bool:
        if not (self.enabled and self.visible):
            return False
        if event.type == pygame.MOUSEBUTTONDOWN and event.button == 1 and self.track.inflate(12, self.rect.height).collidepoint(event.pos):
            self.dragging = True
            self._set_from_x(event.pos[0])
            return True
        if event.type == pygame.MOUSEMOTION and self.dragging:
            self._set_from_x(event.pos[0])
            return True
        if event.type == pygame.MOUSEBUTTONUP and event.button == 1 and self.dragging:
            self.dragging = False
            return True
        return False
 
    def draw(self, surface: Surface) -> None:
        self._draw_label(surface)
        track = self.track
        pygame.draw.rect(surface, NEUTRAL_COLOR, track, border_radius=3)
        fraction = (self.value - self.minimum) / (self.maximum - self.minimum)
        knob_x = track.x + round(fraction * track.width)
        pygame.draw.rect(surface, ACCENT_COLOR, Rect(track.x, track.y, knob_x - track.x, track.height), border_radius=3)
        pygame.draw.circle(surface, TEXT_COLOR, (knob_x, track.centery), int(self.rect.height * 0.26))
        draw_text(
            surface, self.fmt(self.value), int(self.rect.height * 0.5), MUTED_TEXT_COLOR,
            midright=(self.rect.right - 4, self.rect.centery)
        )


class Selector(_LabeledWidget):
    def __init__(self, label: str, options: Sequence[str], index: int, on_change: Callable[[int], None]) -> None:
        super().__init__(label)
        self.options = list(options)
        self.index = index if 0 <= index < len(options) else 0
        self.on_change = on_change

    def _arrows(self) -> tuple[Rect, Rect]:
        control = self.control_rect
        size = int(self.rect.height * 0.8)
        left = Rect(control.x, 0, size, size)
        right = Rect(control.right - size - 4, 0, size, size)
        left.centery = right.centery = self.rect.centery
        return left, right

    def handle_event(self, event: pygame.event.Event) -> bool:
        if not self._clicked(event): return False
        left, right = self._arrows()
        step = -1 if left.collidepoint(event.pos) else 1 if right.collidepoint(event.pos) else 0
        if step == 0: step = 1 if event.pos[0] >= self.control_rect.centerx else -1
        self.index = (self.index + step) % len(self.options)
        self.on_change(self.index)
        return True

    def draw(self, surface: Surface) -> None:
        self._draw_label(surface)
        left, right = self._arrows()
        for arrow, glyph in ((left, "<"), (right, ">")):
            pygame.draw.rect(surface, NEUTRAL_COLOR, arrow, border_radius=4)
            draw_text(surface, glyph, int(self.rect.height * 0.6), center=arrow.center)
        middle = Rect(left.right, self.rect.y, right.x - left.right, self.rect.height)
        draw_text(surface, self.options[self.index], int(self.rect.height * 0.55), max_width=middle.width - 8, center=middle.center)


class TextInput(_LabeledWidget):

    REPEAT_DELAY = 0.4
    REPEAT_INTERVAL = 0.035
    DOUBLE_CLICK = 0.4
    SELECTION_COLOR = (50, 95, 150)

    def __init__(
        self, label: str, text: str = "", *, max_len: int = 64, digits_only: bool = False,
        on_submit: Callable[[], None] | None = None, clearable: bool = True
    ) -> None:
        super().__init__(label)
        self.max_len = max_len
        self.digits_only = digits_only
        self.on_submit = on_submit
        self.clearable = clearable
        self._text = text[:max_len]
        self.cursor = self.anchor = len(self._text)
        self._scroll = 0
        self._focused = False
        self._dragging = False
        self._held: tuple[pygame.event.Event, float] | None = None # keys being held
        self._last_click = 0.0

    @property
    def text(self) -> str:
        return self._text

    @text.setter
    def text(self, value: str) -> None:
        self._text = value
        self.cursor = self.anchor = len(value)
        self._scroll = 0

    @property
    def focused(self) -> bool:
        return self._focused

    @focused.setter
    def focused(self, value: bool) -> None:
        if not value:
            self._held = None
            self._dragging = False
        self._focused = value

    @property
    def selection(self) -> tuple[int, int]:
        return min(self.anchor, self.cursor), max(self.anchor, self.cursor)

    def selected_text(self) -> str:
        start, end = self.selection
        return self._text[start:end]

    def _insert(self, value: str) -> None:
        start, end = self.selection
        room = self.max_len - (len(self._text) - (end - start))
        value = value[: max(room, 0)]
        self._text = self._text[:start] + value + self._text[end:]
        self.cursor = self.anchor = start + len(value)

    def _accepts(self, character: str) -> bool:
        return character.isprintable() and (character.isdigit() or not self.digits_only)

    def _field(self) -> Rect:
        return self.control_rect.inflate(-4, -int(self.rect.height * 0.15))

    def _clear_visible(self) -> bool:
        return self.clearable and self.enabled and bool(self._text)

    def _clear_rect(self) -> Rect:
        field = self._field()
        size = int(field.height * 0.6)
        rect = Rect(0, 0, size, size)
        rect.midright = (field.right - 6, field.centery)
        return rect

    def _text_area(self) -> Rect:
        field = self._field()
        right = self._clear_rect().left - 6 if self._clear_visible() else field.right - 8
        return Rect(field.x + 8, field.y, max(1, right - (field.x + 8)), field.height)

    def _font(self) -> pygame.font.Font:
        return get_font(int(self.rect.height * 0.55))

    def _x_of(self, index: int) -> int:
        return self._font().size(self._text[:index])[0]

    def _index_at(self, x: float) -> int:
        best, best_distance = 0, abs(x)
        for index in range(1, len(self._text) + 1):
            distance = abs(x - self._x_of(index))
            if distance < best_distance:
                best, best_distance = index, distance
        return best

    def _reveal_caret(self, width: int) -> None:
        caret = self._x_of(self.cursor)
        if caret - self._scroll > width - 2:
            self._scroll = caret - width + 2
        if caret - self._scroll < 0:
            self._scroll = caret
        self._scroll = max(0, min(self._scroll, self._x_of(len(self._text)) - width + 2))

    def _word_start(self, index: int) -> int:
        while index > 0 and self._text[index - 1] == " ":
            index -= 1
        while index > 0 and self._text[index - 1] != " ":
            index -= 1
        return index

    def _word_end(self, index: int) -> int:
        while index < len(self._text) and self._text[index] == " ":
            index += 1
        while index < len(self._text) and self._text[index] != " ":
            index += 1
        return index

    def _apply_key(self, event: pygame.event.Event) -> bool:
        """Do what a key press asks. Also used for each repeat of a held key. Returns True if the key was used."""
        key, mods = event.key, event.mod
        ctrl, shift = bool(mods & (pygame.KMOD_CTRL | pygame.KMOD_META)), bool(mods & pygame.KMOD_SHIFT)
        start, end = self.selection
        if key in (pygame.K_BACKSPACE, pygame.K_DELETE):
            if start == end:
                if key == pygame.K_BACKSPACE:
                    start = self._word_start(end) if ctrl else max(0, end - 1)
                else:
                    end = self._word_end(start) if ctrl else min(len(self._text), start + 1)
                self.anchor, self.cursor = start, end
            self._insert("")
        elif key in (pygame.K_LEFT, pygame.K_RIGHT):
            direction = -1 if key == pygame.K_LEFT else 1
            if start != end and not shift:
                target = start if direction < 0 else end  # leaving a selection puts the caret at its edge
            elif ctrl:
                target = self._word_start(self.cursor) if direction < 0 else self._word_end(self.cursor)
            else:
                target = min(max(self.cursor + direction, 0), len(self._text))
            self.cursor = target
            if not shift:
                self.anchor = target
        elif key in (pygame.K_HOME, pygame.K_END):
            self.cursor = 0 if key == pygame.K_HOME else len(self._text)
            if not shift:
                self.anchor = self.cursor
        elif ctrl and key == pygame.K_a:
            self.anchor, self.cursor = 0, len(self._text)
        elif ctrl and key in (pygame.K_c, pygame.K_x):
            if start != end:
                clipboard_put(self.selected_text())
                if key == pygame.K_x:
                    self._insert("")
        elif ctrl and key == pygame.K_v:
            self._insert("".join(c for c in clipboard_get() if self._accepts(c)))
        elif key in (pygame.K_RETURN, pygame.K_KP_ENTER):
            if self.on_submit:
                self.on_submit()
        elif not ctrl and event.unicode and self._accepts(event.unicode):
            self._insert(event.unicode)
        else:
            return False
        return True
 
    def handle_event(self, event: pygame.event.Event) -> bool:
        if not (self.enabled and self.visible):
            return False
        if event.type == pygame.MOUSEBUTTONDOWN and event.button == 1:
            if not self.rect.collidepoint(event.pos):
                self.focused = False
                return False
            self.focused = True
            if self._clear_visible() and self._clear_rect().collidepoint(event.pos):
                self.text = ""
                return True
            index = self._index_at(event.pos[0] - self._text_area().x + self._scroll)
            now = time.monotonic()
            if now - self._last_click < self.DOUBLE_CLICK:
                self.anchor, self.cursor = 0, len(self._text) # double click to select all
            else:
                self.cursor = index
                if not pygame.key.get_mods() & pygame.KMOD_SHIFT:
                    self.anchor = index
                self._dragging = True
            self._last_click = now
            return True
        if event.type == pygame.MOUSEMOTION and self._dragging:
            self.cursor = self._index_at(event.pos[0] - self._text_area().x + self._scroll)
            return True
        if event.type == pygame.MOUSEBUTTONUP and event.button == 1:
            was_dragging, self._dragging = self._dragging, False
            return was_dragging
        if event.type == pygame.KEYUP and self._held is not None and self._held[0].key == event.key:
            self._held = None
            return False
        if not self._focused or event.type != pygame.KEYDOWN:
            return False
        if not self._apply_key(event):
            return False
        ctrl = bool(event.mod & (pygame.KMOD_CTRL | pygame.KMOD_META))
        repeats = event.key in (pygame.K_BACKSPACE, pygame.K_DELETE, pygame.K_LEFT, pygame.K_RIGHT) or (not ctrl and bool(event.unicode))
        self._held = (event, time.monotonic() + self.REPEAT_DELAY) if repeats and event.key not in (pygame.K_RETURN, pygame.K_KP_ENTER) else None
        return True

    def _repeat_held_key(self) -> None:
        """Repeat a key which is being held down. Called from draw(), which runs once per frame, so a field needs no
        update call of its own."""
        if self._held is None:
            return
        event, due = self._held
        if not self._focused or not pygame.key.get_pressed()[event.key]:
            self._held = None
            return
        now = time.monotonic()
        for _ in range(5):  # a slow frame catches up a little, not all at once
            if now < due:
                break
            self._apply_key(event)
            due += self.REPEAT_INTERVAL
        self._held = (event, max(due, now))

    def draw(self, surface: Surface) -> None:
        self._repeat_held_key()
        self._draw_label(surface)
        field = self._field()
        pygame.draw.rect(surface, FIELD_COLOR, field, border_radius=4)
        pygame.draw.rect(surface, FIELD_FOCUS_COLOR if self._focused else NEUTRAL_COLOR, field, 2, border_radius=4)
        area, font = self._text_area(), self._font()
        self._reveal_caret(area.width)
        clip = surface.get_clip()
        surface.set_clip(clip)
        left = area.x - self._scroll
        start, end = self.selection
        if self._focused and start != end:
            selected = Rect(left + self._x_of(start), area.centery - font.get_height() // 2, self._x_of(end) - self._x_of(start), font.get_height())
            pygame.draw.rect(surface, self.SELECTION_COLOR, selected)
        image = font.render(self._text, True, TEXT_COLOR)
        surface.blit(image, image.get_rect(midleft=(left, area.centery)))
        if self._focused and int(time.time() * 2) % 2 == 0:
            x = left + self._x_of(self.cursor)
            pygame.draw.line(surface, TEXT_COLOR, (x, area.centery - font.get_height() // 2), (x, area.centery + font.get_height() // 2), 2)
        surface.set_clip(clip)
        if self._clear_visible():
            clear = self._clear_rect()
            hovered = clear.collidepoint(pygame.mouse.get_pos())
            pygame.draw.circle(surface, NEUTRAL_HOVER_COLOR if hovered else NEUTRAL_COLOR, clear.center, clear.width // 2)
            reach = clear.width // 4
            for a, b in (((-reach, -reach), (reach, reach)), ((-reach, reach), (reach, -reach))):
                pygame.draw.line(surface, TEXT_COLOR, (clear.centerx + a[0], clear.centery + a[1]), (clear.centerx + b[0], clear.centery + b[1]), 2)


class Toasts:
    def __init__(self) -> None:
        self.items: list[list] = []  # [text, seconds left, is error]

    def add(self, text: str, *, error: bool = False, seconds: float = 3.5) -> None:
        if self.items and self.items[-1][0] == text:
            self.items[-1][1] = seconds  # repeating a message only extends the one showing
            return
        self.items.append([text, seconds, error])
        del self.items[:-4]
 
    def update(self, dt: float) -> None:
        for item in self.items:
            item[1] -= dt
        self.items = [item for item in self.items if item[1] > 0]
 
    def draw(self, surface: Surface) -> None:
        width, height = surface.get_size()
        size = max(16, min(width, height) // 28)
        y = height - size
        for text, _, error in reversed(self.items):
            font = fit_font(text, int(width * 0.9) - 2 * size, size)
            image = font.render(text, True, TEXT_COLOR)
            box = image.get_rect().inflate(size, size // 2)
            box.midbottom = (width // 2, y)
            pygame.draw.rect(surface, DANGER_COLOR if error else PANEL_ALT_COLOR, box, border_radius=size // 3)
            surface.blit(image, image.get_rect(center=box.center))
            y = box.top - size // 4
