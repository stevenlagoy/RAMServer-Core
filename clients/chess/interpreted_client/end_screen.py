import pygame
from pygame import Rect, Surface

DIM_COLOR = (0, 0, 0, 150)  # darkens the board behind the panel
PANEL_COLOR = (40, 40, 40)
TEXT_COLOR = (240, 240, 240)
BUTTON_COLOR = (70, 140, 70)
BUTTON_HOVER_COLOR = (90, 170, 90)
BUTTON_WAITING_COLOR = (90, 90, 90)


class EndScreen:
    """Shown over the board when a match ends: the result, and a button to play again.
    After the button is pressed it shows that it is waiting for the opponent to play again too."""

    def __init__(self, won: bool, draw: bool):
        if draw:
            self.title, self.subtitle = "Draw", "The game is drawn"
        elif won:
            self.title, self.subtitle = "You win!", "Checkmate"
        else:
            self.title, self.subtitle = "You lose", "Checkmate"
        self.waiting = False  # play again was pressed
        self._button = Rect(0, 0, 0, 0)  # placed by draw()

    def button_at(self, pos: tuple[int, int]) -> bool:
        return not self.waiting and self._button.collidepoint(pos)

    def draw(self, target: Surface) -> None:
        width, height = target.get_size()
        unit = min(width, height) // 20  # scales the panel with the window

        dim = Surface((width, height), pygame.SRCALPHA)
        dim.fill(DIM_COLOR)
        target.blit(dim, (0, 0))

        panel = Rect(0, 0, unit * 10, unit * 7)
        panel.center = (width // 2, height // 2)
        pygame.draw.rect(target, PANEL_COLOR, panel, border_radius=unit // 2)

        title = pygame.font.Font(None, unit * 2).render(self.title, True, TEXT_COLOR)
        target.blit(title, title.get_rect(center=(panel.centerx, panel.top + unit * 3 // 2)))
        subtitle = pygame.font.Font(None, unit).render(self.subtitle, True, TEXT_COLOR)
        target.blit(subtitle, subtitle.get_rect(center=(panel.centerx, panel.top + unit * 3)))

        self._button = Rect(0, 0, unit * 8, unit * 3 // 2)
        self._button.center = (panel.centerx, panel.bottom - unit * 3 // 2)
        if self.waiting:
            color, label = BUTTON_WAITING_COLOR, "Waiting for opponent..."
        else:
            hovered = self._button.collidepoint(pygame.mouse.get_pos())
            color, label = (BUTTON_HOVER_COLOR if hovered else BUTTON_COLOR), "Play again"
        pygame.draw.rect(target, color, self._button, border_radius=unit // 3)
        text = pygame.font.Font(None, unit).render(label, True, TEXT_COLOR)
        target.blit(text, text.get_rect(center=self._button.center))
