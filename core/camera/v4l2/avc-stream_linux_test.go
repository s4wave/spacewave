//go:build linux

package v4l2

import (
	"bytes"
	"testing"
)

// TestAVCStreamResumesAtIDR checks AVC framing and recovery across a capture gap.
func TestAVCStreamResumesAtIDR(t *testing.T) {
	// The parameter sets can arrive alone and outlive the mapped driver buffer.
	var stream avcStream
	parameters := []byte{0, 0, 0, 1, 0x67, 66, 0, 31, 0x80, 0, 0, 1, 0x68, 0xc0}
	if payload, desc := stream.accessUnit(parameters, 0, false); payload != nil || desc != nil {
		t.Fatal("parameter sets without a picture yielded a frame")
	}
	clear(parameters)

	// Ignore deltas until an IDR can start a group with its cached SPS and PPS.
	delta := []byte{0, 0, 1, 0x41, 0xe0}
	if payload, _ := stream.accessUnit(delta, 1, false); payload != nil {
		t.Fatal("delta yielded a frame before an IDR")
	}
	key := []byte{0, 0, 0, 1, 0x65, 0xb8, 0x10}
	wantKey := []byte{0, 0, 0, 3, 0x65, 0xb8, 0x10}
	wantDesc := []byte{1, 66, 0, 31, 0xff, 0xe1, 0, 5, 0x67, 66, 0, 31, 0x80, 1, 0, 2, 0x68, 0xc0}
	payload, desc := stream.accessUnit(key, 2, false)
	if !bytes.Equal(payload, wantKey) || !bytes.Equal(desc, wantDesc) {
		t.Fatalf("first IDR yielded payload %x and avcC %x", payload, desc)
	}

	// A contiguous delta joins the group. A driver gap seals the prefix.
	wantDelta := []byte{0, 0, 0, 2, 0x41, 0xe0}
	if payload, desc := stream.accessUnit(delta, 3, false); !bytes.Equal(payload, wantDelta) || desc != nil {
		t.Fatalf("delta yielded payload %x and avcC %x", payload, desc)
	}
	for _, seq := range []uint32{5, 6} {
		if payload, _ := stream.accessUnit(delta, seq, false); payload != nil {
			t.Fatalf("delta %d crossed a capture gap", seq)
		}
	}

	// The next IDR needs a complete description even without repeated headers.
	payload, desc = stream.accessUnit(key, 7, false)
	if !bytes.Equal(payload, wantKey) || !bytes.Equal(desc, wantDesc) {
		t.Fatalf("recovery IDR yielded payload %x and avcC %x", payload, desc)
	}
}
