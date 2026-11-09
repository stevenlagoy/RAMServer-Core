import pygame

from .scenes.base import Scene
from .networking import Event, NetworkClient
from .settings import Settings
from .styles import BoardStyle, resolve_style
from .ui import BACKGROUND, Toasts


MIN_WIDTH, MIN_HEIGHT = 320, 320


class App:
    def __init__(
        self, settings: Settings, net: NetworkClient, screen: pygame.Surface,
        *, auto: bool = False, auto_session: str | None = None
    ) -> None:
        self.settings = settings
        self.net = net
        self.screen = screen
        self.auto = auto  # connect and join a session and ready up automatically
        self.auto_session = auto_session
        self.clock = pygame.time.Clock()
        self.toasts = Toasts()
        self.scenes: list[Scene] = []
        self.running = True

    @property
    def style(self) -> BoardStyle:
        return resolve_style(self.settings.style, self.settings.custom_colors)

    def push(self, scene: Scene) -> None:
        self.scenes.append(scene)
        scene.ensure_layout(self.screen)  # so a click which arrives before the first frame lands on the right widget
        scene.on_enter()

    def pop(self) -> None:
        if self.scenes:
            self.scenes.pop()
        if self.scenes:
            self.scenes[-1].on_resume()

    def replace(self, scene: Scene) -> None:
        """Swap the top scene, which is how the lobby becomes a match."""
        if self.scenes:
            self.scenes.pop()
        self.push(scene)
 
    def reset(self, scene: Scene) -> None:
        self.scenes.clear()
        self.push(scene)
 
    def show_connect(self, status: str = "") -> None:
        from .scenes.connect import ConnectScene  # imported here because the scenes import each other
        self.net.close()
        self.auto = False  # an automatic session ended: do not loop
        self.reset(ConnectScene(self, status))
 
    def toast(self, text: str, *, error: bool = False) -> None:
        self.toasts.add(text, error=error)
 
    def open_settings(self) -> None:
        from .scenes.settings_scene import SettingsScene
        self.push(SettingsScene(self))

    def _base_index(self) -> int:
        """Index of the lowest scene which still needs drawing: the topmost scene which is not an overlay."""
        for index in range(len(self.scenes) - 1, -1, -1):
            if not self.scenes[index].overlay:
                return index
        return 0
 
    def _dispatch(self, event: pygame.event.Event) -> None:
        if event.type == pygame.QUIT:
            self.running = False
        elif event.type == pygame.VIDEORESIZE:
            if event.w < MIN_WIDTH or event.h < MIN_HEIGHT:
                pygame.display.set_mode((max(event.w, MIN_WIDTH), max(event.h, MIN_HEIGHT)), pygame.RESIZABLE)
            self.screen = pygame.display.get_surface()
            for scene in list(self.scenes):
                scene.on_resize(self.screen.get_size())
        elif self.scenes:
            self.scenes[-1].handle_event(event)
 
    def _dispatch_net(self, event: Event) -> None:
        if self.scenes:
            self.scenes[self._base_index()].on_net_event(event)
 
    def frame(self, dt: float) -> None:
        """Run one frame: input, server events, update, draw."""
        for event in pygame.event.get():
            self._dispatch(event)
        for server_event in self.net.poll():
            self._dispatch_net(server_event)
        self.screen = pygame.display.get_surface() or self.screen
        visible = list(self.scenes[self._base_index():])
        for scene in visible:
            scene.update(dt)
        self.toasts.update(dt)
        self.screen.fill(BACKGROUND)
        for scene in visible:
            scene.ensure_layout(self.screen)
            scene.draw(self.screen)
        self.toasts.draw(self.screen)
        pygame.display.flip()
 
    def run(self) -> None:
        while self.running:
            self.frame(self.clock.tick(60) / 1000)
        self.settings.save()
        self.net.close(wait=0.5)
        pygame.quit()
 
