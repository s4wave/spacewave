package unixfs_block_fs

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_block "github.com/aperturerobotics/hydra/unixfs/block"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	"github.com/sirupsen/logrus"
)

// TestFS tests the full end-to-end filesystem.
func TestFS(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// init the filesystem root
	btx, bcs := oc.BuildTransaction(nil)
	bcs.SetBlock(unixfs_block.NewFSNode(unixfs_block.NodeType_NodeType_DIRECTORY, 0, nil), true)
	_, err = unixfs_block.NewFSTree(ctx, bcs, unixfs_block.NodeType_NodeType_DIRECTORY)
	if err != nil {
		t.Fatal(err.Error())
	}
	rootRef, _, err := btx.Write(true)
	if err != nil {
		t.Fatal(err.Error())
	}
	oc.SetRootRef(rootRef)

	// construct the fscursor
	wr := NewFSWriter()
	fs := NewFS(ctx, unixfs_block.NodeType_NodeType_DIRECTORY, oc, wr)
	defer fs.Release()
	wr.SetFS(fs)
	ufs := unixfs.NewFS(ctx, le, fs, nil)

	fsHandle, err := ufs.AddRootReference(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fsHandle.Release()
	if fsHandle.GetName() != "" {
		t.Fail()
	}

	testFsHandle := func(t *testing.T, h *unixfs.FSHandle) {
		_, err = h.Lookup(ctx, "does-not-exist")
		if err != unixfs_errors.ErrNotExist {
			t.Fatalf("expected not exist but got %v", err)
		}
		err = h.Mknod(
			ctx,
			true,
			[]string{"test-dir-1"},
			unixfs.NewFSCursorNodeType_Dir(),
			0,
			time.Time{},
		)
		if err != nil {
			t.Fatal(err.Error())
		}
	}

	// First try accessing fsHandle directly.
	t.Run("fsHandle", func(t *testing.T) {
		testFsHandle(t, fsHandle)
	})

	// Test accessing via the FSHandle FSCursor.
	t.Run("fsHandle_FSCursor", func(t *testing.T) {
		fsHandleCursor := unixfs.NewFSHandleCursor(fsHandle)
		fsHandleCursorFS := unixfs.NewFS(ctx, le, fsHandleCursor, nil)
		defer fsHandleCursorFS.Release()

		fsHandleCursorHandle, err := fsHandleCursorFS.AddRootReference(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer fsHandleCursorHandle.Release()

		testFsHandle(t, fsHandleCursorHandle)
	})
}
