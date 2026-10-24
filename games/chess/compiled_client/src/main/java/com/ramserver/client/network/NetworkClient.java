package com.ramserver.client.network;

import com.google.protobuf.ByteString;
import org.ramserver.proto.v1.*;

import java.io.*;
import java.net.Socket;
import java.util.Arrays;

public class NetworkClient {
    private final String host;
    private final int port;
    private Socket socket;
    private OutputStream outputStream;
    private InputStream inputStream;
    private boolean connected = false;
    private long sequenceNumber = 0;

    private String currentSessionId = "";
    private long nextSequence = 1;
    private volatile boolean authenticated = false;
    private volatile String playerId = "";
    private volatile String reconnectToken = "";
    private volatile String sessionId = "";
    private volatile long stateVersion = 0;
    private volatile long pendingLeaveSequence = 0;
    private volatile StateUpdate latestStateUpdate;

    public NetworkClient(String host, int port) {
        this.host = host;
        this.port = port;
    }

    public synchronized void connect(String name) {
        if (connected) {
            System.out.println("Already connected");
            return;
        }

        try {
            socket = new Socket(host, port);
            outputStream = socket.getOutputStream();
            inputStream = socket.getInputStream();
            connected = true;
            System.out.println("Connected to " + host + ":" + port);

            performHandshake(name);

            Thread listeningThread = new Thread(this::listenForServerMessages);
            listeningThread.setDaemon(true);
            listeningThread.start();
        } catch (Exception e) {
            e.printStackTrace();
        }
    }

    private void performHandshake(String name) throws IOException {
        sequenceNumber++;
        ConnectRequest connectRequest =
                ConnectRequest.newBuilder()
                        .setProtocolVersion("1.0")
                        .setClientName("java-client")
                        .build();

        ClientMessage clientMessage = ClientMessage.newBuilder().setSequence(sequenceNumber).setConnectRequest(connectRequest).build();

        byte[] data = clientMessage.toByteArray();
        sendFrame(data);
        System.out.println("Waiting for server response...");

        ServerMessage connResponse = readSync();
        if (connResponse == null || connResponse.hasErrorResponse()) {
            throw new IOException(connResponse.getErrorResponse().toString());
        }
        System.out.println("Received server connect response");

        sequenceNumber++;
        AuthRequest authRequest = AuthRequest.newBuilder().setDisplayName(name).build();
        ClientMessage authMessage = ClientMessage.newBuilder().setSequence(sequenceNumber).setAuthRequest(authRequest).build();

        sendFrame(authMessage.toByteArray());
        System.out.println("Sending auth request");

        ServerMessage authResponse = readSync();
        if (authResponse == null || authResponse.hasErrorResponse()) {
            throw new IOException(authResponse.getErrorResponse().toString());
        }
        System.out.println("Received server auth response");
    }

    private ServerMessage readSync() throws IOException {
        byte[] header = new byte[4];
        int bytesRead = inputStream.readNBytes(header, 0, 4);
        if (bytesRead < 4) return null;

        int length = ((header[0] & 0xFF) << 24
                | ((header[1] & 0xFF) << 16)
                | ((header[2] & 0xFF) << 8)
                | (header[3] & 0xFF)
        );

        if (length == 0) {
            sendFrame(new byte[0]);
            return readSync();
        }

        byte[] payload = inputStream.readNBytes(length);
        return ServerMessage.parseFrom(payload);
    }

    private void listenForServerMessages() {
        try {
            while (connected && !socket.isClosed()) {
                byte[] header = new byte[4];
                int bytesRead = inputStream.readNBytes(header, 0, 4);
                if (bytesRead < 4) {
                    System.out.println("Connection closed by server");
                    break;
                }

                int length = ((header[0] & 0xFF) << 24
                        | ((header[1] & 0xFF) << 16)
                        | ((header[2] & 0xFF) << 8)
                        | (header[3] & 0xFF)
                );

                if (length == 0) {
                    sendFrame(new byte[0]);
                    continue;
                }

                byte[] payload = inputStream.readNBytes(length);
                if (payload.length < length) {
                    System.out.println("Connection closed during read");
                    break;
                }

                ServerMessage serverMessage = ServerMessage.parseFrom(payload);
                trackSession(serverMessage);
                logServerMessage(serverMessage);
                handleServerMessage(serverMessage);
                System.out.print("> ");
            }
        } catch (Exception e) {
            if (connected) {
                System.err.println("Connection closed");
            }
        } finally {
            disconnect();
        }
    }

