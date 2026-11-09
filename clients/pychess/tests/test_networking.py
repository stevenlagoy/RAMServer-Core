"""Tests of parsing, framing, and the bookkeeping in NetworkClient. No server is needed."""
import socket
import struct

import pytest

from .. import networking as n
from ..networking import pb


def server_message(**fields):
    message = pb.ServerMessage()
    message.in_reply_to = fields.pop("in_reply_to", 0)
    for name, init in fields.items():
        getattr(message, name).SetInParent()
        init(getattr(message, name))
    return message


class TestParsing:
    def test_heartbeat_and_empty_messages_give_no_event(self):
        assert n.parse_server_message(pb.ServerMessage()) is None

    def test_connect_and_auth(self):
        assert n.parse_server_message(server_message(connect_response=lambda m: setattr(m, "server_protocol_version", "1.2"))) == n.Connected("1.2")
        auth = server_message(auth_response=lambda m: (setattr(m, "player_id", "p1"), setattr(m, "reconnect_token", "t")))
        assert n.parse_server_message(auth) == n.Authed("p1", "t")

    def test_state_update_with_the_1_2_fields(self):
        def fill(s):
            s.session_id, s.game_id, s.status = "s1", "chess", n.STATUS_IN_PROGRESS
            s.state_version, s.encoded_view, s.active_player_id = 3, b"fen", "p1"
            s.owner_player_id = "p1"
            s.player_order.extend(["p1", "p2"])
            s.annotations["check"] = "white"
            s.annotations["checkers"] = "b4"
            for pid, role, ready in (("p1", "player", True), ("p2", "spectator", False)):
                p = s.players.add()
                p.player_id, p.display_name, p.connected, p.role, p.ready = pid, pid.upper(), True, role, ready

        event = n.parse_server_message(server_message(in_reply_to=9, state_update=fill))
        snap = event.snapshot
        assert snap.in_reply_to == 9 and snap.in_progress and snap.view == b"fen" and snap.version == 3
        assert snap.owner_id == "p1" and snap.player_order == ("p1", "p2")
        assert snap.annotations == {"check": "white", "checkers": "b4"}
        assert snap.player("p1") == n.Player("p1", "P1", True, True, "player")
        assert snap.player("p2").role == "spectator" and snap.player("nobody") is None

    def test_legal_actions_keep_the_draw_accept_payload(self):
        def fill(r):
            for payload in (b"e2e4", b"(=)"):
                r.legal_actions.add().payload = payload

        assert n.parse_server_message(server_message(legal_actions_response=fill)) == n.LegalMovesEvent(("e2e4", "(=)"))

    def test_match_result_carries_the_reason_and_turns(self):
        def fill(m):
            m.winner_player_ids.append("p1")
            m.termination_reason, m.turns = "RESIGNATION", 12

        event = n.parse_server_message(server_message(match_result=fill))
        assert event == n.MatchEnded(n.MatchSummary(("p1",), False, "RESIGNATION", 12))

    def test_session_list_and_history(self):
        def sessions(r):
            info = r.sessions.add()
            info.session_id, info.game_id, info.status, info.owner_player_id, info.joinable = "s1", "chess", 1, "p1", True
            info.players.add().player_id = "p1"

        event = n.parse_server_message(server_message(list_sessions_response=sessions))
        assert event.sessions[0].session_id == "s1" and event.sessions[0].joinable and event.sessions[0].players[0].player_id == "p1"

        def history(r):
            r.session_id = "s1"
            m = r.matches.add()
            m.draw, m.termination_reason, m.turns = True, "STALEMATE", 70

        assert n.parse_server_message(server_message(match_history_response=history)) == n.HistoryEvent("s1", (n.MatchSummary((), True, "STALEMATE", 70),))

    def test_errors_and_leaving(self):
        def error(e):
            e.code, e.message = pb.ERROR_CODE_ILLEGAL_ACTION, "e2e5"

        event = n.parse_server_message(server_message(in_reply_to=4, error_response=error))
        assert event == n.ErrorEvent("ERROR_CODE_ILLEGAL_ACTION", "e2e5", 4, "")
        assert n.readable_error(event) == "Illegal move"
        assert n.readable_error(n.ErrorEvent("ERROR_CODE_FUTURE", "why")) == "ERROR_CODE_FUTURE: why"
        left = server_message(leave_session_response=lambda m: setattr(m, "session_id", "s1"))
        assert n.parse_server_message(left) == n.LeftEvent("s1")


