package main

import (
	"bytes"
	"testing"
)

func TestFrameKind(t *testing.T) {
	tests := []struct {
		name  string
		frame string
		want  string
	}{
		{"welcome", "welcome leanclient", "WELCOME"},
		{"reject", "reject invalid client", "REJECT"},
		{"state", "state 2", "STATE"},
		{"data", "anything else", "DATA"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := frameKind([]byte(tt.frame))
			if got != tt.want {
				t.Fatalf("frameKind(%q) = %q, want %q", tt.frame, got, tt.want)
			}
		})
	}
}

func TestFrameRoundTrip(t *testing.T) {
	want := []byte("hello server")

	var buf bytes.Buffer

	if err := writeFrame(&buf, want); err != nil {
		t.Fatalf("writeFrame() error = %v", err)
	}

	got, err := readFrame(&buf)
	if err != nil {
		t.Fatalf("readFrame() error = %v", err)
	}

	if !bytes.Equal(got, want) {
		t.Fatalf("readFrame() = %q, want %q", got, want)
	}
}
