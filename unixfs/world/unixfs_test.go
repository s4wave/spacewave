package unixfs_world

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/hydra/unixfs"
	"github.com/aperturerobotics/hydra/world"
	world_testbed "github.com/aperturerobotics/hydra/world/testbed"
	world_types "github.com/aperturerobotics/hydra/world/types"
	"github.com/aperturerobotics/timestamp"
	"github.com/pkg/errors"
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
	go tb.Bus.ExecuteController(ctx, opc)

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
	rootFSCursor := NewFSCursor(tb.Logger, eng, objKey, fsType, writer, true)
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

	// apply some ops
	ts := time.Now()
	err = fsHandle.Mknod(ctx, true, []string{"hello-dir-1"}, unixfs.NewFSCursorNodeType_Dir(), 0, ts)
	if err != nil {
		t.Fatal(err.Error())
	}

	dirHandle, err := fsHandle.Lookup(ctx, "hello-dir-1")
	if err != nil {
		t.Fatal(err.Error())
	}

	err = dirHandle.Mknod(ctx, true, []string{"test.txt"}, unixfs.NewFSCursorNodeType_File(), 0, ts)
	if err != nil {
		t.Fatal(err.Error())
	}

	fhandle, err := dirHandle.Lookup(ctx, "test.txt")
	if err != nil {
		t.Fatal(err.Error())
	}

	// write some data
	testData := []byte("hello world")
	err = fhandle.Write(ctx, 0, testData, time.Now())
	if err != nil {
		t.Fatal(err.Error())
	}

	// read data
	buf := make([]byte, 1500)
	nread, err := fhandle.Read(ctx, 0, buf)
	if err != nil {
		t.Fatal(err.Error())
	}
	buf = buf[:nread]
	if !bytes.Equal(buf, testData) {
		t.Fatalf("read incorrect data: %#v != %#v", buf, string(testData))
	}

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
}
