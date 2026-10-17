import socket
import struct
import threading
from typing import Any
from google.protobuf import symbol_database as _symbol_database
from google.protobuf.json_format import MessageToJson

import gen.ramserver.v1.ramserver_pb2  

HOST = "127.0.0.1"
PORT = 9000

ClientMessage: Any = getattr(gen.ramserver.v1.ramserver_pb2, "ClientMessage")
ServerMessage: Any = getattr(gen.ramserver.v1.ramserver_pb2, "ServerMessage")

sequence: int = 1
player_id: str = ""
session_id: str = ""

def connect() -> socket.socket | None:
    global sequence
    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    try:
        sock.connect((HOST, PORT))

        # Start a persistent background thread to listen and print server responses
        listener_thread = threading.Thread(target=listen_and_print_server_messages, args=(sock,), daemon=True)
        listener_thread.start()
        
        client_msg = ClientMessage()
        client_msg.connect_request.protocol_version = "1.0"
        client_msg.connect_request.client_name = "python-chess"
        send_msg(sock, client_msg)
        
        return sock
    except ConnectionRefusedError:
        print(f"Connection refused. Is the server listening on port {PORT}?")
        return None

def authenticate(sock: socket.socket):
    client_msg = ClientMessage()
    client_msg.auth_request.display_name = "Python-Chess"    
    send_msg(sock, client_msg)


def ping(sock: socket.socket):
    client_msg = ClientMessage()
    readable_json = MessageToJson(client_msg, preserving_proto_field_name=True)
    print(f"-> {readable_json}")
    msg_bytes = client_msg.SerializeToString()
    packed_len = struct.pack('>I', len(msg_bytes))
    sock.sendall(packed_len + msg_bytes)

def listen_and_print_server_messages(sock: socket.socket):
    """Continuously intercepts incoming stream packets, unpacks them, and prints them out."""
    global player_id
    try:
        while True:
            msg_bytes = recv_msg(sock)
            if msg_bytes is None:
                print("\n[Server Info] Connection closed by the remote host.")
                break

            server_msg = ServerMessage()
            server_msg.ParseFromString(msg_bytes)

            # Heartbeat
            if server_msg == ServerMessage():
                ping(sock)
                print(player_id)

            readable_json = MessageToJson(server_msg, preserving_proto_field_name=True)
            
            print(f"<- {readable_json}")
            
    except Exception as e:
        print(f"\n[Network Error] Error in listener loop: {e}")


def close(sock: socket.socket) -> None:
    sock.close()


def send_msg(sock: socket.socket, client_msg: Any):
    global sequence
    client_msg.sequence = sequence
    sequence += 1

    readable_json = MessageToJson(client_msg, preserving_proto_field_name=True)
    print(f"-> {readable_json}")

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