package plan9fs

import (
	"context"
	"encoding/binary"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_iofs "github.com/s4wave/spacewave/db/unixfs/iofs"
)

// --- Codec Tests ---

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
	r := NewReadBuffer([]byte{1, 2, 3, 4, 5}) // need 13
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

func TestMessageTooShort(t *testing.T) {
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	_, err := srv.HandleMessage(t.Context(), []byte{1, 2, 3})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMessageSizeMismatch(t *testing.T) {
	// Start a 9P server over the fixture filesystem and release its fids on exit.
	srv := newTestServer(t)
	defer srv.ReleaseAll()

	// Construct a version message whose declared size exceeds its buffer.
	msg := make([]byte, 7)
	binary.LittleEndian.PutUint32(msg[0:4], 100)
	msg[4] = TVERSION

	// Check that the 9P server rejects the inconsistent message size.
	_, err := srv.HandleMessage(t.Context(), msg)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUnknownMessageType(t *testing.T) {
	// Start a 9P server over the fixture filesystem and release its fids on exit.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()

	// Send an unknown message type to the 9P server.
	resp, err := srv.HandleMessage(ctx, buildMessage(255, 1, nil))
	if err != nil {
		t.Fatal(err)
	}

	// should get RLERROR
	if resp[4] != RLERROR {
		t.Fatalf("expected RLERROR for unknown type, got %d", resp[4])
	}
}

// --- Version Tests ---

func TestVersionHandshake(t *testing.T) {
	// Start a 9P server over the fixture filesystem and release its fids on exit.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()

	// Negotiate 9P2000.L and check the returned version and message size.
	resp := sendVersion(t, ctx, srv, 65536, "9P2000.L")
	r := NewReadBuffer(resp[headerSize:])
	msize := r.ReadU32()
	version := r.ReadString()
	if msize != 65536 {
		t.Fatalf("msize: got %d want 65536", msize)
	}
	if version != "9P2000.L" {
		t.Fatalf("version: got %q want 9P2000.L", version)
	}
}

func TestVersionUnknown(t *testing.T) {
	// Start a 9P server over the fixture filesystem and release its fids on exit.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()

	// Request an unsupported 9P version and check the unknown-version response.
	resp := sendVersion(t, ctx, srv, 65536, "9P2000.u")
	r := NewReadBuffer(resp[headerSize:])
	_ = r.ReadU32()
	version := r.ReadString()
	if version != "unknown" {
		t.Fatalf("version: got %q want unknown", version)
	}
}

func TestVersionMsizeNegotiation(t *testing.T) {
	// Start a 9P server over the fixture filesystem and release its fids on exit.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()

	// client requests smaller msize
	resp := sendVersion(t, ctx, srv, 8192, "9P2000.L")
	r := NewReadBuffer(resp[headerSize:])
	msize := r.ReadU32()
	if msize != 8192 {
		t.Fatalf("msize: got %d want 8192 (client's smaller value)", msize)
	}
}

func TestVersionMsizeLarger(t *testing.T) {
	// Start a 9P server over the fixture filesystem and release its fids on exit.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()

	// client requests larger msize; server caps to its default
	resp := sendVersion(t, ctx, srv, 1048576, "9P2000.L")
	r := NewReadBuffer(resp[headerSize:])
	msize := r.ReadU32()
	if msize != defaultMsize {
		t.Fatalf("msize: got %d want %d (server default)", msize, defaultMsize)
	}
}

// --- Attach Tests ---

func TestAttachAndClunk(t *testing.T) {
	// Start a 9P session and negotiate access to the fixture filesystem.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)

	// Attach the root fid and verify that the session can clunk it.
	doAttach(t, ctx, srv, 0)
	doClunk(t, ctx, srv, 0)
}

func TestAttachDuplicateFid(t *testing.T) {
	// Start a 9P session and negotiate access to the fixture filesystem.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)

	// second attach with same fid should error
	payload := NewWriteBuffer(32)
	payload.WriteU32(0) // same fid
	payload.WriteU32(0xFFFFFFFF)
	payload.WriteString("user")
	payload.WriteString("")
	payload.WriteU32(1000)

	// Send the duplicate attach request to the 9P server.
	resp, err := srv.HandleMessage(ctx, buildMessage(TATTACH, 2, payload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	// Check that the 9P server rejects the duplicate fid.
	if resp[4] != RLERROR {
		t.Fatalf("expected RLERROR for duplicate fid, got %d", resp[4])
	}

	// Release the session fids through the 9P clunk handler.
	doClunk(t, ctx, srv, 0)
}

func TestAttachRootQIDType(t *testing.T) {
	// Start a 9P session and negotiate access to the fixture filesystem.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)

	// Encode an attach request for the fixture root directory.
	payload := NewWriteBuffer(32)
	payload.WriteU32(0)
	payload.WriteU32(0xFFFFFFFF)
	payload.WriteString("user")
	payload.WriteString("")
	payload.WriteU32(1000)

	// Attach the fixture root directory through the 9P server.
	resp, err := srv.HandleMessage(ctx, buildMessage(TATTACH, 1, payload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	// Check that the root QID identifies an unversioned directory.
	r := NewReadBuffer(resp[headerSize:])
	qid := r.ReadQID()
	if qid.Type != QidDir {
		t.Fatalf("root QID type: got %d want %d (QidDir)", qid.Type, QidDir)
	}
	if qid.Version != 0 {
		t.Fatalf("root QID version: got %d want 0", qid.Version)
	}

	// Release the attached root fid through the 9P clunk handler.
	doClunk(t, ctx, srv, 0)
}

// --- Walk Tests ---

func TestWalkZeroNames(t *testing.T) {
	// Start a 9P session and negotiate access to the fixture filesystem.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)

	// zero-name walk clones the fid
	walkPayload := NewWriteBuffer(16)
	walkPayload.WriteU32(0) // fid
	walkPayload.WriteU32(1) // newfid
	walkPayload.WriteU16(0)

	// Clone the attached root fid through a walk with no path components.
	resp, err := srv.HandleMessage(ctx, buildMessage(TWALK, 3, walkPayload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	// Check that the cloned fid response contains no walked QIDs.
	if resp[4] != RWALK {
		t.Fatalf("expected RWALK, got %d", resp[4])
	}
	r := NewReadBuffer(resp[headerSize:])
	nwqid := r.ReadU16()
	if nwqid != 0 {
		t.Fatalf("nwqid: got %d want 0 for zero-name walk", nwqid)
	}

	// both fids should work
	doClunk(t, ctx, srv, 0)
	doClunk(t, ctx, srv, 1)
}

func TestWalkSingleComponent(t *testing.T) {
	// Start a 9P session and walk to the fixture entry under test.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)
	doWalk(t, ctx, srv, 0, 1, "hello.txt")

	// verify the walked fid points to a file
	resp := sendGetattr(t, ctx, srv, 1)
	r := NewReadBuffer(resp[headerSize:])
	_ = r.ReadU64()
	qid := r.ReadQID()
	if qid.Type != QidFile {
		t.Fatalf("QID type: got %d want %d (QidFile)", qid.Type, QidFile)
	}

	// Release the session fids through the 9P clunk handler.
	doClunk(t, ctx, srv, 0)
	doClunk(t, ctx, srv, 1)
}

func TestWalkMultiComponent(t *testing.T) {
	// Start a 9P session and negotiate access to the fixture filesystem.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)

	// Encode a walk from the root through subdir to nested.txt.
	walkPayload := NewWriteBuffer(64)
	walkPayload.WriteU32(0)
	walkPayload.WriteU32(1)
	walkPayload.WriteU16(2)
	walkPayload.WriteString("subdir")
	walkPayload.WriteString("nested.txt")

	// Walk both path components through the 9P server.
	resp, err := srv.HandleMessage(ctx, buildMessage(TWALK, 3, walkPayload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	// Check that the walk returns a directory QID followed by a file QID.
	r := NewReadBuffer(resp[headerSize:])
	nwqid := r.ReadU16()
	if nwqid != 2 {
		t.Fatalf("nwqid: got %d want 2", nwqid)
	}
	q1 := r.ReadQID()
	if q1.Type != QidDir {
		t.Fatalf("first qid type: got %d want QidDir", q1.Type)
	}
	q2 := r.ReadQID()
	if q2.Type != QidFile {
		t.Fatalf("second qid type: got %d want QidFile", q2.Type)
	}

	// Release the session fids through the 9P clunk handler.
	doClunk(t, ctx, srv, 0)
	doClunk(t, ctx, srv, 1)
}

func TestWalkNonExistent(t *testing.T) {
	// Start a 9P session and negotiate access to the fixture filesystem.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)

	// Encode a walk from the root to a missing fixture entry.
	walkPayload := NewWriteBuffer(32)
	walkPayload.WriteU32(0)
	walkPayload.WriteU32(1)
	walkPayload.WriteU16(1)
	walkPayload.WriteString("does-not-exist")

	// Walk the missing fixture path through the 9P server.
	resp, err := srv.HandleMessage(ctx, buildMessage(TWALK, 3, walkPayload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	// Check that the missing path returns the missing-entry errno.
	if resp[4] != RLERROR {
		t.Fatalf("expected RLERROR for nonexistent walk, got %d", resp[4])
	}
	r := NewReadBuffer(resp[headerSize:])
	errno := r.ReadU32()
	if errno != ENOENT {
		t.Fatalf("errno: got %d want %d (ENOENT)", errno, ENOENT)
	}

	// Release the session fids through the 9P clunk handler.
	doClunk(t, ctx, srv, 0)
}

func TestWalkPartialSuccess(t *testing.T) {
	// Start a 9P session and negotiate access to the fixture filesystem.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)

	// first component exists, second doesn't
	walkPayload := NewWriteBuffer(64)
	walkPayload.WriteU32(0)
	walkPayload.WriteU32(1)
	walkPayload.WriteU16(2)
	walkPayload.WriteString("subdir")
	walkPayload.WriteString("nope")

	// Walk the partially existing path through the 9P server.
	resp, err := srv.HandleMessage(ctx, buildMessage(TWALK, 3, walkPayload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	// partial walk returns RWALK with the QIDs that succeeded
	if resp[4] != RWALK {
		t.Fatalf("expected RWALK for partial walk, got %d", resp[4])
	}
	r := NewReadBuffer(resp[headerSize:])
	nwqid := r.ReadU16()
	if nwqid != 1 {
		t.Fatalf("nwqid: got %d want 1 (partial)", nwqid)
	}
	q := r.ReadQID()
	if q.Type != QidDir {
		t.Fatalf("partial qid type: got %d want QidDir", q.Type)
	}

	// Release the session fids through the 9P clunk handler.
	doClunk(t, ctx, srv, 0)
	doClunk(t, ctx, srv, 1)
}

func TestWalkReplaceFid(t *testing.T) {
	// Start a 9P session and negotiate access to the fixture filesystem.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)

	// walk with newfid == fid (replaces the fid)
	walkPayload := NewWriteBuffer(32)
	walkPayload.WriteU32(0)
	walkPayload.WriteU32(0) // same fid
	walkPayload.WriteU16(1)
	walkPayload.WriteString("hello.txt")

	// Replace the attached root fid by walking to hello.txt.
	resp, err := srv.HandleMessage(ctx, buildMessage(TWALK, 3, walkPayload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if resp[4] != RWALK {
		t.Fatalf("expected RWALK, got %d", resp[4])
	}

	// fid 0 should now point to hello.txt
	resp = sendGetattr(t, ctx, srv, 0)
	r := NewReadBuffer(resp[headerSize:])
	_ = r.ReadU64()
	qid := r.ReadQID()
	if qid.Type != QidFile {
		t.Fatalf("replaced fid QID type: got %d want QidFile", qid.Type)
	}

	// Release the session fids through the 9P clunk handler.
	doClunk(t, ctx, srv, 0)
}

func TestWalkInvalidFid(t *testing.T) {
	// Start a 9P session and negotiate access to the fixture filesystem.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)

	// Encode a walk that starts from an unattached fid.
	walkPayload := NewWriteBuffer(32)
	walkPayload.WriteU32(99) // non-existent fid
	walkPayload.WriteU32(1)
	walkPayload.WriteU16(0)

	// Check that the 9P server rejects a walk from an unattached fid.
	resp, err := srv.HandleMessage(ctx, buildMessage(TWALK, 3, walkPayload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if resp[4] != RLERROR {
		t.Fatalf("expected RLERROR for invalid fid, got %d", resp[4])
	}
}

// --- Open Tests ---

func TestLopenFile(t *testing.T) {
	// Start a 9P session and walk to the fixture entry under test.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)
	doWalk(t, ctx, srv, 0, 1, "hello.txt")

	// Open hello.txt and check the returned file QID and I/O unit.
	resp := sendLopen(t, ctx, srv, 1)
	r := NewReadBuffer(resp[headerSize:])
	qid := r.ReadQID()
	iounit := r.ReadU32()
	if qid.Type != QidFile {
		t.Fatalf("QID type: got %d want QidFile", qid.Type)
	}
	if iounit == 0 {
		t.Fatal("iounit should be non-zero")
	}

	// Release the session fids through the 9P clunk handler.
	doClunk(t, ctx, srv, 0)
	doClunk(t, ctx, srv, 1)
}

func TestLopenDir(t *testing.T) {
	// Start a 9P session and walk to the fixture entry under test.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)
	doWalk(t, ctx, srv, 0, 1, "subdir")

	// Open subdir and check the returned directory QID.
	resp := sendLopen(t, ctx, srv, 1)
	r := NewReadBuffer(resp[headerSize:])
	qid := r.ReadQID()
	if qid.Type != QidDir {
		t.Fatalf("QID type: got %d want QidDir", qid.Type)
	}

	// Release the session fids through the 9P clunk handler.
	doClunk(t, ctx, srv, 0)
	doClunk(t, ctx, srv, 1)
}

func TestLopenInvalidFid(t *testing.T) {
	// Start a 9P session and negotiate access to the fixture filesystem.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)

	// Encode an open request for an unattached fid.
	payload := NewWriteBuffer(8)
	payload.WriteU32(99) // bad fid
	payload.WriteU32(0)

	// Check that the 9P server rejects opening an unattached fid.
	resp, err := srv.HandleMessage(ctx, buildMessage(TLOPEN, 1, payload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if resp[4] != RLERROR {
		t.Fatalf("expected RLERROR, got %d", resp[4])
	}
}

// --- Read Tests ---

func TestReadFile(t *testing.T) {
	// Start a 9P session and open hello.txt for the requested file operation.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)
	doWalk(t, ctx, srv, 0, 1, "hello.txt")
	sendLopen(t, ctx, srv, 1)

	// Read hello.txt and check its complete fixture contents.
	data := doRead(t, ctx, srv, 1, 0, 65536)
	if string(data) != "world" {
		t.Fatalf("read: got %q want %q", string(data), "world")
	}

	// Release the session fids through the 9P clunk handler.
	doClunk(t, ctx, srv, 0)
	doClunk(t, ctx, srv, 1)
}

func TestReadAtOffset(t *testing.T) {
	// Start a 9P session and open hello.txt for the requested file operation.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)
	doWalk(t, ctx, srv, 0, 1, "hello.txt")
	sendLopen(t, ctx, srv, 1)

	// "world" -> read from offset 2 -> "rld"
	data := doRead(t, ctx, srv, 1, 2, 65536)
	if string(data) != "rld" {
		t.Fatalf("read at offset 2: got %q want %q", string(data), "rld")
	}

	// Release the session fids through the 9P clunk handler.
	doClunk(t, ctx, srv, 0)
	doClunk(t, ctx, srv, 1)
}

func TestReadPastEOF(t *testing.T) {
	// Start a 9P session and open hello.txt for the requested file operation.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)
	doWalk(t, ctx, srv, 0, 1, "hello.txt")
	sendLopen(t, ctx, srv, 1)

	// offset past file end
	data := doRead(t, ctx, srv, 1, 100, 65536)
	if len(data) != 0 {
		t.Fatalf("read past EOF: got %d bytes, want 0", len(data))
	}

	// Release the session fids through the 9P clunk handler.
	doClunk(t, ctx, srv, 0)
	doClunk(t, ctx, srv, 1)
}

func TestReadPartial(t *testing.T) {
	// Start a 9P session and open hello.txt for the requested file operation.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)
	doWalk(t, ctx, srv, 0, 1, "hello.txt")
	sendLopen(t, ctx, srv, 1)

	// request only 3 bytes
	data := doRead(t, ctx, srv, 1, 0, 3)
	if string(data) != "wor" {
		t.Fatalf("read partial: got %q want %q", string(data), "wor")
	}

	// Release the session fids through the 9P clunk handler.
	doClunk(t, ctx, srv, 0)
	doClunk(t, ctx, srv, 1)
}

func TestReadInvalidFid(t *testing.T) {
	// Start a 9P session and negotiate access to the fixture filesystem.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)

	// Encode a file read request for an unattached fid.
	payload := NewWriteBuffer(16)
	payload.WriteU32(99) // bad fid
	payload.WriteU64(0)
	payload.WriteU32(100)

	// Check that the 9P server rejects reading an unattached fid.
	resp, err := srv.HandleMessage(ctx, buildMessage(TREAD, 1, payload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if resp[4] != RLERROR {
		t.Fatalf("expected RLERROR, got %d", resp[4])
	}
}

func TestReadNestedFile(t *testing.T) {
	// Start a 9P session and negotiate access to the fixture filesystem.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)

	// walk two components and read
	walkPayload := NewWriteBuffer(64)
	walkPayload.WriteU32(0)
	walkPayload.WriteU32(1)
	walkPayload.WriteU16(2)
	walkPayload.WriteString("subdir")
	walkPayload.WriteString("nested.txt")

	// Walk to the nested file through the 9P server.
	resp, err := srv.HandleMessage(ctx, buildMessage(TWALK, 3, walkPayload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	// Check that the nested file walk returns a walk response.
	if resp[4] != RWALK {
		t.Fatal("expected RWALK")
	}

	// Open the nested file and check its fixture contents.
	sendLopen(t, ctx, srv, 1)
	data := doRead(t, ctx, srv, 1, 0, 65536)
	if string(data) != "deep" {
		t.Fatalf("nested read: got %q want %q", string(data), "deep")
	}

	// Release the session fids through the 9P clunk handler.
	doClunk(t, ctx, srv, 0)
	doClunk(t, ctx, srv, 1)
}

// --- Write Tests (read-only FS -> EROFS) ---

func TestWriteReadOnlyFS(t *testing.T) {
	// Start a 9P session and open hello.txt for the requested file operation.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)
	doWalk(t, ctx, srv, 0, 1, "hello.txt")
	sendLopen(t, ctx, srv, 1)

	// Encode a write request targeting the read-only hello.txt fixture.
	payload := NewWriteBuffer(32)
	payload.WriteU32(1) // fid
	payload.WriteU64(0) // offset
	payload.WriteU32(3) // count
	payload.WriteBytes([]byte("abc"))

	// Send the file write request through the 9P server.
	resp, err := srv.HandleMessage(ctx, buildMessage(TWRITE, 5, payload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	// Check that the read-only filesystem rejects the file write.
	assertRLError(t, resp, EROFS)

	// Release the session fids through the 9P clunk handler.
	doClunk(t, ctx, srv, 0)
	doClunk(t, ctx, srv, 1)
}

func TestWriteInvalidFid(t *testing.T) {
	// Start a 9P server over the fixture filesystem and release its fids on exit.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()

	// Encode a file write request for an unattached fid.
	payload := NewWriteBuffer(32)
	payload.WriteU32(99) // bad fid
	payload.WriteU64(0)
	payload.WriteU32(1)
	payload.WriteBytes([]byte("x"))

	// Check that the 9P server rejects writing to an unattached fid.
	resp, err := srv.HandleMessage(ctx, buildMessage(TWRITE, 1, payload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if resp[4] != RLERROR {
		t.Fatalf("expected RLERROR, got %d", resp[4])
	}
}

// --- Create Tests (read-only FS -> EROFS) ---

func TestLcreateReadOnlyFS(t *testing.T) {
	// Start a 9P session and negotiate access to the fixture filesystem.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)

	// Encode a file creation request in the read-only root directory.
	payload := NewWriteBuffer(32)
	payload.WriteU32(0) // fid (root dir)
	payload.WriteString("new.txt")
	payload.WriteU32(0)     // flags
	payload.WriteU32(0o644) // mode
	payload.WriteU32(1000)  // gid

	// Send the file creation request through the 9P server.
	resp, err := srv.HandleMessage(ctx, buildMessage(TLCREATE, 2, payload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	// Check that the read-only filesystem rejects file creation.
	assertRLError(t, resp, EROFS)

	// Release the session fids through the 9P clunk handler.
	doClunk(t, ctx, srv, 0)
}

// --- Getattr Tests ---

func TestGetattrFile(t *testing.T) {
	// Start a 9P session and walk to the fixture entry under test.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)
	doWalk(t, ctx, srv, 0, 1, "hello.txt")

	// Read the hello.txt attributes through the 9P server.
	resp := sendGetattr(t, ctx, srv, 1)
	r := NewReadBuffer(resp[headerSize:])
	valid := r.ReadU64()

	// Check the attribute mask, file QID and regular-file mode.
	if valid != GetattrBasic {
		t.Fatalf("valid mask: got %x want %x", valid, GetattrBasic)
	}
	qid := r.ReadQID()
	if qid.Type != QidFile {
		t.Fatalf("qid type: got %d want QidFile", qid.Type)
	}
	mode := r.ReadU32()
	if mode&0o100000 == 0 {
		t.Fatalf("expected S_IFREG in mode %o", mode)
	}

	// Check the fixture file size and reported block size after the identity fields.
	_ = r.ReadU32() // uid
	_ = r.ReadU32() // gid
	_ = r.ReadU64() // nlink
	_ = r.ReadU64() // rdev
	size := r.ReadU64()
	if size != 5 {
		t.Fatalf("size: got %d want 5", size)
	}
	blksize := r.ReadU64()
	if blksize != 4096 {
		t.Fatalf("blksize: got %d want 4096", blksize)
	}

	// Release the session fids through the 9P clunk handler.
	doClunk(t, ctx, srv, 0)
	doClunk(t, ctx, srv, 1)
}

func TestGetattrDir(t *testing.T) {
	// Start a 9P session and walk to the fixture entry under test.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)
	doWalk(t, ctx, srv, 0, 1, "subdir")

	// Read the subdir attributes and check its QID and directory mode.
	resp := sendGetattr(t, ctx, srv, 1)
	r := NewReadBuffer(resp[headerSize:])
	_ = r.ReadU64()
	qid := r.ReadQID()
	if qid.Type != QidDir {
		t.Fatalf("qid type: got %d want QidDir", qid.Type)
	}
	mode := r.ReadU32()
	if mode&0o40000 == 0 {
		t.Fatalf("expected S_IFDIR in mode %o", mode)
	}

	// Release the session fids through the 9P clunk handler.
	doClunk(t, ctx, srv, 0)
	doClunk(t, ctx, srv, 1)
}

func TestGetattrRoot(t *testing.T) {
	// Start a 9P session and negotiate access to the fixture filesystem.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)

	// Read the root attributes and check its directory QID.
	resp := sendGetattr(t, ctx, srv, 0)
	r := NewReadBuffer(resp[headerSize:])
	_ = r.ReadU64()
	qid := r.ReadQID()
	if qid.Type != QidDir {
		t.Fatalf("root qid type: got %d want QidDir", qid.Type)
	}

	// Release the session fids through the 9P clunk handler.
	doClunk(t, ctx, srv, 0)
}

func TestGetattrInvalidFid(t *testing.T) {
	// Start a 9P server over the fixture filesystem and release its fids on exit.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()

	// Encode an attribute request for an unattached fid.
	payload := NewWriteBuffer(12)
	payload.WriteU32(99)
	payload.WriteU64(GetattrAll)

	// Check that the 9P server rejects attributes for an unattached fid.
	resp, err := srv.HandleMessage(ctx, buildMessage(TGETATTR, 1, payload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if resp[4] != RLERROR {
		t.Fatalf("expected RLERROR, got %d", resp[4])
	}
}

// --- Setattr Tests (read-only FS -> EROFS) ---

func TestSetattrModeReadOnly(t *testing.T) {
	// Start a 9P session and walk to the fixture entry under test.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)
	doWalk(t, ctx, srv, 0, 1, "hello.txt")

	// Encode the file mode change and unused identity and size fields.
	payload := NewWriteBuffer(64)
	payload.WriteU32(1)           // fid
	payload.WriteU32(SetattrMode) // valid
	payload.WriteU32(0o644)       // mode
	payload.WriteU32(0)           // uid
	payload.WriteU32(0)           // gid
	payload.WriteU64(0)           // size

	// Complete the mode-change request with unused timestamp fields.
	payload.WriteU64(0) // atime_sec
	payload.WriteU64(0) // atime_nsec
	payload.WriteU64(0) // mtime_sec
	payload.WriteU64(0) // mtime_nsec

	// Check that the read-only filesystem rejects changing the file mode.
	resp, err := srv.HandleMessage(ctx, buildMessage(TSETATTR, 3, payload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	assertRLError(t, resp, EROFS)

	// Release the session fids through the 9P clunk handler.
	doClunk(t, ctx, srv, 0)
	doClunk(t, ctx, srv, 1)
}

func TestSetattrSizeReadOnly(t *testing.T) {
	// Start a 9P session and walk to the fixture entry under test.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)
	doWalk(t, ctx, srv, 0, 1, "hello.txt")

	// Encode the file size change and unused identity fields.
	payload := NewWriteBuffer(64)
	payload.WriteU32(1)           // fid
	payload.WriteU32(SetattrSize) // valid
	payload.WriteU32(0)           // mode
	payload.WriteU32(0)           // uid
	payload.WriteU32(0)           // gid
	payload.WriteU64(10)          // size

	// Complete the size-change request with unused timestamp fields.
	payload.WriteU64(0) // atime_sec
	payload.WriteU64(0) // atime_nsec
	payload.WriteU64(0) // mtime_sec
	payload.WriteU64(0) // mtime_nsec

	// Check that the read-only filesystem rejects changing the file size.
	resp, err := srv.HandleMessage(ctx, buildMessage(TSETATTR, 3, payload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	assertRLError(t, resp, EROFS)

	// Release the session fids through the 9P clunk handler.
	doClunk(t, ctx, srv, 0)
	doClunk(t, ctx, srv, 1)
}

func TestSetattrMtimeReadOnly(t *testing.T) {
	// Start a 9P session and walk to the fixture entry under test.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)
	doWalk(t, ctx, srv, 0, 1, "hello.txt")

	// Encode the modification-time change and unused identity and size fields.
	payload := NewWriteBuffer(64)
	payload.WriteU32(1)                              // fid
	payload.WriteU32(SetattrMtime | SetattrMtimeSet) // valid
	payload.WriteU32(0)                              // mode
	payload.WriteU32(0)                              // uid
	payload.WriteU32(0)                              // gid
	payload.WriteU64(0)                              // size

	// Complete the request with the requested modification timestamp.
	payload.WriteU64(0)          // atime_sec
	payload.WriteU64(0)          // atime_nsec
	payload.WriteU64(1234567890) // mtime_sec
	payload.WriteU64(0)          // mtime_nsec

	// Check that the read-only filesystem rejects changing the modification time.
	resp, err := srv.HandleMessage(ctx, buildMessage(TSETATTR, 3, payload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	assertRLError(t, resp, EROFS)

	// Release the session fids through the 9P clunk handler.
	doClunk(t, ctx, srv, 0)
	doClunk(t, ctx, srv, 1)
}

// --- Readdir Tests ---

func TestReaddirRoot(t *testing.T) {
	// Start a 9P session and negotiate access to the fixture filesystem.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)

	// Read the root directory entries and index their names.
	names := doReaddir(t, ctx, srv, 0)
	found := make(map[string]bool)
	for _, n := range names {
		found[n] = true
	}

	// Check that the root listing includes both fixture entries.
	if !found["hello.txt"] {
		t.Fatalf("missing hello.txt in %v", names)
	}
	if !found["subdir"] {
		t.Fatalf("missing subdir in %v", names)
	}

	// Release the session fids through the 9P clunk handler.
	doClunk(t, ctx, srv, 0)
}

func TestReaddirSubdir(t *testing.T) {
	// Start a 9P session and walk to the fixture entry under test.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)
	doWalk(t, ctx, srv, 0, 1, "subdir")

	// Read the subdirectory entries and index their names.
	names := doReaddir(t, ctx, srv, 1)
	found := make(map[string]bool)
	for _, n := range names {
		found[n] = true
	}

	// Check that the subdirectory listing includes nested.txt.
	if !found["nested.txt"] {
		t.Fatalf("missing nested.txt in subdir readdir: %v", names)
	}

	// Release the session fids through the 9P clunk handler.
	doClunk(t, ctx, srv, 0)
	doClunk(t, ctx, srv, 1)
}

func TestReaddirWithOffset(t *testing.T) {
	// Start a 9P session and negotiate access to the fixture filesystem.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)

	// first readdir at offset 0
	sendLopen(t, ctx, srv, 0)
	payload := NewWriteBuffer(16)
	payload.WriteU32(0)
	payload.WriteU64(0)
	payload.WriteU32(65536)
	resp, err := srv.HandleMessage(ctx, buildMessage(TREADDIR, 5, payload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	// Decode the complete directory listing for comparison with the offset listing.
	names0 := parseReaddirNames(t, resp)

	// readdir at offset 1 should skip first entry
	payload2 := NewWriteBuffer(16)
	payload2.WriteU32(0)
	payload2.WriteU64(1) // offset = 1
	payload2.WriteU32(65536)
	resp2, err := srv.HandleMessage(ctx, buildMessage(TREADDIR, 6, payload2.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	// Decode the directory listing that starts after the first entry.
	names1 := parseReaddirNames(t, resp2)

	// Check that the offset listing omits exactly one directory entry.
	if len(names1) != len(names0)-1 {
		t.Fatalf("offset readdir: got %d entries want %d (one fewer)", len(names1), len(names0)-1)
	}

	// Release the session fids through the 9P clunk handler.
	doClunk(t, ctx, srv, 0)
}

func TestReaddirPastEnd(t *testing.T) {
	// Start a 9P session and negotiate access to the fixture filesystem.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)
	sendLopen(t, ctx, srv, 0)

	// Encode a directory read request whose offset is past the final entry.
	payload := NewWriteBuffer(16)
	payload.WriteU32(0)
	payload.WriteU64(9999) // way past end
	payload.WriteU32(65536)

	// Send the directory read request through the 9P server.
	resp, err := srv.HandleMessage(ctx, buildMessage(TREADDIR, 5, payload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	// Check that a directory read past the final entry returns an empty listing.
	if resp[4] != RREADDIR {
		t.Fatalf("expected RREADDIR, got %d", resp[4])
	}
	r := NewReadBuffer(resp[headerSize:])
	dataLen := r.ReadU32()
	if dataLen != 0 {
		t.Fatalf("expected 0 bytes for offset past end, got %d", dataLen)
	}

	// Release the session fids through the 9P clunk handler.
	doClunk(t, ctx, srv, 0)
}

// --- Mkdir Tests (read-only FS -> EROFS) ---

func TestMkdirReadOnly(t *testing.T) {
	// Start a 9P session and negotiate access to the fixture filesystem.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)

	// Encode a directory creation request in the read-only root.
	payload := NewWriteBuffer(32)
	payload.WriteU32(0)
	payload.WriteString("newdir")
	payload.WriteU32(0o755)
	payload.WriteU32(1000)

	// Check that the read-only filesystem rejects directory creation.
	resp, err := srv.HandleMessage(ctx, buildMessage(TMKDIR, 3, payload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	assertRLError(t, resp, EROFS)

	// Release the session fids through the 9P clunk handler.
	doClunk(t, ctx, srv, 0)
}

// --- Mknod Tests (read-only FS -> EROFS) ---

func TestMknodReadOnly(t *testing.T) {
	// Start a 9P session and negotiate access to the fixture filesystem.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)

	// Encode a device-node creation request in the read-only root.
	payload := NewWriteBuffer(32)
	payload.WriteU32(0)
	payload.WriteString("dev")
	payload.WriteU32(0o644)
	payload.WriteU32(0) // major
	payload.WriteU32(0) // minor
	payload.WriteU32(1000)

	// Check that the read-only filesystem rejects device-node creation.
	resp, err := srv.HandleMessage(ctx, buildMessage(TMKNOD, 3, payload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	assertRLError(t, resp, EROFS)

	// Release the session fids through the 9P clunk handler.
	doClunk(t, ctx, srv, 0)
}

// --- Symlink Tests (read-only FS -> EROFS) ---

func TestSymlinkReadOnly(t *testing.T) {
	// Start a 9P session and negotiate access to the fixture filesystem.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)

	// Encode a symbolic-link creation request in the read-only root.
	payload := NewWriteBuffer(64)
	payload.WriteU32(0)
	payload.WriteString("link")
	payload.WriteString("/some/target")
	payload.WriteU32(1000)

	// Check that the read-only filesystem rejects symbolic-link creation.
	resp, err := srv.HandleMessage(ctx, buildMessage(TSYMLINK, 3, payload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	assertRLError(t, resp, EROFS)

	// Release the session fids through the 9P clunk handler.
	doClunk(t, ctx, srv, 0)
}

// --- Readlink Tests ---

func TestReadlinkNotSymlink(t *testing.T) {
	// Start a 9P session and walk to the fixture entry under test.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)
	doWalk(t, ctx, srv, 0, 1, "hello.txt")

	// Request a symbolic-link target from the regular-file fid.
	payload := NewWriteBuffer(4)
	payload.WriteU32(1) // fid pointing to regular file
	resp, err := srv.HandleMessage(ctx, buildMessage(TREADLINK, 3, payload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	// Check that reading a link target from a regular file returns invalid-argument errno.
	assertRLError(t, resp, EINVAL)

	// Release the session fids through the 9P clunk handler.
	doClunk(t, ctx, srv, 0)
	doClunk(t, ctx, srv, 1)
}

// --- Unlinkat Tests (read-only FS -> EROFS) ---

func TestUnlinkatReadOnly(t *testing.T) {
	// Start a 9P session and negotiate access to the fixture filesystem.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)

	// Encode a request to unlink hello.txt from the read-only root.
	payload := NewWriteBuffer(32)
	payload.WriteU32(0)
	payload.WriteString("hello.txt")
	payload.WriteU32(0) // flags

	// Check that the read-only filesystem rejects unlinking the fixture file.
	resp, err := srv.HandleMessage(ctx, buildMessage(TUNLINKAT, 3, payload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	assertRLError(t, resp, EROFS)

	// Release the session fids through the 9P clunk handler.
	doClunk(t, ctx, srv, 0)
}

// --- Remove Tests ---

func TestRemoveClunksFid(t *testing.T) {
	// Start a 9P session and walk to the fixture entry under test.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)
	doWalk(t, ctx, srv, 0, 1, "hello.txt")

	// TREMOVE returns ENOTSUP (deprecated in 9p2000.L) but still clunks the fid
	payload := NewWriteBuffer(4)
	payload.WriteU32(1)
	resp, err := srv.HandleMessage(ctx, buildMessage(TREMOVE, 3, payload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	assertRLError(t, resp, ENOTSUP)

	// fid 1 should be clunked despite the error
	getattrPayload := NewWriteBuffer(12)
	getattrPayload.WriteU32(1)
	getattrPayload.WriteU64(GetattrAll)
	resp, err = srv.HandleMessage(ctx, buildMessage(TGETATTR, 4, getattrPayload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if resp[4] != RLERROR {
		t.Fatalf("fid 1 should be gone after TREMOVE, got response type %d", resp[4])
	}

	// Release the session fids through the 9P clunk handler.
	doClunk(t, ctx, srv, 0)
}

// --- Clunk Tests ---

func TestClunkInvalidFid(t *testing.T) {
	// Start a 9P server over the fixture filesystem and release its fids on exit.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()

	// Check that the 9P server rejects clunking an unattached fid.
	payload := NewWriteBuffer(4)
	payload.WriteU32(99)
	resp, err := srv.HandleMessage(ctx, buildMessage(TCLUNK, 1, payload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if resp[4] != RLERROR {
		t.Fatalf("expected RLERROR for invalid fid clunk, got %d", resp[4])
	}
}

func TestDoubleClunk(t *testing.T) {
	// Start a 9P session and clunk its attached root fid.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)
	doClunk(t, ctx, srv, 0)

	// second clunk should fail
	payload := NewWriteBuffer(4)
	payload.WriteU32(0)
	resp, err := srv.HandleMessage(ctx, buildMessage(TCLUNK, 3, payload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if resp[4] != RLERROR {
		t.Fatalf("expected RLERROR for double clunk, got %d", resp[4])
	}
}

// --- Stub Handler Tests ---

// TestFsync tests that TFSYNC on an attached fid succeeds.
func TestFsync(t *testing.T) {
	// Attach fid 0.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)

	// Sync it.
	payload := NewWriteBuffer(8)
	payload.WriteU32(0) // fid
	payload.WriteU32(0) // datasync
	resp, err := srv.HandleMessage(ctx, buildMessage(TFSYNC, 1, payload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if resp[4] != RFSYNC {
		t.Fatalf("expected RFSYNC, got %d", resp[4])
	}

	// Release the fid.
	doClunk(t, ctx, srv, 0)
}

func TestLock(t *testing.T) {
	// Start a 9P server over the fixture filesystem and release its fids on exit.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()

	// Request a lock through the 9P server.
	resp, err := srv.HandleMessage(ctx, buildMessage(TLOCK, 1, nil))
	if err != nil {
		t.Fatal(err)
	}

	// Check that the lock response reports successful acquisition.
	if resp[4] != RLOCK {
		t.Fatalf("expected RLOCK, got %d", resp[4])
	}
	r := NewReadBuffer(resp[headerSize:])
	status := r.ReadU8()
	if status != LockSuccess {
		t.Fatalf("lock status: got %d want %d", status, LockSuccess)
	}
}

func TestGetlock(t *testing.T) {
	// Start a 9P session and negotiate access to the fixture filesystem.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)

	// Encode a lock query with a file range, process and client identity.
	payload := NewWriteBuffer(32)
	payload.WriteU32(0)            // fid
	payload.WriteU8(LockTypeRdlck) // type
	payload.WriteU64(100)          // start
	payload.WriteU64(200)          // length
	payload.WriteU32(42)           // proc_id
	payload.WriteString("client")

	// Check that the 9P server responds to the lock query.
	resp, err := srv.HandleMessage(ctx, buildMessage(TGETLOCK, 3, payload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if resp[4] != RGETLOCK {
		t.Fatalf("expected RGETLOCK, got %d", resp[4])
	}

	// Decode the lock range and client identity from the response.
	r := NewReadBuffer(resp[headerSize:])
	typ := r.ReadU8()
	start := r.ReadU64()
	length := r.ReadU64()
	procID := r.ReadU32()
	clientID := r.ReadString()

	// Check that the lock response preserves the requested range and client identity.
	if typ != LockTypeRdlck {
		t.Fatalf("type: got %d", typ)
	}
	if start != 100 {
		t.Fatalf("start: got %d", start)
	}
	if length != 200 {
		t.Fatalf("length: got %d", length)
	}
	if procID != 42 {
		t.Fatalf("proc_id: got %d", procID)
	}
	if clientID != "client" {
		t.Fatalf("client_id: got %q", clientID)
	}

	// Release the session fids through the 9P clunk handler.
	doClunk(t, ctx, srv, 0)
}

func TestFlush(t *testing.T) {
	// Start a 9P server over the fixture filesystem and release its fids on exit.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()

	// Check that the 9P server acknowledges flushing a request tag.
	payload := NewWriteBuffer(2)
	payload.WriteU16(0)
	resp, err := srv.HandleMessage(ctx, buildMessage(TFLUSH, 1, payload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if resp[4] != RFLUSH {
		t.Fatalf("expected RFLUSH, got %d", resp[4])
	}
}

func TestXattrwalkError(t *testing.T) {
	// Start a 9P server over the fixture filesystem and release its fids on exit.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()

	// Check that the 9P server reports extended-attribute walks as unsupported.
	resp, err := srv.HandleMessage(ctx, buildMessage(TXATTRWALK, 1, nil))
	if err != nil {
		t.Fatal(err)
	}
	assertRLError(t, resp, ENOTSUP)
}

func TestXattrcreateError(t *testing.T) {
	// Start a 9P server over the fixture filesystem and release its fids on exit.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()

	// Check that the 9P server reports extended-attribute creation as unsupported.
	resp, err := srv.HandleMessage(ctx, buildMessage(TXATTRCREATE, 1, nil))
	if err != nil {
		t.Fatal(err)
	}
	assertRLError(t, resp, ENOTSUP)
}

func TestLinkError(t *testing.T) {
	// Start a 9P server over the fixture filesystem and release its fids on exit.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()

	// Check that the 9P server reports hard-link creation as unsupported.
	resp, err := srv.HandleMessage(ctx, buildMessage(TLINK, 1, nil))
	if err != nil {
		t.Fatal(err)
	}
	assertRLError(t, resp, ENOTSUP)
}

// --- Statfs Tests ---

func TestStatfs(t *testing.T) {
	// Start a 9P session and negotiate access to the fixture filesystem.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)

	// Request filesystem statistics and check the response type.
	payload := NewWriteBuffer(4)
	payload.WriteU32(0)
	resp, err := srv.HandleMessage(ctx, buildMessage(TSTATFS, 3, payload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if resp[4] != RSTATFS {
		t.Fatalf("expected RSTATFS, got %d", resp[4])
	}

	// Decode the filesystem type and block-capacity statistics.
	r := NewReadBuffer(resp[headerSize:])
	fstype := r.ReadU32()
	bsize := r.ReadU32()
	blocks := r.ReadU64()
	bfree := r.ReadU64()
	bavail := r.ReadU64()

	// Check the reported filesystem type, block size and available capacity.
	if fstype != 0x01021997 {
		t.Fatalf("fstype: got %x want 01021997", fstype)
	}
	if bsize != 4096 {
		t.Fatalf("bsize: got %d want 4096", bsize)
	}
	if blocks == 0 {
		t.Fatal("blocks should be non-zero")
	}
	if bfree != blocks {
		t.Fatalf("bfree: got %d want %d", bfree, blocks)
	}
	if bavail != blocks {
		t.Fatalf("bavail: got %d want %d", bavail, blocks)
	}

	// Read past the file-count fields and check the maximum filename length.
	namelen := r.ReadU64() // files
	_ = namelen
	_ = r.ReadU64() // ffree
	_ = r.ReadU64() // fsid
	namelenVal := r.ReadU32()
	if namelenVal != 256 {
		t.Fatalf("namelen: got %d want 256", namelenVal)
	}

	// Release the session fids through the 9P clunk handler.
	doClunk(t, ctx, srv, 0)
}

// --- Concurrent HandleMessage ---

func TestConcurrentHandleMessage(t *testing.T) {
	// Start a 9P session and negotiate access to the fixture filesystem.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()
	doVersion(t, ctx, srv)

	// attach multiple fids concurrently
	var wg sync.WaitGroup
	errs := make([]error, 10)
	for i := range 10 {
		wg.Add(1)
		go func(fidID int) {
			// Encode a distinct root attach request and signal completion on exit.
			defer wg.Done()
			payload := NewWriteBuffer(32)
			payload.WriteU32(uint32(fidID)) //nolint:gosec
			payload.WriteU32(0xFFFFFFFF)
			payload.WriteString("user")
			payload.WriteString("")
			payload.WriteU32(1000)

			// Attach the root concurrently and retain any failure for this fid.
			resp, err := srv.HandleMessage(ctx, buildMessage(TATTACH, uint16(fidID), payload.Bytes())) //nolint:gosec
			if err != nil {
				errs[fidID] = err
				return
			}

			// Check that the concurrent root attach returns an attach response.
			if resp[4] != RATTACH {
				errs[fidID] = errShortRead // reuse as sentinel
			}
		}(i)
	}
	wg.Wait()

	// Check that every concurrent root attach completed successfully.
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent attach %d: %v", i, err)
		}
	}

	// clunk them all concurrently
	for i := range 10 {
		wg.Add(1)
		go func(fidID int) {
			// Clunk the assigned fid and signal completion on exit.
			defer wg.Done()
			payload := NewWriteBuffer(4)
			payload.WriteU32(uint32(fidID))                                                            //nolint:gosec
			_, err := srv.HandleMessage(ctx, buildMessage(TCLUNK, uint16(fidID+100), payload.Bytes())) //nolint:gosec
			if err != nil {
				errs[fidID] = err
			}
		}(i)
	}
	wg.Wait()

	// Check that every concurrent fid clunk completed successfully.
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent clunk %d: %v", i, err)
		}
	}
}

// --- Serve with Mock Transport ---

func TestServeWithTransport(t *testing.T) {
	// Start a 9P server over the fixture filesystem for the mock transport.
	ctx := t.Context()
	srv := newTestServer(t)
	defer srv.ReleaseAll()

	// build a sequence of messages
	versionPayload := NewWriteBuffer(32)
	versionPayload.WriteU32(65536)
	versionPayload.WriteString("9P2000.L")
	versionMsg := buildMessage(TVERSION, 0, versionPayload.Bytes())

	// Frame an attach request for the mock transport session.
	attachPayload := NewWriteBuffer(32)
	attachPayload.WriteU32(0)
	attachPayload.WriteU32(0xFFFFFFFF)
	attachPayload.WriteString("user")
	attachPayload.WriteString("")
	attachPayload.WriteU32(1000)
	attachMsg := buildMessage(TATTACH, 1, attachPayload.Bytes())

	// Frame a clunk request that releases the mock transport session fid.
	clunkPayload := NewWriteBuffer(4)
	clunkPayload.WriteU32(0)
	clunkMsg := buildMessage(TCLUNK, 2, clunkPayload.Bytes())

	// Queue the version, attach and clunk messages in the mock transport.
	mt := &mockTransport{
		msgs: [][]byte{versionMsg, attachMsg, clunkMsg},
	}

	// Serve until transport runs out of messages
	err := srv.Serve(ctx, mt)
	if err == nil {
		t.Fatal("expected error when transport runs out")
	}

	// Check that the mock transport received each expected response in order.
	if len(mt.responses) != 3 {
		t.Fatalf("expected 3 responses, got %d", len(mt.responses))
	}
	if mt.responses[0][4] != RVERSION {
		t.Fatalf("resp 0: expected RVERSION, got %d", mt.responses[0][4])
	}
	if mt.responses[1][4] != RATTACH {
		t.Fatalf("resp 1: expected RATTACH, got %d", mt.responses[1][4])
	}
	if mt.responses[2][4] != RCLUNK {
		t.Fatalf("resp 2: expected RCLUNK, got %d", mt.responses[2][4])
	}
}

func TestServeContextCancel(t *testing.T) {
	// Start a 9P server with a cancelable transport context.
	ctx, cancel := context.WithCancel(t.Context())
	srv := newTestServer(t)
	defer srv.ReleaseAll()

	// Use a mock transport whose reads wait for context cancellation.
	mt := &blockingTransport{ctx: ctx}

	// Serve the blocking transport and retain its completion error.
	done := make(chan error, 1)
	go func() {
		done <- srv.Serve(ctx, mt)
	}()

	// Cancel the transport context and check that serving ends with an error.
	cancel()
	err := <-done
	if err == nil {
		t.Fatal("expected error on cancel")
	}
}

// --- ReleaseAll Tests ---

func TestReleaseAll(t *testing.T) {
	// Start a 9P session with root, file and directory fids.
	ctx := t.Context()
	srv := newTestServer(t)
	doVersion(t, ctx, srv)
	doAttach(t, ctx, srv, 0)
	doWalk(t, ctx, srv, 0, 1, "hello.txt")
	doWalk(t, ctx, srv, 0, 2, "subdir")

	// Release every fid retained by the 9P server.
	srv.ReleaseAll()

	// all fids should be gone
	payload := NewWriteBuffer(4)
	payload.WriteU32(0)
	resp, err := srv.HandleMessage(ctx, buildMessage(TCLUNK, 1, payload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if resp[4] != RLERROR {
		t.Fatal("fids should be released")
	}
}

// --- Error Mapping ---

func TestToErrno(t *testing.T) {
	tests := []struct {
		err    error
		expect uint32
	}{
		{nil, 0},
		{context.Canceled, EINTR},
	}
	for _, tt := range tests {
		got := toErrno(tt.err)
		if got != tt.expect {
			t.Errorf("toErrno(%v): got %d want %d", tt.err, got, tt.expect)
		}
	}
}

// --- Helper Functions ---

func newTestServer(t *testing.T) *Server {
	// Prepare an in-memory filesystem with root and nested fixture entries.
	t.Helper()
	memFS := fstest.MapFS{
		"hello.txt":         {Data: []byte("world")},
		"subdir":            {Mode: 0o755 | 0o20000000000},
		"subdir/nested.txt": {Data: []byte("deep")},
	}

	// Open a filesystem cursor over the fixture entries.
	cursor, err := unixfs_iofs.NewFSCursor(memFS)
	if err != nil {
		t.Fatal(err)
	}

	// Transfer the fixture cursor into a filesystem handle for the server.
	handle, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		cursor.Release()
		t.Fatal(err)
	}
	return NewServer(handle)
}

func doVersion(t *testing.T, ctx context.Context, srv *Server) {
	t.Helper()
	resp := sendVersion(t, ctx, srv, 65536, "9P2000.L")
	if resp[4] != RVERSION {
		t.Fatalf("expected RVERSION, got %d", resp[4])
	}
}

func sendVersion(t *testing.T, ctx context.Context, srv *Server, msize uint32, version string) []byte {
	// Encode a version request with the requested wire version and message size.
	t.Helper()
	payload := NewWriteBuffer(32)
	payload.WriteU32(msize)
	payload.WriteString(version)

	// Negotiate the requested version through the 9P server.
	resp, err := srv.HandleMessage(ctx, buildMessage(TVERSION, 0, payload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func doAttach(t *testing.T, ctx context.Context, srv *Server, fidID uint32) {
	// Encode a root attach request with the supplied fid and user identity.
	t.Helper()
	payload := NewWriteBuffer(32)
	payload.WriteU32(fidID)
	payload.WriteU32(0xFFFFFFFF)
	payload.WriteString("user")
	payload.WriteString("")
	payload.WriteU32(1000)

	// Attach the root through the 9P server.
	resp, err := srv.HandleMessage(ctx, buildMessage(TATTACH, 1, payload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	// Check that the root attach returns an attach response.
	if resp[4] != RATTACH {
		t.Fatalf("expected RATTACH, got %d", resp[4])
	}
}

func doClunk(t *testing.T, ctx context.Context, srv *Server, fidID uint32) {
	// Encode a clunk request for the supplied fid.
	t.Helper()
	payload := NewWriteBuffer(4)
	payload.WriteU32(fidID)

	// Check that the 9P server acknowledges releasing the fid.
	resp, err := srv.HandleMessage(ctx, buildMessage(TCLUNK, 99, payload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if resp[4] != RCLUNK {
		t.Fatalf("expected RCLUNK, got %d", resp[4])
	}
}

func doWalk(t *testing.T, ctx context.Context, srv *Server, fid, newfid uint32, name string) {
	// Encode a single-component walk from the supplied fid to the new fid.
	t.Helper()
	payload := NewWriteBuffer(32)
	payload.WriteU32(fid)
	payload.WriteU32(newfid)
	payload.WriteU16(1)
	payload.WriteString(name)

	// Check that the 9P server acknowledges walking the fixture path.
	resp, err := srv.HandleMessage(ctx, buildMessage(TWALK, 2, payload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if resp[4] != RWALK {
		t.Fatalf("expected RWALK, got %d", resp[4])
	}
}

func sendLopen(t *testing.T, ctx context.Context, srv *Server, fidID uint32) []byte {
	// Encode an open request for the supplied fid.
	t.Helper()
	payload := NewWriteBuffer(8)
	payload.WriteU32(fidID)
	payload.WriteU32(0)

	// Check that the 9P server acknowledges opening the fid.
	resp, err := srv.HandleMessage(ctx, buildMessage(TLOPEN, 4, payload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if resp[4] != RLOPEN {
		t.Fatalf("expected RLOPEN, got %d", resp[4])
	}
	return resp
}

func sendGetattr(t *testing.T, ctx context.Context, srv *Server, fidID uint32) []byte {
	// Encode an attribute request for the supplied fid.
	t.Helper()
	payload := NewWriteBuffer(12)
	payload.WriteU32(fidID)
	payload.WriteU64(GetattrAll)

	// Check that the 9P server returns the requested attributes.
	resp, err := srv.HandleMessage(ctx, buildMessage(TGETATTR, 3, payload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if resp[4] != RGETATTR {
		t.Fatalf("expected RGETATTR, got %d", resp[4])
	}
	return resp
}

func doRead(t *testing.T, ctx context.Context, srv *Server, fidID uint32, offset uint64, count uint32) []byte {
	// Encode a file read request with the supplied fid, offset and count.
	t.Helper()
	payload := NewWriteBuffer(16)
	payload.WriteU32(fidID)
	payload.WriteU64(offset)
	payload.WriteU32(count)

	// Check that the 9P server acknowledges the file read.
	resp, err := srv.HandleMessage(ctx, buildMessage(TREAD, 5, payload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if resp[4] != RREAD {
		t.Fatalf("expected RREAD, got %d", resp[4])
	}

	// Decode the length-prefixed file contents from the read response.
	r := NewReadBuffer(resp[headerSize:])
	dataLen := r.ReadU32()
	return r.ReadBytes(int(dataLen))
}

func doReaddir(t *testing.T, ctx context.Context, srv *Server, fidID uint32) []string {
	// Open the supplied directory fid for listing.
	t.Helper()
	sendLopen(t, ctx, srv, fidID)

	// Encode a directory read request starting at the first entry.
	payload := NewWriteBuffer(16)
	payload.WriteU32(fidID)
	payload.WriteU64(0)
	payload.WriteU32(65536)

	// Check that the 9P server acknowledges the directory read.
	resp, err := srv.HandleMessage(ctx, buildMessage(TREADDIR, 5, payload.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if resp[4] != RREADDIR {
		t.Fatalf("expected RREADDIR, got %d", resp[4])
	}
	return parseReaddirNames(t, resp)
}

func parseReaddirNames(t *testing.T, resp []byte) []string {
	// Decode the length-prefixed directory listing from the response.
	t.Helper()
	r := NewReadBuffer(resp[headerSize:])
	dataLen := r.ReadU32()
	data := r.ReadBytes(int(dataLen))
	dr := NewReadBuffer(data)

	// Collect directory entry names until the listing ends or decoding fails.
	var names []string
	for dr.Remaining() > 0 {
		dr.ReadQID()
		dr.ReadU64()
		dr.ReadU8()
		name := dr.ReadString()
		if dr.Err() != nil {
			break
		}
		names = append(names, name)
	}
	return names
}

func assertRLError(t *testing.T, resp []byte, expectedErrno uint32) {
	// Check that the 9P response carries an error message type.
	t.Helper()
	if resp[4] != RLERROR {
		t.Fatalf("expected RLERROR, got %d", resp[4])
	}

	// Decode the error payload and check its errno against the expected failure.
	r := NewReadBuffer(resp[headerSize:])
	errno := r.ReadU32()
	if errno != expectedErrno {
		t.Fatalf("errno: got %d want %d", errno, expectedErrno)
	}
}

// --- Mock Transport ---

type mockTransport struct {
	msgs      [][]byte
	idx       int
	responses [][]byte
}

func (m *mockTransport) ReadMessage(_ context.Context) ([]byte, error) {
	if m.idx >= len(m.msgs) {
		return nil, errShortRead
	}
	msg := m.msgs[m.idx]
	m.idx++
	return msg, nil
}

func (m *mockTransport) WriteMessage(_ context.Context, data []byte) error {
	m.responses = append(m.responses, data)
	return nil
}

type blockingTransport struct {
	ctx context.Context
}

func (b *blockingTransport) ReadMessage(ctx context.Context) ([]byte, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (b *blockingTransport) WriteMessage(_ context.Context, _ []byte) error {
	return nil
}
