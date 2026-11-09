"""Tests of how the client reaches a server: address parsing, and real connections over TCP and WebSocket to small
servers started in the test. The WebSocket tests need the websocket-client and websockets packages."""
import socket
import struct
import threading
import time

import pytest
import base64
import hashlib


import clients.pychess.networking as n
from clients.pychess.networking import pb


class TestAddresses:
    @pytest.mark.parametrize("text,kind,display", [
        ("127.0.0.1:8080", "tcp", "127.0.0.1:8080"),
        ("localhost", "tcp", "localhost:8080"),
        ("  example.org:1234  ", "tcp", "example.org:1234"),
        ("tcp://example.org:80", "tcp", "example.org:80"),
        ("[::1]:9100", "tcp", "::1:9100"),
        ("[::1]", "tcp", "::1:8080"),
        ("ws://localhost:8080/connect", "ws", "ws://localhost:8080/connect"),
        ("WSS://example.org/connect", "ws", "WSS://example.org/connect"),
        ("https://example.org/connect", "ws", "wss://example.org/connect"),
        ("http://example.org/connect", "ws", "ws://example.org/connect"),
        (n.HOSTED_SERVER, "ws", n.HOSTED_SERVER),
        (n.LOCAL_SERVER, "tcp", n.LOCAL_SERVER),
    ])
    def test_understood_forms(self, text, kind, display):
        address = n.parse_server_address(text)
        assert address.kind == kind and address.display == display

    def test_fields(self):
        tcp = n.parse_server_address("example.org:1234")
        assert (tcp.host, tcp.port) == ("example.org", 1234)
        assert n.parse_server_address("ws://example.org/x").url == "ws://example.org/x"

    @pytest.mark.parametrize("text", ["", "   ", ":8080", "example.org:0", "example.org:99999", "example.org:abc",
                                      "a:b:c", "::1", "ws://", "wss:///path", "[::1]8080", "exa/mple:90"])
    def test_rejected_forms_explain_themselves(self, text):
        with pytest.raises(ValueError) as error:
            n.parse_server_address(text)
        assert str(error.value)


# --- Small servers which speak the handshake ---



def reply(payload_name, in_reply_to, init):
    message = pb.ServerMessage()
    message.in_reply_to = in_reply_to
    getattr(message, payload_name).SetInParent()
    init(getattr(message, payload_name))
    return message.SerializeToString()


def frame(payload: bytes) -> bytes:
    return struct.pack(">I", len(payload)) + payload


class FrameBuffer:
    """Pulls complete frames out of whatever bytes have arrived."""

    def __init__(self):
        self.data = bytearray()

    def feed(self, data: bytes):
        self.data.extend(data)
        frames = []
        while len(self.data) >= 4:
            length = struct.unpack(">I", self.data[:4])[0]
            if len(self.data) < 4 + length:
                break
            frames.append(bytes(self.data[4:4 + length]))
            del self.data[:4 + length]
        return frames


class Script:
    """What a server does for each frame: answer the handshake, then send a heartbeat and count the reply."""

    def __init__(self):
        self.received = []
        self.heartbeat_replies = 0

    def respond(self, payload: bytes) -> list[bytes]:
        """Frames to send back, each already length-prefixed."""
        if payload == b"":
            self.heartbeat_replies += 1
            return []
        message = pb.ClientMessage()
        message.ParseFromString(payload)
        self.received.append(message)
        kind = message.WhichOneof("payload")
        if kind == "connect_request":
            return [frame(reply("connect_response", message.sequence, lambda m: setattr(m, "server_protocol_version", "1.2")))]
        if kind == "auth_request":
            def fill(m):
                m.player_id, m.reconnect_token = "p1", "secret"
            return [frame(reply("auth_response", message.sequence, fill)), frame(b"")]  # then a heartbeat
        return []


def collect(client, wanted, timeout=3.0):
    """Poll until the events include every type in wanted, or the time runs out."""
    events = []
    deadline = time.time() + timeout
    while time.time() < deadline:
        events.extend(client.poll())
        if all(any(isinstance(e, kind) for e in events) for kind in wanted):
            break
        time.sleep(0.01)
    return events


@pytest.fixture
def client():
    c = n.NetworkClient(debug=False)
    yield c
    c.close()


