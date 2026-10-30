package com.ramserver.client.core;

import com.ramserver.client.network.NetworkClient;
import com.ramserver.client.ui.CLI;
import com.ramserver.client.ui.ChessTUI;

public class ClientMain {
    public static void main(String[] args) {
        try {
            System.out.println("Starting client...");
            String host = "127.0.0.1";
            int port = 9000;

            NetworkClient client = new NetworkClient(host, port);

            if (args.length > 0 && args[0].equals("--tui")) {
                new ChessTUI(client).run();
            } else {
                new CLI(client).run();
            }
        } catch (Exception e) {
            System.err.println("CRITICAL CLIENT CRASH:");
            e.printStackTrace();
        }

    }
}
