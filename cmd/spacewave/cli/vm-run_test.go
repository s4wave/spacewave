//go:build !js

package spacewave_cli

import (
	"context"
	"errors"
	"io"
	"testing"
)

type chunkReader struct {
	chunks [][]byte
	idx    int
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if r.idx >= len(r.chunks) {
		return 0, io.EOF
	}
	n := copy(p, r.chunks[r.idx])
	r.idx++
	return n, nil
}

func newEscapeReader(chunks ...[]byte) (*serialEscapeReader, context.Context) {
	ctx, cancel := context.WithCancel(context.Background())
	return &serialEscapeReader{src: &chunkReader{chunks: chunks}, quit: cancel}, ctx
}

func TestSerialEscapeReaderPassThrough(t *testing.T) {
	r, ctx := newEscapeReader([]byte("hello"))
	buf := make([]byte, 16)
	n, err := r.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf[:n]) != "hello" {
		t.Fatalf("payload = %q, want hello", buf[:n])
	}
	if ctx.Err() != nil {
		t.Fatal("plain input must not quit")
	}
}

func TestSerialEscapeReaderQuit(t *testing.T) {
	r, ctx := newEscapeReader([]byte{'h', 'i', serialEscapePrefix, 'x'})
	buf := make([]byte, 16)
	n, err := r.Read(buf)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("err = %v, want EOF", err)
	}
	if string(buf[:n]) != "hi" {
		t.Fatalf("flushed payload = %q, want hi", buf[:n])
	}
	if ctx.Err() == nil {
		t.Fatal("Ctrl-A x must cancel the context")
	}
}

func TestSerialEscapeReaderLiteralPrefix(t *testing.T) {
	r, _ := newEscapeReader([]byte{serialEscapePrefix, serialEscapePrefix})
	buf := make([]byte, 16)
	n, err := r.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if n != 1 || buf[0] != serialEscapePrefix {
		t.Fatalf("payload = %v, want single Ctrl-A", buf[:n])
	}
}

func TestSerialEscapeReaderUnknownEscape(t *testing.T) {
	r, ctx := newEscapeReader([]byte{serialEscapePrefix, 'y'})
	buf := make([]byte, 16)
	n, err := r.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf[:n]) != string([]byte{serialEscapePrefix, 'y'}) {
		t.Fatalf("payload = %v, want Ctrl-A then y", buf[:n])
	}
	if ctx.Err() != nil {
		t.Fatal("unknown escape must not quit")
	}
}

func TestSerialEscapeReaderPrefixAcrossReads(t *testing.T) {
	r, _ := newEscapeReader([]byte{serialEscapePrefix}, []byte{'a'})
	buf := make([]byte, 16)
	n, err := r.Read(buf)
	if err != nil || n != 0 {
		t.Fatalf("first read = (%d, %v), want (0, nil)", n, err)
	}
	n, err = r.Read(buf)
	if err != nil {
		t.Fatalf("second read: %v", err)
	}
	if string(buf[:n]) != string([]byte{serialEscapePrefix, 'a'}) {
		t.Fatalf("payload = %v, want Ctrl-A then a", buf[:n])
	}
}
