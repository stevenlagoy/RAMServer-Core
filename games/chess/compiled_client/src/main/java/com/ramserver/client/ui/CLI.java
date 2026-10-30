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
        CommandHandler commands = new CommandHandler(
                client,
                System.out::println
        );
        System.out.println("CLI Client Started");
        System.out.println("Type 'help' for available commands.");
        while (running) {
            System.out.print("> ");
            if (!scanner.hasNextLine()) {
                System.out.println("no console detected");
                break;
            }
            String input = scanner.nextLine().trim();

            if (input.isEmpty()) continue;

            running = commands.execute(input);
        }
    }
}
