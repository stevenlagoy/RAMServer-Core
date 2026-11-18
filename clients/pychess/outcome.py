from __future__ import annotations

REASON_TEXT = {
    "CHECKMATE": "Checkmate",
    "STALEMATE": "Stalemate",
    "INSUFFICIENT_MATERIAL": "Insufficient material",
    "MUTUAL_DRAW_OFFER": "Draw by agreement",
    "AUTOMATIC_DRAW": "Automatic draw (75-move rule or fivefold repetition)",
    "RESIGNATION": "Resignation",
}


def reason_text(reason: str) -> str:
    """Readable text for a termination_reason. Reasons this client does not know are made readable as they are."""
    if not reason:
        return ""
    return REASON_TEXT.get(reason) or reason.replace("_", " ").capitalize()


def result_text(winners: list[str] | tuple[str, ...], draw: bool, reason: str, my_id: str,
                names: dict[str, str], spectating: bool = False) -> tuple[str, str]:
    """(title, subtitle) for the end screen."""
    reason_label = reason_text(reason)
    if draw:
        return "Draw", reason_label or "The game is drawn"
    resigned = reason == "RESIGNATION"
    if spectating:
        winner = names.get(winners[0], "A player") if winners else "A player"
        return f"{winner} wins", reason_label or "Game over"
    if my_id in winners:
        return "You win!", "Your opponent resigned" if resigned else (reason_label or "Game over")
    return "You lose", "You resigned" if resigned else (reason_label or "Game over")


def history_text(winners: list[str] | tuple[str, ...], draw: bool, reason: str, turns: int, my_id: str) -> str:
    """One line describing a finished match, for the lobby's match history."""
    reason_label = reason_text(reason).lower()
    if draw:
        outcome = "Draw"
    elif my_id in winners:
        outcome = "Won"
    else:
        outcome = "Lost"
    text = outcome + (f" by {reason_label}" if reason_label and not draw else f" ({reason_label})" if reason_label else "")
    return text + (f", {turns} half-moves" if turns else "")