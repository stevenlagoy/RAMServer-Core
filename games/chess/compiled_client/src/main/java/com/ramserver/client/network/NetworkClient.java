package com.ramserver.client.network;

import org.ramserver.proto.v1.ActionRequest;
import org.ramserver.proto.v1.AuthRequest;
import org.ramserver.proto.v1.ClientMessage;
import org.ramserver.proto.v1.ServerMessage;

import java.io.*;
import java.net.Socket;

public class NetworkClient {
    private final String host;
    private final int port;
    private Socket socket;
    private OutputStream outputStream;
    private InputStream inputStream;
    private boolean connected = false;

    public NetworkClient(String host, int port) {
        this.host = host;
        this.port = port;
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
        } catch (Exception e) {
            e.printStackTrace();
        }
    }

    private void listenForServerMessages() {
        try {
            while (connected && !socket.isClosed()) {
                ServerMessage serverMessage = ServerMessage.parseDelimitedFrom(inputStream);
                if (serverMessage == null) {
                    System.out.println("Connection closed");
                    break;
                }
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

    private void handleServerMessage(ServerMessage serverMessage) {
        if (serverMessage.hasAuthResponse()) {
            System.out.println("Auth response received: " + serverMessage.getAuthResponse());
        } else if (serverMessage.hasStateUpdate()) {
            System.out.println("State update received: " + serverMessage.getStateUpdate());
        } else if (serverMessage.hasErrorResponse()) {
            System.out.println("Error response received: " + serverMessage.getErrorResponse());
        } else {
            System.out.println("Message received");
        }
    }

    public void sendAuthRequest(String name) {
        if (!checkConnection()) return;

        ClientMessage clientMsg = ClientMessage.newBuilder()
                .setAuthRequest(AuthRequest.newBuilder().build())
                .build();

        sendMessage(clientMsg);
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
            clientMsg.writeDelimitedTo(outputStream);
            outputStream.flush();
            System.out.println("Sent message: " + clientMsg);
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

    public synchronized void disconnect() {
        connected = false;
        try {
            if (socket != null && !socket.isClosed()) {
                socket.close();
            }
        } catch (Exception ignored) {}
    }
}
