package com.ramserver.client.network;

import com.google.protobuf.ByteString;
import com.ramserver.client.ui.CLIRenderer;
import org.ramserver.proto.v1.*;

import java.io.*;
import java.net.Socket;

public class NetworkClient {
    private final String host;
    private final int port;
    private volatile Socket socket;
    private OutputStream outputStream;
    private InputStream inputStream;
    private volatile boolean connected = false;
    private long sequenceNumber = 0;
    private static final int MAX_FRAME_SIZE = 65536;

    private volatile String currentSessionId = "";
    private volatile String playerId = "";
    private volatile StateUpdate latestStateUpdate;
    private volatile long pendingLeaveSequence = 0;

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

            Socket listeningSocket = socket;

            Thread listeningThread = new Thread(
                    () -> listenForServerMessages(listeningSocket)
            );

            listeningThread.setDaemon(true);
            listeningThread.start();
        } catch (Exception e) {
            System.err.println("Connection failed: " + e.getMessage());
            disconnect();
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

        if (connResponse == null) {
            throw new IOException("Server disconnected during handshake");
        }

        if (connResponse.hasErrorResponse()) {
            throw new IOException(connResponse.getErrorResponse().toString());
        }

        if (!connResponse.hasConnectResponse()) {
            throw new IOException("Expected ConnectResponse, received: "
                    + connResponse.getPayloadCase());
        }

        System.out.println("Received server connect response");

        sequenceNumber++;
        AuthRequest authRequest = AuthRequest.newBuilder().setDisplayName(name).build();
        ClientMessage authMessage = ClientMessage.newBuilder().setSequence(sequenceNumber).setAuthRequest(authRequest).build();

        sendFrame(authMessage.toByteArray());
        System.out.println("Sending auth request");

        ServerMessage authResponse = readSync();

        if (authResponse == null) {
            throw new IOException("Server disconnected during authentication");
        }

        if (authResponse.hasErrorResponse()) {
            throw new IOException(authResponse.getErrorResponse().toString());
        }

        if (!authResponse.hasAuthResponse()) {
            throw new IOException("Expected AuthResponse, received: "
                    + authResponse.getPayloadCase());
        }

        playerId = authResponse.getAuthResponse().getPlayerId();

        if (playerId.isEmpty()) {
            throw new IOException("Authentication response missing player ID");
        }

        System.out.println("Authenticated as " + name);
        System.out.println("Player ID: " + playerId);
    }

    private ServerMessage readSync() throws IOException {
        return readFrame(socket);
    }



    private ServerMessage readFrame(Socket connection) throws IOException {
        DataInputStream in = new DataInputStream(
                connection.getInputStream()
        );

        while (true) {
            int length = in.readInt();

            if (length < 0 || length > MAX_FRAME_SIZE) {
                throw new IOException("Invalid frame length: " + length);
            }

            // Zero-length frames are heartbeats
            if (length == 0) {
                synchronized (this) {
                    if (socket != connection || !connected) {
                        throw new IOException("Connection is no longer active");
                    }

                    sendFrame(new byte[0]);
                }
                continue;
            }

            byte[] payload = new byte[length];
            in.readFully(payload);

            return ServerMessage.parseFrom(payload);
        }
    }




    private void listenForServerMessages(Socket connection) {
        try {
            while (connected && socket == connection
                    && !connection.isClosed()) {

                ServerMessage serverMessage = readFrame(connection);

                synchronized (this) {
                    if (socket != connection || !connected) {
                        break;
                    }

                    trackSession(serverMessage);
                    logServerMessage(serverMessage);
                }

                System.out.print("> ");
            }
        } catch (Exception e) {
            if (connected && socket == connection) {
                System.err.println("Connection closed: " + e.getMessage());
            }
        } finally {
            synchronized (this) {
                if (socket == connection) {
                    disconnect();
                }
            }
        }
    }

    private void logServerMessage(ServerMessage msg) {
        String kind = "UNKNOWN";
        String body = msg.toString().replace("\n", " ");

        if (msg.hasConnectResponse()) kind = "CONNECTED";
        else if (msg.hasAuthResponse()) kind = "AUTHED";
        else if (msg.hasStateUpdate()) {
            printStateUpdate(msg.getStateUpdate());
            return;
        }
        else if (msg.hasErrorResponse()) {
            ErrorResponse error = msg.getErrorResponse();

            System.out.println("\n[<- ERROR]");
            System.out.println("Code: " + error.getCode());
            System.out.println("Message: " + error.getMessage());

            return;
        }
        else if (msg.hasMatchResult()) {
            MatchResult result = msg.getMatchResult();

            System.out.println("\n[<- MATCH RESULT]");
            System.out.println("Session: " + result.getSessionId());
            System.out.println("Reason: " + result.getReason());

            if (result.getDraw()) {
                System.out.println("Result: Draw");
            } else if (result.getWinnerPlayerIdsCount() > 0) {
                for (String winnerId : result.getWinnerPlayerIdsList()) {
                    String winnerName = winnerId;

                    if (latestStateUpdate != null) {
                        for (var player : latestStateUpdate.getPlayersList()) {
                            if (player.getPlayerId().equals(winnerId)) {
                                winnerName = player.getDisplayName();
                                break;
                            }
                        }
                    }

                    if (winnerId.equals(playerId)) {
                        winnerName += " (you)";
                    }

                    System.out.println("Winner: " + winnerName);
                }
            } else {
                System.out.println("Result: No winner specified");
            }

            return;
        }
        else if (msg.hasLeaveSessionResponse()) kind = "LEFT";

        System.out.println("\n[<- " + kind + "] " + body);
    }


    private void printStateUpdate(StateUpdate state) {
        System.out.println("\n[STATE]");
        System.out.println("Session: " + state.getSessionId());
        System.out.println("Game: " + state.getGameId());
        System.out.println("Status: " + state.getStatus());
        System.out.println("State version: " + state.getStateVersion());

        System.out.println("Players:");
        for (var player : state.getPlayersList()) {
            String name = player.getDisplayName();

            if (player.getPlayerId().equals(playerId)) {
                name += " (you)";
            }

            System.out.println(" - " + name);
        }

        String activePlayer = state.getActivePlayerId();

        if (!activePlayer.isEmpty()) {
            String turn = activePlayer.equals(playerId)
                    ? "You" : activePlayer;

            if (!activePlayer.equals(playerId)) {
                for (var player : state.getPlayersList()) {
                    if (player.getPlayerId().equals(activePlayer)) {
                        turn = player.getDisplayName();
                        break;
                    }
                }
            }

            System.out.println("Turn: " + turn);
        }

        if (!state.getEncodedView().isEmpty()) {
            CLIRenderer.printBoard(state.getEncodedView().toStringUtf8());
        }
    }

    public void printSessionStatus() {
        StateUpdate state = latestStateUpdate;

        if (state == null) {
            System.out.println("No session state available.");
            return;
        }

        printStateUpdate(state);
    }


    // Formerly sendActionRequest
    public void sendMove(String uciMove) {
        if (!checkConnection()) return;
        StateUpdate state = latestStateUpdate;

        String activePlayer = state.getActivePlayerId();

        if (!activePlayer.isEmpty() && !activePlayer.equals(playerId)) {
            System.out.println("It is not your turn.");
            return;
        }

        ClientMessage clientMsg = ClientMessage.newBuilder()
                .setActionRequest(ActionRequest.newBuilder().setSessionId(state.getSessionId()).setExpectedStateVersion(state.getStateVersion()).setEncodedAction(ByteString.copyFromUtf8(uciMove)).build())
                .build();

        sendMessage(clientMsg);
    }

    public void createSession() {
        if (!checkConnection()) return;
        try {
            ClientMessage createMessage = ClientMessage.newBuilder().setJoinSessionRequest(JoinSessionRequest.newBuilder().setGameId("chess").build()).build();
            sendMessage(createMessage);
        } catch (Exception e) {
            System.err.println("Error creating session");
        }
    }

    public void joinSession(String sessionId) {
        if (!checkConnection()) return;
        try {
            ClientMessage createMessage = ClientMessage.newBuilder().setJoinSessionRequest(JoinSessionRequest.newBuilder().setGameId("chess").setSessionId(sessionId).build()).build();
            sendMessage(createMessage);
        } catch (Exception e) {
            System.err.println("Error joining session");
        }
    }

    public void leaveSession() {
        if (!checkConnection()) return;

        if (currentSessionId.isEmpty()) {
            System.out.println("You are not in a session.");
            return;
        }

        if (pendingLeaveSequence != 0) {
            System.out.println("Already waiting for leave confirmation.");
            return;
        }

        ClientMessage message = ClientMessage.newBuilder()
                .setLeaveSessionRequest(
                        LeaveSessionRequest.newBuilder()
                                .setSessionId(currentSessionId)
                                .build()
                )
                .build();

        sendMessage(message);
    }

    private synchronized void sendFrame(byte[] payload) throws IOException {
        if (payload.length > MAX_FRAME_SIZE) {
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
            long sequence = ++sequenceNumber;

            ClientMessage finalMsg = clientMsg.toBuilder()
                    .setSequence(sequence)
                    .build();

            if (finalMsg.hasLeaveSessionRequest()) {
                pendingLeaveSequence = sequence;
            }

            sendFrame(finalMsg.toByteArray());

            System.out.println("Sent message: " + finalMsg);

        } catch (IOException e) {
            if (clientMsg.hasLeaveSessionRequest()) {
                pendingLeaveSequence = 0;
            }

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

    private synchronized void trackSession(ServerMessage serverMessage) {
        if (serverMessage.hasStateUpdate()) {
            StateUpdate update = serverMessage.getStateUpdate();

            currentSessionId = update.getSessionId();
            latestStateUpdate = update;

        } else if (serverMessage.hasLeaveSessionResponse()) {
            String responseSessionId =
                    serverMessage.getLeaveSessionResponse().getSessionId();

            if (pendingLeaveSequence != 0
                    && serverMessage.getInReplyTo() == pendingLeaveSequence
                    && responseSessionId.equals(currentSessionId)) {

                System.out.println("Successfully left session: "
                        + currentSessionId);

                currentSessionId = "";
                latestStateUpdate = null;
                pendingLeaveSequence = 0;
            }

        } else if (serverMessage.hasErrorResponse()) {
            if (pendingLeaveSequence != 0
                    && serverMessage.getInReplyTo() == pendingLeaveSequence) {

                pendingLeaveSequence = 0;
                System.out.println("Leave request failed; still in session.");
            }
        }
    }

    public synchronized void disconnect() {
        connected = false;

        currentSessionId = "";
        playerId = "";
        latestStateUpdate = null;
        sequenceNumber = 0;
        pendingLeaveSequence = 0;

        try {
            if (socket != null && !socket.isClosed()) {
                socket.close();
            }
        } catch (IOException e) {
            System.err.println("Error closing connection: " + e.getMessage());
        }

        socket = null;
        inputStream = null;
        outputStream = null;
    }

    public boolean isConnected() {
        return connected;
    }

    public String getPlayerId() {
        return playerId;
    }

    public StateUpdate getLatestStateUpdate() {
        return latestStateUpdate;
    }
}
