//go:build darwin || linux || windows

package s4db

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/pkg/errors"
)

// TestBuildOwnerRacedOpen preserves a writer that initialized a newly created
// file before its builder could acquire ownership.
func TestBuildOwnerRacedOpen(t *testing.T) {
	// Commit real data in a separate process during the pre-claim interval.
	const writerEnv = "S4DB_TEST_BUILD_INTERVENING_WRITER"
	if path := os.Getenv(writerEnv); path != "" {
		db := openTest(t, path, Options{})
		defer db.Close()
		tx, err := db.NewTransaction(t.Context(), true)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Discard()
		if err := tx.Set(t.Context(), []byte("accepted"), []byte("keep")); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(t.Context()); err != nil {
			t.Fatal(err)
		}
		return
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "raced-build.s4wave")
	f, closeFile, err := OpenFile(path, func(name string) (*os.File, error) {
		return openNativeFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer closeFile()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestBuildOwnerRacedOpen$", "-test.timeout=15s")
	cmd.Env = append(os.Environ(), writerEnv+"="+path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("intervening writer: %v\n%s", err, out)
	}

	// Resume the same production claim/build path after the accepted writer closes.
	s := &osFile{File: f, path: path, closeFile: closeFile}
	called := false
	err = buildNative(ctx, s, Options{}, func(func([]byte, []byte) error) error {
		called = true
		return nil
	})
	if called || !errors.Is(err, os.ErrExist) {
		t.Fatalf("builder resumed after intervening writes: source=%t error=%v", called, err)
	}
	db := openTest(t, path, Options{ReadOnly: true})
	defer db.Close()
	tx, err := db.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	value, found, err := tx.Get(ctx, []byte("accepted"))
	if err != nil || !found || string(value) != "keep" {
		t.Fatalf("intervening commit lost: found=%t value=%q error=%v", found, value, err)
	}
}

// buildValue returns the value of key number i: mostly inline, every seventh
// in a value chunk, and every ten-thousandth past a whole chunk, so a large
// value also arrives while a value chunk is partly filled.
func buildValue(i int) []byte {
	// Pick the size, then fill it from the index.
	n := 40 + i%200
	switch {
	case i%10_000 == 0:
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
	f, closeFile, err := OpenFile(path, func(name string) (*os.File, error) {
		return os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}

	// Build with small runs.
	s := &osFile{File: f, path: path, closeFile: closeFile}
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

	// Read every key and its length from the index back.
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
			if l := it.(*Iterator).ValueLen(); l != len(want) {
				t.Fatalf("key %d: index length %d, want %d", n, l, len(want))
			}
			n++
		}
		if err := it.Err(); err != nil || n != keys+extra {
			t.Fatalf("read %d keys, want %d: %v", n, keys+extra, err)
		}
	}
	checkKeys(0)

	// Get every key, scan in reverse and seek across the level's many runs.
	m := make(model, keys)
	for n := range keys {
		m[string(key(n))] = buildValue(n)
	}
	m.check(t, db)

	// Write past the memtable budget, so a flush assigns a run ID from the
	// next record, and read again.
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

	// A valid checksum cannot make an older record layout safe to interpret.
	for _, oldVersion := range []bool{true, false} {
		t.Run(fmt.Sprintf("unsupported-header-%t", oldVersion), func(t *testing.T) {
			// Build a real file, then change only its format declaration.
			path := filepath.Join(t.TempDir(), "unsupported.s4db")
			if err := Build(ctx, path, Options{}, func(func([]byte, []byte) error) error { return nil }); err != nil {
				t.Fatal(err)
			}
			f, err := os.OpenFile(path, os.O_RDWR, 0)
			if err != nil {
				t.Fatal(err)
			}

			// Encode the unsupported declaration with a valid checksum.
			h := newHeader(defaultMemtable)
			if oldVersion {
				h.version = 2
			}
			h.incompat = 0
			if _, err := f.WriteAt(h.encode(), headerPage*pageSize); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}

			// Refuse the unsupported layout before any log or checkpoint is replayed.
			if db, err := Open(path, Options{}); !errors.Is(err, ErrUnsupported) {
				if db != nil {
					_ = db.Close()
				}
				t.Fatalf("unsupported header: %v", err)
			}
		})
	}
}
