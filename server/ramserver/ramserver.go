package ramserver

import (
	"context"
	"net"

	"github.com/stevenlagoy/ramserver-core/server/game"
	"github.com/stevenlagoy/ramserver-core/server/internal/server"
)

/*
This is an embeddable facade for external developers to use the server. Game registries can be passed to Serve().
*/

type Config = server.ServerConfig

func DefaultConfig() server.ServerConfig { return server.DefaultConfig() }

func Serve(ctx context.Context, l net.Listener, cfg Config, games *game.Registry) error {
	return server.Serve(ctx, l, cfg, games)
}

/*
main.go in an external repository could look like this:

// my-repo/cmd/mygame/main.go
games := game.NewRegistry()
games.Register("mygame", func() game.Game { return &mygame.Game{} })
ramserver.Serve(ctx, listener, ramserver.DefaultConfig(), games)
*/