import pygame

from dataclasses import dataclass
from pathlib import Path

pygame.init()

screen_width, screen_height = 1280, 720

screen = pygame.display.set_mode((screen_width, screen_height))
pygame.display.set_caption("Chess Test")

clock = pygame.time.Clock()

running = True

assetsPath = Path().cwd() / "assets"

def fileOrdToChar(file: int) -> str:
    return chr(file + 96)

def fileCharToOrd(file: str) -> int:
    return int(file) - 96

@dataclass
class PieceType:
    name: str
    type: str
    symbol: str
    color: str # light or dark
    sprite: pygame.Surface

@dataclass
class ColorScheme:
    lightColor: tuple[int, int, int]
    darkColor: tuple[int, int, int]
    backgroundColor: tuple[int, int, int]

whiteAndBlack = ColorScheme((255, 255, 255), (32, 32, 32), (128, 128, 128))
tanAndBrown = ColorScheme((252, 246, 199), (54, 38, 20), (68, 60, 52))

piece_symbols = {"bishop": "B", "king": "K", "knight": "N", "pawn": "P", "queen": "Q", "rook": "R"} # Pawns have a symbol in FEN but not in move notation
pieces: list[PieceType] = []
def load_pieces() -> None:
    global pieces
    piece_assets = [
        "bishop-w.svg", "king-w.svg", "king-w.svg", "knight-w.svg", "pawn-w.svg", "queen-w.svg", "rook-w.svg",
        "bishop-b.svg", "king-b.svg", "king-b.svg", "knight-b.svg", "pawn-b.svg", "queen-b.svg", "rook-b.svg"
    ]
    for asset in piece_assets:
        piece_type = asset.split("-")[0]
        piece_color = "white" if asset.split("-")[1].split(".")[0] == "w" else "black"
        symbol = piece_symbols[piece_type]
        symbol = symbol.lower() if piece_color == "black" else symbol # lowercase black's symbols (FEN)
        sprite = pygame.image.load(assetsPath / asset)
        name = piece_color + " " + piece_type
        pieces.append(PieceType(name, piece_type, symbol, piece_color, sprite))
load_pieces()
print(pieces)

start_FEN = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"
end_FEN_ex = "r1bk3r/p2pBpNp/n4n2/1p1NP2P/6P1/3P4/P1P1K3/q5b1"
def load_FEN(FEN: str, screen: pygame.Surface) -> None:
    for rank, line in enumerate(FEN.split(" ")[0].split("/")):
        skip = 0
        for file, char in enumerate(line):
            if char.isnumeric():
                skip += int(char) - 1
                continue
            piece = None
            for p in pieces:
                if p.symbol == char:
                    piece = p
            if piece is None: continue
            square_x, square_y = get_square_coords(rank + 1, file + 1 + skip)
            screen.blit(piece.sprite, (square_x, square_y))

ranks, files = 8, 8
square_side_length = 45
center = screen_width / 2, screen_height / 2
board_width = files * square_side_length
board_height = ranks * square_side_length
board_top_left_corner_x = center[0] - (board_height / 2)
board_top_left_corner_y = center[1] - (board_width / 2)

def get_square_coords(rank: int, file: int | str) -> tuple[float, float]:
    if isinstance(file, str): file = fileCharToOrd(file)
    top_left_corner_x = (file * square_side_length) + board_top_left_corner_x
    top_left_corner_y = (rank * square_side_length) + board_top_left_corner_y
    return top_left_corner_x, top_left_corner_y

while running:
    # Look for actions (keyboard, mouse, quitting)
    for event in pygame.event.get():
        if event.type == pygame.QUIT:
            running = False

    # --- GAME LOGIC ---

    # --- RENDERING / DRAWING ---
    colorScheme = tanAndBrown
    screen.fill(colorScheme.backgroundColor)
    for rank in range(ranks):
        for file in range(files):
            color = colorScheme.darkColor if rank % 2 + file % 2 == 1 else colorScheme.lightColor
            top_left_corner_x, top_left_corner_y = get_square_coords(rank + 1, file + 1)
            rect = pygame.Rect(top_left_corner_x, top_left_corner_y, square_side_length, square_side_length)
            pygame.draw.rect(screen, color, rect)
    load_FEN(end_FEN_ex, screen)

    # (Draw your game characters, shapes, or images here!)

    # Update the display to show the changes
    pygame.display.flip()

    # --- FRAME RATE CONTROL ---
    # Limit the game to 60 frames per second (FPS)
    clock.tick(60)

pygame.quit()