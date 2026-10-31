package s4db

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// commitKey commits one key in its own write transaction.
func commitKey(t *testing.T, ctx context.Context, db *DB, key string) {
	// Write the key and commit it durably.
	t.Helper()
	tx, err := db.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, []byte(key), []byte("value "+key)); err != nil {
		tx.Discard()
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// TestLastUsageSkipsWriterLock checks that a commit samples the space, that
// LastUsage returns the sample while a writer holds the lock, and that the
// sample matches Usage of the same state.
func TestLastUsageSkipsWriterLock(t *testing.T) {
	// Commit a key, which takes the first sample.
	ctx := context.Background()
	db := openTest(t, filepath.Join(t.TempDir(), "usage.s4wave"), Options{SingleProcess: true})
	defer db.Close()
	commitKey(t, ctx, db, "a")

	// Hold the writer lock and read the sample.
	if err := db.w.lock(ctx); err != nil {
		t.Fatal(err)
	}
	type sample struct {
		u  Usage
		at time.Time
		ok bool
	}
	got := make(chan sample, 1)
	go func() {
		u, at, ok := db.LastUsage()
		got <- sample{u, at, ok}
	}()
	var s sample
	select {
	case s = <-got:
	case <-time.After(5 * time.Second):
		t.Fatal("LastUsage waited for the writer lock")
	}
	db.w.unlock(nil)
	if !s.ok || s.at.IsZero() {
		t.Fatalf("no sample after a commit: %+v", s)
	}

	// The sample matches the locked report of the same state.
	u, err := db.Usage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if s.u.FileBytes != u.FileBytes || s.u.FreeBytes != u.FreeBytes || s.u.LogBytes != u.LogBytes ||
		s.u.PendingBytes != u.PendingBytes || s.u.IndexBytes != u.IndexBytes || s.u.ValueBytes != u.ValueBytes {
		t.Fatalf("sample %+v differs from usage %+v", s.u, u)
	}
	if u.FileBytes == 0 {
		t.Fatal("usage reports an empty file")
	}
}

// TestOldestReadFollowsSnapshots checks that an open snapshot holds
// OldestRead at its commit while later commits pass it, and that discarding
// it lets OldestRead reach the latest commit.
func TestOldestReadFollowsSnapshots(t *testing.T) {
	// Commit a key and open a snapshot of it.
	ctx := context.Background()
	db := openTest(t, filepath.Join(t.TempDir(), "pin.s4wave"), Options{SingleProcess: true})
	defer db.Close()
	commitKey(t, ctx, db, "base")
	snap, err := db.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	held := db.Seq()

	// Commit past it; the snapshot holds the oldest read.
	for i := range 3 {
		commitKey(t, ctx, db, "next/"+strconv.Itoa(i))
	}
	if seq, _ := db.OldestRead(); seq != held || db.Seq() <= held {
		t.Fatalf("OldestRead = %d with Seq %d; want the snapshot's %d", seq, db.Seq(), held)
	}

	// Discarding the snapshot releases it.
	snap.Discard()
	if seq, _ := db.OldestRead(); seq != db.Seq() {
		t.Fatalf("OldestRead = %d after discard; want Seq %d", seq, db.Seq())
	}
}
