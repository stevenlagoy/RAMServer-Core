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

    public static void printBoard(String fenBoard) {
        char[][] board = parseFEN(fenBoard);
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

    public static char[][] parseFEN(String fen) {
        char[][] board = new char[8][8];
        if (fen == null || fen.isEmpty()) return board;

        String boardParts = fen.split(" ")[0];
        String[] ranks = boardParts.split("/");

        for (int r = 0; r < ranks.length; r++) {
            int c = 0;
            for (int i = 0; i < ranks[r].length(); i++) {
                char ch = ranks[r].charAt(i);
                if (Character.isDigit(ch)) {
                    int empty = Character.getNumericValue(ch);
                    c += empty;
                } else {
                    if (c < 8) {
                        board[r][c] = ch;
                        c++;
                    }
                }
            }
        }
        return board;
    }
}
