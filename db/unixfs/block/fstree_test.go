package unixfs_block

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/file"
	"github.com/s4wave/spacewave/db/testbed"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	"github.com/sirupsen/logrus"
)

// TestBasicDirectory is a simple directory test.
func TestBasicDirectory(t *testing.T) {
	// Prepare logging for the directory test.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start a testbed for the directory tree.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Record the testbed volume identifier.
	vol := tb.Volume
	volID := vol.GetID()
	t.Log(volID)

	// Open an empty object cursor for the directory tree.
	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Initialize the root directory in a block transaction.
	btx, bcs := oc.BuildTransaction(nil)
	bcs.SetBlock(NewFSNode(NodeType_NodeType_DIRECTORY, 0, nil), true)
	ftree, err := NewFSTree(ctx, bcs, NodeType_NodeType_DIRECTORY)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the new root directory has no entries.
	dirents, err := ReaddirAll(ctx, ftree)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(dirents) != 0 {
		t.Fail()
	}

	// test adding directory entries
	t.Log(ftree.GetCursorRef().MarshalString())
	cursors, err := ftree.Mkdir(0, nil, "test-directory")
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(cursors) != 1 {
		t.Fail()
	}

	// Verify repeated directory creation returns the same inode.
	cursors2, err := ftree.Mkdir(0, nil, "test-directory")
	if err != nil {
		t.Fatal(err.Error())
	}
	b1, _ := cursors["test-directory"].bcs.GetBlock()
	b2, _ := cursors2["test-directory"].bcs.GetBlock()
	if b1 != b2 {
		t.Fail()
	}

	// sanity check
	if ftree.GetFSNode().GetModTime().SizeVT() == 0 {
		t.Fatal("modification time was zero after making fstree")
	}

	// Persist the directory transaction.
	_, bcs, err = btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Reload the persisted directory tree.
	ftree, err = NewFSTree(ctx, bcs, NodeType_NodeType_DIRECTORY)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the persisted tree contains the created directory.
	dirents, err = ReaddirAll(ctx, ftree)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(dirents) != 1 ||
		dirents["test-directory"].GetNodeType() != NodeType_NodeType_DIRECTORY {
		t.Fail()
	}
}

func TestEmptyFstree(t *testing.T) {
	// Prepare logging for the empty tree test.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start a testbed for an empty block reference.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Record the testbed volume identifier.
	vol := tb.Volume
	volID := vol.GetID()
	t.Log(volID)

	// Open an empty object cursor for the filesystem tree.
	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Initialize a directory at an empty block reference.
	btx, bcs := oc.BuildTransactionAtRef(nil, &block.BlockRef{})
	bcs.SetBlock(NewFSNode(NodeType_NodeType_DIRECTORY, 0, nil), true)
	ftree, err := NewFSTree(ctx, bcs, NodeType_NodeType_DIRECTORY)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the initialized directory has no entries.
	dirents, err := ReaddirAll(ctx, ftree)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(dirents) != 0 {
		t.Fail()
	}

	// Verify a missing directory entry returns no inode.
	de, err := ftree.Lookup("noexist")
	if err != nil {
		t.Fatal(err.Error())
	}
	if de != nil {
		t.Fail()
	}

	// test adding directory entries
	t.Log(ftree.GetCursorRef().MarshalString())
	cursors, err := ftree.Mkdir(0, nil, "test-directory")
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(cursors) != 1 {
		t.Fail()
	}

	// Verify repeated directory creation returns the same inode.
	cursors2, err := ftree.Mkdir(0, nil, "test-directory")
	if err != nil {
		t.Fatal(err.Error())
	}
	b1, _ := cursors["test-directory"].bcs.GetBlock()
	b2, _ := cursors2["test-directory"].bcs.GetBlock()
	if b1 != b2 {
		t.Fail()
	}

	// Persist the directory transaction.
	_, bcs, err = btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Reload the persisted directory tree.
	ftree, err = NewFSTree(ctx, bcs, NodeType_NodeType_DIRECTORY)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the reloaded tree contains the created directory.
	dirents, err = ReaddirAll(ctx, ftree)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(dirents) != 1 ||
		dirents["test-directory"].GetNodeType() != NodeType_NodeType_DIRECTORY {
		t.Fail()
	}
}

// TestBasicFile is a simple file test.
func TestBasicFile(t *testing.T) {
	// Prepare logging for the file test.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start a testbed for file writes and renames.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Record the testbed volume identifier.
	vol := tb.Volume
	volID := vol.GetID()
	t.Log(volID)

	// Open an empty object cursor for the file tree.
	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Initialize the root directory in a block transaction.
	btx, bcs := oc.BuildTransaction(nil)
	bcs.SetBlock(NewFSNode(NodeType_NodeType_DIRECTORY, 0, nil), true)
	ftree, err := NewFSTree(ctx, bcs, NodeType_NodeType_DIRECTORY)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create and persist a child directory before writing a file.
	_, err = ftree.Mkdir(0, nil, "test-directory")
	if err != nil {
		t.Fatal(err.Error())
	}
	_, bcs, err = btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Reload the tree containing the persisted child directory.
	ftree, err = NewFSTree(ctx, bcs, NodeType_NodeType_DIRECTORY)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the file entry and open its file handle.
	childFtree, err := ftree.Mknod("test-file", NodeType_NodeType_FILE, nil, 0, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	fh, err := childFtree.BuildFileHandle(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Write the expected file contents and persist the transaction.
	fhw := file.NewWriter(fh, btx, nil)
	expected := "test 1234"
	err = fhw.WriteBytes(0, []byte(expected))
	if err != nil {
		t.Fatal(err.Error())
	}
	_, bcs, err = btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Reload the persisted tree before renaming the file.
	ftree, err = NewFSTree(ctx, bcs, NodeType_NodeType_DIRECTORY)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Rename the file twice within the reloaded tree.
	tts := FillPlaceholderTimestamp(nil)
	err = CopyOrRename(ftree, []string{"test-file"}, []string{"renamed-file"}, true, tts)
	if err != nil {
		t.Fatal(err.Error())
	}
	err = CopyOrRename(ftree, []string{"renamed-file"}, []string{"renamed-2"}, true, tts)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Resolve the final file name and require its inode to exist.
	renamedf, _, err := ftree.LookupFollowDirent("renamed-2")
	if renamedf == nil && err == nil {
		err = unixfs_errors.ErrNotExist
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open the renamed file and read its persisted contents.
	fh, err = renamedf.BuildFileHandle(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	buf := make([]byte, 10)
	n, err := fh.Read(buf)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the renamed file preserves its content length and bytes.
	if n != len(expected) {
		t.Fatalf("read %d expected 9", n)
	}
	buf = buf[:n]
	if string(buf) != expected {
		t.Fatalf("read %s != expected %s", buf, expected)
	}
}
