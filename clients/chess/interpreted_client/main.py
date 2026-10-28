import base64

import pygame
import time
import argparse

from piece import Piece, PlayerColor
from board import Board, START_FEN
from move import Move
from styles import BoardStyle
from end_screen import EndScreen
import networking

SCREEN_WIDTH, SCREEN_HEIGHT = 1280, 720  # initial window size; the window can be resized
MIN_WIDTH, MIN_HEIGHT = 320, 320
BOARD_FILL = 0.9  # board size as a fraction of the window's shorter side

SCREEN_CAPTION = "Chess"


def layout(board: Board, screen: pygame.Surface) -> None:
    """Size and center the board for the current window size."""
    width, height = screen.get_size()
    board.resize(int(min(width, height) * BOARD_FILL))
    board.center_on((width // 2, height // 2))


def legal_moves_to(piece: Piece, square: tuple[int, int]) -> list[Move]:
    """The legal moves taking piece to square: none, one, or one per piece a pawn can promote to."""
    return [
        m for m in networking.legal_moves
        if (m.from_rank, m.from_file) == (piece.rank, piece.file) and (m.to_rank, m.to_file) == square
    ]


def play_or_choose(moves: list[Move]) -> tuple[Move | None, list[Move]]:
    """Returns (move to play, promotion moves to choose from): a single move is played, several open the picker."""
    return (moves[0], []) if len(moves) == 1 else (None, moves)


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
    screen = pygame.display.set_mode((SCREEN_WIDTH, SCREEN_HEIGHT), pygame.RESIZABLE)
    board = Board(0, networking.own_color or PlayerColor.WHITE)
    layout(board, screen)
    board.from_FEN(START_FEN)
    clock = pygame.time.Clock()
    style = BoardStyle.STANDARD if args.style.upper() == "STANDARD" else BoardStyle.COLORFUL
    board.style = style
    running = True
    held_piece: Piece | None = None  # being dragged
    selected: Piece | None = None  # picked by a click or a drag; its legal moves are shown
    deselect_on_release = False  # clicking the selected piece again, without dragging it, deselects it
    promotions: list[Move] = []  # one move per promotion piece, while the player picks one
    my_turn = False  # pieces can only be picked up on your turn
    end_screen: EndScreen | None = None  # shown once a match ends, until the next one starts

    # Main loop
    while running:
        move: Move | None = None

        # Input
        for event in pygame.event.get():
            if event.type == pygame.QUIT:
                running = False
            elif event.type == pygame.VIDEORESIZE:
                if event.w < MIN_WIDTH or event.h < MIN_HEIGHT:
                    screen = pygame.display.set_mode((max(event.w, MIN_WIDTH), max(event.h, MIN_HEIGHT)), pygame.RESIZABLE)
                screen = pygame.display.get_surface()
                layout(board, screen)
            elif end_screen is not None:
                # The end screen covers the board, so only its button takes input
                if event.type == pygame.MOUSEBUTTONDOWN and event.button == 1 and end_screen.button_at(event.pos):
                    networking.ready(sock)  # the rematch starts once both players are ready
                    end_screen.waiting = True
            elif event.type == pygame.KEYDOWN and event.key == pygame.K_ESCAPE:
                promotions, selected = [], None
            elif event.type == pygame.MOUSEBUTTONDOWN and event.button == 1 and promotions:
                # Clicking a piece in the picker plays that promotion; clicking anywhere else cancels the move
                choice = board.promotion_choice_at(board.screen_to_board(event.pos))
                move = next((m for m in promotions if m.promotion == choice), None)
                promotions, selected = [], None
            elif event.type == pygame.MOUSEBUTTONDOWN and event.button == 1:
                pos = board.screen_to_board(event.pos)
                square = board.square_at(pos)
                piece = board.piece_at_square(*square) if square is not None else None
                if selected is not None and square is not None and (moves := legal_moves_to(selected, square)):
                    move, promotions = play_or_choose(moves)  # second click of click-to-move
                    selected = None
                # Only your own pieces can be picked up, and only on your turn. Your color is unknown until the match starts
                elif my_turn and piece is not None and piece.color == networking.own_color:
                    deselect_on_release = piece is selected
                    selected = held_piece = piece
                    held_piece.x, held_piece.y = pos
                else:
                    selected = None
            elif event.type == pygame.MOUSEMOTION and held_piece is not None:
                held_piece.x, held_piece.y = board.screen_to_board(event.pos)
            elif event.type == pygame.MOUSEBUTTONUP and event.button == 1 and held_piece is not None:
                held_piece.x = held_piece.y = None  # snap back to its square unless the drop is a move
                square = board.square_at(board.screen_to_board(event.pos))
                if square is not None and (moves := legal_moves_to(held_piece, square)):
                    move, promotions = play_or_choose(moves)
                    selected = None
                elif square == (held_piece.rank, held_piece.file) and deselect_on_release:
                    selected = None
                held_piece = None  # an illegal drop leaves the piece selected, so a click can still move it
        
        # Logic
        if (state := networking.get_recent_state()) is not None:
            fen = bytes(state.view).decode("utf-8").split()[0]
            board.from_FEN(fen)
            print(f"networking.own_color = {networking.own_color}")
            print(f"board.view_for = {board.view_for}")
            print(f"Black pieces: {[(p.rank, p.file) for p in board.pieces if p.color == PlayerColor.BLACK]}")
            board.view_for = networking.own_color or PlayerColor.WHITE
            held_piece = selected = None  # the board was rebuilt, so these pieces no longer exist
            promotions = []
            my_turn = state.my_turn
            if my_turn:
                networking.request_legal_moves(sock)
            if end_screen is not None and end_screen.waiting:
                end_screen = None  # the rematch has started
        if (outcome := networking.get_match_outcome()) is not None:
            end_screen = EndScreen(outcome.won, outcome.draw)
            held_piece = selected = None
            promotions = []
            my_turn = False
        if move is not None:
            # Submit the action
            networking.send_action(sock, move)
            board.preview(move)
            my_turn = False  # until the server's reply; a rejected move restores the last good state, and with it your turn
        board.promotion = ((promotions[0].to_rank, promotions[0].to_file), networking.own_color) if promotions and networking.own_color else None
        board.selected = (selected.rank, selected.file) if selected is not None else None
        board.targets = {
            (m.to_rank, m.to_file) for m in networking.legal_moves
            if selected is not None and (m.from_rank, m.from_file) == (selected.rank, selected.file)
        }
        
        # Render
        screen.fill(style.background_color)
        board.draw(screen)
        if end_screen is not None:
            end_screen.draw(screen)
        pygame.display.flip()
        clock.tick(60)

    # Pygame teardown
    pygame.quit()
    networking.close(sock)

if __name__ == "__main__":
    main()