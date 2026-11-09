import os
import queue
import socket
import struct
import threading
import time
from typing import Any
from urllib.parse import urlsplit
from google.protobuf import symbol_database as _symbol_database
from google.protobuf import text_format
from google.protobuf.json_format import MessageToJson
from dataclasses import dataclass, field

from .gen.ramserver.v1 import ramserver_pb2 as _pb

pb: Any = _pb

PROTOCOL_VERSION = "1.2"
GAME_ID = "chess"
DEFAULT_MAX_FRAME = 64 << 10
CONNECT_TIMEOUT = 5.0
CLOSE_GRACE = 1.0
DEFAULT_PORT = 8080
LOCAL_SERVER = "127.0.0.1:8080"
HOSTED_SERVER = "wss://ramserver-core-80752473398.us-east5.run.app/connect"

# SessionStatus values
STATUS_WAITING = 1
STATUS_READY = 2
STATUS_STARTING = 3
STATUS_IN_PROGRESS = 4
STATUS_CLOSED = 5
STATUS_NAMES = {
    STATUS_WAITING: "waiting", STATUS_READY: "ready", STATUS_STARTING: "starting",
    STATUS_IN_PROGRESS: "in progress", STATUS_CLOSED: "closed"
}


def max_frame() -> int:
    """Largest payload accepted, from RAMSERVER_MAX_FRAME_BYTES as in docs/framing.md."""
    try:
        return int(os.environ.get("RAMSERVER_MAX_FRAME_BYTES") or DEFAULT_MAX_FRAME)
    except ValueError:
        return DEFAULT_MAX_FRAME


ClientMessage: Any = getattr(_pb, "ClientMessage")
ServerMessage: Any = getattr(_pb, "ServerMessage")


@dataclass(frozen=True)
class Player:
    player_id: str
    display_name: str = ""
    connected: bool = True
    ready: bool = False
    role: str = ""  # empty when the server is older than protocol 1.2


@dataclass(frozen=True)
class Snapshot:
    session_id: str
    game_id: str
    status: int
    players: tuple[Player, ...] = ()
    owner_id: str = ""
    version: int = 0
    view: bytes = b""
    active_player_id: str = ""
    player_order: tuple[str, ...] = ()
    annotations: dict[str, str] = field(default_factory=dict)
    in_reply_to: int = 0

    @property
    def in_progress(self) -> bool:
        return self.status == STATUS_IN_PROGRESS
 
    def player(self, player_id: str) -> Player | None:
        return next((p for p in self.players if p.player_id == player_id), None)


@dataclass(frozen=True)
class SessionInfo:
    session_id: str
    game_id: str
    status: int
    players: tuple[Player, ...]
    owner_id: str
    joinable: bool


@dataclass(frozen=True)
class MatchSummary:
    winners: tuple[str, ...]
    draw: bool
    reason: str  # the game's termination reason, such as "CHECKMATE"
    turns: int


@dataclass(frozen=True)
class Connected:
    server_version: str
 
@dataclass(frozen=True)
class Authed:
    player_id: str
    reconnect_token: str
 
@dataclass(frozen=True)
class StateEvent:
    snapshot: Snapshot
 
@dataclass(frozen=True)
class LegalMovesEvent:
    payloads: tuple[str, ...]
 
@dataclass(frozen=True)
class MatchEnded:
    summary: MatchSummary
 
@dataclass(frozen=True)
class SessionsEvent:
    sessions: tuple[SessionInfo, ...]
 
@dataclass(frozen=True)
class HistoryEvent:
    session_id: str
    matches: tuple[MatchSummary, ...]
 
@dataclass(frozen=True)
class LeftEvent:
    session_id: str
 
@dataclass(frozen=True)
class ErrorEvent:
    code: str  # the ErrorCode name, such as "ERROR_CODE_ILLEGAL_ACTION"
    message: str
    in_reply_to: int = 0
    session_id: str = ""
 
@dataclass(frozen=True)
class Disconnected:
    reason: str

Event = Connected | Authed | StateEvent | LegalMovesEvent | MatchEnded | SessionsEvent | HistoryEvent | LeftEvent | ErrorEvent | Disconnected
 
