package com.ramserver.client.ui;

import com.ramserver.client.network.NetworkClient;

import java.util.Scanner;

// TEMPORARY MVP - SHOULD BE REPLACED SOON (tm)
public class CLI implements Runnable {
    private final NetworkClient client;
    private boolean running = true;

    public CLI(NetworkClient client) {
        this.client = client;
    }

    @Override
    public void run() {
        Scanner scanner = new Scanner(System.in);
        System.out.println("CLI Client Started");
        System.out.println("Type 'connect' to connect to the server");

        while (running) {
            System.out.print("> ");
            if (!scanner.hasNextLine()) {
                System.out.println("no console detected");
                break;
            }
            String input = scanner.nextLine().trim();

            if (input.isEmpty()) continue;

            String[] parts = input.split(" ", 2);
            String command = parts[0].toLowerCase();
            String args = parts.length > 1 ? parts[1] : "";

            switch (command) {
                case "connect" -> {
                    if (args.isEmpty()) {
                        System.out.println("Usage: connect <name>");
                    } else {
                        client.connect(args);
                    }
                }
                case "create" -> client.createSession();
                case "join" -> {
                    if (args.isEmpty()) {
                        System.out.println("Usage: join <session_id>");
                    } else {
                        client.joinSession(args);
                    }
                }
                case "leave" -> client.leaveSession();
                case "move" -> {
                    if (args.isEmpty()) {
                        System.out.println("Usage: move <notation>");
                    } else {
                        client.sendMove(args);
                    }
                }
                case "disconnect" -> client.disconnect();
            }
        }
    }
}
