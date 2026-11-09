import os

import pygame
import argparse

from .app import App
from .scenes.connect import ConnectScene
from .settings import Settings
from .styles import style_names
from .networking import NetworkClient, HOSTED_SERVER

SCREEN_WIDTH, SCREEN_HEIGHT = 1280, 720  # initial window size; the window can be resized
SCREEN_CAPTION = "Chess"


def main() -> None:
    parser = argparse.ArgumentParser(description="Play chess!")
    parser.add_argument("-s", "--session", type=str, default=None, help="Session ID of the chess session to join (implies --auto)")
    parser.add_argument("--style", type=str, default=None, help=f"Board style for this run: {', '.join(style_names())}")
    parser.add_argument("--server", type=str, default=None, help="Server for this run: host:port for TCP, or a ws:// or wss:// URL (default: the last one used)")
    parser.add_argument("--hosted", action="store_true", help="Use the hosted server (a shortcut for --server with its URL)")
    parser.add_argument("--name", type=str, default=None, help="Display name (default: the last one used)")
    parser.add_argument("--auto", action="store_true", help="Connect, join a session and ready up without any input")
    parser.add_argument("--debug", action="store_true", help="Print button clicks and messages to and from the server")
    args = parser.parse_args()
    if args.debug:
        os.environ["RAMCHESS_DEBUG"] = "1"
 
    settings = Settings.load()
    settings.server = HOSTED_SERVER if args.hosted else (args.server or settings.server)
    settings.display_name = args.name or settings.display_name
    if args.style:
        if args.style.upper() in style_names():
            settings.style = args.style.upper()
        else:
            parser.error(f"unknown style {args.style!r}; choose from {', '.join(style_names())}")
 
    pygame.init()
    pygame.display.set_caption(SCREEN_CAPTION)
    screen = pygame.display.set_mode((SCREEN_WIDTH, SCREEN_HEIGHT), pygame.RESIZABLE)
    app = App(settings, NetworkClient(), screen, auto=args.auto or args.session is not None, auto_session=args.session)
    app.push(ConnectScene(app))
    app.run()
 
 
if __name__ == "__main__":
    main()
