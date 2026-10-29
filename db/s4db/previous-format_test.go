//go:build darwin || linux || windows

package s4db

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/pkg/errors"
)

// packedValue is the value testdata/packed.s4wave.gz holds under key i at
// generation gen: inline below every seventh key, which holds a 4 KiB value
// outside the index.
func packedValue(i, gen int) []byte {
	n := 40 + i%90
	if i%7 == 0 {
		n = 4000 + i%5*300
	}
	return bytes.Repeat(fmt.Appendf(nil, "%d.%d;", i, gen), n)[:n]
}

// TestOpenPrevious reads a file the packed value format wrote, holding keys
// 0 to 1,999 in its checkpoint and a log after it that rewrites every third
// key, adds keys 2,000 to 2,199 and deletes every eleventh. Only a read-only
// open succeeds; it serves every key through the old record headers,
// refuses writes, and changes nothing but its reader slot.
func TestOpenPrevious(t *testing.T) {
	// Unpack the fixture.
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "packed.s4wave")
	before := unpackFixture(t, "testdata/packed.s4wave.gz", path)

	// A read-only open of a missing file creates nothing.
	missing := filepath.Join(filepath.Dir(path), "missing.s4wave")
	if _, err := Open(missing, Options{ReadOnly: true}); err == nil {
		t.Fatal("read-only open of a missing file succeeded")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("read-only open created the file: %v", err)
	}

	// A writable open refuses the format; a read-only one replays the log.
	if _, err := Open(path, Options{}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("writable open: %v, want ErrUnsupported", err)
	}
	db, err := Open(path, Options{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if st := db.cur.Load(); st.seq <= st.ckpt {
		t.Fatalf("no log after the checkpoint: seq %d, checkpoint %d", st.seq, st.ckpt)
	}

	// Every key reads its last value, and the deleted ones are gone.
	tx, err := db.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	want := 0
	for i := range 2200 {
		data, found, err := tx.Get(ctx, fmt.Appendf(nil, "k%05d", i))
		if err != nil {
			t.Fatalf("key %d: %v", i, err)
		}
		if i < 2000 && i%11 == 0 {
			if found {
				t.Fatalf("deleted key %d found", i)
			}
			continue
		}
		want++
		gen := 0
		if i%3 == 0 || i >= 2000 {
			gen = 1
		}
		if !found || !bytes.Equal(data, packedValue(i, gen)) {
			t.Fatalf("key %d: found %v, %d bytes", i, found, len(data))
		}
	}
	if n, err := tx.Size(ctx); err != nil || n != uint64(want) {
		t.Fatalf("Size = %d, %v; want %d", n, err, want)
	}
	tx.Discard()

	// Writes are refused.
	if _, err := db.NewTransaction(ctx, true); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("write transaction: %v, want ErrReadOnly", err)
	}
	if err := db.Compact(ctx); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("Compact: %v, want ErrReadOnly", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// Only the reader slot changed.
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	slot := func(b []byte) []byte {
		b = bytes.Clone(b)
		clear(b[slotPage*pageSize : (slotPage+1)*pageSize])
		return b
	}
	if !bytes.Equal(slot(before), slot(after)) {
		t.Fatal("the read-only open changed the file")
	}
}

// unpackFixture writes the gzipped file src to dst and returns its bytes.
func unpackFixture(t *testing.T, src, dst string) []byte {
	// Decompress the fixture.
	gz, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	zr, err := gzip.NewReader(gz)
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}

	// Write it where the test opens it.
	if err := os.WriteFile(dst, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return b
}
