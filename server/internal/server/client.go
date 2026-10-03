package server

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"runtime/debug"
	"sync"
	"syscall"
	"time"

	"github.com/stevenlagoy/ramserver-core/server/internal/game"
	"github.com/stevenlagoy/ramserver-core/server/internal/session"
	"github.com/stevenlagoy/ramserver-core/server/internal/transport"
)

// Internal model of clients held by server
type Client struct {
	connection    net.Conn
	out           chan []byte // buffered: outQueueSize messages. Only writeLoop may call transport.WriteFrame on connection; send messages through this channel instead of writing directly.
	id            string      // placeholder: player/session ID
	match         *match.Match
	config        ServerConfig
	games         *game.Registry
	sessions      *session.SessionStore
	authenticated bool             // true after a successful handshake; readLoop rejects actions while false
	session       *session.Session // nil until CreateLobby or JoinLobby succeed
}

func NewClient(conn net.Conn, match *match.Match, config ServerConfig) *Client {
	return &Client{
		connection: conn,
		out:        make(chan []byte, config.OutQueueSize),
		id:         conn.RemoteAddr().String(),
		match:      match,
		config:     config,
		sessions:   sessions,
	}
}

// Send a message to this client.
// Send does not block flow; clients with full queues are dropped to avoid stalling broadcast
func (c *Client) Send(message []byte) {
	select {
	case c.out <- message:
	default:
		log.Printf("%s: send queue full, dropping client", c.id)
		c.connection.Close() // Too slow; drop to avoid stalling broadcast
	}
}

func recoverPanic(where string) {
	if r := recover(); r != nil {
		log.Printf("panic in %s: %v\n%s", where, r, debug.Stack())
	}
}

// Activates this client and begins the read and write loops
func (c *Client) Run(ctx context.Context) {
	defer recoverPanic(("client " + c.id)) // Runs last

	connCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	defer wg.Wait() // Runs third after writeLoop finished
	defer cancel()  // Runs second

	// Closing connection unblocks pending reads and writes
	go func() {
		<-connCtx.Done()
		c.connection.Close()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer recoverPanic("writeLoop " + c.id)
		defer cancel() // write failure ends whole client
		c.writeLoop(connCtx)
	}()

	c.readLoop(connCtx)
}

func (c *Client) readLoop(ctx context.Context) {
	reader := bufio.NewReader(c.connection)
	receivedFirst := false
	for {
		deadline := c.config.ReadTimeout
		if !receivedFirst {
			deadline = c.config.FirstMessageTimeout
		}
		c.connection.SetReadDeadline(time.Now().Add(deadline))
		frame, err := transport.ReadFrame(reader)
		if err != nil {
			switch {
			case errors.Is(err, io.EOF), errors.Is(err, net.ErrClosed):
				// Normal disconnect or shutdown: no log
			case errors.Is(err, syscall.ECONNRESET):
				// Client disconnected abruptly (happens in test teardowns): no log
			case errors.Is(err, os.ErrDeadlineExceeded):
				log.Printf("%s: read timeout", c.id)
			default:
				log.Printf("%s: read error: %v", c.id, err)
			}
			return
		}
		if len(frame) == 0 {
			continue // Heartbeat
		}
		receivedFirst = true

		if !c.authenticated {
			newID, err := transport.DecodeHello(frame)
			if err != nil {
				c.Send(transport.EncodeReject(0, err))
				continue // Give the client another chance
			}
			c.id = newID
			c.authenticated = true

			member := match.Member{ID: c.id, Sender: c}
			if !c.match.Register(ctx, member) {
				return
			}
			defer c.match.Unregister(ctx, member) // runs when readLoop returns; safe, at most once per connection

			c.Send(transport.EncodeWelcome(c.id))
			continue // Hello frame is not an action
		}
		c.dispatch(ctx, frame)
	}
}

func (c *Client) writeLoop(ctx context.Context) {
	ping := time.NewTicker(c.config.PingInterval)
	defer ping.Stop()
	for {
		var payload []byte // nil payload = zero-length heartbeat frame
		select {
		case payload = <-c.out:
		case <-ping.C:
		case <-ctx.Done():
			return
		}
		c.connection.SetWriteDeadline(time.Now().Add(c.config.WriteTimeout))
		if err := transport.WriteFrame(c.connection, payload); err != nil {
			if !errors.Is(err, net.ErrClosed) {
				log.Printf("%s: write error: %v", c.id, err)
			}
			return
		}
	}
}

func (c *Client) dispatch(ctx context.Context, frame []byte) {
	msg, err := transport.DecodeClientMessage(frame)
	if err != nil {
		c.Send(transport.EncodeReject(0, err))
		return
	}
	switch msg.Kind {
	case transport.KindCreateLobby:
		c.handleCreateLobby(ctx, msg)
	case transport.KindJoinLobby:
		c.handleJoinLobby(ctx, msg)
	case transport.KindSetReady:
		if c.session != nil {
			c.session.SetReady(ctx, c.ID, msg.Ready)
		}
	case transport.KindSubmitAction:
		if c.session == nil {
			c.Send(transport.EncodeReject(msg.Action.ID, errNotInSession))
			return
		}
		action := msg.Action
		action.ActorID = c.ID // Don't trust IDs claimed by frame
		c.session.Submit(ctx, action)
	case transport.KindRequestLegalMoves:
		if c.session != nil {
			c.Send(transport.EncodeLegalMoves(c.session.LegalMoves(ctx, c.ID)))
		}
	case transport.KindLeaveLobby:
		if c.session != nil {
			c.session.Leave(ctx, c.ID)
			c.session = nil
		}
	default:
		c.Send(transport.EncodeReject(0, fmt.Errorf("unknown message kind %v", msg.Kind)))
	}
}

func (c *Client) handleCreateLobby(ctx context.Context, msg transport.ClientMessage) {
	g, err := c.games.New(msg.GameName)
	if err != nil {
		c.Send(transport.EncodeReject(0, err))
		return
	}
	s, err := c.sessions.Create(ctx, newSessionID(), msg.GameName, g, msg.Public, c.ID, c)
	if err != nil {
		c.Send(transport.EncodeReject(0, err))
		return
	}
	c.session = s
}

func (c *Client) handleJoinLobby(ctx context.Context, msg transport.ClientMessage) {
	s, ok := c.sessions.Get(msg.SessionID)
	if !ok {
		c.Send(transport.EncodeReject(0, errSessionNotFound))
		return
	}
	if err := s.Join(ctx, c.ID, c); err != nil {
		c.Send(transport.EncodeReject(0, err))
		return
	}
	c.session = s
}

// Placeholder ID scheme. TODO: replace based on protocol
func newSessionID() string {
	return fmt.Sprintf("session-%d", time.Now().UnixNano())
}
