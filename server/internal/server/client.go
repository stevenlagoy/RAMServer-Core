package server

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log"
	"net"
	"os"
	"runtime/debug"
	"sync"
	"syscall"
	"time"

	"github.com/stevenlagoy/ramserver-core/server/internal/match"
	"github.com/stevenlagoy/ramserver-core/server/internal/transport"
)

type Client struct {
	connection    net.Conn
	out           chan []byte // buffered: outQueueSize messages. Only writeLoop may call transport.WriteFrame on connection; send messages through this channel instead of writing directly.
	id            string      // placeholder: player/session ID
	match         *match.Match
	config        ServerConfig
	authenticated bool // true after a successful handshake; readLoop rejects actions while false
}

func NewClient(conn net.Conn, match *match.Match, config ServerConfig) *Client {
	return &Client{
		connection: conn,
		out:        make(chan []byte, config.OutQueueSize),
		id:         conn.RemoteAddr().String(),
		match:      match,
		config:     config,
	}
}

// send never blocks. A client with full queue is dropped so it can't stall a broadcast
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
				continue // Give the client another  chance
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

		action, err := transport.DecodeAction(c, frame)
		if err != nil {
			c.Send(transport.EncodeReject(0, err))
			continue
		}
		select {
		case c.match.Inbox() <- action:
		case <-ctx.Done():
			return
		}
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