def tcp_server(script, *, close_after_auth=False):
    listener = socket.socket()
    listener.bind(("127.0.0.1", 0))
    listener.listen(1)

    def run():
        conn, _ = listener.accept()
        with conn, listener:
            buffer = FrameBuffer()
            while True:
                try:
                    data = conn.recv(4096)
                except OSError:
                    return
                if not data:
                    return
                for payload in buffer.feed(data):
                    for out in script.respond(payload):
                        conn.sendall(out)
                    if close_after_auth and script.received and script.received[-1].WhichOneof("payload") == "auth_request":
                        return

    threading.Thread(target=run, daemon=True).start()
    return listener.getsockname()[1]


class TestTcp:
    def test_handshake_heartbeat_and_actions(self, client):
        script = Script()
        port = tcp_server(script)
        client.connect(f"127.0.0.1:{port}", "Alice")
        events = collect(client, [n.Connected, n.Authed])
        assert n.Connected("1.2") in events and n.Authed("p1", "secret") in events
        assert client.player_id == "p1" and client.supports_lobby_features
        assert [m.WhichOneof("payload") for m in script.received] == ["connect_request", "auth_request"]
        assert script.received[0].connect_request.protocol_version == n.PROTOCOL_VERSION
        assert script.received[1].auth_request.display_name == "Alice"
        deadline = time.time() + 2
        while script.heartbeat_replies == 0 and time.time() < deadline:
            time.sleep(0.01)
        assert script.heartbeat_replies == 1  # the heartbeat was answered with a heartbeat

        client.send_action(b"e2e4", 3)
        deadline = time.time() + 2
        while len(script.received) < 3 and time.time() < deadline:
            time.sleep(0.01)
        assert script.received[2].action_request.encoded_action == b"e2e4"

    def test_refused_connection_reports_where_it_tried(self, client):
        listener = socket.socket()
        listener.bind(("127.0.0.1", 0))
        port = listener.getsockname()[1]
        listener.close()  # nothing is listening now
        client.connect(f"127.0.0.1:{port}", "Alice")
        events = collect(client, [n.Disconnected])
        assert len(events) == 1 and events[0].reason.startswith(f"Could not connect to 127.0.0.1:{port}")

    def test_server_closing_the_connection_is_reported(self, client):
        port = tcp_server(Script(), close_after_auth=True)
        client.connect(f"127.0.0.1:{port}", "Alice")
        events = collect(client, [n.Authed, n.Disconnected])
        assert any(isinstance(e, n.Disconnected) for e in events)
        assert client.player_id == "" and client.transport is None

    def test_reconnecting_ignores_the_old_connection(self, client):
        first = Script()
        client.connect(f"127.0.0.1:{tcp_server(first)}", "Alice")
        collect(client, [n.Authed])
        second = Script()
        client.connect(f"127.0.0.1:{tcp_server(second)}", "Bob")
        events = collect(client, [n.Authed])
        assert not any(isinstance(e, n.Disconnected) for e in events)  # closing the first is not a disconnect
        assert second.received[1].auth_request.display_name == "Bob"


websockets_sync = pytest.importorskip("websockets.sync.server", reason="pip install websockets")
pytest.importorskip("websocket", reason="pip install websocket-client")


def ws_server(script, *, split=False, path_check=None, close_after_auth=False):
    """A WebSocket server. With split set, each reply is sent in pieces of three bytes in separate messages, and the
    heartbeat is glued to the auth reply, so frames never line up with messages."""
    def handler(connection):
        if path_check:
            assert connection.request.path == path_check
        buffer = FrameBuffer()
        try:
            for message in connection:
                data = message if isinstance(message, bytes) else message.encode()
                for payload in buffer.feed(data):
                    outgoing = b"".join(script.respond(payload))
                    if split:
                        for i in range(0, len(outgoing), 3):
                            connection.send(outgoing[i:i + 3])
                    elif outgoing:
                        connection.send(outgoing)
                    if close_after_auth and script.received and script.received[-1].WhichOneof("payload") == "auth_request":
                        connection.close()
                        return
        except Exception:
            return

    server = websockets_sync.serve(handler, "127.0.0.1", 0)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    return server, server.socket.getsockname()[1]