    private void logServerMessage(ServerMessage msg) {
        String kind = "UNKNOWN";
        String body = msg.toString().replace("\n", " ");

        if (msg.hasConnectResponse()) kind = "CONNECTED";
        else if (msg.hasAuthResponse()) kind = "AUTHED";
        else if (msg.hasStateUpdate()) kind = "STATE";
        else if (msg.hasErrorResponse()) kind = "ERROR";
        else if (msg.hasMatchResult()) kind = "RESULT";
        else if (msg.hasLeaveSessionResponse()) kind = "LEFT";

        System.out.println("\n[<- " + kind + "] " + body);
    }

    private void handleServerMessage(ServerMessage serverMessage) {
        System.out.println("Response type: " + serverMessage.getPayloadCase()
                + " | Reply to: " + serverMessage.getInReplyTo());

        if (serverMessage.hasConnectResponse()) {
            System.out.println("Handshake successful: "
                    + serverMessage.getConnectResponse());

        } else if (serverMessage.hasAuthResponse()) {
            var response = serverMessage.getAuthResponse();

            if (response.getPlayerId().isEmpty() ||
                    response.getReconnectToken().isEmpty()) {
                System.err.println("Invalid authentication response");
                return;
            }

            playerId = response.getPlayerId();
            reconnectToken = response.getReconnectToken();
            sessionId = response.getSessionId();
            authenticated = true;

            System.out.println("Authentication successful!");
            System.out.println("Player ID: " + playerId);

            if (!sessionId.isEmpty()) {
                System.out.println("Session ID: " + sessionId);
            }

        } else if (serverMessage.hasStateUpdate()) {

            StateUpdate state = serverMessage.getStateUpdate();

            // Store the complete authoritative snapshot
            latestStateUpdate = state;

            // Maintain our existing fields
            sessionId = state.getSessionId();
            stateVersion = state.getStateVersion();

            // Display the updated state
            printStateUpdate(state);

        } else if (serverMessage.hasLeaveSessionResponse()) {
            var response = serverMessage.getLeaveSessionResponse();

            if (serverMessage.getInReplyTo() == pendingLeaveSequence
                    && response.getSessionId().equals(sessionId)) {

                System.out.println("Successfully left session: " + sessionId);

                sessionId = "";
                stateVersion = 0;
                pendingLeaveSequence = 0;
                latestStateUpdate = null;

            } else {
                System.out.println("Unexpected leave response received.");
            }
        } else if (serverMessage.hasErrorResponse()) {
            System.out.println("Error response received: " + serverMessage.getErrorResponse());
        } else {
            System.out.println("Message received: " + serverMessage.getDescriptorForType().getFullName());
        }
    }

    // Formerly sendActionRequest
    public void sendMove(String uciMove) {
        if (!checkConnection()) return;

        ClientMessage clientMsg = ClientMessage.newBuilder()
                .setActionRequest(ActionRequest.newBuilder().setSessionId(currentSessionId).setEncodedAction(ByteString.copyFromUtf8(uciMove)).build())
                .build();

        sendMessage(clientMsg);
    }

