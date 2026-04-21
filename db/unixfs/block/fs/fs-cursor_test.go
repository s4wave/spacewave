package unixfs_block_fs

import (
	"bytes"
	"context"
	"io"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_e2e "github.com/s4wave/spacewave/db/unixfs/e2e"
	"github.com/sirupsen/logrus"
)

// TestBuildPath tests building the path to a cursor.
func TestBuildPath(t *testing.T) {
	// create cursor hierarchy
	fs := &FS{}
	root := &FSCursor{}
	tail := root
	for i := range 10 {
		tail = &FSCursor{
			fs:     fs,
			parent: tail,
			depth:  tail.depth + 1,
			name:   strconv.Itoa(i),
		}
	}
	ctx := context.Background()
	tpath, err := tail.getOrBuildPath(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	t.Logf("%#v\n", tpath)
	if len(tpath) != 10 {
		t.Fail()
	}
	for i := range 10 {
		if tpath[i] != strconv.Itoa(i) {
			t.Fail()
		}
	}
	for tail.parent != nil {
		tail = tail.parent
	}
}

// TestFSCursor performs basic sanity checks on the fs cursor.
func TestFSCursor(t *testing.T) {
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

	// build the test filesystem
	btx, bcs := oc.BuildTransaction(nil)
	bcs.SetBlock(unixfs_block.NewFSNode(unixfs_block.NodeType_NodeType_DIRECTORY, 0, nil), true)

	// make some dirs
	root, err := unixfs_block.NewFSTree(ctx, bcs, unixfs_block.NodeType_NodeType_DIRECTORY)
	if err != nil {
		t.Fatal(err.Error())
	}
	err = unixfs_block.Mknod(
		root,
		[][]string{{"dir1"}, {"dir2"}, {"dir2", "dir3"}},
		unixfs_block.NodeType_NodeType_DIRECTORY,
		0,
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// make some files
	err = unixfs_block.Mknod(
		root,
		[][]string{{"dir2", "dir3", "file1"}},
		unixfs_block.NodeType_NodeType_FILE,
		0,
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// write some data
	testData := []byte("testing 123")
	err = unixfs_block.WriteAt(
		ctx,
		root,
		nil,
		[]string{"dir2", "dir3", "file1"},
		0, int64(len(testData)),
		bytes.NewReader(testData),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	res, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Logf("wrote initial fs: %s", res.MarshalString())
	oc.SetRootRef(res)

	fs := NewFS(ctx, unixfs_block.NodeType_NodeType_DIRECTORY, oc, nil)
	pc, err := fs.GetProxyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	ops, err := pc.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	dirCs, err := ops.Lookup(ctx, "dir2")
	if err != nil {
		t.Fatal(err.Error())
	}

	ops, err = dirCs.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	dirCs, err = ops.Lookup(ctx, "dir3")
	if err != nil {
		t.Fatal(err.Error())
	}

	ops, err = dirCs.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	fileCs, err := ops.Lookup(ctx, "file1")
	if err != nil {
		t.Fatal(err.Error())
	}

	outData := make([]byte, 20)
	ops, err = fileCs.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	readn, err := ops.ReadAt(ctx, 0, outData)
	if err == io.EOF && readn != 0 {
		err = nil
	}
	if err != nil {
		t.Fatal(err.Error())
	}
	outData = outData[:readn]
	if !bytes.Equal(outData, testData) {
		t.Fail()
	} else {
		t.Logf("read data correctly: %s", string(outData))
	}
}

// TestFSCursorSymlinkSize verifies that GetSize returns the symlink target
// path length, not 0. This is critical for go-git hash compatibility: the
// blob header uses the size, so a mismatch causes all symlinks to show as
// modified in worktree.Status().
func TestFSCursorSymlinkSize(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)
	_ = le

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Build a directory with a symlink.
	btx, bcs := oc.BuildTransaction(nil)
	bcs.SetBlock(unixfs_block.NewFSNode(unixfs_block.NodeType_NodeType_DIRECTORY, 0, nil), true)

	root, err := unixfs_block.NewFSTree(ctx, bcs, unixfs_block.NodeType_NodeType_DIRECTORY)
	if err != nil {
		t.Fatal(err.Error())
	}

	symlinkTarget := "../../some/relative/target.md"
	targetParts, targetAbsolute := unixfs.SplitPath(symlinkTarget)
	_, err = root.Symlink(true, "mylink", &unixfs_block.FSSymlink{
		TargetPath: &unixfs_block.FSPath{
			Nodes:    targetParts,
			Absolute: targetAbsolute,
		},
	}, nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	res, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	oc.SetRootRef(res)

	// Open via FS cursor and check GetSize.
	fs := NewFS(ctx, unixfs_block.NodeType_NodeType_DIRECTORY, oc, nil)
	pc, err := fs.GetProxyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	ops, err := pc.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	linkCs, err := ops.Lookup(ctx, "mylink")
	if err != nil {
		t.Fatal(err.Error())
	}

	linkOps, err := linkCs.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	if !linkOps.GetIsSymlink() {
		t.Fatal("expected symlink node")
	}

	size, err := linkOps.GetSize(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	reconstructed := unixfs.JoinPath(targetParts, targetAbsolute)
	expectedSize := uint64(len(reconstructed))
	if size != expectedSize {
		t.Errorf("GetSize = %d, want %d (len of %q)", size, expectedSize, reconstructed)
	}
	t.Logf("symlink size=%d target=%q", size, reconstructed)
}

// TestFSHandle performs the test suite on the cursor.
func TestFSHandle(t *testing.T) {
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

	// build the test filesystem
	btx, bcs := oc.BuildTransaction(nil)
	bcs.SetBlock(unixfs_block.NewFSNode(unixfs_block.NodeType_NodeType_DIRECTORY, 0, nil), true)

	res, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Logf("wrote initial fs: %s", res.MarshalString())
	oc.SetRootRef(res)

	writer := NewFSWriter()
	fs := NewFS(ctx, unixfs_block.NodeType_NodeType_DIRECTORY, oc, writer)
	writer.SetFS(fs)

	handle, err := unixfs.NewFSHandle(fs)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer handle.Release()

	if err := unixfs_e2e.TestUnixFS(ctx, handle); err != nil {
		t.Fatal(err.Error())
	}
}

// TestFS_MknodWithContent tests MknodWithContent through the block-level FSWriter.
// This exercises the unixfs_block.FSWriter.MknodWithContent path which builds
// the blob in a detached transaction and must flush it before extracting the ref.
func TestFS_MknodWithContent(t *testing.T) {
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

	btx, bcs := oc.BuildTransaction(nil)
	bcs.SetBlock(unixfs_block.NewFSNode(unixfs_block.NodeType_NodeType_DIRECTORY, 0, nil), true)

	res, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	oc.SetRootRef(res)

	writer := NewFSWriter()
	fs := NewFS(ctx, unixfs_block.NodeType_NodeType_DIRECTORY, oc, writer)
	writer.SetFS(fs)

	handle, err := unixfs.NewFSHandle(fs)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer handle.Release()

	content := []byte("block-level MknodWithContent test: verifies blob ref is computed after detached tx write")
	err = handle.MknodWithContent(
		ctx,
		"test-block.txt",
		unixfs.NewFSCursorNodeType_File(),
		int64(len(content)),
		bytes.NewReader(content),
		0o644,
		time.Now(),
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	fileHandle, err := handle.Lookup(ctx, "test-block.txt")
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fileHandle.Release()

	size, err := fileHandle.GetSize(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if size != uint64(len(content)) {
		t.Fatalf("expected size %d but got %d", len(content), size)
	}

	buf := make([]byte, len(content))
	n, err := fileHandle.ReadAt(ctx, 0, buf)
	if err != nil {
		t.Fatal(err.Error())
	}
	if int(n) != len(content) {
		t.Fatalf("expected to read %d bytes but got %d", len(content), n)
	}
	if !bytes.Equal(buf[:n], content) {
		t.Fatalf("content mismatch: %q != %q", buf[:n], content)
	}
}