class TestWebSocket:
    def test_handshake_and_heartbeat(self, client):
        script = Script()
        server, port = ws_server(script, path_check="/connect")
        try:
            client.connect(f"ws://127.0.0.1:{port}/connect", "Alice")
            events = collect(client, [n.Connected, n.Authed])
            assert n.Connected("1.2") in events and n.Authed("p1", "secret") in events
            deadline = time.time() + 2
            while script.heartbeat_replies == 0 and time.time() < deadline:
                time.sleep(0.01)
            assert script.heartbeat_replies == 1
            client.send_action(b"resign", 0)
            deadline = time.time() + 2
            while len(script.received) < 3 and time.time() < deadline:
                time.sleep(0.01)
            assert script.received[2].action_request.encoded_action == b"resign"
        finally:
            client.close()
            server.shutdown()

    def test_frames_which_do_not_line_up_with_messages(self, client):
        script = Script()
        server, port = ws_server(script, split=True)
        try:
            client.connect(f"ws://127.0.0.1:{port}/connect", "Alice")
            events = collect(client, [n.Connected, n.Authed])
            assert n.Connected("1.2") in events and n.Authed("p1", "secret") in events
        finally:
            client.close()
            server.shutdown()

    def test_server_closing_is_reported(self, client):
        server, port = ws_server(Script(), close_after_auth=True)
        try:
            client.connect(f"ws://127.0.0.1:{port}/connect", "Alice")
            events = collect(client, [n.Authed, n.Disconnected])
            assert any(isinstance(e, n.Disconnected) for e in events)
            assert client.player_id == ""
        finally:
            client.close()
            server.socket.close()  # server.shutdown() waits out a close timeout after the server itself closed a connection

    def test_refused_handshake_reports_the_reason(self, client):
        listener = socket.socket()
        listener.bind(("127.0.0.1", 0))
        listener.listen(1)

        def refuse():
            conn, _ = listener.accept()
            conn.recv(4096)
            conn.sendall(b"HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\n\r\n")
            conn.close()

        threading.Thread(target=refuse, daemon=True).start()
        client.connect(f"ws://127.0.0.1:{listener.getsockname()[1]}/connect", "Alice")
        events = collect(client, [n.Disconnected])
        listener.close()
        assert events and "404" in events[0].reason and "Could not connect" in events[0].reason

    def test_unreachable_websocket_host(self, client):
        listener = socket.socket()
        listener.bind(("127.0.0.1", 0))
        port = listener.getsockname()[1]
        listener.close()
        client.connect(f"ws://127.0.0.1:{port}/connect", "Alice")
        assert collect(client, [n.Disconnected])

    def test_text_messages_are_refused(self, client):
        def handler(connection):
            connection.send("hello")  # text, not binary
            time.sleep(0.5)

        server = websockets_sync.serve(handler, "127.0.0.1", 0)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        try:
            client.connect(f"ws://127.0.0.1:{server.socket.getsockname()[1]}/connect", "Alice")
            events = collect(client, [n.Disconnected])
            assert any(isinstance(e, n.Disconnected) and "binary" in e.reason for e in events)
        finally:
            client.close()
            server.shutdown()


class TestWebSocketTransportAlone:
    class FakeWs:
        def __init__(self, messages):
            self.messages = list(messages)
            self.sent = []

        def recv(self):
            item = self.messages.pop(0)
            if isinstance(item, Exception):
                raise item
            return item

        def send(self, data, opcode=None):
            self.sent.append((data, opcode))

        def close(self):
            self.closed = True

    def transport(self, messages):
        import websocket
        return n.WebSocketTransport(self.FakeWs(messages), websocket)

    def test_reads_are_served_from_the_buffer(self):
        t = self.transport([b"abcdef", b"gh"])
        assert t.recv(4) == b"abcd" and t.recv(10) == b"ef" and t.recv(1) == b"g"

    def test_empty_binary_messages_are_skipped(self):
        assert self.transport([b"", b"", b"x"]).recv(5) == b"x"

    def test_close_frame_and_closed_exception_read_as_end_of_stream(self):
        import websocket
        assert self.transport([""]).recv(4) == b""
        assert self.transport([websocket.WebSocketConnectionClosedException("gone")]).recv(4) == b""

    def test_other_websocket_errors_are_connection_errors(self):
        import websocket
        with pytest.raises(ConnectionError):
            self.transport([websocket.WebSocketProtocolException("bad frame")]).recv(4)

    def test_sends_one_binary_message_per_frame(self):
        import websocket
        t = self.transport([])
        n.write_frame(t, b"hi")
        assert t.ws.sent == [(b"\x00\x00\x00\x02hi", websocket.ABNF.OPCODE_BINARY)]


