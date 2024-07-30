package unixfs_block_fs

import (
	"bytes"
	"context"
	"io"
	"strconv"
	"testing"

	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_block "github.com/aperturerobotics/hydra/unixfs/block"
	unixfs_e2e "github.com/aperturerobotics/hydra/unixfs/e2e"
	"github.com/sirupsen/logrus"
)

// TestBuildPath tests building the path to a cursor.
func TestBuildPath(t *testing.T) {
	// create cursor hierarchy
	fs := &FS{}
	root := &FSCursor{}
	tail := root
	for i := 0; i < 10; i++ {
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
	for i := 0; i < 10; i++ {
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