ERROR_MESSAGES = {
    "ERROR_CODE_INVALID_PROTOCOL_VERSION": "This client's protocol version is not supported by the server",
    "ERROR_CODE_AUTH_FAILED": "Authentication failed",
    "ERROR_CODE_UNKNOWN_GAME": "The server does not offer this game",
    "ERROR_CODE_SESSION_NOT_FOUND": "That lobby does not exist",
    "ERROR_CODE_SESSION_NOT_JOINABLE": "That lobby cannot be joined: it is full or its match has started",
    "ERROR_CODE_OUT_OF_TURN": "It is not your turn",
    "ERROR_CODE_ILLEGAL_ACTION": "Illegal move",
    "ERROR_CODE_MALFORMED_MESSAGE": "The server could not read that message",
    "ERROR_CODE_INVALID_STATE": "That is not allowed right now",
    "ERROR_CODE_NOT_IN_SESSION": "You are not in that lobby",
    "ERROR_CODE_ALREADY_IN_SESSION": "You are already in a lobby",
    "ERROR_CODE_INVALID_ARGUMENT": "The server rejected a value",
    "ERROR_CODE_STALE_STATE": "The board changed before your move arrived",
    "ERROR_CODE_INTERNAL": "The server had an internal error",
}
 
 
def readable_error(event: ErrorEvent) -> str:
    """Text for a player. Unknown codes show the server message with the code."""
    known = ERROR_MESSAGES.get(event.code)
    if known:
        return known
    return f"{event.code}: {event.message}" if event.message else event.code


def _players(rows: Any) -> tuple[Player, ...]:
    return tuple(Player(r.player_id, r.display_name, r.connected, r.ready, r.role) for r in rows)
 
 
def _summary(m: Any) -> MatchSummary:
    return MatchSummary(tuple(m.winner_player_ids), m.draw, m.termination_reason, m.turns)


def parse_server_message(msg: Any) -> Event | None:
    """Turn a ServerMessage into an event. Handles heartbeats and unknown messages."""
    kind = msg.WhichOneof("payload")
    if kind == "connect_response":
        return Connected(msg.connect_response.server_protocol_version)
    if kind == "auth_response":
        return Authed(msg.auth_response.player_id, msg.auth_response.reconnect_token)
    if kind == "state_update":
        s = msg.state_update
        return StateEvent(Snapshot(
            s.session_id, s.game_id, int(s.status), _players(s.players), s.owner_player_id, s.state_version,
            bytes(s.encoded_view), s.active_player_id, tuple(s.player_order), dict(s.annotations), msg.in_reply_to,
        ))
    if kind == "legal_actions_response":
        return LegalMovesEvent(tuple(bytes(a.payload).decode("ascii", "replace") for a in msg.legal_actions_response.legal_actions))
    if kind == "match_result":
        return MatchEnded(_summary(msg.match_result))
    if kind == "list_sessions_response":
        return SessionsEvent(tuple(
            SessionInfo(i.session_id, i.game_id, int(i.status), _players(i.players), i.owner_player_id, i.joinable)
            for i in msg.list_sessions_response.sessions
        ))
    if kind == "match_history_response":
        h = msg.match_history_response
        return HistoryEvent(h.session_id, tuple(_summary(m) for m in h.matches))
    if kind == "leave_session_response":
        return LeftEvent(msg.leave_session_response.session_id)
    if kind == "error_response":
        e = msg.error_response
        return ErrorEvent(pb.ErrorCode.Name(e.code), e.message, msg.in_reply_to, e.session_id)
    return None


@dataclass(frozen=True)
class ServerAddress:
    """TCP host and port, or a WebSocket URL."""
 
    kind: str  # "tcp" or "ws"
    host: str = ""
    port: int = 0
    url: str = ""
 
    @property
    def display(self) -> str:
        return self.url if self.kind == "ws" else f"{self.host}:{self.port}"