def silent_websocket_server():
    """Completes the WebSocket handshake and sends one ConnectResponse, then stops reading and never answers a close frame,
    like a server instance which has stalled."""
    listener = socket.socket()
    listener.bind(("127.0.0.1", 0))
    listener.listen(1)

    def run():
        conn, _ = listener.accept()
        request = b""
        while b"\r\n\r\n" not in request:
            request += conn.recv(4096)
        key = next(line.split(b": ")[1] for line in request.split(b"\r\n") if line.lower().startswith(b"sec-websocket-key"))
        accept = base64.b64encode(hashlib.sha1(key + b"258EAFA5-E914-47DA-95CA-C5AB0DC85B11").digest())
        conn.sendall(b"HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " + accept + b"\r\n\r\n")
        body = frame(reply("connect_response", 1, lambda m: setattr(m, "server_protocol_version", "1.2")))
        conn.sendall(bytes([0x82, len(body)]) + body)  # one unmasked binary WebSocket message
        time.sleep(30)

    threading.Thread(target=run, daemon=True).start()
    return listener.getsockname()[1]


def connect_and_wait(client, address):
    client.connect(address, "Alice")
    deadline = time.time() + 3
    while not client.server_version and time.time() < deadline:
        client.poll()
        time.sleep(0.01)
    assert client.server_version == "1.2", "did not connect"


class TestNeverBlocks:
    def test_closing_a_stalled_websocket_returns_at_once(self, client):
        connect_and_wait(client, f"ws://127.0.0.1:{silent_websocket_server()}/connect")
        client.session_id = "s1"  # in a lobby, so close() also queues a leave message
        started = time.time()
        client.close()
        assert time.time() - started < 0.5

    def test_leaving_the_app_waits_at_most_what_it_is_told(self, client):
        connect_and_wait(client, f"ws://127.0.0.1:{silent_websocket_server()}/connect")
        started = time.time()
        client.close(wait=0.3)
        assert time.time() - started < 1.0

    def test_the_leave_message_is_sent_before_the_connection_closes(self, client):
        script = Script()
        client.connect(f"127.0.0.1:{tcp_server(script)}", "Alice")
        collect(client, [n.Authed])
        client.session_id = "s1"
        client.close(wait=2.0)
        kinds = [m.WhichOneof("payload") for m in script.received]
        assert kinds[-1] == "leave_session_request" and script.received[-1].leave_session_request.session_id == "s1"

    def test_sending_does_not_wait_for_a_server_which_stopped_reading(self, client):
        listener = socket.socket()
        listener.bind(("127.0.0.1", 0))
        listener.listen(1)
        held = []
        threading.Thread(target=lambda: held.append(listener.accept()[0]) or time.sleep(30), daemon=True).start()
        client.connect(f"127.0.0.1:{listener.getsockname()[1]}", "Alice")
        deadline = time.time() + 2
        while client._outbox is None and time.time() < deadline:
            time.sleep(0.01)
        client.session_id = "s1"
        started = time.time()
        for _ in range(400):  # far more than the sockets can buffer
            client.send_action(bytes(60000), 0)
        assert time.time() - started < 1.0
        started = time.time()
        client.close()
        assert time.time() - started < 0.5
        writer = client._writer
        assert writer is None  # forgotten by the client; the stuck thread is released by the timer below
        time.sleep(n.CLOSE_GRACE + 0.5)
        assert held and held[0].fileno() != -1  # the server side is untouched; only the client dropped its end

    def test_a_write_failure_is_reported_once(self, client):
        script = Script()
        port = tcp_server(script, close_after_auth=True)
        client.connect(f"127.0.0.1:{port}", "Alice")
        events = collect(client, [n.Authed, n.Disconnected])
        time.sleep(0.2)
        client.send_action(b"e2e4", 0)
        events += client.poll()
        assert sum(isinstance(e, n.Disconnected) for e in events) == 1

    def test_debug_output_shows_traffic_with_times(self, capsys):
        script = Script()
        c = n.NetworkClient(debug=True)
        try:
            c.connect(f"127.0.0.1:{tcp_server(script)}", "Alice")
            collect(c, [n.Authed])
            time.sleep(0.1)
        finally:
            c.close()
        out = capsys.readouterr().out
        assert "-> sequence: 1 connect_request" in out and "<- in_reply_to: 1 connect_response" in out
        assert "<- heartbeat" in out and "connecting to 127.0.0.1" in out
        assert out.splitlines()[0].startswith("[") and ":" in out.splitlines()[0][:10]

    def test_nothing_is_printed_without_debug(self, client, capsys):
        client.connect(f"127.0.0.1:{tcp_server(Script())}", "Alice")
        collect(client, [n.Authed])
        assert capsys.readouterr().out == ""
