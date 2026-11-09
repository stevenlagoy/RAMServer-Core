from dataclasses import dataclass, field
import math

from .piece import Piece, PieceType, PlayerColor


BASE_DURATION = 0.25 # seconds for a move at regular speed
LIFT_SCALE = 0.18 # pieces which jump (knight) grow by this much in the middle of a jump
GHOST_VISIBLE_UNTIL = 0.85 # captured pieces dissapear once the capturing piece is this far through the animation


def duration_for(speed: float) -> float:
    return BASE_DURATION / max(speed, 0.1)


def ease_in_out(t: float) -> float:
    return t * t * (3 - 2 * t)


@dataclass
class PieceAnimation:
    piece: Piece
    start: tuple[int, int]
    end: tuple[int, int]
    duration: float
    elapsed: float = 0.0
    lift: bool = False
    shown_type: PieceType | None = None

    @property
    def raw_progress(self) -> float:
        return 1.0 if self.duration <= 0 else min(1.0, self.elapsed / self.duration)

    @property
    def done(self) -> bool:
        return self.raw_progress >= 1.0

    def position(self) -> tuple[float, float]:
        t = ease_in_out(self.raw_progress)
        return (self.start[0] + (self.end[0] - self.start[0]) * t, self.start[1] + (self.end[1] - self.start[1]) * t)

    @property
    def scale(self) -> float:
        return 1.0 + LIFT_SCALE * math.sin(math.pi * self.raw_progress) if self.lift else 1.0


@dataclass
class MovePlan:
    animations: list[PieceAnimation]
    ghosts: list[Piece] = field(default_factory=list)
    mover_color: PlayerColor = PlayerColor.WHITE

    def advance(self, dt: float) -> None:
        for animation in self.animations: animation.elapsed += dt

    @property
    def active(self) -> bool:
        return any(not a.done for a in self.animations)

    def visible_ghosts(self) -> list[Piece]:
        lead = self.animations[0].raw_progress if self.animations else 1.0
        return self.ghosts if lead < GHOST_VISIBLE_UNTIL else []


def _piece_at(pieces: list[Piece], rank: int, file: int) -> Piece | None:
    for piece in pieces:
        if piece.rank == rank and piece.file == file: return piece
    return None


def plan_move(before: list[Piece], after: list[Piece], uci: str, duration: float) -> MovePlan | None:
    from .move import Move
    move = Move.from_uci(uci)
    if move is None: return None
    start, end = (move.from_rank, move.from_file), (move.to_rank, move.to_file)
    mover = _piece_at(after, *end)
    origin = _piece_at(before, *start)
    if mover is None or origin is None or mover.color != origin.color:
        return None
    # The piece which arrived must be the one which left or a promoted pawn
    if (mover.piece_type != origin.piece_type) if move.promotion is None else (origin.piece_type != PieceType.PAWN or mover.piece_type != move.promotion):
        return None

    plan = MovePlan(
        [PieceAnimation(
            mover, start, end, duration,
            lift=origin.piece_type == PieceType.KNIGHT,
            shown_type=origin.piece_type if move.promotion else None,
        )],
        mover_color=mover.color,
    )

    captured = _piece_at(before, *end)
    if captured is not None and captured.color != origin.color:
        plan.ghosts.append(Piece(captured.rank, captured.file, captured.color, captured.piece_type))
    elif origin.piece_type == PieceType.PAWN and start[1] != end[1] and captured is None:
        # En passant
        passed = _piece_at(before, start[0], end[1])
        if passed is not None and passed.color != origin.color and passed.piece_type == PieceType.PAWN:
            plan.ghosts.append(Piece(passed.rank, passed.file, passed.color, passed.piece_type))
 
    if origin.piece_type == PieceType.KING and abs(end[1] - start[1]) == 2 and start[0] == end[0]:
        kingside = end[1] > start[1]
        rook_from = (start[0], 7 if kingside else 0)
        rook_to = (start[0], end[1] - 1 if kingside else end[1] + 1)
        rook = _piece_at(after, *rook_to)
        if rook is not None and rook.piece_type == PieceType.ROOK and rook.color == origin.color:
            plan.animations.append(PieceAnimation(rook, rook_from, rook_to, duration))
    return plan
 
