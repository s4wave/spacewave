package plan9fs

import (
	"encoding/binary"
	"testing"
)

func TestCodecRoundTrip(t *testing.T) {
	// Encode primitive values and a directory QID in one wire buffer.
	buf := NewWriteBuffer(64)
	buf.WriteU8(42)
	buf.WriteU16(1234)
	buf.WriteU32(567890)
	buf.WriteU64(0xdeadbeef12345678)
	buf.WriteString("hello")
	buf.WriteQID(QID{Type: QidDir, Version: 1, Path: 99})

	// Check that the wire buffer preserves each integer width and value.
	r := NewReadBuffer(buf.Bytes())
	if v := r.ReadU8(); v != 42 {
		t.Fatalf("u8: got %d want 42", v)
	}
	if v := r.ReadU16(); v != 1234 {
		t.Fatalf("u16: got %d want 1234", v)
	}
	if v := r.ReadU32(); v != 567890 {
		t.Fatalf("u32: got %d want 567890", v)
	}
	if v := r.ReadU64(); v != 0xdeadbeef12345678 {
		t.Fatalf("u64: got %x want deadbeef12345678", v)
	}

	// Check that the wire buffer preserves the string and directory QID.
	if v := r.ReadString(); v != "hello" {
		t.Fatalf("string: got %q want hello", v)
	}
	q := r.ReadQID()
	if q.Type != QidDir || q.Version != 1 || q.Path != 99 {
		t.Fatalf("qid: got %+v", q)
	}

	// Check that decoding consumes the complete buffer without an error.
	if r.Err() != nil {
		t.Fatalf("unexpected error: %v", r.Err())
	}
	if r.Remaining() != 0 {
		t.Fatalf("remaining: got %d want 0", r.Remaining())
	}
}

func TestCodecEmptyString(t *testing.T) {
	// Encode an empty string with its wire length prefix.
	buf := NewWriteBuffer(4)
	buf.WriteString("")

	// Check that decoding the empty string succeeds.
	r := NewReadBuffer(buf.Bytes())
	if v := r.ReadString(); v != "" {
		t.Fatalf("string: got %q want empty", v)
	}
	if r.Err() != nil {
		t.Fatalf("unexpected error: %v", r.Err())
	}
}

func TestCodecLongString(t *testing.T) {
	// Prepare a string whose payload exceeds a short wire buffer.
	long := make([]byte, 1000)
	for i := range long {
		long[i] = 'x'
	}
	s := string(long)

	// Encode the long string with its wire length prefix.
	buf := NewWriteBuffer(len(long) + 2)
	buf.WriteString(s)

	// Check that decoding preserves the complete long string.
	r := NewReadBuffer(buf.Bytes())
	if v := r.ReadString(); v != s {
		t.Fatalf("string length: got %d want %d", len(v), len(s))
	}
}

func TestCodecShortReadU8(t *testing.T) {
	r := NewReadBuffer(nil)
	r.ReadU8()
	if r.Err() == nil {
		t.Fatal("expected error")
	}
}

func TestCodecShortReadU16(t *testing.T) {
	r := NewReadBuffer([]byte{1})
	r.ReadU16()
	if r.Err() == nil {
		t.Fatal("expected error")
	}
}

func TestCodecShortReadU32(t *testing.T) {
	r := NewReadBuffer([]byte{1, 2, 3})
	r.ReadU32()
	if r.Err() == nil {
		t.Fatal("expected error")
	}
}

func TestCodecShortReadU64(t *testing.T) {
	r := NewReadBuffer([]byte{1, 2, 3, 4, 5, 6, 7})
	r.ReadU64()
	if r.Err() == nil {
		t.Fatal("expected error")
	}
}

func TestCodecShortReadString(t *testing.T) {
	// length prefix says 10, but only 2 bytes of data
	buf := NewWriteBuffer(4)
	buf.WriteU16(10)
	buf.WriteU8('a')
	buf.WriteU8('b')
	r := NewReadBuffer(buf.Bytes())
	r.ReadString()
	if r.Err() == nil {
		t.Fatal("expected error on short string")
	}
}

func TestCodecShortReadBytes(t *testing.T) {
	r := NewReadBuffer([]byte{1, 2})
	v := r.ReadBytes(5)
	if v != nil {
		t.Fatal("expected nil on short read")
	}
	if r.Err() == nil {
		t.Fatal("expected error")
	}
}

func TestCodecShortReadQID(t *testing.T) {
	r := NewReadBuffer([]byte{1, 2, 3, 4, 5}) // need 13 bytes
	r.ReadQID()
	if r.Err() == nil {
		t.Fatal("expected error on short QID")
	}
}

