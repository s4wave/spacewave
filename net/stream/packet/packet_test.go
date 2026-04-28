package stream_packet

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
)

// rwBuf is a bytes.Buffer that satisfies io.ReadWriteCloser for Session tests.
type rwBuf struct {
	bytes.Buffer
}

func (b *rwBuf) Close() error { return nil }

// TestSendMsgRecvMsgRoundtrip verifies that a non-empty message round-trips
// through SendMsg then RecvMsg with byte-for-byte equality of the encoded body.
func TestSendMsgRecvMsgRoundtrip(t *testing.T) {
	buf := &rwBuf{}
	sess := NewSession(buf, 1<<20)

	out := &timestamppb.Timestamp{Seconds: 1234567890, Nanos: 42}
	if err := sess.SendMsg(out); err != nil {
		t.Fatalf("SendMsg: %v", err)
	}

	in := &timestamppb.Timestamp{}
	if err := sess.RecvMsg(in); err != nil {
		t.Fatalf("RecvMsg: %v", err)
	}

	if in.GetSeconds() != out.GetSeconds() || in.GetNanos() != out.GetNanos() {
		t.Fatalf("roundtrip mismatch: got seconds=%d nanos=%d, want seconds=%d nanos=%d",
			in.GetSeconds(), in.GetNanos(), out.GetSeconds(), out.GetNanos())
	}
}

// TestSendMsgRecvMsgEmpty verifies that an empty message (SizeVT()==0) is sent
// as just the 4-byte zero length prefix and decodes back to a Reset message.
func TestSendMsgRecvMsgEmpty(t *testing.T) {
	buf := &rwBuf{}
	sess := NewSession(buf, 1<<20)

	out := &timestamppb.Timestamp{}
	if err := sess.SendMsg(out); err != nil {
		t.Fatalf("SendMsg: %v", err)
	}

	if got, want := buf.Len(), 4; got != want {
		t.Fatalf("empty message wrote %d bytes, want %d", got, want)
	}

	in := &timestamppb.Timestamp{Seconds: 999, Nanos: 999}
	if err := sess.RecvMsg(in); err != nil {
		t.Fatalf("RecvMsg: %v", err)
	}

	if in.GetSeconds() != 0 || in.GetNanos() != 0 {
		t.Fatalf("empty roundtrip should Reset target, got seconds=%d nanos=%d",
			in.GetSeconds(), in.GetNanos())
	}
}

// TestSendMsgWireFormat pins the wire format: 4-byte little-endian length
// prefix followed by the marshaled body, where the body equals MarshalVT().
func TestSendMsgWireFormat(t *testing.T) {
	buf := &rwBuf{}
	sess := NewSession(buf, 1<<20)

	out := &timestamppb.Timestamp{Seconds: 1234567890, Nanos: 42}
	if err := sess.SendMsg(out); err != nil {
		t.Fatalf("SendMsg: %v", err)
	}

	wire := buf.Bytes()
	if len(wire) < 4 {
		t.Fatalf("wire too short: %d bytes", len(wire))
	}

	gotLen := binary.LittleEndian.Uint32(wire[:4])
	body := wire[4:]
	if int(gotLen) != len(body) {
		t.Fatalf("length prefix=%d, body=%d bytes", gotLen, len(body))
	}

	want, err := out.MarshalVT()
	if err != nil {
		t.Fatalf("MarshalVT: %v", err)
	}
	if !bytes.Equal(body, want) {
		t.Fatalf("wire body mismatch:\n got: %x\nwant: %x", body, want)
	}
}

// TestRecvMsgRejectsOversize verifies that a length prefix exceeding
// maxMessageSize is rejected without reading the body.
func TestRecvMsgRejectsOversize(t *testing.T) {
	buf := &rwBuf{}
	binary.Write(&buf.Buffer, binary.LittleEndian, uint32(100))
	sess := NewSession(buf, 50)

	in := &timestamppb.Timestamp{}
	err := sess.RecvMsg(in)
	if err == nil {
		t.Fatalf("RecvMsg should reject len=100 with maxMessageSize=50")
	}
}

// TestRecvMsgEOF verifies that a closed stream with no data returns EOF.
func TestRecvMsgEOF(t *testing.T) {
	buf := &rwBuf{}
	sess := NewSession(buf, 1<<20)

	in := &timestamppb.Timestamp{}
	err := sess.RecvMsg(in)
	if err != io.EOF {
		t.Fatalf("RecvMsg on empty stream: got %v, want io.EOF", err)
	}
}
