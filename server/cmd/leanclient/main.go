package main

import (
	"bufio"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
)

const (
	defaultAddr = "127.0.0.1:9000"
	maxFrame    = 64 << 10
)

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

func frameKind(frame []byte) string {
	text := string(frame)
	switch {
	case strings.HasPrefix(text, "welcome "):
		return "WELCOME"
	case strings.HasPrefix(text, "reject "):
		return "REJECT"
	case strings.HasPrefix(text, "state "):
		return "STATE"
	default:
		return "DATA"
	}
}

func logReceived(frame []byte) {
	fmt.Printf("<- %-9s %s\n", frameKind(frame), frame)
}

func sendFrame(conn net.Conn, kind string, payload []byte) error {
	if err := writeFrame(conn, payload); err != nil {
		return err
	}
	fmt.Printf("-> %-9s %s\n", kind, payload)
	return nil
}

// readApplicationFrame waits for the next non-heartbeat frame. Heartbeats are
// answered immediately so the server's read deadline remains alive.
func readApplicationFrame(conn net.Conn, reader *bufio.Reader) ([]byte, error) {
	for {
		frame, err := readFrame(reader)
		if err != nil {
			return nil, err
		}
		if len(frame) == 0 {
			fmt.Println("<- HEARTBEAT")
			if err := writeFrame(conn, nil); err != nil {
				return nil, fmt.Errorf("reply to heartbeat: %w", err)
			}
			fmt.Println("-> HEARTBEAT reply")
			continue
		}

		logReceived(frame)
		return frame, nil
	}
}

func receiveLoop(conn net.Conn, reader *bufio.Reader) error {
	for {
		if _, err := readApplicationFrame(conn, reader); err != nil {
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
	hello := flag.String("hello", "leanclient", "handshake frame payload")
	send := flag.String("send", "", "send one frame after the handshake, print one response, then exit")
	flag.Parse()

	conn, err := net.Dial("tcp", *addr)
	if err != nil {
		fmt.Println("Error connecting to server:", err)
		return
	}
	defer conn.Close()

	fmt.Println("Connected to", *addr)
	reader := bufio.NewReader(conn)

	if err := sendFrame(conn, "HELLO", []byte(*hello)); err != nil {
		fmt.Println("Error sending handshake:", err)
		return
	}

	// Wait for the server's handshake response before accepting action frames.
	response, err := readApplicationFrame(conn, reader)
	if err != nil {
		fmt.Println("Connection closed during handshake:", err)
		return
	}
	if frameKind(response) == "REJECT" {
		return
	}

	if *send != "" {
		if err := sendFrame(conn, "FRAME", []byte(*send)); err != nil {
			fmt.Println("Error sending frame:", err)
			return
		}
		if _, err := readApplicationFrame(conn, reader); err != nil {
			fmt.Println("Connection closed while waiting for response:", err)
		}
		return
	}

	fmt.Println("Enter frame payloads, one per line. Ctrl+C or EOF exits.")

	receiveDone := make(chan error, 1)
	go func() {
		receiveDone <- receiveLoop(conn, reader)
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
				err := <-inputDone
				if err != nil {
					fmt.Println("Error reading stdin:", err)
				}
				return
			}
			if line == "" {
				fmt.Println("Empty input skipped; zero-length frames are reserved for heartbeats.")
				continue
			}
			if err := sendFrame(conn, "FRAME", []byte(line)); err != nil {
				fmt.Println("Error sending frame:", err)
				return
			}
		case err := <-receiveDone:
			if err != nil {
				fmt.Println("Connection closed:", err)
			}
			return
		}
	}
}