def parse_server_address(text: str) -> ServerAddress:
    """Accepts `host`, `host:port`, `[::1]:port`, `tcp://host:port`, and
    `ws://` or `wss://` URLs (`http://` and `https://` are taken as their WebSocket forms). Raises ValueError."""
    text = text.strip()
    if not text:
        raise ValueError("Enter the server's address")
    lowered = text.lower()
    if lowered.startswith(("http://", "https://")):
        text = "ws" + text[4:]
        lowered = text.lower()
    if lowered.startswith(("ws://", "wss://")):
        if not urlsplit(text).hostname:
            raise ValueError("The WebSocket address has no host name")
        return ServerAddress("ws", url=text)
    if lowered.startswith("tcp://"):
        text = text[len("tcp://"):]
    port_text = ""
    if text.startswith("["):  # an IPv6 address: [::1]:8080
        host, _, rest = text[1:].partition("]")
        if rest and not rest.startswith(":"):
            raise ValueError("Write the address as host:port")
        port_text = rest[1:]
    elif text.count(":") == 1:
        host, port_text = text.split(":")
    elif ":" in text:
        raise ValueError("Put an IPv6 address in brackets, like [::1]:8080")
    else:
        host = text
    if not host or "/" in host:
        raise ValueError("Write the address as host:port, or as a ws:// or wss:// URL")
    if port_text:
        if not port_text.isdigit() or not 1 <= int(port_text) <= 65535:
            raise ValueError("The port must be a number from 1 to 65535")
        return ServerAddress("tcp", host=host, port=int(port_text))
    return ServerAddress("tcp", host=host, port=DEFAULT_PORT)



class TcpTransport:
    def __init__(self, sock: socket.socket) -> None:
        self.sock = sock
 
    def sendall(self, data: bytes) -> None:
        self.sock.sendall(data)
 
    def recv(self, n: int) -> bytes:
        return self.sock.recv(n)
 
    def close(self) -> None:
        try:
            self.sock.shutdown(socket.SHUT_RDWR)  # wakes a read which is blocked in another thread
        except OSError:
            pass
        self.sock.close()
 
 
class WebSocketTransport:
    """Frames inside binary WebSocket messages. One message can hold part of a frame, one frame, or several, so
    reads are served from a buffer and the frame reader sees the same byte stream as with TCP."""
 
    def __init__(self, ws: Any, websocket_module: Any) -> None:
        self.ws = ws
        self.websocket = websocket_module
        self.buffer = bytearray()
 
    def sendall(self, data: bytes) -> None:
        try:
            self.ws.send(data, opcode=self.websocket.ABNF.OPCODE_BINARY)
        except self.websocket.WebSocketException as error:
            raise ConnectionError(str(error) or "WebSocket send failed") from error
 
    def recv(self, n: int) -> bytes:
        """Up to n bytes, or b"" once the connection has closed."""
        while not self.buffer:
            try:
                message = self.ws.recv()
            except self.websocket.WebSocketConnectionClosedException:
                return b""
            except self.websocket.WebSocketException as error:
                raise ConnectionError(str(error) or "WebSocket error") from error
            if isinstance(message, str):
                if message == "":  # websocket-client reports a close frame as an empty string
                    return b""
                raise ConnectionError("The server sent text over the WebSocket, but this protocol is binary")
            self.buffer.extend(message or b"")
        result = bytes(self.buffer[:n])
        del self.buffer[:n]
        return result
 
    def close(self) -> None:
        try:
            self.ws.close(timeout=1)
        except Exception:  # closing a connection which already failed is not an error worth reporting
            pass
 
 
def open_transport(address: ServerAddress) -> TcpTransport | WebSocketTransport:
    """Connect to a server. Raises OSError, or RuntimeError when a WebSocket is wanted and the library is missing."""
    if address.kind == "ws":
        try:
            import websocket
        except ImportError as error:
            raise RuntimeError("WebSocket servers need the websocket-client package: pip install websocket-client") from error
        ws = websocket.create_connection(address.url, timeout=CONNECT_TIMEOUT, enable_multithread=True)
        ws.settimeout(None)  # the timeout was only for connecting: a quiet connection is normal
        return WebSocketTransport(ws, websocket)
    sock = socket.create_connection((address.host, address.port), timeout=CONNECT_TIMEOUT)
    sock.settimeout(None)
    return TcpTransport(sock)
 
 
def describe_error(error: BaseException) -> str:
    """Short text for a connection failure."""
    return getattr(error, "strerror", None) or str(error) or type(error).__name__


def recv_all(transport: Any, n: int) -> bytes | None:
    """Read exactly n bytes, or None if the connection closed first."""
    data = bytearray()
    while len(data) < n:
        packet = transport.recv(n - len(data))
        if not packet:
            return None
        data.extend(packet)
    return bytes(data)
 
 
