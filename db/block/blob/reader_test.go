package blob

import (
	"bytes"
	"io"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
)

// TestRawBlobReadSeek checks raw reads and repositioning through the reader API.
func TestRawBlobReadSeek(t *testing.T) {
	// Open a raw reader over known content and release its read state after the test.
	blob := buildMockRawBlob()
	rdr := NewRawReader(t.Context(), blob)
	t.Cleanup(func() { _ = rdr.Close() })

	// Read the complete value before seeking back to its beginning.
	data, err := io.ReadAll(rdr)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(blob.RawData, data) {
		t.Fatalf("read %q, want %q", data, blob.RawData)
	}

	// Reposition through the public interface and read the same leading bytes.
	position, err := rdr.Seek(0, io.SeekStart)
	if err != nil || position != 0 {
		t.Fatalf("seek = %d, %v", position, err)
	}
	buf := make([]byte, 4)
	n, err := rdr.Read(buf)
	if err != nil || n != len(buf) {
		t.Fatalf("read = %d, %v", n, err)
	}
	if !bytes.Equal(buf, blob.RawData[:len(buf)]) {
		t.Fatalf("prefix = %q, want %q", buf, blob.RawData[:len(buf)])
	}
}

// TestChunkedReaderZeroTailAdvances checks a persisted blob grown past its chunks.
func TestChunkedReaderZeroTailAdvances(t *testing.T) {
	// Build one chunked value through the ordinary block store.
	ctx := t.Context()
	store := block_mock.NewMockStore(0)
	tx, cursor := block.NewTransaction(store, nil, nil, nil)
	data := bytes.Repeat([]byte("chunk-data"), 20)
	opts := &BuildBlobOpts{RawHighWaterMark: 1}
	blob, err := BuildBlob(ctx, int64(len(data)), bytes.NewReader(data), cursor, opts)
	if err != nil {
		t.Fatal(err)
	}

	// Extend its logical size without adding stored zero chunks.
	const zeros = 17
	if err := blob.Truncate(ctx, cursor, opts, int64(len(data)+zeros)); err != nil {
		t.Fatal(err)
	}

	// Persist and reopen so the reader follows the actual stored chunk index.
	ref, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	_, cursor = block.NewTransaction(store, nil, ref, nil)
	rdr, err := NewReader(ctx, cursor)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rdr.Close() })

	// Bound the read one byte past the declared end so a broken reader cannot loop.
	got, err := io.ReadAll(io.LimitReader(rdr, int64(len(data)+zeros+1)))
	if err != nil {
		t.Fatal(err)
	}
	want := append(bytes.Clone(data), make([]byte, zeros)...)
	if !bytes.Equal(got, want) {
		t.Fatalf("read %d bytes, want %d bytes with a zero tail", len(got), len(want))
	}

	// Repeated small tail reads advance the observable position and reach EOF once.
	if _, err := rdr.Seek(int64(len(data)), io.SeekStart); err != nil {
		t.Fatal(err)
	}
	for offset := 0; offset < zeros; {
		// Check this bounded read and the position it publishes.
		buf := bytes.Repeat([]byte{1}, 4)
		n, err := rdr.Read(buf)
		if err != nil || n != min(len(buf), zeros-offset) {
			t.Fatalf("tail read at %d = %d, %v", offset, n, err)
		}
		if !bytes.Equal(buf[:n], make([]byte, n)) {
			t.Fatalf("tail read at %d is not zero-filled", offset)
		}
		offset += n
		position, err := rdr.Seek(0, io.SeekCurrent)
		if err != nil || position != int64(len(data)+offset) {
			t.Fatalf("tail position = %d, %v", position, err)
		}
	}

	// Exhaustion remains stable after the final implicit zero byte.
	buf := make([]byte, 1)
	for range 2 {
		if n, err := rdr.Read(buf); n != 0 || err != io.EOF {
			t.Fatalf("exhausted read = %d, %v", n, err)
		}
	}
}
