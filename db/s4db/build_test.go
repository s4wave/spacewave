package s4db

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/pkg/errors"
)

// buildValue returns the value of key number i: mostly inline, every seventh
// in a value chunk, and the first past a whole chunk.
func buildValue(i int) []byte {
	// Pick the size, then fill it from the index.
	n := 40 + i%200
	switch {
	case i == 0:
		n = chunkSize + 1234
	case i%7 == 0:
		n = inlineMax + 1 + i%3000
	}
	b := make([]byte, n)
	for j := range b {
		b[j] = byte(i + j)
	}
	return b
}

// TestBuild builds a file from a sorted stream with small runs, so the runs
// outnumber one record's ids, then opens it, reads every key back, writes
// past the memtable budget so a flush adds a run, and reads again. It also
// checks that an out-of-order key fails and leaves no file.
func TestBuild(t *testing.T) {
	// Create the file.
	ctx := context.Background()
	const keys = 40_000
	path := filepath.Join(t.TempDir(), "built.s4db")
	key := func(i int) []byte { return binary.BigEndian.AppendUint64([]byte("k"), uint64(i)) }
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	id, err := openFiles.claim(f)
	if err != nil {
		t.Fatal(err)
	}

	// Build with small runs.
	s := &osFile{File: f, path: path, id: id}
	err = build(ctx, s, Options{memtable: minMemtable}, 64<<10, func(yield func(key, value []byte) error) error {
		for i := range keys {
			if err := yield(key(i), buildValue(i)); err != nil {
				return err
			}
		}
		return nil
	})
	if cerr := s.close(); err == nil {
		err = cerr
	}
	if err != nil {
		t.Fatal(err)
	}

	// Open and check the shape.
	db, err := Open(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	st, err := db.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("built: %d keys, runs per level %v, %d chunks, %d bytes", st.KeyCount, st.Runs, st.Chunks.Run+st.Chunks.Value+st.Chunks.Manifest+st.Chunks.Log, st.FileBytes)
	if st.KeyCount != keys || len(st.Runs) < 2 || st.Runs[0] != 0 || st.Runs[len(st.Runs)-1] <= 16 {
		t.Fatalf("stats %+v", st)
	}

	// Read every key back.
	checkKeys := func(extra int) {
		// Iterate the whole key space on a read transaction.
		tx, err := db.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Discard()
		it := tx.Iterate(ctx, nil, true, false)
		defer it.Close()

		// Compare each pair with the one written.
		n := 0
		for it.Next() {
			want := buildValue(n)
			if n >= keys {
				want = []byte(fmt.Sprintf("extra %d", n))
			}
			got, err := it.Value()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(it.Key(), key(n)) || !bytes.Equal(got, want) {
				t.Fatalf("key %d: got %q (%d bytes), want %d bytes", n, it.Key(), len(got), len(want))
			}
			n++
		}
		if err := it.Err(); err != nil || n != keys+extra {
			t.Fatalf("read %d keys, want %d: %v", n, keys+extra, err)
		}
	}
	checkKeys(0)

	// Write past the memtable budget, so a flush names a run by the next
	// record, and read again.
	const extra = 3000
	for i := keys; i < keys+extra; i += 500 {
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		for j := i; j < i+500; j++ {
			if err := tx.Set(ctx, key(j), []byte(fmt.Sprintf("extra %d", j))); err != nil {
				t.Fatal(err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	checkKeys(extra)

	// An out-of-order stream fails and leaves no file.
	bad := filepath.Join(t.TempDir(), "bad.s4db")
	err = Build(ctx, bad, Options{}, func(yield func(key, value []byte) error) error {
		if err := yield([]byte("b"), []byte("1")); err != nil {
			return err
		}
		return yield([]byte("a"), []byte("2"))
	})
	if !errors.Is(err, ErrOutOfOrder) {
		t.Fatalf("out of order: %v", err)
	}
	if _, err := os.Stat(bad); !os.IsNotExist(err) {
		t.Fatalf("bad file left: %v", err)
	}
}
