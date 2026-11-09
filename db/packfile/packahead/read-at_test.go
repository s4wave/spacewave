package packahead

import (
	"bytes"
	"io"
	"testing"
)

// eofReaderAt returns io.EOF with any read that reaches the end of its data,
// as the io.ReaderAt contract allows.
type eofReaderAt []byte

func (r eofReaderAt) ReadAt(p []byte, off int64) (int, error) {
	n := copy(p, r[off:])
	if int(off)+n == len(r) {
		return n, io.EOF
	}
	return n, nil
}

// TestReadAtFullAcceptsEOF checks a full read ending at the end of the file
// succeeds when the reader reports io.EOF with it, and a short read fails.
func TestReadAtFullAcceptsEOF(t *testing.T) {
	data := eofReaderAt("pack bytes")
	buf := make([]byte, 5)
	if err := readAtFull(data, buf, 5); err != nil {
		t.Fatalf("full read at the end: %v", err)
	}
	if !bytes.Equal(buf, []byte("bytes")) {
		t.Fatalf("read %q, want %q", buf, "bytes")
	}
	if err := readAtFull(data, make([]byte, 0), 10); err != nil {
		t.Fatalf("empty read at the end: %v", err)
	}
	if err := readAtFull(data, make([]byte, 6), 5); err != io.EOF {
		t.Fatalf("short read = %v, want io.EOF", err)
	}
}