def read_frame(transport: Any) -> bytes | None:
    """Read one frame: a 4-byte big-endian length, then that many bytes. A zero-length frame is a heartbeat.
    Returns None when the connection closed, and raises ValueError for a frame over the size limit."""
    header = recv_all(transport, 4)
    if header is None:
        return None
    length = struct.unpack(">I", header)[0]
    if length > max_frame():
        raise ValueError(f"frame of {length} bytes is over the limit of {max_frame()}")
    return recv_all(transport, length)
 
 
def write_frame(transport: Any, payload: bytes) -> None:
    """Send a frame in a single call, so a WebSocket carries it as one message."""
    transport.sendall(struct.pack(">I", len(payload)) + payload)


def version_at_least(version: str, major: int, minor: int) -> bool:
    try:
        have_major, have_minor = (int(part) for part in version.split(".")[:2])
    except ValueError:
        return False
    return (have_major, have_minor) >= (major, minor)


class NetworkClient:
    def __init__(self, debug: bool | None = None):
        self.debug = bool(os.environ.get("RAMCHESS_DEBUG")) if debug is None else debug
        self.events: queue.Queue[Event] = queue.Queue()
        self.transport: TcpTransport | WebSocketTransport | None = None
        self.server_version = ""
        self.player_id = ""
        self.reconnect_token = ""
        self.session_id = ""
        self.display_name = ""
        self._sequence = 0
        self._write_lock = threading.Lock()
        self._outbox: queue.Queue[bytes | None] | None = None
        self._writer: threading.Thread | None = None
        self._generation = 0  # bumped by each connect and close, so events from an old connection are dropped
 
    # Capabilities, from the server's protocol version
    @property
    def supports_lobby_features(self) -> bool:
        """Session listing, match history, explicit creation, roles and ready flags, annotations, and resigning."""
        return version_at_least(self.server_version, 1, 2)


    def connect(self, address: str | ServerAddress, display_name: str) -> None:
        """Start connecting in the background, over TCP or a WebSocket depending on the address. The outcome arrives
        as Connected, then Authed, or as Disconnected. Raises ValueError for an address which cannot be understood."""
        target = address if isinstance(address, ServerAddress) else parse_server_address(address)
        self.close()
        self.display_name = display_name
        self._sequence = 0
        self._generation += 1
        threading.Thread(target=self._run, args=(self._generation, target), daemon=True).start()

    def _log(self, text: str) -> None:
        if self.debug:
            print(f"[{time.strftime('%H:%M:%S')}] {text}", flush=True)
 
    def _run(self, generation: int, address: ServerAddress) -> None:
        def put(event: Event) -> None:
            if generation == self._generation:
                self.events.put(event)

        try:
            transport = open_transport(address)
        except Exception as error:  # OSError, a refused WebSocket handshake, or a missing library
            self._log(f"connect failed: {describe_error(error)}")
            put(Disconnected(f"Could not connect to {address.display}: {describe_error(error)}"))
            return
        if generation != self._generation:
            transport.close()
            return

        outbox: queue.Queue[bytes | None] = queue.Queue()
        reported = threading.Event()

        def report(reason: str) -> None:
            if not reported.is_set():
                reported.set()
                self._log(f"disconnected: {reason}")
                put(Disconnected(reason))

        writer = threading.Thread(target=self._write_loop, args=(transport, outbox, reported), daemon=True)
        self._outbox, self._writer, self.transport = outbox, writer, transport
        writer.start()
        self._log("connected; sending ConnectRequest")
        message = pb.ClientMessage()
        message.connect_request.protocol_version = PROTOCOL_VERSION
        message.connect_request.client_name = "python-chess"
        self._send(message)
        try:
            while True:
                frame = read_frame(transport)
                if frame is None:
                    report("The server closed the connection")
                    return
                if len(frame) == 0:
                    self._log("<- heartbeat")
                    outbox.put(b"")
                    continue
                server_message = pb.ServerMessage()
                try:
                    server_message.ParseFromString(frame)
                except Exception:  # an undecodable frame must not end the connection
                    self._log(f"<- undecodeable frame of {len(frame)} bytes")
                    continue
                if self.debug:
                    self._log(f"<- {text_format.MessageToString(server_message, as_one_line=True)}")
                event = parse_server_message(server_message)
                if event is not None:
                    put(event)
        except (OSError, ValueError) as error:
            report(f"Connection lost: {error}")
        finally:
            outbox.put(None)
            transport.close()  # finish the closing handshake, so the server is not left waiting for it


    def _write_loop(self, transport: TcpTransport | WebSocketTransport, outbox: queue.Queue, report: Any) -> None:
        try:
            while True:
                payload = outbox.get()
                if payload is None: return
                write_frame(transport, payload)
        except OSError as error:
            report(f"Connection lost: {error}")
        finally: transport.close()
 
    def poll(self) -> list[Event]:
        """Drain the queue. Call from the main thread once per frame."""
        events: list[Event] = []
        while True:
            try:
                event = self.events.get_nowait()
            except queue.Empty:
                return events
            if isinstance(event, Connected):
                self.server_version = event.server_version
                self.authenticate()
            elif isinstance(event, Authed):
                self.player_id, self.reconnect_token = event.player_id, event.reconnect_token
            elif isinstance(event, StateEvent) and event.snapshot.session_id and event.snapshot.status != STATUS_CLOSED:
                self.session_id = event.snapshot.session_id
            elif isinstance(event, LeftEvent) and event.session_id == self.session_id:
                self.session_id = ""
            elif isinstance(event, Disconnected):
                self.transport = self._outbox = None
                self.session_id = self.player_id = ""
            events.append(event)


    def _send(self, message: Any) -> None:
        outbox = self._outbox
        if outbox is None:
            return
        with self._write_lock:
            self._sequence += 1
            message.sequence = self._sequence
            if self.debug:
                self._log(f"-> {text_format.MessageToString(message, as_one_line=True)}")
            outbox.put(message.SerializeToString())

    def authenticate(self) -> None:
        message = pb.ClientMessage()
        message.auth_request.display_name = self.display_name
        self._send(message)
 
    def join_session(self, session_id: str = "", game_id: str = GAME_ID) -> None:
        """Join a specific session, or with no session_id join an open one and create one if none is open."""
        message = pb.ClientMessage()
        message.join_session_request.game_id = game_id
        message.join_session_request.session_id = session_id
        self._send(message)
 
    def create_session(self, private: bool = False, game_id: str = GAME_ID) -> None:
        """Always make a new session. Servers older than 1.2 ignore the flags and join an open session instead."""
        message = pb.ClientMessage()
        message.join_session_request.game_id = game_id
        message.join_session_request.create_new = True
        message.join_session_request.private_session = private
        self._send(message)
 
    def leave_session(self) -> None:
        if not self.session_id:
            return
        message = pb.ClientMessage()
        message.leave_session_request.session_id = self.session_id
        self._send(message)
 
    def set_ready(self, ready: bool = True) -> None:
        message = pb.ClientMessage()
        message.set_ready_request.session_id = self.session_id
        message.set_ready_request.ready = ready
        self._send(message)
 
    def request_legal_moves(self) -> None:
        message = pb.ClientMessage()
        message.legal_actions_request.session_id = self.session_id
        self._send(message)
 
    def request_sessions(self, game_id: str = GAME_ID) -> None:
        if not self.supports_lobby_features:
            return
        message = pb.ClientMessage()
        message.list_sessions_request.game_id = game_id
        self._send(message)
 
    def request_history(self) -> None:
        if not self.supports_lobby_features or not self.session_id:
            return
        message = pb.ClientMessage()
        message.match_history_request.session_id = self.session_id
        self._send(message)
 
    def send_action(self, payload: bytes, expected_state_version: int = 0) -> None:
        """Send a game action. expected_state_version 0 skips the server's staleness check."""
        message = pb.ClientMessage()
        message.action_request.session_id = self.session_id
        message.action_request.encoded_action = payload
        message.action_request.expected_state_version = expected_state_version
        self._send(message)
 
    def close(self, wait: float = 0.0) -> None:
        """Leave the session and drop the connection. Events from the old connection are discarded."""
        self.leave_session()
        outbox, writer, transport = self._outbox, self._writer, self.transport
        self._generation += 1
        self._outbox = self._writer = self.transport = None
        self.session_id = self.player_id = ""
        if outbox is not None: outbox.put(None)
        if transport is not None:
            transport.close()
            # Close the connection even if the writer is stuck
            timer = threading.Timer(CLOSE_GRACE, transport.close)
            timer.daemon = True
            timer.start()
        if wait > 0 and writer is not None:
            writer.join(wait)
