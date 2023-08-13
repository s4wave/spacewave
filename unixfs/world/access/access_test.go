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
	world_testbed "github.com/aperturerobotics/hydra/world/testbed"
	billy_util "github.com/go-git/go-billy/v5/util"
	"github.com/sirupsen/logrus"
)

func TestUnixFSWorldAccessController(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	btb, err := testbed.NewTestbed(ctx, le, testbed.WithVerbose(true))
	if err != nil {
		t.Fatal(err.Error())
	}

	objKey := "test-fs"
	rootRef, tb, err := unixfs_world.BuildTestbed(
		btb,
		objKey,
		true,
		world_testbed.WithWorldVerbose(true),
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	rbfs := unixfs.NewBillyFS(ctx, rootRef, "", time.Now())
	testData := []byte("hello world")
	if err := billy_util.WriteFile(rbfs, "/bat/baz/test-file.txt", testData, 0755); err != nil {
		t.Fatal(err.Error())
	}

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
}

func TestUnixFSWorldAccessController_AccessFunc(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	btb, err := testbed.NewTestbed(ctx, le, testbed.WithVerbose(true))
	if err != nil {
		t.Fatal(err.Error())
	}

	objKey := "test-fs"
	rootRef, tb, err := unixfs_world.BuildTestbed(
		btb,
		objKey,
		true,
		world_testbed.WithWorldVerbose(true),
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rootRef.Release()
	tb.StaticResolver.AddFactory(NewFactory(tb.Bus))

	rbfs := unixfs.NewBillyFS(ctx, rootRef, "", time.Now())
	testData := []byte("hello world")
	if err := billy_util.WriteFile(rbfs, "/bat/baz/test-file.txt", testData, 0755); err != nil {
		t.Fatal(err.Error())
	}

	// construct the access func
	unixFsID := "test-fs"
	accessFn := NewAccessUnixFSFunc(tb.Bus, &Config{FsId: unixFsID, FsRef: &unixfs_world.UnixfsRef{ObjectKey: objKey}})

	// access it!
	fsh, fshRel, err := accessFn(ctx, nil)
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
}
