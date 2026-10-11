package main

import (
	"bufio"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	pb "github.com/stevenlagoy/ramserver-core/server/gen/ramserver/v1"
	"github.com/stevenlagoy/ramserver-core/server/internal/transport"
)

const (
	defaultAddr = "127.0.0.1:9000"
	maxFrame    = 64 << 10
)

const helpText = `Commands:
  connect [protocol_version]               send ConnectRequest
  auth <display_name> [player_id token]    send AuthRequest
  create <game_id>                         create a session (JoinSessionRequest without session_id)
  join <game_id> <session_id>              join an existing session
  leave [session_id]                       leave a session (default: current session)
  action <payload>                         send ActionRequest to the current session
  raw <text>                               send text as an arbitrary frame (not protobuf)
  help                                     show this list`

func writeFrame(w io.Writer, payload []byte) error {
	if len(payload) > maxFrame {
		return fmt.Errorf("frame too large: %d bytes (max %d)", len(payload), maxFrame)
	}

	buf := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(buf, uint32(len(payload)))
	copy(buf[4:], payload)
	_, err := w.Write(buf)
	return err
}

func readFrame(r io.Reader) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}

	n := binary.BigEndian.Uint32(header[:])
	if n > maxFrame {
		return nil, fmt.Errorf("frame too large: %d bytes (max %d)", n, maxFrame)
	}

	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func getTime() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

// Short label for a client message, shown in the log
func clientMessageKind(msg *pb.ClientMessage) string {
	switch msg.GetPayload().(type) {
	case *pb.ClientMessage_ConnectRequest:
		return "CONNECT"
	case *pb.ClientMessage_AuthRequest:
		return "AUTH"
	case *pb.ClientMessage_JoinSessionRequest:
		return "JOIN"
	case *pb.ClientMessage_LeaveSessionRequest:
		return "LEAVE"
	case *pb.ClientMessage_ActionRequest:
		return "ACTION"
	default:
		return "UNKNOWN"
	}
}

// Short label for a server message, shown in the log
func serverMessageKind(msg *pb.ServerMessage) string {
	switch msg.GetPayload().(type) {
	case *pb.ServerMessage_ConnectResponse:
		return "CONNECTED"
	case *pb.ServerMessage_AuthResponse:
		return "AUTHED"
	case *pb.ServerMessage_StateUpdate:
		return "STATE"
	case *pb.ServerMessage_ErrorResponse:
		return "ERROR"
	case *pb.ServerMessage_MatchResult:
		return "RESULT"
	case *pb.ServerMessage_LeaveSessionResponse:
		return "LEFT"
	default:
		return "UNKNOWN"
	}
}

func logLine(arrow, kind, body string) {
	fmt.Printf("[%s] %s %-9s %s\n", getTime(), arrow, kind, body)
}

