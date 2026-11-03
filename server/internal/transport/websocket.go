package transport

import (
	"bytes"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WebSocketNetConn adapts a WebSocket connection to a byte-stream net.Conn
type WebSocketNetConn struct {
	conn    *websocket.Conn
	readMu  sync.Mutex
	writeMu sync.Mutex
	reader  *bytes.Reader
}

var _ net.Conn = (*WebSocketNetConn)(nil)

func NewWebSocketNetConn(conn *websocket.Conn) *WebSocketNetConn {
	conn.SetReadLimit(64<<10 + 4)
	return &WebSocketNetConn{conn: conn}
}

func (c *WebSocketNetConn) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	c.readMu.Lock()
	defer c.readMu.Unlock()

	for {
		if c.reader != nil && c.reader.Len() > 0 {
			return c.reader.Read(p)
		}
		messageType, message, err := c.conn.ReadMessage()
		if err != nil {
			return 0, err
		}
		if messageType != websocket.BinaryMessage {
			return 0, fmt.Errorf("expected binary WebSocket message, got type %d", messageType)
		}
		c.reader = bytes.NewReader(message)
	}
}

func (c *WebSocketNetConn) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if err := c.conn.WriteMessage(websocket.BinaryMessage, p); err != nil {
		return 0, err
	}

	return len(p), nil
}

func (c *WebSocketNetConn) Close() error {
	_ = c.conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		time.Now().Add(time.Second),
	)
	return c.conn.Close()
}

func (c *WebSocketNetConn) LocalAddr() net.Addr {
	return c.conn.NetConn().LocalAddr()
}

func (c *WebSocketNetConn) RemoteAddr() net.Addr {
	return c.conn.NetConn().RemoteAddr()
}

func (c *WebSocketNetConn) SetDeadline(t time.Time) error {
	if err := c.conn.SetReadDeadline(t); err != nil {
		return err
	}
	return c.conn.SetWriteDeadline(t)
}

func (c *WebSocketNetConn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

func (c *WebSocketNetConn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}
