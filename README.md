# Pychess

Pychess is a client created by @jonehayd and @stevenlagoy with Python to play chess with RAMServer.

### Creators

This client was created by @jonehayd and @stevenlagoy.

This client demonstrates the first reference game for RAMServer-Core and was created concurrently with JVM-Chess, though the teams were indepdendent when creating their clients.

**@stevenlagoy / Steven LaGoy** is the lead developer of RAMServer-Core.

**Hayden Jones / Hayden Jones** is the protocol designer of RAMServer-Core.

### Games

This client can play chess version 1.0 using the basic ruleset and protocol version 1.2.

### Technologies

This client was implemented in Python using Pygame for UI and rendering.

It uses the `websocket` library for networking and Google's `protobuf` library for protocol communication.

The client was tested with pytest

## How To Use

### Requirements

You must have Python 3.12+ on your machine. Other requirements are found in `clients/pychess/requirements.txt`, which can be installed using the commands given in "Execution" below.

### Execution

Start by generating the required protocol file(s) by running the following in the root directory:

```
buf generate
```

The client can be run by running `main.py` in the `clients/pychess/` directory:

```
cd clients/pychess
pip install -r requirements.txt
python -m main
```

This will open the client UI. Commands are identical on any platform.

### Connection

On the screen which opens when the client starts, determine whether you want to connect to a local server over TCP or to a hosted server using a websocket URL. Select the appropriate button "Local server" or "Hosted server". After this, enter the address and port for a local server, or the websocket url (like "wss://my-url.etc/connect") for a hosted server. Enter a display name for yourself and press "Connect". If the server is found, you will be sent to the "Chess lobby" screen.

You can create a public session using the "Create lobby" button, or a private session with "Create private lobby". "Quick match" will place you in an open session and mark you as ready if any exist, or will create a new session for you if there are none. To join a specific lobby, put its session ID or join token in the "Lobby ID" box and then press "Join lobby by ID". You can also click "Join" on one of the open lobbies on the right to join it.

Once in a lobby, you will see the ID at the top left, and connected players and roles on the left. To mark yourself as ready or not ready, press "Ready" or "Cancel ready" at the bottom. You can disconnect from the lobby by pressing "Leave lobby", and can disconnect fully from the server with the "Disconnect" button on the top right. If any matches have been played, you can see their results and some details on the right side.

Once two players are present in a session and both are ready, a game of chess will start.

### Play

Once a game has started, both players will see a view of the chess board from their side. The player to move will have a green bar on their side of the board, and will be shown with the green "To move" marker on the right.

To make a move, click on the piece you'd like to place, and then on the square you want to move it to. Alternately, click and drag the piece to its new location. Valid squares for movement are shown with a green marker. Special movements like castling and en passant work the same way.

To capture, drag or click with your capturing piece on top of the opponent's piece.

When a king is in check, red markers will indicate the attacking piece. If your king is in check, squares which are being attacked will be marked with a red X.

When promoting, the possible pieces will be displayed on top of your pawn. Click one of these piece types to promote to that type.

You may offer a draw after completing any move, which will show a message "[player] offers a draw" to the other player which can be accepted or declined. If both players accept, the game will end in a tie. Otherwise, play continues. You may also resign the game, which causes you to instantly lose the game. This also happens if you disconnect from the session during an ongoing match.

When a player wins by checkmate, ties by stalemate, accepts a draw, or resigns, a popup will display whether you won, tied, or lost, and the reason for the game ending. You may press "Play again" to offer a rematch (which marks you as ready in the lobby), and other players will see the message "You opponent wants to play again" and can either start a new game immediately or return to the lobby. 

There are some configurable settings accessible with the "Settings" button, either from the game screen or the session screen. These include how the board appears, whether board coordinates and other effects are shown, the speed of animations, and visual effects for kings in check. 

### Disconnection and Shutdown

When in a game, press "Resign" to return to the lobby screen. To leave a session, press "Leave lobby" on the lobby screen.

To disconnect from the server, press the "Disconnect" button on the top right of the lobby screen.

To close the client completely, close the window as for any other application. Note that this may take a few seconds when connected to a hosted server and in a session or ongoing match.

## Features

User customization was a priority for how the game screen looks. Users may select from several premade color schemes or may choose their own colors for light and dark squares on the game board.

Most of our extra time was spent on the animations for pieces moving, as well as the visual effects for valid moves and kings in check.