package esphome

import (
	"bytes"
	"testing"
)

func TestEncodeFrameRoundTrips(t *testing.T) {
	payload := []byte{0x0a, 0x03, 'a', 'b', 'c'}
	frame := EncodeFrame(2, payload)
	decoder := NewFrameDecoder()
	frames, err := decoder.Push(frame)
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if len(frames) != 1 {
		t.Fatalf("frames = %d, want 1", len(frames))
	}
	if frames[0].MessageID != 2 || !bytes.Equal(frames[0].Payload, payload) {
		t.Fatalf("frame = %+v, want id 2 payload %v", frames[0], payload)
	}
}

func TestFrameDecoderSplitsCoalescedAndFragmentedFrames(t *testing.T) {
	first := EncodeFrame(1, nil)
	second := EncodeFrame(25, []byte{1, 2, 3})
	joined := append(append([]byte{}, first...), second...)

	// The split lands inside the second frame, so the complete first frame
	// decodes immediately and only the second frame waits for more bytes.
	decoder := NewFrameDecoder()
	half := len(joined) / 2
	if len(first) > half || half >= len(joined) {
		t.Fatalf("split point %d does not fragment the second frame", half)
	}
	got, err := decoder.Push(joined[:half])
	if err != nil {
		t.Fatalf("Push(first half) error = %v", err)
	}
	if len(got) != 1 || got[0].MessageID != 1 {
		t.Fatalf("first half decoded %+v, want one frame with id 1", got)
	}
	got, err = decoder.Push(joined[half:])
	if err != nil {
		t.Fatalf("Push(second half) error = %v", err)
	}
	if len(got) != 1 || got[0].MessageID != 25 {
		t.Fatalf("second half decoded %+v, want one frame with id 25", got)
	}
	if !bytes.Equal(got[0].Payload, []byte{1, 2, 3}) {
		t.Fatalf("reassembled payload = %v, want [1 2 3]", got[0].Payload)
	}
}

func TestFrameDecoderRejectsBadPreamble(t *testing.T) {
	decoder := NewFrameDecoder()
	if _, err := decoder.Push([]byte{0xff}); err == nil {
		t.Fatal("Push() error = nil, want frame format error")
	}
}

func TestFrameDecoderRejectsOversizedLength(t *testing.T) {
	decoder := NewFrameDecoder()
	chunk := []byte{Preamble, 0xff, 0xff, 0xff, 0xff, 0x7f, 0x01}
	if _, err := decoder.Push(chunk); err == nil {
		t.Fatal("Push() error = nil, want oversized frame error")
	}
}
