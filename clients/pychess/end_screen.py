import pygame
from pygame import Rect, Surface

from .ui import BUTTON_DISABLED_COLOR, NEUTRAL_COLOR, NEUTRAL_HOVER_COLOR, draw_text

DIM_COLOR = (0, 0, 0, 150)  # darkens the board behind the panel
PANEL_COLOR = (40, 40, 40)
TEXT_COLOR = (240, 240, 240)
BUTTON_COLOR = (70, 140, 70)
BUTTON_HOVER_COLOR = (90, 170, 90)
BUTTON_WAITING_COLOR = (90, 90, 90)


class EndScreen:
    """Shown over the board when a match ends: the result, and a button to play again.
    After the button is pressed it shows that it is waiting for the opponent to play again too."""

    AGAIN = "again"
    LOBBY = "lobby"

    def __init__(self, title: str, subtitle: str, *, can_rematch: bool = True):
        self.title, self.subtitle = title, subtitle
        self.can_rematch = can_rematch # specatators can't play again
        self.waiting = False  # play again was pressed
        self.opponent_ready = False # opponent pressed it first
        self._again = Rect(0, 0, 0, 0)  # placed by draw()
        self._lobby = Rect(0, 0, 0, 0)

    def button_at(self, pos: tuple[int, int]) -> str | None:
        if self.can_rematch and not self.waiting and self._again.collidepoint(pos):
            return self.AGAIN
        if self._lobby.collidepoint(pos):
            return self.LOBBY
        return None


    def draw(self, target: Surface) -> None:
        width, height = target.get_size()
        unit = max(10, min(width, height)) // 20  # scales the panel with the window

        dim = Surface((width, height), pygame.SRCALPHA)
        dim.fill(DIM_COLOR)
        target.blit(dim, (0, 0))

        panel = Rect(0, 0, unit * 11, unit * 9)
        panel.center = (width // 2, height // 2)
        pygame.draw.rect(target, PANEL_COLOR, panel, border_radius=unit // 2)

        draw_text(target, self.title, unit * 2, TEXT_COLOR, max_width=panel.width - unit, center=(panel.centerx, panel.top + unit * 3 // 2))
        draw_text(target, self.subtitle, unit, TEXT_COLOR, max_width=panel.width - unit, center=(panel.centerx, panel.top + unit * 3))
        if self.opponent_ready and not self.waiting:
            draw_text(target, "Your opponent wants to play again", unit * 3 // 4, (170, 170, 170),
                      max_width=panel.width - unit, center=(panel.centerx, panel.top + unit * 4))

        mouse = pygame.mouse.get_pos()
        self._again = Rect(0, 0, unit * 9, unit * 3 // 2)
        self._again.center = (panel.centerx, panel.bottom - unit * 7 // 2)
        self._lobby = Rect(0, 0, unit * 9, unit * 3 // 2)
        self._lobby.center = (panel.centerx, panel.bottom - unit * 3 // 2 - unit // 4)
        if self.can_rematch:
            if self.waiting:
                color, label = BUTTON_DISABLED_COLOR, "Waiting for opponent..."
            else:
                color, label = (BUTTON_HOVER_COLOR if self._again.collidepoint(mouse) else BUTTON_COLOR), "Play again"
            pygame.draw.rect(target, color, self._again, border_radius=unit // 3)
            draw_text(target, label, unit, TEXT_COLOR, max_width=self._again.width - 8, center=self._again.center)
        color = NEUTRAL_HOVER_COLOR if self._lobby.collidepoint(mouse) else NEUTRAL_COLOR
        pygame.draw.rect(target, color, self._lobby, border_radius=unit // 3)
        draw_text(target, "Back to lobby", unit, TEXT_COLOR, max_width=self._lobby.width - 8, center=self._lobby.center)