func TestCodecErrorSticky(t *testing.T) {
	// once an error occurs, subsequent reads also fail
	r := NewReadBuffer([]byte{1})
	r.ReadU32()
	if r.Err() == nil {
		t.Fatal("expected error")
	}

	// subsequent reads should still have error and return zero values
	v := r.ReadU8()
	if v != 0 {
		t.Fatalf("expected 0 after error, got %d", v)
	}
	if r.Err() == nil {
		t.Fatal("error should be sticky")
	}
}

func TestCodecReadBytes(t *testing.T) {
	// Read a prefix of the wire bytes and check the unread remainder.
	data := []byte{0xaa, 0xbb, 0xcc, 0xdd}
	r := NewReadBuffer(data)
	v := r.ReadBytes(3)
	if len(v) != 3 || v[0] != 0xaa || v[1] != 0xbb || v[2] != 0xcc {
		t.Fatalf("ReadBytes: got %x", v)
	}
	if r.Remaining() != 1 {
		t.Fatalf("remaining: got %d want 1", r.Remaining())
	}
}

func TestCodecBoundaryValues(t *testing.T) {
	// Encode the minimum and maximum eight-bit and sixteen-bit values.
	buf := NewWriteBuffer(32)
	buf.WriteU8(0)
	buf.WriteU8(255)
	buf.WriteU16(0)
	buf.WriteU16(65535)

	// Encode the minimum and maximum thirty-two-bit and sixty-four-bit values.
	buf.WriteU32(0)
	buf.WriteU32(0xFFFFFFFF)
	buf.WriteU64(0)
	buf.WriteU64(0xFFFFFFFFFFFFFFFF)

	// Check that decoding preserves the eight-bit and sixteen-bit boundaries.
	r := NewReadBuffer(buf.Bytes())
	if r.ReadU8() != 0 {
		t.Fatal("u8 min")
	}
	if r.ReadU8() != 255 {
		t.Fatal("u8 max")
	}
	if r.ReadU16() != 0 {
		t.Fatal("u16 min")
	}
	if r.ReadU16() != 65535 {
		t.Fatal("u16 max")
	}

	// Check that decoding preserves the thirty-two-bit and sixty-four-bit boundaries.
	if r.ReadU32() != 0 {
		t.Fatal("u32 min")
	}
	if r.ReadU32() != 0xFFFFFFFF {
		t.Fatal("u32 max")
	}
	if r.ReadU64() != 0 {
		t.Fatal("u64 min")
	}
	if r.ReadU64() != 0xFFFFFFFFFFFFFFFF {
		t.Fatal("u64 max")
	}
}
func TestBuildMessage(t *testing.T) {
	// Frame a payload with a message type and request tag.
	payload := []byte{0x01, 0x02, 0x03}
	msg := buildMessage(42, 100, payload)

	// Check the framed message length and type against its wire header.
	if len(msg) != headerSize+3 {
		t.Fatalf("len: got %d want %d", len(msg), headerSize+3)
	}
	size := binary.LittleEndian.Uint32(msg[0:4])
	if size != uint32(headerSize+3) {
		t.Fatalf("size: got %d", size)
	}
	if msg[4] != 42 {
		t.Fatalf("type: got %d", msg[4])
	}

	// Check that framing preserves the request tag and payload bytes.
	tag := binary.LittleEndian.Uint16(msg[5:7])
	if tag != 100 {
		t.Fatalf("tag: got %d", tag)
	}
	if msg[7] != 1 || msg[8] != 2 || msg[9] != 3 {
		t.Fatalf("payload mismatch")
	}
}

func TestBuildMessageNilPayload(t *testing.T) {
	msg := buildMessage(RCLUNK, 5, nil)
	if len(msg) != headerSize {
		t.Fatalf("len: got %d want %d", len(msg), headerSize)
	}
}

func TestBuildErrorResponse(t *testing.T) {
	// Build an error response carrying the missing-entry errno.
	msg := buildErrorResponse(7, ENOENT)

	// Check that the error response preserves its message type and request tag.
	if msg[4] != RLERROR {
		t.Fatalf("type: got %d want %d", msg[4], RLERROR)
	}
	tag := binary.LittleEndian.Uint16(msg[5:7])
	if tag != 7 {
		t.Fatalf("tag: got %d want 7", tag)
	}

	// Check that the error response payload contains the expected errno.
	r := NewReadBuffer(msg[headerSize:])
	errno := r.ReadU32()
	if errno != ENOENT {
		t.Fatalf("errno: got %d want %d", errno, ENOENT)
	}
}

// --- Message Framing Tests ---