    private void printStateUpdate(StateUpdate state) {
        System.out.println("Session ID: " + state.getSessionId());
        System.out.println("Game: " + state.getGameId());
        System.out.println("Status: " + state.getStatus());
        System.out.println("State version: " + state.getStateVersion());

        System.out.println("Players:");

        for (var player : state.getPlayersList()) {
            System.out.println(" - " + player.getDisplayName()
                    + " (" + player.getPlayerId() + ")"
                    + (player.getConnected() ? "" : " [Disconnected]"));
        }

        if (!state.getActivePlayerId().isEmpty()) {
            String activePlayer = state.getActivePlayerId();

            if (activePlayer.equals(playerId)) {
                System.out.println("Current turn: You");
            } else {
                for (var player : state.getPlayersList()) {
                    if (player.getPlayerId().equals(activePlayer)) {
                        activePlayer = player.getDisplayName();
                        break;
                    }
                }

                System.out.println("Current turn: " + activePlayer);
            }
        }

        if (!state.getPlayerOrderList().isEmpty()) {
            System.out.println("Player order: "
                    + state.getPlayerOrderList());
        }

        if (!state.getEncodedView().isEmpty()) {
            System.out.println("Game state received: "
                    + state.getEncodedView().size() + " bytes");
        }
    }

    public void createSession() {
        if (!checkConnection()) return;
        try {
            sequenceNumber++;
            ClientMessage createMessage = ClientMessage.newBuilder().setJoinSessionRequest(JoinSessionRequest.newBuilder().setGameId("chess").build()).build();
            sendMessage(createMessage);
        } catch (Exception e) {
            System.err.println("Error creating session");
        }
    }

    public void joinSession(String sessionId) {
        if (!checkConnection()) return;
        try {
            sequenceNumber++;
            ClientMessage createMessage = ClientMessage.newBuilder().setJoinSessionRequest(JoinSessionRequest.newBuilder().setGameId("chess").setSessionId(sessionId).build()).build();
            sendMessage(createMessage);
        } catch (Exception e) {
            System.err.println("Error joining session");
        }
    }

    public void leaveSession() {
        if (!checkConnection() || currentSessionId.isEmpty()) return;

        try {
            sequenceNumber++;
            ClientMessage createMessage = ClientMessage.newBuilder().setLeaveSessionRequest(LeaveSessionRequest.newBuilder().setSessionId(currentSessionId).build()).build();
            sendMessage(createMessage);
        } catch (Exception e) {
            System.err.println("Error leaving session");
        }
    }

    private synchronized void sendFrame(byte[] payload) throws IOException {
        if (payload.length > 65536) {
            throw new IOException("Frame too large");
        }

        byte[] frame = new byte[4 + payload.length];
        frame[0] = (byte) ((payload.length >> 24) & 0xFF);
        frame[1] = (byte) ((payload.length >> 16) & 0xFF);
        frame[2] = (byte) ((payload.length >> 8) & 0xFF);
        frame[3] = (byte) (payload.length & 0xFF);
        System.arraycopy(payload, 0, frame, 4, payload.length);
        outputStream.write(frame);
        outputStream.flush();
    }

    private synchronized void sendMessage(ClientMessage clientMsg) {
        try {
            sequenceNumber++;
            ClientMessage finalMsg = clientMsg.toBuilder()
                    .setSequence(sequenceNumber)
                    .build();
            byte[] payload = finalMsg.toByteArray();
            sendFrame(payload);
            System.out.println("Sent message: " + finalMsg);
        } catch (Exception e) {
            System.err.println("Error sending message: " + clientMsg);
        }
    }

    private boolean checkConnection() {
        if (!connected) {
            System.out.println("Please connect to the server first");
            return false;
        }
        return true;
    }

    private synchronized void trackSession(ServerMessage serverMessage) {
        if (serverMessage.hasStateUpdate()) {
            currentSessionId = serverMessage.getStateUpdate().getSessionId();
        } else if (serverMessage.hasLeaveSessionResponse()) {
            if (serverMessage.getLeaveSessionResponse().getSessionId().equals(currentSessionId)) {
                currentSessionId = "";
            }
        }
    }

    public synchronized void disconnect() {
        connected = false;
        authenticated = false;
        pendingLeaveSequence = 0;
        sessionId = "";
        stateVersion = 0;
        latestStateUpdate = null;
        try {
            if (socket != null && !socket.isClosed()) {
                socket.close();
            }
        } catch (Exception ignored) {}
    }
}
