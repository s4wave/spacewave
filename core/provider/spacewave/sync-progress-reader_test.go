package provider_spacewave

import (
	"bytes"
	"io"
	"testing"
	"time"
)

// TestSyncPushProgressReaderRoundtripPassThrough verifies the reader passes
// underlying bytes through unchanged and reports the final byte total at EOF.
func TestSyncPushProgressReaderRoundtripPassThrough(t *testing.T) {
	body := bytes.Repeat([]byte("a"), 1024)
	src := bytes.NewReader(body)

	var calls []int64
	r := newSyncPushProgressReader(src, func(sent int64) {
		calls = append(calls, sent)
	})

	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("payload mismatch: got %d bytes, want %d", len(got), len(body))
	}
	if len(calls) == 0 {
		t.Fatalf("expected at least the EOF callback")
	}
	if final := calls[len(calls)-1]; final != int64(len(body)) {
		t.Fatalf("final callback sent=%d, want %d", final, len(body))
	}
}

// TestSyncPushProgressReaderThrottle verifies two reads in tight succession
// produce at most one mid-stream callback before the throttle window elapses.
func TestSyncPushProgressReaderThrottle(t *testing.T) {
	src := &chunkedReader{chunks: [][]byte{
		bytes.Repeat([]byte("a"), 32),
		bytes.Repeat([]byte("b"), 32),
	}}

	var midCount int
	r := newSyncPushProgressReader(src, func(sent int64) {
		midCount++
	})

	buf := make([]byte, 64)
	if _, err := r.Read(buf); err != nil {
		t.Fatalf("Read 1: %v", err)
	}
	if _, err := r.Read(buf); err != nil {
		t.Fatalf("Read 2: %v", err)
	}

	if midCount > 1 {
		t.Fatalf("expected at most 1 mid-stream callback within throttle window, got %d", midCount)
	}
}

// TestSyncPushProgressReaderFiresAfterInterval verifies the callback fires
// once a Read happens after syncPushProgressInterval elapses.
func TestSyncPushProgressReaderFiresAfterInterval(t *testing.T) {
	src := &chunkedReader{chunks: [][]byte{
		bytes.Repeat([]byte("a"), 32),
		bytes.Repeat([]byte("b"), 32),
	}}

	var callCount int
	var lastSent int64
	r := newSyncPushProgressReader(src, func(sent int64) {
		callCount++
		lastSent = sent
	})

	r.next = time.Now().Add(-time.Second).UnixNano()

	buf := make([]byte, 64)
	if _, err := r.Read(buf); err != nil {
		t.Fatalf("Read: %v", err)
	}

	if callCount != 1 {
		t.Fatalf("expected 1 mid-stream callback after expired throttle, got %d", callCount)
	}
	if lastSent != 32 {
		t.Fatalf("callback sent=%d, want 32", lastSent)
	}
}

// chunkedReader returns one chunk per Read call to simulate a streaming source.
type chunkedReader struct {
	chunks [][]byte
}

func (r *chunkedReader) Read(p []byte) (int, error) {
	if len(r.chunks) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.chunks[0])
	r.chunks = r.chunks[1:]
	return n, nil
}
