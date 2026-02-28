package unixfs_access_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_access "github.com/aperturerobotics/hydra/unixfs/access"
	unixfs_billy "github.com/aperturerobotics/hydra/unixfs/billy"
	unixfs_world_testbed "github.com/aperturerobotics/hydra/unixfs/world/testbed"
	world_testbed "github.com/aperturerobotics/hydra/world/testbed"
	"github.com/blang/semver/v4"
	billy_util "github.com/go-git/go-billy/v6/util"
	"github.com/sirupsen/logrus"
)

func TestFSCursor(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	btb, err := testbed.NewTestbed(ctx, le, testbed.WithVerbose(true))
	if err != nil {
		t.Fatal(err.Error())
	}

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

	rbfs := unixfs_billy.NewBillyFS(ctx, rootRef, "", time.Now())
	testData := []byte("hello world")
	if err := billy_util.WriteFile(rbfs, "/bat/baz/test-file.txt", testData, 0o755); err != nil {
		t.Fatal(err.Error())
	}

	testJsData := []byte("console.log(\"hello world\")\n")
	if err := billy_util.WriteFile(rbfs, "/bat/baz/script.js", testJsData, 0o755); err != nil {
		t.Fatal(err.Error())
	}

	// construct the AccessUnixFS handler
	unixFsID := "test-fs"
	accessCtrl := unixfs_access.NewControllerWithHandle(
		tb.Logger,
		tb.Bus,
		controller.NewInfo("hydra/unixfs/access/test", semver.MustParse("0.0.1"), "access test unixfs"),
		[]string{unixFsID},
		rootRef,
	)
	accessRel, err := tb.Bus.AddController(ctx, accessCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer accessRel()

	// create the access function accessing via the bus
	accessFunc := unixfs_access.NewAccessUnixFSViaBusFunc(tb.Bus, unixFsID, false)

	// create the access function fscursor
	accessFuncFsCursor := unixfs_access.NewFSCursor(accessFunc)

	// create the handle with the cursor
	accessFuncFsh, err := unixfs.NewFSHandle(accessFuncFsCursor)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer accessFuncFsh.Release()

	// try accessing the files
	accessFuncFshBilly := unixfs_billy.NewBillyFS(ctx, accessFuncFsh, "", time.Now())
	readData, err := billy_util.ReadFile(accessFuncFshBilly, "/bat/baz/test-file.txt")
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(readData, testData) {
		t.Fatal("test data mismatch")
	}
}
