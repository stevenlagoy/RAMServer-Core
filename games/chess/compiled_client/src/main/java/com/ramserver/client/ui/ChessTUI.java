package com.ramserver.client.ui;

import com.googlecode.lanterna.TextColor;
import com.googlecode.lanterna.graphics.TextGraphics;
import com.googlecode.lanterna.input.KeyStroke;
import com.googlecode.lanterna.input.KeyType;
import com.googlecode.lanterna.screen.Screen;
import com.googlecode.lanterna.terminal.DefaultTerminalFactory;
import com.googlecode.lanterna.TerminalPosition;
import com.ramserver.client.network.NetworkClient;
import java.util.ArrayDeque;
import java.util.Deque;
import org.ramserver.proto.v1.StateUpdate;

import java.io.IOException;



public class ChessTUI {

    private final NetworkClient client;

    public ChessTUI(NetworkClient client) {
        this.client = client;
    }

    public void run() throws IOException {
        DefaultTerminalFactory factory = new DefaultTerminalFactory()
                .setTerminalEmulatorTitle("RAMServer - Chess");

        try (Screen screen = factory.createScreen()) {
            screen.startScreen();
            screen.setCursorPosition(null);

            TextGraphics graphics = screen.newTextGraphics();
            graphics.setBackgroundColor(TextColor.ANSI.BLACK);
            graphics.setForegroundColor(TextColor.ANSI.WHITE);
            graphics.fill(' ');

            graphics.putString(2, 1, "RAMSERVER / CHESS");
            graphics.putString(2, 2, "Press ESC to exit");

            drawGame(screen, graphics, null, false, "");

            graphics.putString(2, 14, "COMMAND INPUT");
            graphics.putString(2, 15, "Enter: submit | Backspace: delete | ESC: exit");

            StringBuilder command = new StringBuilder();
            Deque<String> messages = new ArrayDeque<>();

            CommandHandler commands = new CommandHandler(client, message -> {
                messages.addLast(message);

                while (messages.size() > 4) {
                    messages.removeFirst();
                }
            });

            boolean running = true;

            StateUpdate displayedState = null;
            boolean displayedConnected = false;

            while (running) {
                StateUpdate currentState = client.getLatestStateUpdate();
                boolean currentlyConnected = client.isConnected();

                if (currentState != displayedState
                        || currentlyConnected != displayedConnected) {

                    drawGame(
                            screen,
                            graphics,
                            currentState,
                            currentlyConnected,
                            client.getPlayerId()
                    );

                    displayedState = currentState;
                    displayedConnected = currentlyConnected;
                }
                int width = screen.getTerminalSize().getColumns();

                // Keep the command visible within the terminal width.
                int maxLength = Math.max(0, width - 6);
                String input = command.toString();

                if (input.length() > maxLength) {
                    input = input.substring(input.length() - maxLength);
                }

                // Clear and redraw the command line.
                graphics.setBackgroundColor(TextColor.ANSI.BLACK);
                graphics.setForegroundColor(TextColor.ANSI.WHITE);

                graphics.putString(2, 17, " ".repeat(Math.max(0, width - 2)));
                graphics.putString(2, 17, "> " + input);

                // Clear the message area.
                for (int i = 0; i < 4; i++) {
                    graphics.putString(
                            2, 19 + i,
                            " ".repeat(Math.max(0, width - 2))
                    );
                }

                // Display the latest messages.
                int row = 0;

                for (String message : messages) {
                    graphics.putString(
                            2, 19 + row,
                            message.substring(0, Math.min(message.length(), maxLength))
                    );
                    row++;
                }

                // Position the cursor after the typed text.
                screen.setCursorPosition(
                        new TerminalPosition(4 + input.length(), 17)
                );

                screen.refresh();

                KeyStroke key = screen.pollInput();

                if (key == null) {
                    try {
                        Thread.sleep(50);
                    } catch (InterruptedException e) {
                        Thread.currentThread().interrupt();
                        break;
                    }

                    continue;
                }

                if (key.getKeyType() == KeyType.Escape
                        || key.getKeyType() == KeyType.EOF) {
                    break;
                }

                switch (key.getKeyType()) {
                    case Character -> {
                        char ch = key.getCharacter();

                        if (!Character.isISOControl(ch)) {
                            command.append(ch);
                        }
                    }

                    case Backspace -> {
                        if (!command.isEmpty()) {
                            command.deleteCharAt(command.length() - 1);
                        }
                    }

                    case Enter -> {
                        String submittedCommand = command.toString().trim();

                        if (!submittedCommand.isEmpty()) {
                            messages.addLast("> " + submittedCommand);

                            while (messages.size() > 4) {
                                messages.removeFirst();
                            }

                            running = commands.execute(submittedCommand);
                        }

                        command.setLength(0);
                    }

                    default -> {
                        // Ignore unsupported keys.
                    }
                }
            }
        }
        finally {
            client.disconnect();
        }
    }
    private void drawGame(
            Screen screen,
            TextGraphics graphics,
            StateUpdate state,
            boolean connected,
            String playerId
    ) {
        graphics.setBackgroundColor(TextColor.ANSI.BLACK);
        graphics.setForegroundColor(TextColor.ANSI.WHITE);

        // Clear the previous board and status area.
        int width = Math.max(0, screen.getTerminalSize().getColumns() - 2);

        for (int y = 4; y <= 12; y++) {
            graphics.putString(2, y, " ".repeat(width));
        }

        // Read the latest authoritative board position.
        char[][] board = new char[8][8];

        if (state != null && !state.getEncodedView().isEmpty()) {
            board = CLIRenderer.parseFEN(
                    state.getEncodedView().toStringUtf8()
            );
        }

        TextColor lightSquare = new TextColor.RGB(75, 85, 105);
        TextColor darkSquare = new TextColor.RGB(45, 55, 75);
        TextColor whitePiece = new TextColor.RGB(255, 240, 210);
        TextColor blackPiece = new TextColor.RGB(165, 205, 245);

        for (int r = 0; r < 8; r++) {
            for (int c = 0; c < 8; c++) {
                char piece = board[r][c];

                if (piece == '\0') {
                    piece = ' ';
                }

                graphics.setBackgroundColor(
                        (r + c) % 2 == 0 ? lightSquare : darkSquare
                );

                graphics.setForegroundColor(
                        Character.isUpperCase(piece)
                                ? whitePiece : blackPiece
                );

                graphics.putString(4 + c * 4, 4 + r, " " + piece + "  ");
            }
        }

        graphics.setBackgroundColor(TextColor.ANSI.BLACK);
        graphics.setForegroundColor(TextColor.ANSI.WHITE);

        // Rank labels.
        for (int r = 0; r < 8; r++) {
            graphics.putString(2, 4 + r, String.valueOf(8 - r));
        }

        // File labels.
        for (int c = 0; c < 8; c++) {
            graphics.putString(
                    5 + c * 4, 12,
                    String.valueOf((char) ('a' + c))
            );
        }

        // Live game information.
        graphics.putString(40, 4, "GAME STATUS");
        graphics.putString(40, 12,
                connected ? "Connected" : "Disconnected");

        if (state == null) {
            graphics.putString(40, 6, "No active session");
            return;
        }

        graphics.putString(40, 6, "Players:");

        for (int i = 0; i < Math.min(2, state.getPlayersCount()); i++) {
            var player = state.getPlayers(i);
            String name = player.getDisplayName();

            if (player.getPlayerId().equals(playerId)) {
                name += " (you)";
            }

            graphics.putString(40, 7 + i, " - " + name);
        }

        String turn = "-";
        String activeId = state.getActivePlayerId();

        if (!activeId.isEmpty()) {
            turn = activeId.equals(playerId) ? "You" : activeId;

            if (!activeId.equals(playerId)) {
                for (var player : state.getPlayersList()) {
                    if (player.getPlayerId().equals(activeId)) {
                        turn = player.getDisplayName();
                        break;
                    }
                }
            }
        }

        graphics.putString(40, 10, "Turn: " + turn);
        graphics.putString(40, 11, "Status: " + state.getStatus());
    }
}