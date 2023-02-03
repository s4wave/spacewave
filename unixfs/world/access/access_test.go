package unixfs_world_access

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_access "github.com/aperturerobotics/hydra/unixfs/access"
	unixfs_world "github.com/aperturerobotics/hydra/unixfs/world"
	billy_util "github.com/go-git/go-billy/v5/util"
)

func TestUnixFSWorldAccessController(t *testing.T) {
	ctx := context.Background()
	objKey := "test-fs"
	fs, tb, err := unixfs_world.BuildTestbed(
		ctx,
		objKey,
		true,
		testbed.WithVerbose(true),
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// fill the sample filesystem
	rootRef, err := fs.AddRootReference(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rootRef.Release()

	rbfs := unixfs.NewBillyFS(ctx, rootRef, "", time.Now())
	testData := []byte("hello world")
	if err := billy_util.WriteFile(rbfs, "/bat/baz/test-file.txt", testData, 0755); err != nil {
		t.Fatal(err.Error())
	}

	// wait a moment for the write to be confirmed
	// TODO: This is a bug that currently is being fixed
	<-time.After(time.Millisecond * 100)

	// construct the AccessUnixFS handler
	unixFsID := "test-fs"
	accessCtrl, err := NewController(
		tb.Logger,
		tb.Bus,
		&Config{FsId: unixFsID, FsRef: &unixfs_world.UnixfsRef{ObjectKey: objKey}},
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	accessRel, err := tb.Bus.AddController(ctx, accessCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer accessRel()

	// access it!
	accessUfs, ufsRef, err := unixfs_access.ExAccessUnixFS(ctx, tb.Bus, unixFsID, false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ufsRef.Release()

	fsh, fshRel, err := accessUfs(ctx, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fshRel()

	bfs := unixfs.NewBillyFS(ctx, fsh, "/", time.Now())
	rd, err := billy_util.ReadFile(bfs, "bat/baz/test-file.txt")
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(rd, testData) {
		t.Fail()
	}

	// test accessing with access cursor
	fsCursor := unixfs_access.NewFSCursor(accessUfs)
	accessFs := unixfs.NewFS(ctx, tb.Logger, fsCursor, []string{"bat"})
	fsHandle, err := accessFs.AddRootReference(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	fsHandleAfs := unixfs.NewAferoFS(ctx, fsHandle, "/bat/", time.Now())
	fi, err := fsHandleAfs.Stat("baz/test-file.txt")
	if err != nil {
		t.Fatal(err.Error())
	}
	tb.Logger.Infof("successfully stat() via unixfs_access FSCursor: %s", fi.Name())
	// fsHandle := unixfs.newfs
}
