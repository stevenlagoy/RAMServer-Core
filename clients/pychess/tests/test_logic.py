"""Tests of the client's logic which needs no window: FEN, settings, styles, moves, outcome wording."""
import json

from ..fen import parse_fen
from ..move import ACCEPT_DRAW, Move
from ..outcome import history_text, reason_text, result_text
from ..piece import PieceType
from ..settings import Settings, default_custom_colors
from ..styles import PRESETS, resolve_style, style_names

START = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"


class TestFen:
    def test_start_position(self):
        view = parse_fen(START)
        assert view and view.white_to_move and view.fullmove == 1 and view.plies == 0

    def test_plies_count_each_side(self):
        assert parse_fen("rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 1").plies == 1
        assert parse_fen("4k3/8/8/8/8/8/8/R3K3 w - - 149 100").plies == 198
        assert parse_fen("4k3/8/8/8/8/8/8/R3K3 b - - 149 100").plies == 199

    def test_bad_input_gives_none(self):
        for bad in ["", "8/8/8/8/8/8/8/8 w - -", "8/8/8 w - - 0 1", START.replace(" w ", " x "),
                    START.replace(" 0 1", " a 1"), START.replace(" 0 1", " 0 0"), START.replace(" 0 1", " -1 1")]:
            assert parse_fen(bad) is None, bad


class TestSettings:
    def test_round_trip(self, tmp_path):
        path = tmp_path / "nested" / "settings.json"
        s = Settings(style="OCEAN", show_coordinates=True, animation_speed=2.0, show_unsafe_squares="always", server="wss://example.org/connect")
        s.custom_colors["dark"] = [1, 2, 3]
        assert s.save(path)
        assert Settings.load(path) == s
        assert not list(path.parent.glob("*.tmp"))  # the atomic write leaves no temporary file

    def test_missing_or_broken_file_gives_defaults(self, tmp_path):
        assert Settings.load(tmp_path / "none.json") == Settings()
        (tmp_path / "bad.json").write_text("{not json")
        assert Settings.load(tmp_path / "bad.json") == Settings()
        (tmp_path / "list.json").write_text("[1]")
        assert Settings.load(tmp_path / "list.json") == Settings()

    def test_invalid_values_fall_back_one_by_one(self, tmp_path):
        path = tmp_path / "s.json"
        path.write_text(json.dumps({
            "show_coordinates": "yes", "animation_speed": 99, "server": "", "show_unsafe_squares": "sometimes",
            "show_last_move": False, "style": "OCEAN", "unknown": 1,
            "custom_colors": {"light": [300, -5, 7], "dark": "red", "border": [1, 2]},
        }))
        s = Settings.load(path)
        defaults = Settings()
        assert s.show_coordinates == defaults.show_coordinates
        assert s.animation_speed == 4.0  # clamped, not dropped
        assert s.server == defaults.server
        assert s.show_unsafe_squares == defaults.show_unsafe_squares
        assert s.show_last_move is False and s.style == "OCEAN"
        assert s.custom_colors["light"] == [255, 0, 7]
        assert s.custom_colors["dark"] == default_custom_colors()["dark"]
        assert s.custom_colors["border"] == default_custom_colors()["border"]

    def test_reset_keeps_connection_details(self):
        s = Settings(server="example.org:1", display_name="Zed", style="OCEAN", show_coordinates=True)
        s.reset_appearance()
        assert (s.server, s.display_name) == ("example.org:1", "Zed")
        assert s.style == "STANDARD" and not s.show_coordinates

    def test_settings_saved_with_a_host_and_port_are_migrated(self, tmp_path):
        path = tmp_path / "old.json"
        path.write_text(json.dumps({"host": "example.org", "port": 9100, "style": "OCEAN"}))
        s = Settings.load(path)
        assert s.server == "example.org:9100" and s.style == "OCEAN"
        path.write_text(json.dumps({"host": "example.org", "port": 0}))
        assert Settings.load(path).server == "example.org:8080"
        path.write_text(json.dumps({"host": "old.example", "server": "wss://new.example/connect"}))
        assert Settings.load(path).server == "wss://new.example/connect"  # the new setting wins

    def test_unwritable_location_reports_failure(self, tmp_path):
        blocker = tmp_path / "file"
        blocker.write_text("x")
        assert Settings().save(blocker / "settings.json") is False


