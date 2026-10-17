import pygame

from piece import Piece
from board import Board, START_FEN
from move import Move
from styles import BoardStyle
import networking

SCREEN_WIDTH, SCREEN_HEIGHT = 1280, 720
BOARD_WIDTH, BOARD_HEIGHT = 600, 600

SCREEN_CAPTION = "Chess"


board_top = (SCREEN_HEIGHT // 2) - (BOARD_HEIGHT // 2)
board_left = (SCREEN_WIDTH // 2) - (BOARD_WIDTH // 2)

def screen_pos_to_board_pos(x: float | tuple[float, float], y: float = 0.0) -> tuple[float, float]:
    if isinstance(x, tuple):
        x, y = x
    return x - board_left, y - board_top


def main() -> None:
    # Connect and authenticate with server
    sock = networking.connect()
    networking.authenticate(sock)

    # Pygame setup
    pygame.init()
    pygame.display.set_caption(SCREEN_CAPTION)
    screen = pygame.display.set_mode((SCREEN_WIDTH, SCREEN_HEIGHT))
    board = Board((BOARD_WIDTH, BOARD_HEIGHT))
    board.from_FEN(START_FEN)
    clock = pygame.time.Clock()
    style = BoardStyle.STANDARD
    board.style = style
    running = True
    mouse_origin: tuple[float, float] = (0, 0)
    dragging: bool = False
    held_piece: Piece | None = None

    # Main loop
    while running:
        move: Move | None = None

        # Input
        for event in pygame.event.get():
            if event.type == pygame.QUIT:
                running = False
            elif event.type == pygame.MOUSEBUTTONDOWN:
                mouse_origin = screen_pos_to_board_pos(pygame.mouse.get_pos())
                dragging = True
                held_piece = board.piece_at_coords(mouse_origin)
            elif event.type == pygame.MOUSEBUTTONUP:
                mouse_end = screen_pos_to_board_pos(pygame.mouse.get_pos())
                dragging = False
                if held_piece is None: continue
                square = board.coords_to_square(mouse_end)
                held_piece.x = None
                held_piece.y = None
                from_rank = held_piece.rank
                from_file = held_piece.file
                to_rank = square[0]
                to_file = square[1]
                held_piece.rank = to_rank
                held_piece.file = to_file
                move = Move(from_rank, to_rank, from_file, to_file)
            if dragging:
                mouse_pos = screen_pos_to_board_pos(pygame.mouse.get_pos())
                if held_piece is None: continue
                held_piece.x = mouse_pos[0]
                held_piece.y = mouse_pos[1]
        
        # Logic
        if move is not None:
            # Submit the action
            pass
        
        # Render
        screen.fill(style.background_color)
        board.render()
        screen.blit(board, (board_left, board_top))
        pygame.display.flip()
        clock.tick(60)

    # Pygame teardown
    pygame.quit()
    networking.close(sock)

if __name__ == "__main__":
    main()