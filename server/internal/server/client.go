package server

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
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
	ID            string      // exported: persists across sessions/matches and used in lobby history
	config        ServerConfig
	games         *game.Registry
	sessions      *session.SessionStore
	DisplayName   string           // cosmetic label from AuthRequest; never used as identity
	connected     bool             // true after a successful ConnectRequest
	authenticated bool             // true after a successful AuthRequest; readLoop rejects session messages while false
	session       *session.Session // nil until a JoinSessionRequest succeeds
}

// Create a new client from the given connection, match, and server configuration
func NewClient(conn net.Conn, config ServerConfig, games *game.Registry, sessions *session.SessionStore) *Client {
	return &Client{
		connection: conn,
		out:        make(chan []byte, config.OutQueueSize),
		ID:         conn.RemoteAddr().String(), // placeholder until handshake assigns real id
		config:     config,
		games:      games,
		sessions:   sessions,
	}
}

var (
	errNotInSession    = transport.NewError(transport.CodeNotInSession, "not a member of that session")
	errSessionNotFound = transport.NewError(transport.CodeSessionNotFound, "session not found")
)

// Send a message to this client.
// Send does not block flow; clients with full queues are dropped to avoid stalling broadcast
func (c *Client) Send(message []byte) {
	select {
	case c.out <- message:
	default:
		log.Printf("%s: send queue full, dropping client", c.connection.RemoteAddr().String())
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
	defer recoverPanic(("client " + c.connection.RemoteAddr().String())) // Runs last

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
		defer recoverPanic("writeLoop " + c.connection.RemoteAddr().String())
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
				log.Printf("%s: read timeout", c.ID)
			default:
				log.Printf("%s: read error: %v", c.ID, err)
			}
			return
		}
		if len(frame) == 0 {
			continue // Heartbeat
		}
		receivedFirst = true

		msg, err := transport.DecodeClientMessage(frame)
		if err != nil {
			c.Send(transport.EncodeError(msg.Sequence, err)) // Sequence is 0 if the envelope did not parse
			continue
		}
		if !c.authenticated {
			c.handshake(msg)
			continue
		}
		c.dispatch(ctx, msg)
	}
}

// Handles one pre-authentication message: ConnectRequest, then AuthRequest
func (c *Client) handshake(msg transport.ClientMessage) {
	switch {
	case msg.Kind == transport.KindConnect && !c.connected:
		if !transport.CompatibleVersion(msg.ProtocolVersion) {
			c.Send(transport.EncodeError(msg.Sequence, transport.NewError(transport.CodeInvalidProtocolVersion,
				"client protocol %q is incompatible with server protocol %q", msg.ProtocolVersion, transport.ProtocolVersion)))
			c.closeAfterFlush()
			return
		}
		c.connected = true
		c.Send(transport.EncodeConnectResponse(msg.Sequence, c.connection.RemoteAddr().String()))
	case msg.Kind == transport.KindAuth && c.connected:
		if msg.DisplayName == "" {
			c.Send(transport.EncodeError(msg.Sequence, transport.NewError(transport.CodeInvalidArgument, "display_name is required")))
			return
		}
		if msg.PlayerID != "" || msg.ReconnectToken != "" {
			// TODO: resume identities once issued tokens are persisted
			c.Send(transport.EncodeError(msg.Sequence, transport.NewError(transport.CodeAuthFailed, "unknown player_id")))
			return
		}
		c.ID = newRandomID()
		c.DisplayName = msg.DisplayName
		c.authenticated = true
		// TODO: persist (c.ID, reconnectToken) so a later AuthRequest can resume this identity
		reconnectToken := newRandomID()
		c.Send(transport.EncodeAuthResponse(msg.Sequence, c.ID, reconnectToken, ""))
	default:
		c.Send(transport.EncodeError(msg.Sequence, transport.NewError(transport.CodeInvalidState,
			"%v not valid before authentication completes", msg.Kind)))
	}
}

// Gives writeLoop time to send queued replies, then closes the connection
func (c *Client) closeAfterFlush() {
	time.AfterFunc(c.config.CloseTimeout, func() { c.connection.Close() })
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
				log.Printf("%s: write error: %v", c.ID, err)
			}
			return
		}
	}
}

func (c *Client) dispatch(ctx context.Context, msg transport.ClientMessage) {
	switch msg.Kind {
	case transport.KindJoinSession:
		c.handleJoinSession(ctx, msg)
	case transport.KindLeaveSession:
		if c.session == nil || c.session.ID != msg.SessionID {
			c.Send(transport.EncodeError(msg.Sequence, errNotInSession))
			return
		}
		c.session.Leave(ctx, c.ID)
		c.session = nil
		c.Send(transport.EncodeLeaveSessionResponse(msg.Sequence, msg.SessionID))
	case transport.KindAction:
		if c.session == nil || c.session.ID != msg.SessionID {
			c.Send(transport.EncodeError(msg.Sequence, errNotInSession))
			return
		}
		// TODO: enforce msg.ExpectedStateVersion once sessions track state_version
		action := msg.Action
		action.ActorID = c.ID // Don't trust IDs claimed by frame
		c.session.Submit(ctx, action)
	case transport.KindConnect, transport.KindAuth:
		c.Send(transport.EncodeError(msg.Sequence, transport.NewError(transport.CodeInvalidState, "already authenticated")))
	default:
		c.Send(transport.EncodeError(msg.Sequence, transport.NewError(transport.CodeMalformedMessage, "unknown message kind %v", msg.Kind)))
	}
}

func (c *Client) handleJoinSession(ctx context.Context, msg transport.ClientMessage) {
	if c.session != nil {
		c.Send(transport.EncodeError(msg.Sequence, transport.NewError(transport.CodeAlreadyInSession, "already in session %q", c.session.ID)))
		return
	}
	if msg.GameID == "" {
		c.Send(transport.EncodeError(msg.Sequence, transport.NewError(transport.CodeInvalidArgument, "game_id is required")))
		return
	}
	// Create a new session
	if msg.SessionID == "" {
		// TODO: let the game's policy select an existing session before creating one
		g, err := c.games.New(msg.GameID)
		if err != nil {
			c.Send(transport.EncodeError(msg.Sequence, transport.WrapError(transport.CodeUnknownGame, err)))
			return
		}
		s, err := c.sessions.Create(ctx, newSessionID(), msg.GameID, g, true)
		if err != nil {
			c.Send(transport.EncodeError(msg.Sequence, err))
			return
		}
		// Joining like any other member assigns a role and replies with the creator's StateUpdate
		if err := s.Join(ctx, c.ID, c.DisplayName, msg.Sequence, game.Role(msg.RequestedRole), c); err != nil {
			c.sessions.Remove(s.ID) // never had a member
			c.Send(transport.EncodeError(msg.Sequence, err))
			return
		}
		c.session = s
		return
	}
	// Join an existing session
	s, ok := c.sessions.Get(msg.SessionID)
	if !ok || s.GameName != msg.GameID {
		c.Send(transport.EncodeError(msg.Sequence, errSessionNotFound))
		return
	}
	if err := s.Join(ctx, c.ID, c.DisplayName, msg.Sequence, game.Role(msg.RequestedRole), c); err != nil {
		c.Send(transport.EncodeError(msg.Sequence, err))
		return
	}
	c.session = s
}

func newSessionID() string {
	return "session-" + newRandomID()
}

// Unguessable identifier; also used for reconnect tokens, which must stay secret
func newRandomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("crypto/rand: %v", err))
	}
	return hex.EncodeToString(b[:])
}
