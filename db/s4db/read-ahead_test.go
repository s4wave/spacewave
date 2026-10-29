//go:build darwin || linux || windows

package s4db

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/kvtx"
)

// TestBuildReadAheadFailure fails the index reads of a cold build over a
// three-level tree partway through, while the build reads ahead, and checks
// that the build returns the error.
func TestBuildReadAheadFailure(t *testing.T) {
	// Open a file without the background cleaner.
	ctx := context.Background()
	db := openTest(t, filepath.Join(t.TempDir(), "ahead.s4wave"), Options{SingleProcess: true})
	defer db.Close()
	if wait, _ := db.cleaner.SetRoutine(nil); wait != nil {
		<-wait
	}

	// Commit each change in one transaction.
	val := make([]byte, 150)
	keys := checkpointKeys()[:60000]
	commit := func(fn func(tx *Tx)) {
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		fn(tx.(*Tx))
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}

	// Commit the keys and fold them into the tree.
	commit(func(tx *Tx) {
		for _, key := range keys {
			_ = tx.Set(ctx, key, val)
		}
	})
	if err := db.w.lock(ctx); err != nil {
		t.Fatal(err)
	}
	err := db.w.checkpointDurable(ctx)
	db.w.unlock(err)
	if err != nil {
		t.Fatal(err)
	}

	// Check the root has more inner children than the build reads ahead.
	root, err := db.p.node(db.cur.Load().root)
	if err != nil {
		t.Fatal(err)
	}
	kid, err := db.p.node(root.kid(0))
	if err != nil || root.count() <= aheadBatches || kid.leaf {
		t.Fatalf("root has %d children, want over %d inner ones: %v", root.count(), aheadBatches, err)
	}

	// Change every key, then fail index reads after the first 50 of a cold
	// build.
	commit(func(tx *Tx) {
		for _, key := range keys {
			_ = tx.Set(ctx, key, val[:100])
		}
	})
	file := db.p.r
	db.p.cache = newCache(db.opts.CacheBytes)
	db.p.r = &failReader{r: file, after: 50}
	defer func() { db.p.r = file }()

	// The build returns the read error.
	done := make(chan error, 1)
	go func() {
		tree := db.w.newTree(db.cur.Load())
		tree.b.out.reserve = func(n uint64) ([]run, error) {
			return nil, errors.New("reserved")
		}
		done <- tree.build()
	}()
	select {
	case err := <-done:
		if !errors.Is(err, errRead) {
			t.Fatalf("build = %v, want the read error", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("build hung after a failed read")
	}
}

// errRead is the error failReader returns.
var errRead = errors.New("read failed")

// failReader fails each read after the first after reads.
type failReader struct {
	// r reads the file.
	r io.ReaderAt
	// after is the reads that succeed.
	after int64
	// n counts the reads.
	n atomic.Int64
}

// ReadAt reads, or fails once after reads have passed.
func (f *failReader) ReadAt(b []byte, off int64) (int, error) {
	if f.n.Add(1) > f.after {
		return 0, errRead
	}
	return f.r.ReadAt(b, off)
}

// TestIteratorValueAhead scans stored values through iterators that read
// ahead, over a tree the overlay and a transaction's buffer shadow, and
// checks every value against the expected contents.
func TestIteratorValueAhead(t *testing.T) {
	// Open a file without the background cleaner.
	ctx := context.Background()
	db := openTest(t, filepath.Join(t.TempDir(), "values.s4wave"), Options{SingleProcess: true})
	defer db.Close()
	if wait, _ := db.cleaner.SetRoutine(nil); wait != nil {
		<-wait
	}

	// Values name their key and generation, and the store holds more of
	// them than the read-ahead budget.
	want := make(map[string][]byte)
	value := func(key string, gen, n int) []byte {
		return bytes.Repeat([]byte(fmt.Sprintf("%s/%d.", key, gen)), n/8)
	}

	// Commit values larger than the inline limit.
	tx, err := db.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 4000 {
		key := fmt.Sprintf("k%05d", i)
		want[key] = value(key, 0, 4096)
		_ = tx.Set(ctx, []byte(key), want[key])
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Fold them into the tree.
	if err := db.w.lock(ctx); err != nil {
		t.Fatal(err)
	}
	err = db.w.checkpointDurable(ctx)
	db.w.unlock(err)
	if err != nil {
		t.Fatal(err)
	}

	// change rewrites every nth key at a generation, deleting it when n is 0.
	change := func(tx kvtx.Tx, gen, every, n int) {
		for i := 0; i < 4000; i += every {
			key := fmt.Sprintf("k%05d", i)
			if n == 0 {
				delete(want, key)
				_ = tx.Delete(ctx, []byte(key))
				continue
			}
			want[key] = value(key, gen, n)
			_ = tx.Set(ctx, []byte(key), want[key])
		}
	}

	// Shadow some tree values in the overlay with stored and inline values
	// and deletions.
	tx, err = db.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	change(tx, 1, 5, 1024)
	change(tx, 2, 11, 0)
	change(tx, 3, 13, 64)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Shadow more in the buffer of the transaction that scans.
	tx, err = db.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	change(tx, 4, 17, 2048)
	keys := slices.Sorted(maps.Keys(want))

	// scan checks the keys and values an iterator returns from seek, taking
	// each value twice.
	scan := func(prefix string, reverse bool, seek string) {
		// List the keys expected from the seek on.
		t.Helper()
		var exp []string
		for _, key := range keys {
			if strings.HasPrefix(key, prefix) && (seek == "" || key >= seek) {
				exp = append(exp, key)
			}
		}
		if reverse {
			slices.Reverse(exp)
		}

		// Read 100 values before seeking, so the read-ahead runs.
		it := tx.Iterate(ctx, []byte(prefix), true, reverse)
		defer it.Close()
		if seek != "" {
			for range 100 {
				if !it.Next() {
					t.Fatal("scan ended before the seek")
				}
				_, _ = it.Value()
			}
			if err := it.Seek([]byte(seek)); err != nil {
				t.Fatal(err)
			}
		} else {
			it.Next()
		}

		// Check each key and its value, taken twice.
		for i, key := range exp {
			if !it.Valid() || string(it.Key()) != key {
				t.Fatalf("scan %q reverse %v at %d: key %q, want %q: %v", prefix, reverse, i, it.Key(), key, it.Err())
			}
			for range 2 {
				got, err := it.Value()
				if err != nil || !bytes.Equal(got, want[key]) {
					t.Fatalf("value of %q: %d bytes, want %d: %v", key, len(got), len(want[key]), err)
				}
			}
			it.Next()
		}
		if it.Valid() || it.Err() != nil {
			t.Fatalf("scan %q reverse %v: extra key %q: %v", prefix, reverse, it.Key(), it.Err())
		}
	}
	scan("", false, "")
	scan("", true, "")
	scan("k01", false, "")
	scan("k01", true, "")
	scan("", false, "k02500")
}
