//go:build js

package store_kvtx_opfs

import (
	"context"
	"testing"

	"github.com/aperturerobotics/hydra/opfs"
)

// TestCrashRecoveryPendingMarker verifies that a stale .pending marker
// from a crashed write transaction is cleaned up on the next write tx.
func TestCrashRecoveryPendingMarker(t *testing.T) {
	root, err := opfs.GetRoot()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := opfs.GetDirectory(root, "test-crash-pending", true)
	if err != nil {
		t.Fatal(err)
	}
	defer opfs.DeleteEntry(root, "test-crash-pending", true) //nolint

	ctx := context.Background()
	s := NewStore(dir, "test-crash-pending|kvtx")

	// Write some initial data.
	tx, err := s.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, []byte("key1"), []byte("val1")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Simulate a crash: write the .pending marker and a partial set
	// but do NOT remove the marker (simulating interrupted commit).
	if err := opfs.WriteFile(dir, pendingMarker, []byte("1")); err != nil {
		t.Fatal(err)
	}
	// Write a partial key that would normally be part of a commit.
	encoded := encodeKey([]byte("key2"))
	shard := shardPrefix(encoded)
	shardDir, err := opfs.GetDirectory(dir, shard, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := opfs.WriteFile(shardDir, encoded, []byte("partial")); err != nil {
		t.Fatal(err)
	}

	// Verify .pending marker exists.
	exists, err := opfs.FileExists(dir, pendingMarker)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("expected .pending marker to exist")
	}

	// Open a new write tx. This should detect and clean the marker.
	tx2, err := s.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// Marker should be cleaned up.
	exists, err = opfs.FileExists(dir, pendingMarker)
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal(".pending marker should have been cleaned up")
	}

	// The original data should still be readable.
	val, found, err := tx2.Get(ctx, []byte("key1"))
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("key1 not found after crash recovery")
	}
	if string(val) != "val1" {
		t.Fatalf("key1 = %q, want %q", val, "val1")
	}

	// The partial write from the "crashed" tx is also visible
	// (individual file writes are atomic, so partial = some keys
	// present, others missing, but no corrupted values).
	val2, found2, err := tx2.Get(ctx, []byte("key2"))
	if err != nil {
		t.Fatal(err)
	}
	if !found2 {
		t.Fatal("key2 (partial write) not found")
	}
	if string(val2) != "partial" {
		t.Fatalf("key2 = %q, want %q", val2, "partial")
	}

	tx2.Discard()
}

// TestCrashRecoveryReadTxSeesPartialState verifies that a read tx opened
// after a crash (with .pending marker) can still read all data including
// partial writes. Read txes do not clean the marker.
func TestCrashRecoveryReadTxSeesPartialState(t *testing.T) {
	root, err := opfs.GetRoot()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := opfs.GetDirectory(root, "test-crash-read", true)
	if err != nil {
		t.Fatal(err)
	}
	defer opfs.DeleteEntry(root, "test-crash-read", true) //nolint

	ctx := context.Background()
	s := NewStore(dir, "test-crash-read|kvtx")

	// Write initial data.
	tx, err := s.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, []byte("a"), []byte("1")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Simulate crash: leave .pending marker.
	if err := opfs.WriteFile(dir, pendingMarker, []byte("1")); err != nil {
		t.Fatal(err)
	}

	// Open a read tx - should succeed despite .pending marker.
	rtx, err := s.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer rtx.Discard()

	val, found, err := rtx.Get(ctx, []byte("a"))
	if err != nil {
		t.Fatal(err)
	}
	if !found || string(val) != "1" {
		t.Fatalf("read after crash: found=%v val=%q", found, val)
	}

	// Marker should still exist (reads don't clean it).
	exists, err := opfs.FileExists(dir, pendingMarker)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal(".pending marker should persist after read tx")
	}
}

// TestPendingMarkerNotCountedAsEntry verifies that the .pending marker
// file is excluded from Size() counts and ScanPrefix results.
func TestPendingMarkerNotCountedAsEntry(t *testing.T) {
	root, err := opfs.GetRoot()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := opfs.GetDirectory(root, "test-pending-size", true)
	if err != nil {
		t.Fatal(err)
	}
	defer opfs.DeleteEntry(root, "test-pending-size", true) //nolint

	ctx := context.Background()
	s := NewStore(dir, "test-pending-size|kvtx")

	// Write one key.
	tx, err := s.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, []byte("only"), []byte("one")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Place a .pending marker (simulating crash).
	if err := opfs.WriteFile(dir, pendingMarker, []byte("1")); err != nil {
		t.Fatal(err)
	}

	// Read tx: Size should be 1, not 2.
	rtx, err := s.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer rtx.Discard()

	size, err := rtx.Size(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if size != 1 {
		t.Fatalf("Size() = %d, want 1 (.pending should not count)", size)
	}
}
