import socket
import struct
import threading
from typing import Any
from google.protobuf import symbol_database as _symbol_database
from google.protobuf.json_format import MessageToJson
from dataclasses import dataclass

import gen.ramserver.v1.ramserver_pb2

from piece import PlayerColor
from move import Move

HOST = "127.0.0.1"
PORT = 9000
SILENT = False

ClientMessage: Any = getattr(gen.ramserver.v1.ramserver_pb2, "ClientMessage")
ServerMessage: Any = getattr(gen.ramserver.v1.ramserver_pb2, "ServerMessage")

sequence: int = 1
player_id: str = ""
session_id: str = ""
own_color: PlayerColor | None = None  # known once the match starts

def connect() -> socket.socket:
    global sequence
    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    try:
        sock.connect((HOST, PORT))

        # Start a persistent background thread to listen and print server responses
        listener_thread = threading.Thread(target=listen_and_print_server_messages, args=(sock,), daemon=True)
        listener_thread.start()

        client_msg = ClientMessage()
        client_msg.connect_request.protocol_version = "1.1"
        client_msg.connect_request.client_name = "python-chess"
        send_msg(sock, client_msg)

        return sock
    except ConnectionRefusedError as e:
        raise ConnectionRefusedError(f"Connection refused. Is the server listening on port {PORT}?") from e

def authenticate(sock: socket.socket):
    client_msg = ClientMessage()
    client_msg.auth_request.display_name = "Python-Chess"
    send_msg(sock, client_msg)

def join_chess(sock: socket.socket, session_id: str | None = None):
    client_msg = ClientMessage()
    client_msg.join_session_request.game_id = "chess"
    if session_id is not None:
        client_msg.join_session_request.session_id = session_id
    send_msg(sock, client_msg)

def ready(sock: socket.socket):
    client_msg = ClientMessage()
    client_msg.set_ready_request.session_id = session_id
    client_msg.set_ready_request.ready = True
    send_msg(sock, client_msg)

def request_legal_moves(sock: socket.socket):
    client_msg = ClientMessage()
    client_msg.legal_actions_request.session_id = session_id
    send_msg(sock, client_msg)

def ping(sock: socket.socket):
    client_msg = ClientMessage()
    readable_json = MessageToJson(client_msg, preserving_proto_field_name=True)
    if not SILENT: print(f"-> {readable_json}")
    msg_bytes = client_msg.SerializeToString()
    packed_len = struct.pack('>I', len(msg_bytes))
    sock.sendall(packed_len + msg_bytes)

def send_action(sock: socket.socket, move: Move):
    global session_id
    client_msg = ClientMessage()
    client_msg.action_request.session_id = session_id
    client_msg.action_request.encoded_action = move.encode()
    client_msg.action_request.expected_state_version = 1 # TODO
    send_msg(sock, client_msg)

@dataclass
class State:
    status: str
    version: int
    view: bytearray
    my_turn: bool

recent_state: State | None = None
prev_state: State | None = None
last_good_state: State | None = None
legal_moves: list[Move] = []  # your moves for the current turn, from the latest LegalActionsResponse

def get_recent_state():
    global recent_state, prev_state
    res = recent_state
    if res is not None:
        prev_state = recent_state
        recent_state = None
    return res

def listen_and_print_server_messages(sock: socket.socket):
    """Continuously intercepts incoming stream packets, unpacks them, and prints them out."""
    global player_id, session_id, recent_state, prev_state, last_good_state, own_color, legal_moves
    try:
        while True:
            msg_bytes = recv_msg(sock)
            if msg_bytes is None:
                if not SILENT: print("\n[Server Info] Connection closed by the remote host.")
                break

            server_msg = ServerMessage()
            server_msg.ParseFromString(msg_bytes)

            if server_msg.auth_response is not None and server_msg.auth_response.player_id != "":
                player_id = server_msg.auth_response.player_id
                print(player_id)

            # A message field is never None in Python protobuf, so check which payload was actually sent
            if server_msg.HasField("state_update"):
                session_id = server_msg.state_update.session_id or session_id
                state = State(
                    server_msg.state_update.status,
                    server_msg.state_update.state_version,
                    server_msg.state_update.encoded_view,
                    server_msg.state_update.active_player_id == player_id,
                )
                print(player_id)
                print(server_msg.state_update.player_order)
                player_order = server_msg.state_update.player_order
                if len(player_order) >= 2 and player_id in player_order:
                    own_color = (
                        PlayerColor.WHITE
                        if player_order[0] == player_id
                        else PlayerColor.BLACK
                    )
                    print(f"My player ID: {player_id}")
                    print(f"My color: {own_color}")
                if state.view != b'' and state != last_good_state:
                    last_good_state = state
                    recent_state = state
                    legal_moves = []  # for the previous position; main requests new ones on your turn

            if server_msg.HasField("legal_actions_response"):
                payloads = (bytes(spec.payload).decode("ascii", "replace") for spec in server_msg.legal_actions_response.legal_actions)
                legal_moves = [move for payload in payloads if (move := Move.from_uci(payload)) is not None]

            if server_msg.HasField("error_response"):
                recent_state = last_good_state

            # Heartbeat
            if server_msg == ServerMessage():
                ping(sock)

            readable_json = MessageToJson(server_msg, preserving_proto_field_name=True)

            if not SILENT: print(f"<- {readable_json}")

    except Exception as e:
        print(f"\n[Network Error] Error in listener loop: {e}")


def close(sock: socket.socket) -> None:
    client_msg = ClientMessage()
    client_msg.leave_session_request.session_id = session_id
    send_msg(sock, client_msg)
    sock.close()


def send_msg(sock: socket.socket, client_msg: Any):
    global sequence
    client_msg.sequence = sequence
    sequence += 1

    readable_json = MessageToJson(client_msg, preserving_proto_field_name=True)
    if not SILENT: print(f"-> {readable_json}")

    msg_bytes = client_msg.SerializeToString()
    packed_len = struct.pack('>I', len(msg_bytes))
    sock.sendall(packed_len + msg_bytes)


def recv_msg(sock: socket.socket) -> bytes | None:
    """Reads a 4-byte length prefix and then reads that exact amount of data."""
    raw_msglen = recv_all(sock, 4)
    if not raw_msglen:
        return None
    msglen = struct.unpack('>I', raw_msglen)[0]
    return recv_all(sock, msglen)


def recv_all(sock: socket.socket, n: int) -> bytes | None:
    """Helper function to loop until 'n' bytes are fully read."""
    data = bytearray()
    while len(data) < n:
        packet = sock.recv(n - len(data))
        if not packet:
            return None
        data.extend(packet)
    return bytes(data)
