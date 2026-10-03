package transport_controller

import (
	"bytes"
	"testing"
)

// TestReadStreamEstablishHeader tests reading a stream establish header.
func TestReadStreamEstablishHeader(t *testing.T) {
	// Write a protocol establishment header into the stream buffer.
	buf := &bytes.Buffer{}
	obj := &StreamEstablish{ProtocolId: "testing"}
	if _, err := writeStreamEstablishHeader(buf, obj); err != nil {
		t.Fatal(err.Error())
	}

	// Read the establishment header through the stream decoder.
	readEst, err := readStreamEstablishHeader(buf)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the decoded header preserves the requested protocol.
	if readEst.ProtocolId != obj.ProtocolId {
		t.Fatalf("object decoded incorrectly: %s != %s", readEst.ProtocolId, obj.ProtocolId)
	}
}
