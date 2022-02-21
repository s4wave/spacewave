package unixfs_world

import (
	"bytes"
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/aperturerobotics/hydra/unixfs"
	"github.com/aperturerobotics/hydra/world"
	world_testbed "github.com/aperturerobotics/hydra/world/testbed"
	world_types "github.com/aperturerobotics/hydra/world/types"
	"github.com/aperturerobotics/timestamp"
	"github.com/pkg/errors"
	// 	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
)

// InitTestbed inits a testbed with a new fs.
func InitTestbed(t *testing.T) (*world_testbed.Testbed, *unixfs.FS) {
	ctx := context.Background()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// provide op handlers to bus
	engineID := tb.EngineID
	opc := world.NewLookupOpController("test-fs-ops", engineID, LookupFsOp)
	go func() {
		_ = tb.Bus.ExecuteController(ctx, opc)
	}()

	// hack: wait for it to start
	<-time.After(time.Millisecond * 100)

	// uses directive to look up the engine
	eng := tb.Engine
	// uses short-lived engine txs to implement world state
	ws := world.NewEngineWorldState(ctx, eng, true)

	sender := tb.Volume.GetPeerID()
	objKey := "test-git-repo"
	fsType := FSType_FSType_FS_NODE
	err = FsInit(
		ctx,
		ws,
		sender,
		objKey,
		fsType,
		nil,
		0,
		true,
		time.Now(),
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// check type
	ts := world_types.NewTypesState(ctx, ws)
	typeID, err := ts.GetObjectType(objKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if typeID != FSNodeTypeID {
		t.Fatalf("expected type id %s but got %q", FSObjectTypeID, typeID)
	}
	t.Logf("filesystem initialized w/ type: %s", typeID)

	// construct full fs
	writer := NewFSWriter(ws, objKey, fsType, sender)
	watchWorldChanges := false
	rootFSCursor := NewFSCursor(tb.Logger, ws, objKey, fsType, writer, watchWorldChanges)
	return tb, unixfs.NewFS(ctx, tb.Logger, rootFSCursor, nil)
}

// TestFsBasic runs a basic test.
func TestFsBasic(t *testing.T) {
	tb, ufs := InitTestbed(t)
	ctx := tb.Context

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
	tb, ufs := InitTestbed(t)
	ctx := tb.Context

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
