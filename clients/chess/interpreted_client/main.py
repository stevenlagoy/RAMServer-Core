import base64

import pygame
import time
import argparse

from piece import Piece, PlayerColor
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
    parser = argparse.ArgumentParser(description="Play chess!")
    parser.add_argument("-s", "--session", type=str, default=None, help="Session ID of the chess session to join (Optional)")
    parser.add_argument("--style", type=str, default="STANDARD", help="Color style to use for the board")
    args = parser.parse_args()

    # Connect and authenticate with server
    networking.SILENT = False
    sock = networking.connect()
    networking.authenticate(sock)
    time.sleep(0.05)
    networking.join_chess(sock, args.session)
    time.sleep(0.05)
    networking.ready(sock)

    # Pygame setup
    pygame.init()
    pygame.display.set_caption(SCREEN_CAPTION)
    screen = pygame.display.set_mode((SCREEN_WIDTH, SCREEN_HEIGHT))
    board = Board((BOARD_WIDTH, BOARD_HEIGHT), networking.own_color)
    board.from_FEN(START_FEN)
    clock = pygame.time.Clock()
    style = BoardStyle.STANDARD if args.style.upper() == "STANDARD" else BoardStyle.COLORFUL
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
                move = Move(from_rank, from_file, to_rank, to_file)
            if dragging:
                mouse_pos = screen_pos_to_board_pos(pygame.mouse.get_pos())
                if held_piece is None: continue
                held_piece.x = mouse_pos[0]
                held_piece.y = mouse_pos[1]
        
        # Logic
        if (state := networking.get_recent_state()) is not None:
            fen = bytes(state.view).decode("utf-8").split()[0]
            board.from_FEN(fen)
            print(f"networking.own_color = {networking.own_color}")
            print(f"board.view_for = {board.view_for}")
            print(f"Black pieces: {[(p.rank, p.file) for p in board.pieces if p.color == PlayerColor.BLACK]}")
            board.view_for = networking.own_color
        if move is not None:
            # Submit the action
            networking.send_action(sock, move)
        
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