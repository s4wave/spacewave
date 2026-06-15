package unixfs_overlay

import (
	"bytes"
	"context"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
)

// TestOverlayFSCursorRename proves rename works through the overlay via the
// FSHandle.Rename path the v86fs server uses. Both cases were unimplemented:
// overlay MoveTo/MoveFrom returned (false, nil), so FSHandle.Rename fell through
// to ErrCrossFsRename, which the guest saw as EIO and which corrupted apt's
// package cache when it renamed its temp file over the read-only lower copy.
func TestOverlayFSCursorRename(t *testing.T) {
	t.Run("upper temp over lower target", func(t *testing.T) {
		ctx := context.Background()
		lower := mustLower(t)
		upper := mustUpper(t)
		root := NewOverlayFSCursor(lower, upper)
		defer root.Release()

		rootHandle, err := unixfs.NewFSHandle(root)
		if err != nil {
			t.Fatal(err)
		}
		defer rootHandle.Release()

		// apt writes a temp file then atomically renames it over the target.
		if err := rootHandle.MknodWithContent(ctx, "base.txt.tmp", unixfs.NewFSCursorNodeType_File(), int64(len("rewritten")), bytes.NewReader([]byte("rewritten")), 0o644, overlayTestTime); err != nil {
			t.Fatalf("create temp file: %v", err)
		}

		srcHandle, err := rootHandle.Lookup(ctx, "base.txt.tmp")
		if err != nil {
			t.Fatalf("lookup temp file: %v", err)
		}
		if err := srcHandle.Rename(ctx, rootHandle, "base.txt", overlayTestTime); err != nil {
			t.Fatalf("rename temp over lower target: %v", err)
		}

		if got := mustReadFile(t, root, "base.txt"); got != "rewritten" {
			t.Fatalf("renamed content: got %q want %q", got, "rewritten")
		}
		if got := mustReadFile(t, lower, "base.txt"); got != "lower base" {
			t.Fatalf("lower target mutated: %q", got)
		}
		if _, err := mustOps(t, root).Lookup(ctx, "base.txt.tmp"); err != unixfs_errors.ErrNotExist {
			t.Fatalf("source temp still visible: %v", err)
		}
	})

	t.Run("lower source copy-up and whiteout", func(t *testing.T) {
		ctx := context.Background()
		lower := mustLower(t)
		upper := mustUpper(t)
		root := NewOverlayFSCursor(lower, upper)
		defer root.Release()

		rootHandle, err := unixfs.NewFSHandle(root)
		if err != nil {
			t.Fatal(err)
		}
		defer rootHandle.Release()

		srcHandle, err := rootHandle.Lookup(ctx, "lower-only.txt")
		if err != nil {
			t.Fatalf("lookup lower source: %v", err)
		}
		if err := srcHandle.Rename(ctx, rootHandle, "moved.txt", overlayTestTime); err != nil {
			t.Fatalf("rename lower-backed source: %v", err)
		}

		if got := mustReadFile(t, root, "moved.txt"); got != "only lower" {
			t.Fatalf("moved content: got %q want %q", got, "only lower")
		}
		if _, err := mustOps(t, root).Lookup(ctx, "lower-only.txt"); err != unixfs_errors.ErrNotExist {
			t.Fatalf("source still visible after move: %v", err)
		}
		if !slices.Contains(mustReadDirNames(t, upper), ".wh.lower-only.txt") {
			t.Fatalf("upper whiteout for moved source missing: %v", mustReadDirNames(t, upper))
		}
		if got := mustReadFile(t, lower, "lower-only.txt"); got != "only lower" {
			t.Fatalf("lower source mutated: %q", got)
		}
	})
}
