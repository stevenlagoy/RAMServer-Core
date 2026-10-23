package com.ramserver.client.core;

import com.ramserver.client.network.NetworkClient;
import com.ramserver.client.ui.CLI;

public class ClientMain {
    public static void main(String[] args) {
        try {
            System.out.println("Starting client...");
            String host = "127.0.0.1";
            int port = 9000;

            NetworkClient client = new NetworkClient(host, port);
            CLI cli = new CLI(client);

            cli.run();
        } catch (Exception e) {
            System.err.println("CRITICAL CLIENT CRASH:");
            e.printStackTrace();
        }

    }
}
