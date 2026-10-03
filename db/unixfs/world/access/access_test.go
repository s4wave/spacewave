package unixfs_world_access

import (
	"bytes"
	"context"
	"testing"
	"time"

	billy_util "github.com/go-git/go-billy/v6/util"
	"github.com/s4wave/spacewave/db/testbed"
	unixfs_access "github.com/s4wave/spacewave/db/unixfs/access"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	unixfs_world_testbed "github.com/s4wave/spacewave/db/unixfs/world/testbed"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/sirupsen/logrus"
)

func TestUnixFSWorldAccessController(t *testing.T) {
	// Prepare the World access test context and logger.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the storage testbed for the World filesystem.
	btb, err := testbed.NewTestbed(ctx, le, testbed.WithVerbose(true))
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create a World-backed filesystem root for the test.
	objKey := "test-fs"
	rootRef, tb, err := unixfs_world_testbed.BuildTestbed(
		btb,
		objKey,
		true,
		world_testbed.WithWorldVerbose(true),
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Save the test payload in the World-backed filesystem.
	rbfs := unixfs_billy.NewBillyFS(ctx, rootRef, "", time.Now())
	testData := []byte("hello world")
	if err := billy_util.WriteFile(rbfs, "/bat/baz/test-file.txt", testData, 0o755); err != nil {
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

	// Attach the filesystem access controller to the test bus.
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

	// Acquire a filesystem handle through the resolved access function.
	fsh, fshRel, err := accessUfs(ctx, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fshRel()

	// Read the saved payload through the acquired filesystem handle.
	bfs := unixfs_billy.NewBillyFS(ctx, fsh, "/", time.Now())
	rd, err := billy_util.ReadFile(bfs, "bat/baz/test-file.txt")
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify that World access returns the saved payload.
	if !bytes.Equal(rd, testData) {
		t.Fail()
	}
}

func TestUnixFSWorldAccessController_AccessFunc(t *testing.T) {
	// Prepare the World access test context and logger.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the storage testbed for the World filesystem.
	btb, err := testbed.NewTestbed(ctx, le, testbed.WithVerbose(true))
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create a World-backed filesystem root for the test.
	objKey := "test-fs"
	rootRef, tb, err := unixfs_world_testbed.BuildTestbed(
		btb,
		objKey,
		true,
		world_testbed.WithWorldVerbose(true),
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rootRef.Release()

	// Register the filesystem access factory for controller resolution.
	tb.StaticResolver.AddFactory(NewFactory(tb.Bus))

	// Save the test payload in the World-backed filesystem.
	rbfs := unixfs_billy.NewBillyFS(ctx, rootRef, "", time.Now())
	testData := []byte("hello world")
	if err := billy_util.WriteFile(rbfs, "/bat/baz/test-file.txt", testData, 0o755); err != nil {
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

	// Read the saved payload through the acquired filesystem handle.
	bfs := unixfs_billy.NewBillyFS(ctx, fsh, "/", time.Now())
	rd, err := billy_util.ReadFile(bfs, "bat/baz/test-file.txt")
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify that World access returns the saved payload.
	if !bytes.Equal(rd, testData) {
		t.Fail()
	}
}
