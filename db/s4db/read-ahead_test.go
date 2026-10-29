//go:build darwin || linux || windows

package s4db

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
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
