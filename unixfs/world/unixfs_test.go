package unixfs_world

import (
	"bytes"
	"context"
	"strconv"
	"testing"
	"time"

	hydra_testbed "github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/unixfs"
	world_testbed "github.com/aperturerobotics/hydra/world/testbed"
	"github.com/aperturerobotics/timestamp"
	billy_util "github.com/go-git/go-billy/v5/util"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var objKey = "test/fs/1"

// TestFsBasic runs a basic test.
func TestFsBasic(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(logger)

	tb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	wtb, err := world_testbed.NewTestbed(tb)
	if err != nil {
		t.Fatal(err.Error())
	}

	watchWorldChanges := false // TODO: test with both false/true
	ufs, err := InitTestbed(wtb, objKey, watchWorldChanges)
	if err != nil {
		t.Fatal(err.Error())
	}

	fsHandle, err := ufs.AddRootReference(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	ts := time.Now()

	// create hello-dir-1
	err = fsHandle.Mknod(ctx, true, []string{"hello-dir-1"}, unixfs.NewFSCursorNodeType_Dir(), 0, ts)
	if err != nil {
		t.Fatal(err.Error())
	}

	// get a handle to hello-dir-1
	dirHandle, err := fsHandle.Lookup(ctx, "hello-dir-1")
	if err != nil {
		t.Fatal(err.Error())
	}

	// create a new file test.txt in hello-dir-1
	err = dirHandle.Mknod(ctx, true, []string{"test.txt"}, unixfs.NewFSCursorNodeType_File(), 0, ts)
	if err != nil {
		t.Fatal(err.Error())
	}

	// lookup test.txt in hello-dir-1
	fhandle, err := dirHandle.Lookup(ctx, "test.txt")
	if err != nil {
		t.Fatal(err.Error())
	}

	// write some data to test.txt
	testData := []byte("hello world")
	err = fhandle.Write(ctx, 0, testData, time.Now())
	if err != nil {
		t.Fatal(err.Error())
	}

	// read data
	checkReadFromFhandle := func() {
		buf := make([]byte, 1500)
		nread, err := fhandle.Read(ctx, 0, buf)
		if err != nil {
			t.Fatal(err.Error())
		}
		buf = buf[:nread]
		if !bytes.Equal(buf, testData) {
			t.Fatalf("read incorrect data: %#v != %#v", buf, string(testData))
		}
	}
	checkReadFromFhandle()

	// change permissions
	err = fhandle.SetPermissions(ctx, 0644, ts)
	if err != nil {
		t.Fatal(err.Error())
	}

	// change mod time
	nts := timestamp.Now()
	setTs := nts.ToTime()
	err = fhandle.SetModTimestamp(ctx, setTs)
	if err != nil {
		t.Fatal(err.Error())
	}

	getTs, err := fhandle.GetModTimestamp(ctx)
	if err == nil && !getTs.Equal(setTs) {
		err = errors.Errorf("failed to update ts: expected %s but got %s", setTs.String(), getTs.String())
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	// rename to renamed-dir-1
	/*
		err = dirHandle.Rename(ctx, fsHandle, "renamed-dir-1", ts)
		if err != nil {
			t.Fatal(err.Error())
		}

		// ensure old path doesn't exist
		_, err = fsHandle.Lookup(ctx, "hello-dir-1")
		if err != unixfs_errors.ErrNotExist {
			t.Fatal(err.Error())
		}
	*/

	// ensure new path exists
	/*
		dirHandle, err = fsHandle.Lookup(ctx, "renamed-dir-1")
		if err != nil {
			t.Fatal(err.Error())
		}
	*/

	// ensure file exists
	fhandle, err = dirHandle.Lookup(ctx, "test.txt")
	if err != nil {
		t.Fatal(err.Error())
	}
	checkReadFromFhandle()

	// test renaming twice in a row
	err = dirHandle.Rename(ctx, fsHandle, "renamed-2", ts)
	if err != nil {
		t.Fatal(err.Error())
	}
	err = dirHandle.Rename(ctx, fsHandle, "renamed-3", ts)
	if err != nil {
		t.Fatal(err.Error())
	}
}

// TestFsRename tests random renames.
func TestFsRename(t *testing.T) {
	ctx := context.Background()
	ufs, _, err := BuildTestbed(ctx, objKey, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	fsHandle, err := ufs.AddRootReference(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// create files
	nfilenames := 100
	fileNames := make([]string, nfilenames)
	for i := range fileNames {
		fileNames[i] = "file-" + strconv.Itoa(i)
	}

	// create them
	ts := time.Now()
	err = fsHandle.Mknod(ctx, true, fileNames, unixfs.NewFSCursorNodeType_File(), 0, ts)
	if err != nil {
		t.Fatal(err.Error())
	}

	// check they all exist & open handles
	fsHandles := make([]*unixfs.FSHandle, nfilenames)
	for i, fileName := range fileNames {
		fileHandle, err := fsHandle.Lookup(ctx, fileName)
		if err != nil {
			t.Fatal(err.Error())
		}
		fsHandles[i] = fileHandle
	}

	checkErr := func(err error) {
		if err != nil {
			t.Fatal(err.Error())
		}
	}

	swap := func(i, j int) {
		filei, filej := fileNames[i], fileNames[j]
		filek := "file-tmp"

		// XXX: is it possible to swap files without a tmp file?

		fhi, fhj := fsHandles[i], fsHandles[j]
		checkErr(fhi.Rename(ctx, fsHandle, filek, ts))
		checkErr(fhj.Rename(ctx, fsHandle, filei, ts))
		checkErr(fhi.Rename(ctx, fsHandle, filej, ts))

		fileNames[i], fileNames[j] = filej, filei
	}

	// rename them randomly
	// rand.Shuffle(nfilenames, swap)
	for x := 0; x < len(fileNames)/2; x++ {
		j := len(fileNames) - x - 1
		swap(x, j)
	}

	// release handles
	for _, h := range fsHandles {
		h.Release()
	}

	// build handles again
	for i, fileName := range fileNames {
		fileHandle, err := fsHandle.Lookup(ctx, fileName)
		checkErr(err)
		fsHandles[i] = fileHandle
	}

	// release handles
	for _, h := range fsHandles {
		h.Release()
	}
}

// TestFsBilly_WriteFile tests reading from a file immediately after writing it.
func TestFsBilly_WriteFile(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(logger)

	tb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	wtb, err := world_testbed.NewTestbed(tb)
	if err != nil {
		t.Fatal(err.Error())
	}

	watchWorldChanges := true // TODO: test with both false/true
	ufs, err := InitTestbed(wtb, objKey, watchWorldChanges)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ufs.Release()

	fsHandle, err := ufs.AddRootReference(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fsHandle.Release()

	// create test fs (backed by a block graph + Hydra world)
	bfs := unixfs.NewBillyFilesystem(ctx, fsHandle, "", time.Now())

	// create test script
	filename := "test.js"
	data := []byte("Hello world!\n")
	err = billy_util.WriteFile(bfs, filename, data, 0755)
	if err != nil {
		t.Fatal(err.Error())
	}

	// TODO: requires a slight delay for the fscursors to update
	// TODO: This is a bug that currently is being fixed
	time.Sleep(time.Millisecond * 5)

	// read file size & check
	fi, err := bfs.Stat(filename)
	if err != nil {
		t.Fatal(err.Error())
	}
	if s := int(fi.Size()); s < len(data) {
		t.Fatalf("expected size %d but got %d", len(data), s)
	}
}
