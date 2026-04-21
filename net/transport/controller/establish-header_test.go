package transport_controller

import (
	"bytes"
	"testing"
)

// TestReadStreamEstablishHeader tests reading a stream establish header.
func TestReadStreamEstablishHeader(t *testing.T) {
	buf := &bytes.Buffer{}
	obj := &StreamEstablish{ProtocolId: "testing"}
	if _, err := writeStreamEstablishHeader(buf, obj); err != nil {
		t.Fatal(err.Error())
	}
	readEst, err := readStreamEstablishHeader(buf)
	if err != nil {
		t.Fatal(err.Error())
	}
	if readEst.ProtocolId != obj.ProtocolId {
		t.Fatalf("object decoded incorrectly: %s != %s", readEst.ProtocolId, obj.ProtocolId)
	}
}
