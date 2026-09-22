package transport

import (
	"errors"
	"testing"
)

func TestEncodersNeverProduceEmptyFrame(t *testing.T) {
	if got := EncodeReject(1, errors.New("bad move")); len(got) == 0 {
		t.Fatal("EncodeReject returned an empty frame, which readers treat as a heartbeat")
	}
	if got := EncodeState(0); len(got) == 0 {
		t.Fatal("EncodeState returned an empty frame, which readers treat as a heartbeat")
	}
}