class TestFraming:
    def test_round_trip_including_heartbeat(self):
        a, b = socket.socketpair()
        with a, b:
            for payload in (b"", b"hello", bytes(1000)):
                n.write_frame(a, payload)
                assert n.read_frame(b) == payload

    def test_frames_split_across_reads_are_reassembled(self):
        a, b = socket.socketpair()
        with a, b:
            data = struct.pack(">I", 5) + b"hello"
            for byte in data:
                a.sendall(bytes([byte]))
            assert n.read_frame(b) == b"hello"

    def test_closed_connection_gives_none(self):
        a, b = socket.socketpair()
        a.close()
        with b:
            assert n.read_frame(b) is None

    def test_oversized_frame_is_refused_before_reading_it(self):
        a, b = socket.socketpair()
        with a, b:
            a.sendall(struct.pack(">I", n.max_frame() + 1))
            with pytest.raises(ValueError):
                n.read_frame(b)

    def test_limit_comes_from_the_environment(self, monkeypatch):
        monkeypatch.setenv("RAMSERVER_MAX_FRAME_BYTES", "100")
        assert n.max_frame() == 100
        monkeypatch.setenv("RAMSERVER_MAX_FRAME_BYTES", "junk")
        assert n.max_frame() == n.DEFAULT_MAX_FRAME


class RecordingClient(n.NetworkClient):
    """A client which records what it would send, instead of writing to a socket."""

    def __init__(self):
        super().__init__(debug=False)
        self.sent = []

    def _send(self, message):
        self._sequence += 1
        message.sequence = self._sequence
        self.sent.append(message)


class TestClient:
    def test_version_comparison(self):
        assert n.version_at_least("1.2", 1, 2) and n.version_at_least("1.10", 1, 2) and n.version_at_least("2.0", 1, 2)
        assert not n.version_at_least("1.1", 1, 2) and not n.version_at_least("", 1, 2) and not n.version_at_least("x.y", 1, 2)

    def test_connecting_signs_in_and_tracks_identity_and_session(self):
        c = RecordingClient()
        c.display_name = "Alice"
        c.events.put(n.Connected("1.2"))
        assert c.poll() == [n.Connected("1.2")]
        assert c.server_version == "1.2" and c.supports_lobby_features
        assert c.sent[-1].auth_request.display_name == "Alice"

        c.events.put(n.Authed("p1", "token"))
        c.poll()
        assert c.player_id == "p1" and c.reconnect_token == "token"

        c.events.put(n.StateEvent(n.Snapshot("s1", "chess", n.STATUS_WAITING)))
        c.poll()
        assert c.session_id == "s1"
        c.events.put(n.LeftEvent("s1"))
        c.poll()
        assert c.session_id == ""

    def test_a_closed_session_is_not_remembered(self):
        c = RecordingClient()
        c.events.put(n.StateEvent(n.Snapshot("s1", "chess", n.STATUS_CLOSED)))
        c.poll()
        assert c.session_id == ""

    def test_disconnect_forgets_identity(self):
        c = RecordingClient()
        c.player_id, c.session_id = "p1", "s1"
        c.events.put(n.Disconnected("gone"))
        assert c.poll() == [n.Disconnected("gone")]
        assert c.player_id == c.session_id == ""

    def test_messages_carry_the_session_and_rising_sequences(self):
        c = RecordingClient()
        c.session_id = "s1"
        c.server_version = "1.2"
        c.set_ready(True)
        c.request_legal_moves()
        c.send_action(b"e2e4", 7)
        c.join_session("s2")
        c.create_session(private=True)
        c.request_sessions()
        c.request_history()
        c.leave_session()
        kinds = [m.WhichOneof("payload") for m in c.sent]
        assert kinds == ["set_ready_request", "legal_actions_request", "action_request", "join_session_request",
                         "join_session_request", "list_sessions_request", "match_history_request", "leave_session_request"]
        assert [m.sequence for m in c.sent] == list(range(1, 9))
        action = c.sent[2].action_request
        assert (action.session_id, action.encoded_action, action.expected_state_version) == ("s1", b"e2e4", 7)
        assert c.sent[3].join_session_request.session_id == "s2"
        create = c.sent[4].join_session_request
        assert create.create_new and create.private_session and create.session_id == "" and create.game_id == "chess"

    def test_old_servers_are_not_sent_new_messages(self):
        c = RecordingClient()
        c.session_id, c.server_version = "s1", "1.1"
        c.request_sessions()
        c.request_history()
        assert c.sent == [] and not c.supports_lobby_features

    def test_leaving_without_a_session_sends_nothing(self):
        c = RecordingClient()
        c.leave_session()
        assert c.sent == []

    def test_stale_connection_events_are_dropped(self):
        # The generation check lives in the reader thread's put(): a closed client bumps it
        c = RecordingClient()
        generation = c._generation
        c.close()
        assert c._generation == generation + 1