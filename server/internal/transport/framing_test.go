package transport

import (
	"bufio"
	"bytes"
	"testing"
)

func TestFrameRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
	}{
		{"heartbeat", nil},
		{"small", []byte("hello")},
		{"max size", make([]byte, MaxFrame)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteFrame(&buf, tt.payload); err != nil {
				t.Fatal(err)
			}
			got, err := ReadFrame(bufio.NewReader(&buf))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, tt.payload) {
				t.Fatalf("got %d bytes, want %d", len(got), len(tt.payload))
			}
		})
	}
}

func TestReadFrameRejectsOversize(t *testing.T) {
	r := bufio.NewReader(bytes.NewReader([]byte{0xFF, 0xFF, 0xFF, 0xFF}))
	if _, err := ReadFrame(r); err == nil {
		t.Fatal("expected error for oversized frame")
	}
}