// Builds the message for one input line. currentSession fills in leave and
// action when the line names no session. Exactly one of msg and raw is set.
func parseCommand(line, currentSession string) (msg *pb.ClientMessage, raw []byte, err error) {
	cmd, rest, _ := strings.Cut(strings.TrimSpace(line), " ")
	rest = strings.TrimSpace(rest)
	args := strings.Fields(rest)

	switch cmd {
	case "connect":
		version := transport.ProtocolVersion
		if len(args) > 0 {
			version = args[0]
		}
		return &pb.ClientMessage{Payload: &pb.ClientMessage_ConnectRequest{ConnectRequest: &pb.ConnectRequest{
			ProtocolVersion: version,
			ClientName:      "leanclient",
		}}}, nil, nil
	case "auth":
		if len(args) != 1 && len(args) != 3 {
			return nil, nil, errors.New("usage: auth <display_name> [player_id token]")
		}
		req := &pb.AuthRequest{DisplayName: args[0]}
		if len(args) == 3 {
			req.PlayerId, req.ReconnectToken = args[1], args[2]
		}
		return &pb.ClientMessage{Payload: &pb.ClientMessage_AuthRequest{AuthRequest: req}}, nil, nil
	case "create":
		if len(args) != 1 {
			return nil, nil, errors.New("usage: create <game_id>")
		}
		return &pb.ClientMessage{Payload: &pb.ClientMessage_JoinSessionRequest{JoinSessionRequest: &pb.JoinSessionRequest{
			GameId: args[0],
		}}}, nil, nil
	case "join":
		if len(args) != 2 {
			return nil, nil, errors.New("usage: join <game_id> <session_id>")
		}
		return &pb.ClientMessage{Payload: &pb.ClientMessage_JoinSessionRequest{JoinSessionRequest: &pb.JoinSessionRequest{
			GameId:    args[0],
			SessionId: args[1],
		}}}, nil, nil
	case "leave":
		sessionID := currentSession
		if len(args) > 0 {
			sessionID = args[0]
		}
		if sessionID == "" {
			return nil, nil, errors.New("not in a session; use: leave <session_id>")
		}
		return &pb.ClientMessage{Payload: &pb.ClientMessage_LeaveSessionRequest{LeaveSessionRequest: &pb.LeaveSessionRequest{
			SessionId: sessionID,
		}}}, nil, nil
	case "action":
		if rest == "" {
			return nil, nil, errors.New("usage: action <payload>")
		}
		if currentSession == "" {
			return nil, nil, errors.New("not in a session; create or join one first")
		}
		return &pb.ClientMessage{Payload: &pb.ClientMessage_ActionRequest{ActionRequest: &pb.ActionRequest{
			SessionId:     currentSession,
			EncodedAction: []byte(rest),
		}}}, nil, nil
	case "raw":
		if rest == "" {
			return nil, nil, errors.New("usage: raw <text>; zero-length frames are reserved for heartbeats")
		}
		return nil, []byte(rest), nil
	default:
		return nil, nil, fmt.Errorf("unknown command %q; type help for the list", cmd)
	}
}

// One connection to the server. The receive goroutine and the input loop both
// write (heartbeat replies and commands), so writes are serialized by writeMu.
type client struct {
	conn    net.Conn
	reader  *bufio.Reader
	writeMu sync.Mutex
	seq     uint64 // last sequence sent; only the sending goroutine touches it

	mu        sync.Mutex
	sessionID string // learned from StateUpdate, cleared by LeaveSessionResponse
}

func newClient(conn net.Conn) *client {
	return &client{conn: conn, reader: bufio.NewReader(conn)}
}

func (c *client) currentSession() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessionID
}

func (c *client) writeRaw(payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return writeFrame(c.conn, payload)
}

// Assigns the next sequence number to msg and sends it
func (c *client) send(msg *pb.ClientMessage) error {
	c.seq++
	msg.Sequence = c.seq
	frame, err := proto.Marshal(msg)
	if err != nil {
		return err
	}
	if err := c.writeRaw(frame); err != nil {
		return err
	}
	logLine("->", clientMessageKind(msg), prototext.MarshalOptions{}.Format(msg))
	return nil
}

func (c *client) sendRaw(payload []byte) error {
	if err := c.writeRaw(payload); err != nil {
		return err
	}
	logLine("->", "RAW", fmt.Sprintf("%q", payload))
	return nil
}

// Waits for the next server message, answering heartbeats along the way so the
// server's read deadline stays alive
func (c *client) next() (*pb.ServerMessage, error) {
	for {
		frame, err := readFrame(c.reader)
		if err != nil {
			return nil, err
		}
		if len(frame) == 0 {
			if err := c.writeRaw(nil); err != nil {
				return nil, fmt.Errorf("reply to heartbeat: %w", err)
			}
			continue
		}
		var msg pb.ServerMessage
		if err := proto.Unmarshal(frame, &msg); err != nil {
			logLine("<-", "UNDECODED", fmt.Sprintf("%q", frame))
			continue
		}
		c.track(&msg)
		logLine("<-", serverMessageKind(&msg), prototext.MarshalOptions{}.Format(&msg))
		return &msg, nil
	}
}

