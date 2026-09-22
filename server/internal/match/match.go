package match

import (
	"context"
	"errors"
	"log"
	"runtime/debug"

	"github.com/stevenlagoy/ramserver-core/server/internal/game"
	"github.com/stevenlagoy/ramserver-core/server/internal/transport"
)

type Match struct {
	game   game.Game
	inbox  chan game.Action
	join   chan Member
	leave  chan Member
	joined map[string]Member
}

type Member struct {
	ID     string
	Sender game.Sender
}

func (m Member) Send(message []byte) {
	m.Sender.Send(message)
}

func NewMatch(g game.Game) *Match {
	return &Match{
		game:   g,
		inbox:  make(chan game.Action, 64),
		join:   make(chan Member),
		leave:  make(chan Member),
		joined: make(map[string]Member),
	}
}

func (m *Match) Inbox() chan<- game.Action { return m.inbox }

func (m *Match) Register(ctx context.Context, member Member) bool {
	select {
	case m.join <- member:
		return true
	case <-ctx.Done():
		return false
	}
}

func (m *Match) Unregister(ctx context.Context, member Member) {
	select {
	case m.leave <- member:
	case <-ctx.Done():
	}
}

func (m *Match) Run(ctx context.Context) {
	for {
		select {
		case member := <-m.join:
			m.joined[member.ID] = member
		case member := <-m.leave:
			if m.joined[member.ID] == member { // Ignore stale leave from replaced connection
				delete(m.joined, member.ID)
			}
		case action := <-m.inbox:
			m.handle(action)
		case <-ctx.Done():
			return
		}
	}
}

func (m *Match) handle(action game.Action) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("game panic on action: %d: %v\n%s", action.ID, r, debug.Stack())
			action.From.Send(transport.EncodeReject(action.ID, errors.New("internal error")))
		}
	}()
	if err := m.game.Validate(action); err != nil {
		action.From.Send(transport.EncodeReject(action.ID, err))
		return
	}
	m.game.Apply(action)
	m.broadcast(transport.EncodeState(m.game.State()))
}

func (m *Match) broadcast(message []byte) {
	for _, member := range m.joined {
		member.Send(message) // shared slice: read-only
	}
}
