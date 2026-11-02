package match

import "github.com/stevenlagoy/ramserver-core/server/internal/server"

type Turn struct {
	player server.Client
	action any
}