func (c *client) track(msg *pb.ServerMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if s := msg.GetStateUpdate(); s != nil {
		c.sessionID = s.GetSessionId()
	}
	if l := msg.GetLeaveSessionResponse(); l != nil && l.GetSessionId() == c.sessionID {
		c.sessionID = ""
	}
}

// Sends msg and waits for the next server message, which during the handshake is its reply
func (c *client) request(msg *pb.ClientMessage) (*pb.ServerMessage, error) {
	if err := c.send(msg); err != nil {
		return nil, err
	}
	return c.next()
}

// Runs ConnectRequest then AuthRequest, returning the issued player_id
func (c *client) handshake(version, displayName string) (string, error) {
	connect, _, _ := parseCommand("connect "+version, "")
	resp, err := c.request(connect)
	if err != nil {
		return "", err
	}
	if e := resp.GetErrorResponse(); e != nil {
		return "", fmt.Errorf("connect rejected: %s: %s", e.GetCode(), e.GetMessage())
	}
	resp, err = c.request(&pb.ClientMessage{Payload: &pb.ClientMessage_AuthRequest{AuthRequest: &pb.AuthRequest{DisplayName: displayName}}})
	if err != nil {
		return "", err
	}
	if e := resp.GetErrorResponse(); e != nil {
		return "", fmt.Errorf("auth rejected: %s: %s", e.GetCode(), e.GetMessage())
	}
	return resp.GetAuthResponse().GetPlayerId(), nil
}

// Parses and sends one input line
func (c *client) runCommand(line string) error {
	msg, raw, err := parseCommand(line, c.currentSession())
	if err != nil {
		return err
	}
	if raw != nil {
		return c.sendRaw(raw)
	}
	return c.send(msg)
}

func (c *client) receiveLoop() error {
	for {
		if _, err := c.next(); err != nil {
			return err
		}
	}
}

func main() {
	addrDefault := os.Getenv("RAMSERVER_ADDR")
	if addrDefault == "" {
		addrDefault = defaultAddr
	}

	addr := flag.String("addr", addrDefault, "server address")
	name := flag.String("name", "leanclient", "display name sent in AuthRequest")
	version := flag.String("version", transport.ProtocolVersion, "protocol version sent in ConnectRequest")
	handshake := flag.Bool("handshake", true, "send ConnectRequest and AuthRequest automatically on connect")
	send := flag.String("send", "", "run one command after the handshake, print one response, then exit")
	flag.Parse()

	conn, err := net.Dial("tcp", *addr)
	if err != nil {
		fmt.Println("Error connecting to server:", err)
		return
	}
	defer conn.Close()

	fmt.Println("Connected to", *addr)
	c := newClient(conn)

	if *handshake {
		if _, err := c.handshake(*version, *name); err != nil {
			fmt.Println("Handshake failed:", err)
			return
		}
	}

	if *send != "" {
		if err := c.runCommand(*send); err != nil {
			fmt.Println("Error:", err)
			return
		}
		if _, err := c.next(); err != nil {
			fmt.Println("Connection closed while waiting for response:", err)
		}
		return
	}

	fmt.Println("Type a command, or help for the list. Exit with Ctrl+C or EOF.")

	receiveDone := make(chan error, 1)
	go func() {
		receiveDone <- c.receiveLoop()
	}()

	input := make(chan string)
	inputDone := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Buffer(make([]byte, 1024), maxFrame+1)
		for scanner.Scan() {
			input <- scanner.Text()
		}
		close(input)
		inputDone <- scanner.Err()
	}()

	for {
		select {
		case line, ok := <-input:
			if !ok {
				if err := <-inputDone; err != nil {
					fmt.Println("Error reading stdin:", err)
				}
				return
			}
			switch strings.TrimSpace(line) {
			case "":
				continue
			case "help":
				fmt.Println(helpText)
				continue
			}
			if err := c.runCommand(line); err != nil {
				fmt.Println("Error:", err)
			}
		case err := <-receiveDone:
			if err != nil {
				fmt.Println("Connection closed:", err)
			}
			return
		}
	}
}
