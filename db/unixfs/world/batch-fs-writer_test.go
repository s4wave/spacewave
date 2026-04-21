package unixfs_world

import (
	"context"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/unixfs"
)

// TestBatchFSWriter_Validation covers argument-validation paths that do not
// require a backing world state. Data-carrying flows are covered by the
// testbed-based tests.
func TestBatchFSWriter_Validation(t *testing.T) {
	ctx := context.Background()
	ts := time.Unix(1_700_000_000, 0)
	b := NewBatchFSWriter(nil, "test/fs", FSType_FSType_FS_NODE, "")

	if err := b.AddFile(ctx, nil, "", unixfs.NewFSCursorNodeType_File(), 0, nil, 0o644, ts); err == nil {
		t.Fatal("AddFile with empty name should error")
	}
	if err := b.AddFile(ctx, nil, "x", unixfs.NewFSCursorNodeType_File(), -1, nil, 0o644, ts); err == nil {
		t.Fatal("AddFile with negative dataLen should error")
	}
	if err := b.AddDir(ctx, nil, "", 0o755, ts); err == nil {
		t.Fatal("AddDir with empty name should error")
	}
	if err := b.AddSymlink(ctx, nil, "", []string{"x"}, false, ts); err == nil {
		t.Fatal("AddSymlink with empty name should error")
	}
	if err := b.AddSymlink(ctx, nil, "lnk", nil, false, ts); err == nil {
		t.Fatal("AddSymlink with empty target should error")
	}

	// Lifecycle checks: Commit on an empty writer short-circuits before
	// touching WorldState. The second Commit and any subsequent Add* must
	// then fail.
	if err := b.Commit(ctx); err != nil {
		t.Fatalf("empty Commit: %v", err)
	}
	if err := b.Commit(ctx); err == nil {
		t.Fatal("second Commit should error")
	}
	if err := b.AddDir(ctx, nil, "late", 0o755, ts); err == nil {
		t.Fatal("AddDir after Commit should error")
	}
	b.Release()
}

// TestBatchFSWriter_ReleaseRejects covers iter 8: Release-before-Commit
// discards pending state and rejects every subsequent call.
func TestBatchFSWriter_ReleaseRejects(t *testing.T) {
	ctx := context.Background()
	ts := time.Unix(1_700_000_000, 0)
	b := NewBatchFSWriter(nil, "test/fs", FSType_FSType_FS_NODE, "")

	b.Release()
	if err := b.AddDir(ctx, nil, "x", 0o755, ts); err == nil {
		t.Fatal("AddDir after Release should error")
	}
	if err := b.AddFile(ctx, nil, "x", unixfs.NewFSCursorNodeType_File(), 0, nil, 0o644, ts); err == nil {
		t.Fatal("AddFile after Release should error")
	}
	if err := b.AddSymlink(ctx, nil, "x", []string{"y"}, false, ts); err == nil {
		t.Fatal("AddSymlink after Release should error")
	}
	if err := b.Commit(ctx); err == nil {
		t.Fatal("Commit after Release should error")
	}
	// double Release is a no-op.
	b.Release()
}
