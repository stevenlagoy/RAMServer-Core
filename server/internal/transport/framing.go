// Package transport handles message framing over TCP stream
package transport

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
)

const MaxFrame = 64 << 10

// Transport contract: 4-byte big-endian length prefix, then payload.
// Zero-length frame is a heartbeat in both directions and not delivered to game logic.

func ReadFrame(r *bufio.Reader) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(header[:])
	if n > MaxFrame {
		return nil, fmt.Errorf("Frame too large: %d", n)
	}
	buffer := make([]byte, n)
	if _, err := io.ReadFull(r, buffer); err != nil {
		return nil, err
	}
	return buffer, nil
}

// writeFrame sends header and payload in single Write call
// Does not modify payload, so slice is shared among clients.

func WriteFrame(w io.Writer, payload []byte) error {
	if len(payload) > MaxFrame {
		return fmt.Errorf("frame too large: %d", len(payload))
	}
	buf := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(buf, uint32(len(payload)))
	copy(buf[4:], payload)
	_, err := w.Write(buf)
	return err
}
