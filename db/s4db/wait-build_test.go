//go:build darwin || linux || windows

package s4db

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
)

// TestUserCommitSkipsBuildWait holds a background checkpoint while a
// background bulk commit pushes the overlay past its bound, and checks that a
// user commit as large returns while the background commit waits for the
// build, and that only the background wait is counted.
func TestUserCommitSkipsBuildWait(t *testing.T) {
	// Open a file with small checkpoints whose builds wait for the gate.
	ctx := context.Background()
	opts := Options{CheckpointMin: 16 << 10, CheckpointMax: 64 << 10}
	db := openTest(t, filepath.Join(t.TempDir(), "wait.s4wave"), opts)
	defer db.Close()
	started, gate := make(chan struct{}, 1), make(chan struct{})
	db.w.beforeBuild = func() {
		select {
		case started <- struct{}{}:
		default:
		}
		<-gate
	}
	released := false
	defer func() {
		if !released {
			close(gate)
		}
	}()

	// Commit a background batch over twice CheckpointMax, which starts the
	// build and waits for it.
	bulk := make(chan error, 1)
	go func() {
		bulk <- commitBulk(block.WithBackground(ctx), db, "bulk")
	}()
	select {
	case <-started:
	case err := <-bulk:
		t.Fatalf("bulk commit returned %v before the build started", err)
	case <-time.After(5 * time.Second):
		t.Fatal("the build did not start")
	}
	if got, limit := db.cur.Load().overlayBytes, 2*opts.CheckpointMax; got <= limit {
		t.Fatalf("overlay holds %d bytes, want over %d", got, limit)
	}

	// A user commit as large returns while the build holds.
	user := make(chan error, 1)
	go func() {
		user <- commitBulk(ctx, db, "user")
	}()
	select {
	case err := <-user:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("user commit waited for the build")
	}

	// The bulk commit still waits, and returns once the build ends.
	select {
	case err := <-bulk:
		t.Fatalf("bulk commit returned %v while the build held", err)
	case <-time.After(100 * time.Millisecond):
	}
	released = true
	close(gate)
	select {
	case err := <-bulk:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("bulk commit did not return after the build")
	}
	if db.BuildWait() < 100*time.Millisecond {
		t.Fatalf("build wait %v, want the background commit's wait", db.BuildWait())
	}

	// The stats count the wait and, once it ends, the checkpoint it waited
	// for. The user commit did not wait.
	_ = db.w.builds.WaitIdle(context.Background(), nil)
	stats := db.Stats()
	if stats.BuildWaits != 1 || stats.BuildWaitTime != db.BuildWait() {
		t.Fatalf("build waits %d for %v, want 1 for %v", stats.BuildWaits, stats.BuildWaitTime, db.BuildWait())
	}
	if stats.Checkpoints < 1 || stats.CheckpointBytes == 0 || stats.CheckpointTime < 100*time.Millisecond {
		t.Fatalf("checkpoints %d of %d bytes in %v", stats.Checkpoints, stats.CheckpointBytes, stats.CheckpointTime)
	}

	// Both commits' keys read back.
	rtx, err := db.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer rtx.Discard()
	for _, key := range []string{"bulk/000799", "user/000799"} {
		if _, ok, err := rtx.Get(ctx, []byte(key)); err != nil || !ok {
			t.Fatalf("get %s = %v %v", key, ok, err)
		}
	}
}

// TestCommitWaitMaxRaisesBound holds a background checkpoint and checks that
// a background bulk commit past twice CheckpointMax returns while a larger
// CommitWaitMax still covers the overlay.
func TestCommitWaitMaxRaisesBound(t *testing.T) {
	// Open a file with small checkpoints, a large commit wait bound, and
	// builds that wait for the gate.
	ctx := context.Background()
	opts := Options{CheckpointMin: 16 << 10, CheckpointMax: 64 << 10, CommitWaitMax: 8 << 20}
	db := openTest(t, filepath.Join(t.TempDir(), "wait.s4wave"), opts)
	defer db.Close()
	started, gate := make(chan struct{}, 1), make(chan struct{})
	defer close(gate)
	db.w.beforeBuild = func() {
		select {
		case started <- struct{}{}:
		default:
		}
		<-gate
	}

	// Commit a background batch over twice CheckpointMax while the build
	// holds.
	done := make(chan error, 1)
	go func() {
		done <- commitBulk(block.WithBackground(ctx), db, "bulk")
	}()

	// It returns without waiting for the build it started.
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the build did not start")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("bulk commit waited for the build")
	}
	if got := db.cur.Load().overlayBytes; got <= 2*opts.CheckpointMax {
		t.Fatalf("overlay holds %d bytes, want over %d", got, 2*opts.CheckpointMax)
	}
	if stats := db.Stats(); stats.BuildWaits != 0 {
		t.Fatalf("build waits %d, want 0", stats.BuildWaits)
	}
}

// commitBulk commits 800 keys of 100 bytes under prefix in one transaction.
func commitBulk(ctx context.Context, db *DB, prefix string) error {
	// Write the keys in one transaction and commit it.
	tx, err := db.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	val := make([]byte, 100)
	for i := range 800 {
		if err := tx.Set(ctx, fmt.Appendf(nil, "%s/%06d", prefix, i), val); err != nil {
			tx.Discard()
			return err
		}
	}
	return tx.Commit(ctx)
}
