"""Test setup: run pygame without a window, and make the client's modules importable.

The tests need the generated protobuf code (`buf generate` from the repository root) and pytest."""
import os
import sys
from pathlib import Path

os.environ.setdefault("SDL_VIDEODRIVER", "dummy")
os.environ.setdefault("SDL_AUDIODRIVER", "dummy")
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

import pytest

pytest.importorskip("gen.ramserver.v1.ramserver_pb2", reason="run `buf generate` first")


@pytest.fixture
def pieces_from_fen():
    """Build the Piece list for a FEN's placement, the way the client does."""
    from .board import Board
    from .piece import PlayerColor

    def build(fen: str):
        board = Board(0, PlayerColor.WHITE)
        board.from_FEN(fen)
        return board.pieces

    return build