package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
)

const defaultAddr = "127.0.0.1:9000"

func writeFrame(w io.Writer, payload []byte) error {
	buf := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(buf, uint32(len(payload)))
	copy(buf[4:], payload)
	_, err := w.Write(buf)
	return err
}

func writeHello(conn net.Conn) error {
	return writeFrame(conn, []byte(`{"token": "hello", "content": "world"}`))
}

func readFrame(r io.Reader) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	payload := make([]byte, binary.BigEndian.Uint32(header[:]))
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func main() {
	addr := os.Getenv("RAMSERVER_ADDR")
	if addr == "" {
		addr = defaultAddr
	}

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		fmt.Println("Error connecting to server: ", err)
		return
	}
	defer conn.Close()

	if err := writeHello(conn); err != nil {
		fmt.Println("Error sending data to server: ", err)
		return
	}

	reader := bufio.NewReader(conn)
	for {
		frame, err := readFrame(reader)
		if err != nil {
			fmt.Println("Connection closed: ", err)
			return
		}
		if len(frame) == 0 {
			fmt.Println("ping received; replying")
			writeFrame(conn, nil) // heartbeat reply keeps the server's read deadline alive
			continue
		}
		fmt.Printf("Received: %s\n", frame)
	}
}
