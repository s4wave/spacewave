package unixfs_block_fs

import (
	"bytes"
	"context"
	"path"
	"slices"
	"testing"
	"time"

	billy_util "github.com/go-git/go-billy/v6/util"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	"github.com/sirupsen/logrus"
)

// TestFS tests the full end-to-end filesystem.
func TestFS(t *testing.T) {
	// Prepare the filesystem interface test context and logger.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start a testbed shared by the filesystem handle cases.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Build a fresh writable filesystem handle for each interface case.
	buildFsHandle := func() *unixfs.FSHandle {
		// Open an empty bucket cursor for the filesystem root.
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

		// Persist the empty directory and install its root reference.
		rootRef, _, err := btx.Write(ctx, true)
		if err != nil {
			t.Fatal(err.Error())
		}
		oc.SetRootRef(rootRef)

		// construct the fscursor
		wr := NewFSWriter()
		fs := NewFS(ctx, unixfs_block.NodeType_NodeType_DIRECTORY, oc, wr)

		// Connect the filesystem writer to the constructed filesystem.
		// defer fs.Release()
		wr.SetFS(fs)

		// Open the filesystem root handle.
		fsHandle, err := unixfs.NewFSHandle(fs)
		if err != nil {
			t.Fatal(err.Error())
		}

		// Check that the filesystem root handle has an empty name.
		if fsHandle.GetName() != "" {
			fsHandle.Release()
			t.Fail()
		}
		return fsHandle
	}

	// Exercise directory creation and file readback through a filesystem handle.
	testFsHandle := func(t *testing.T, h *unixfs.FSHandle) {
		// Check that a missing filesystem entry returns ErrNotExist.
		_, err = h.Lookup(ctx, "does-not-exist")
		if err != unixfs_errors.ErrNotExist {
			t.Fatalf("expected not exist but got %v", err)
		}

		// Create a directory beneath the filesystem root.
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

		// Open the new directory for file creation.
		dirHandle, err := h.Lookup(ctx, testDirName)
		if err != nil {
			t.Fatal(err.Error())
		}

		// Create a file beneath the new directory.
		testFilename := "test.txt"
		testFilePath := path.Join(testDirName, testFilename)
		if err := dirHandle.Mknod(
			ctx,
			true,
			[]string{testFilename},
			unixfs.NewFSCursorNodeType_File(),
			0o644,
			time.Time{},
		); err != nil {
			t.Fatal(err.Error())
		}

		// Resolve the new file through its path from the root.
		fileContents := []byte("hello world")
		fileHandle, fileHandlePts, err := h.LookupPath(ctx, testFilePath)
		if err != nil {
			t.Fatal(err.Error())
		}

		// Check that path lookup traversed the directory and file names.
		if !slices.Equal(fileHandlePts, []string{testDirName, testFilename}) {
			t.FailNow()
		}

		// Write the initial contents through the resolved file handle.
		if err := fileHandle.WriteAt(ctx, 0, fileContents, time.Time{}); err != nil {
			t.Fatal(err.Error())
		}

		// Read the file through the Billy filesystem adapter.
		bfs := unixfs_billy.NewBillyFS(ctx, h, "", time.Time{})
		readData, err := billy_util.ReadFile(bfs, testFilePath)
		if err != nil {
			t.Fatal(err.Error())
		}

		// Check that the Billy adapter returned the written file contents.
		if !bytes.Equal(readData, fileContents) {
			t.FailNow()
		}
	}

	// First try accessing fsHandle directly.
	t.Run("fsHandle", func(t *testing.T) {
		// Open a fresh filesystem handle for the direct interface case.
		fsHandle := buildFsHandle()
		defer fsHandle.Release()

		// Exercise directory creation and file readback on the direct handle.
		testFsHandle(t, fsHandle)
	})

	// Test accessing via the FSHandle FSCursor.
	t.Run("fsHandle_FSCursor", func(t *testing.T) {
		// Open a fresh filesystem handle for the cursor adapter case.
		fsHandle := buildFsHandle()

		// NOTE: we pass releaseHandle to true below: the fsHandleCursor will release the fs handle.
		// defer fsHandle.Release()

		// Wrap the cursor in a handle that releases the underlying filesystem.
		fsHandleCursor := unixfs.NewFSHandleCursor(fsHandle, true, nil)
		fsHandleCursorHandle, err := unixfs.NewFSHandle(fsHandleCursor)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer fsHandleCursorHandle.Release()

		// Exercise directory creation and file readback through the cursor adapter.
		testFsHandle(t, fsHandleCursorHandle)
	})

	// Test accessing via the FSHandle FSCursor.
	t.Run("fsHandle_FSCursor_WithGetter", func(t *testing.T) {
		// Open a fresh filesystem handle for the cursor getter case.
		fsHandle := buildFsHandle()
		defer fsHandle.Release()

		// Provide cursors by cloning the live filesystem handle.
		fsHandleCursorGetter := unixfs.NewFSCursorGetter(func(ctx context.Context) (unixfs.FSCursor, error) {
			// Reject a cursor request after the filesystem handle is released.
			if fsHandle.CheckReleased() {
				return nil, unixfs_errors.ErrReleased
			}

			// Clone the filesystem handle for the returned cursor.
			cursorHandle, err := fsHandle.Clone(ctx)
			if err != nil {
				return nil, err
			}
			return unixfs.NewFSHandleCursor(cursorHandle, true, nil), nil
		})

		// Open a filesystem handle through the cursor getter.
		fsHandleCursorHandle, err := unixfs.NewFSHandle(fsHandleCursorGetter)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer fsHandleCursorHandle.Release()

		// Exercise directory creation and file readback through the cursor getter.
		testFsHandle(t, fsHandleCursorHandle)
	})
}
