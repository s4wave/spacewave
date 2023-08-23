package unixfs_block_fs

import (
	"bytes"
	"context"
	"path"
	"testing"
	"time"

	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_billy "github.com/aperturerobotics/hydra/unixfs/billy"
	unixfs_block "github.com/aperturerobotics/hydra/unixfs/block"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	billy_util "github.com/go-git/go-billy/v5/util"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
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

	buildFsHandle := func() *unixfs.FSHandle {
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
		// defer fs.Release()
		wr.SetFS(fs)

		fsHandle, err := unixfs.NewFSHandle(fs)
		if err != nil {
			t.Fatal(err.Error())
		}
		if fsHandle.GetName() != "" {
			fsHandle.Release()
			t.Fail()
		}
		return fsHandle
	}

	testFsHandle := func(t *testing.T, h *unixfs.FSHandle) {
		_, err = h.Lookup(ctx, "does-not-exist")
		if err != unixfs_errors.ErrNotExist {
			t.Fatalf("expected not exist but got %v", err)
		}
		testDirName := "test-dir-1"
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

		dirHandle, err := h.Lookup(ctx, testDirName)
		if err != nil {
			t.Fatal(err.Error())
		}

		testFilename := "test.txt"
		testFilePath := path.Join(testDirName, testFilename)
		if err := dirHandle.Mknod(
			ctx,
			true,
			[]string{testFilename},
			unixfs.NewFSCursorNodeType_File(),
			0644,
			time.Time{},
		); err != nil {
			t.Fatal(err.Error())
		}

		fileContents := []byte("hello world")
		fileHandle, fileHandlePts, err := h.LookupPath(ctx, testFilePath)
		if err != nil {
			t.Fatal(err.Error())
		}
		if !slices.Equal(fileHandlePts, []string{testDirName, testFilename}) {
			t.FailNow()
		}
		if err := fileHandle.WriteAt(ctx, 0, fileContents, time.Time{}); err != nil {
			t.Fatal(err.Error())
		}

		bfs := unixfs_billy.NewBillyFS(ctx, h, "", time.Time{})
		readData, err := billy_util.ReadFile(bfs, testFilePath)
		if err != nil {
			t.Fatal(err.Error())
		}
		if !bytes.Equal(readData, fileContents) {
			t.FailNow()
		}
	}

	// First try accessing fsHandle directly.
	t.Run("fsHandle", func(t *testing.T) {
		fsHandle := buildFsHandle()
		defer fsHandle.Release()

		testFsHandle(t, fsHandle)
	})

	// Test accessing via the FSHandle FSCursor.
	t.Run("fsHandle_FSCursor", func(t *testing.T) {
		fsHandle := buildFsHandle()

		// NOTE: we pass releaseHandle to true below: the fsHandleCursor will release the fs handle.
		// defer fsHandle.Release()

		fsHandleCursor := unixfs.NewFSHandleCursor(fsHandle, true)
		fsHandleCursorHandle, err := unixfs.NewFSHandle(fsHandleCursor)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer fsHandleCursorHandle.Release()

		testFsHandle(t, fsHandleCursorHandle)
	})

	// Test accessing via the FSHandle FSCursor.
	t.Run("fsHandle_FSCursor_WithGetter", func(t *testing.T) {
		fsHandle := buildFsHandle()
		defer fsHandle.Release()

		fsHandleCursorGetter := unixfs.NewFSCursorGetter(func(ctx context.Context) (unixfs.FSCursor, error) {
			if fsHandle.CheckReleased() {
				return nil, unixfs_errors.ErrReleased
			}

			cursorHandle, err := fsHandle.Clone(ctx)
			if err != nil {
				return nil, err
			}
			return unixfs.NewFSHandleCursor(cursorHandle, true), nil
		})

		fsHandleCursorHandle, err := unixfs.NewFSHandle(fsHandleCursorGetter)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer fsHandleCursorHandle.Release()

		testFsHandle(t, fsHandleCursorHandle)
	})
}
