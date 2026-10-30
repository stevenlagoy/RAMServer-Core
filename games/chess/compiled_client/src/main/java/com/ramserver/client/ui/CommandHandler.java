package com.ramserver.client.ui;

import com.ramserver.client.network.NetworkClient;

import java.util.Locale;
import java.util.function.Consumer;

public class CommandHandler {
    private final NetworkClient client;
    private final Consumer<String> output;

    public CommandHandler(NetworkClient client, Consumer<String> output) {
        this.client = client;
        this.output = output;
    }

    // Returns false when the application should exit.
    public boolean execute(String input) {
        if (input == null || input.isBlank()) {
            return true;
        }

        String[] parts = input.trim().split("\\s+", 2);
        String command = parts[0].toLowerCase(Locale.ROOT);
        String args = parts.length > 1 ? parts[1].trim() : "";

        switch (command) {
            case "connect" -> {
                if (args.isEmpty()) {
                    output.accept("Usage: connect <name>");
                } else {
                    client.connect(args);
                }
            }

            case "create" -> client.createSession();

            case "join" -> {
                if (args.isEmpty()) {
                    output.accept("Usage: join <session_id>");
                } else {
                    client.joinSession(args);
                }
            }

            case "leave" -> client.leaveSession();

            case "move" -> {
                if (args.isEmpty()) {
                    output.accept("Usage: move <uci>");
                } else {
                    client.sendMove(args);
                }
            }

            case "status" -> client.printSessionStatus();

            case "disconnect" -> client.disconnect();

            case "help" -> {
                output.accept("Commands: connect <name>, disconnect");
                output.accept("create, join <session_id>, leave");
                output.accept("move <uci>, status, help, exit");
            }

            case "exit", "quit" -> {
                client.disconnect();
                return false;
            }

            case "ready" -> client.ready();


            default -> output.accept(
                    "Unknown command. Type 'help' for commands."
            );
        }

        return true;
    }
}