class TestStyles:
    def test_presets_resolve_by_name(self):
        for name in PRESETS:
            assert resolve_style(name, {}) is PRESETS[name]
        assert resolve_style("ocean", {}) is PRESETS["OCEAN"]

    def test_unknown_name_gives_standard(self):
        assert resolve_style("NOPE", {}) is PRESETS["STANDARD"]

    def test_custom_uses_the_chosen_colors(self):
        style = resolve_style("CUSTOM", {"light": [1, 2, 3], "dark": [4, 5, 6], "background": [7, 8, 9], "border": [10, 11, 12]})
        assert (style.light_color, style.dark_color, style.background_color, style.border_color) == ((1, 2, 3), (4, 5, 6), (7, 8, 9), (10, 11, 12))

    def test_custom_is_listed_last(self):
        assert style_names()[-1] == "CUSTOM"


class TestMove:
    def test_plain_and_promotion(self):
        move = Move.from_uci("e7e8q")
        assert (move.from_rank, move.from_file, move.to_rank, move.to_file) == (6, 4, 7, 4)
        assert move.promotion == PieceType.QUEEN and not move.offer_draw
        assert move.encode() == b"e7e8q"

    def test_draw_offer_suffix_round_trips(self):
        move = Move.from_uci("e2e4(=)")
        assert move.offer_draw and move.squares() == "e2e4"
        assert move.encode() == b"e2e4(=)"
        assert Move.from_uci("e7e8q(=)").encode() == b"e7e8q(=)"

    def test_accepting_is_not_a_move(self):
        assert Move.from_uci("(=)") is None
        assert ACCEPT_DRAW == b"(=)"

    def test_rejects_other_text(self):
        for bad in ["", "e2", "e2e4x", "E2E4", "e2e4(==)", "resign", "e9e4"]:
            assert Move.from_uci(bad) is None, bad


class TestOutcomeText:
    names = {"a": "Alice", "b": "Bob"}

    def test_reason_text(self):
        assert reason_text("CHECKMATE") == "Checkmate"
        assert reason_text("") == ""
        assert reason_text("SOMETHING_NEW") == "Something new"

    def test_player_results(self):
        assert result_text(["a"], False, "CHECKMATE", "a", self.names) == ("You win!", "Checkmate")
        assert result_text(["b"], False, "CHECKMATE", "a", self.names) == ("You lose", "Checkmate")
        assert result_text(["b"], False, "RESIGNATION", "a", self.names) == ("You lose", "You resigned")
        assert result_text(["a"], False, "RESIGNATION", "a", self.names) == ("You win!", "Your opponent resigned")

    def test_draws_and_spectators(self):
        assert result_text([], True, "STALEMATE", "a", self.names) == ("Draw", "Stalemate")
        assert result_text([], True, "", "a", self.names) == ("Draw", "The game is drawn")
        assert result_text(["b"], False, "CHECKMATE", "a", self.names, spectating=True) == ("Bob wins", "Checkmate")

    def test_old_servers_send_no_reason(self):
        assert result_text(["a"], False, "", "a", self.names) == ("You win!", "Game over")

    def test_history_lines(self):
        assert history_text(["a"], False, "CHECKMATE", 41, "a") == "Won by checkmate, 41 half-moves"
        assert history_text(["b"], False, "RESIGNATION", 7, "a") == "Lost by resignation, 7 half-moves"
        assert history_text([], True, "STALEMATE", 70, "a") == "Draw (stalemate), 70 half-moves"
        assert history_text([], True, "", 0, "a") == "Draw"