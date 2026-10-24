package com.ramserver.client.network;

import org.ramserver.proto.v1.ActionRequest;
import org.ramserver.proto.v1.AuthRequest;
import org.ramserver.proto.v1.ClientMessage;
import org.ramserver.proto.v1.ServerMessage;
import org.ramserver.proto.v1.ConnectRequest;
import org.ramserver.proto.v1.JoinSessionRequest;
import org.ramserver.proto.v1.LeaveSessionRequest;
import org.ramserver.proto.v1.StateUpdate;

import java.io.*;
import java.net.Socket;

public class NetworkClient {
    private final String host;
    private final int port;
    private Socket socket;
    private OutputStream outputStream;
    private InputStream inputStream;
    private boolean connected = false;
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

    public boolean isAuthenticated() {
        return authenticated;
    }

    public String getPlayerId() {
        return playerId;
    }

    public String getSessionId() {
        return sessionId;
    }

    public synchronized void connect() {
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

            Thread listeningThread = new Thread(this::listenForServerMessages);
            listeningThread.setDaemon(true);
            listeningThread.start();

            ClientMessage message = ClientMessage.newBuilder()
                    .setSequence(nextSequence++)
                    .setConnectRequest(ConnectRequest.newBuilder()
                            .setProtocolVersion("1.0")
                            .setClientName("JVM-chess")
                            .build())
                    .build();

            sendMessage(message);

        } catch (Exception e) {
            e.printStackTrace();
        }
    }

    private void listenForServerMessages() {
        try {
            DataInputStream in = new DataInputStream(inputStream);

            while (connected && !socket.isClosed()) {
                int length = in.readInt();

                if (length < 0 || length > 65536) {
                    throw new IOException("Invalid frame length: " + length);
                }

                // Server heartbeat
                if (length == 0) {
                    sendHeartbeat();
                    continue;
                }

                byte[] payload = new byte[length];
                in.readFully(payload);

                ServerMessage message = ServerMessage.parseFrom(payload);
                handleServerMessage(message);

                System.out.print("> ");
            }
        } catch (IOException e) {
            if (connected) {
                System.err.println("Connection closed: " + e.getMessage());
            }
        } finally {
            disconnect();
        }
    }

    private synchronized void sendHeartbeat() throws IOException {
        DataOutputStream out = new DataOutputStream(outputStream);
        out.writeInt(0);
        out.flush();
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
            var error = serverMessage.getErrorResponse();

            System.out.println("Error response received: " + error);

            if (serverMessage.getInReplyTo() == pendingLeaveSequence) {
                pendingLeaveSequence = 0;
                System.out.println("Leave failed. Still in session: " + sessionId);
            }
        } else {
            System.out.println("Message received");
        }
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

    public void sendAuthRequest(String name) {
        AuthRequest authRequest = AuthRequest.newBuilder()
                .setDisplayName(name)
                .build();

        ClientMessage message = ClientMessage.newBuilder()
                .setSequence(nextSequence++)
                .setAuthRequest(authRequest)
                .build();

        if (authenticated) {
            System.out.println("Already authenticated as " + playerId);
            return;
        }

        sendMessage(message);
    }

    public void sendActionRequest(String action) {
        if (!checkConnection()) return;

        ClientMessage clientMsg = ClientMessage.newBuilder()
                .setActionRequest(ActionRequest.newBuilder().build())
                .build();

        sendMessage(clientMsg);
    }

    public boolean sendPing() {
        try {
            socket.getOutputStream().write("PING\n".getBytes());
            socket.getOutputStream().flush();

            socket.setSoTimeout(2000);

            BufferedReader reader = new BufferedReader(new InputStreamReader(socket.getInputStream()));
            String response = reader.readLine();
            return "PONG".equalsIgnoreCase(response);
        } catch (IOException e) {
            System.err.println("Ping error: " + e.getMessage());
            return false;
        }
    }

    private synchronized void sendMessage(ClientMessage clientMsg) {
        try {
            byte[] payload = clientMsg.toByteArray();

            DataOutputStream out = new DataOutputStream(outputStream);
            out.writeInt(payload.length);
            out.write(payload);
            out.flush();

            System.out.println("Sent message: " + clientMsg);
        } catch (IOException e) {
            System.err.println("Error sending message: " + e.getMessage());
        }
    }

    private boolean checkConnection() {
        if (!connected) {
            System.out.println("Please connect to the server first");
            return false;
        }
        return true;
    }

    public void printSessionStatus() {
        StateUpdate state = latestStateUpdate;

        if (state == null) {
            System.out.println("No session state available.");
            return;
        }

        printStateUpdate(state);
    }



    public void sendJoinSessionRequest(String gameId, String targetSessionId) {
        if (!checkConnection()) return;

        if (!authenticated) {
            System.out.println("Please authenticate first.");
            return;
        }

        if (!sessionId.isEmpty()) {
            System.out.println("Already in session: " + sessionId);
            return;
        }

        if (gameId == null || gameId.isBlank()) {
            System.out.println("Game ID is required.");
            return;
        }

        JoinSessionRequest request = JoinSessionRequest.newBuilder()
                .setGameId(gameId)
                .setSessionId(targetSessionId)
                .build();

        ClientMessage message = ClientMessage.newBuilder()
                .setSequence(nextSequence++)
                .setJoinSessionRequest(request)
                .build();

        sendMessage(message);
    }

    public void sendLeaveSessionRequest() {
        if (!checkConnection()) return;

        if (!authenticated) {
            System.out.println("Please authenticate first.");
            return;
        }

        if (sessionId.isEmpty()) {
            System.out.println("You are not in a session.");
            return;
        }

        if (pendingLeaveSequence != 0) {
            System.out.println("Already waiting for leave confirmation.");
            return;
        }

        long sequence = nextSequence++;

        LeaveSessionRequest request = LeaveSessionRequest.newBuilder()
                .setSessionId(sessionId)
                .build();

        ClientMessage message = ClientMessage.newBuilder()
                .setSequence(sequence)
                .setLeaveSessionRequest(request)
                .build();

        pendingLeaveSequence = sequence;
        sendMessage(message);
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
