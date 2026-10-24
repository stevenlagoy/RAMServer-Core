package com.ramserver.client.ui;

public class CLIRenderer {
    public static void printBoard(char[][] board) {
        System.out.println("  +---+---+---+---+---+---+---+---+");
        for (int r = 0; r < 8; r++) {
            System.out.print((8-r) + "|");
            for (int c = 0; c < 8; c++) {
                char ch = board[r][c];
                System.out.print(" " + (ch == '\0' ? '.' : ch) + " |");
            }
            System.out.println();
            System.out.println("  +---+---+---+---+---+---+---+---+");
        }
        System.out.println("    a   b   c   d   e   f   g   h  ");
    }
}
