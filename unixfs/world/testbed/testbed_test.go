package unixfs_world_testbed

import (
	"context"
	"testing"
	"time"

	hydra_testbed "github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_billy "github.com/aperturerobotics/hydra/unixfs/billy"
	unixfs_e2e "github.com/aperturerobotics/hydra/unixfs/e2e"
	unixfs_world "github.com/aperturerobotics/hydra/unixfs/world"
	"github.com/aperturerobotics/hydra/world"
	world_testbed "github.com/aperturerobotics/hydra/world/testbed"
	world_types "github.com/aperturerobotics/hydra/world/types"
	billy_util "github.com/go-git/go-billy/v5/util"
	"github.com/sirupsen/logrus"
)

var objKey = "test/fs"

// TestFs runs the e2e tests.
func TestFs(t *testing.T) {
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

	watchWorldChanges := true
	fsHandle, err := InitTestbed(wtb, objKey, watchWorldChanges)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fsHandle.Release()

	if err := unixfs_e2e.TestUnixFS(ctx, fsHandle); err != nil {
		t.Fatal(err.Error())
	}
}

// TestFs_SingleTxn runs the e2e tests with a single transaction.
func TestFs_SingleTxn(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(logger)

	htb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	tb, err := world_testbed.NewTestbed(htb)
	if err != nil {
		t.Fatal(err.Error())
	}

	// provide op handlers to bus
	engineID := tb.EngineID
	opc := world.NewLookupOpController("test-fs-ops", engineID, unixfs_world.LookupFsOp)
	_, err = tb.Bus.AddController(ctx, opc, nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	// uses directive to look up the engine
	eng := tb.Engine
	sender := tb.Volume.GetPeerID()
	fsType := unixfs_world.FSType_FSType_FS_NODE

	// init the fs
	if err := func() error {
		// build a write txn
		wtx, err := eng.NewTransaction(ctx, true)
		if err != nil {
			return err
		}
		defer wtx.Discard()

		typeID, _ := unixfs_world.FSTypeToTypeID(fsType)
		_, _, err = unixfs_world.FsInit(
			ctx,
			wtx,
			sender,
			objKey,
			fsType,
			nil,
			true,
			time.Now(),
		)
		if err != nil {
			return err
		}

		// check type
		if err := world_types.CheckObjectType(ctx, wtx, objKey, typeID); err != nil {
			return err
		}

		return wtx.Commit(ctx)
	}(); err != nil {
		t.Fatal(err.Error())
	}

	// construct full fs
	tb.Logger.Debug("filesystem initialized")

	buildFsh := func() (wtx world.Tx, fsh *unixfs.FSHandle, err error) {
		wtx, err = eng.NewTransaction(ctx, true)
		if err != nil {
			return nil, nil, err
		}

		fsCursor, _ := unixfs_world.NewFSCursorWithWriter(ctx, le, wtx, objKey, fsType, sender)
		fsh, err = unixfs.NewFSHandle(fsCursor)
		if err != nil {
			wtx.Discard()
			fsCursor.Release()
			return nil, nil, err
		}

		return wtx, fsh, nil
	}

	// quick test using a temporary (not written) txn
	// we expect to be able to do everything on a temporary fs txn without committing
	if err := func() error {
		wtx, fsh, err := buildFsh()
		if err != nil {
			return err
		}
		defer wtx.Discard()
		defer fsh.Release()

		if err := fsh.Mknod(ctx, false, []string{"mydir"}, unixfs.NewFSCursorNodeType_Dir(), 0644, time.Now()); err != nil {
			return err
		}

		if err := fsh.MkdirAll(ctx, []string{"test", "dir"}, 0700, time.Now()); err != nil {
			return err
		}

		if err := fsh.Mknod(ctx, false, []string{"hello.txt", "world.md"}, unixfs.NewFSCursorNodeType_File(), 0644, time.Now()); err != nil {
			return err
		}

		// success
		return wtx.Commit(ctx)
	}(); err != nil {
		t.Fatal(err.Error())
	}

	// full test on write txn with commit
	// we expect to be able to do everything on a temporary fs txn without committing
	if err := func() error {
		wtx, fsh, err := buildFsh()
		if err != nil {
			return err
		}
		defer wtx.Discard()
		defer fsh.Release()

		if err := unixfs_e2e.TestUnixFS(ctx, fsh); err != nil {
			return err
		}

		// success
		return wtx.Commit(ctx)
	}(); err != nil {
		t.Fatal(err.Error())
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
	fsHandle, err := InitTestbed(wtb, objKey, watchWorldChanges)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fsHandle.Release()

	// create test fs (backed by a block graph + Hydra world)
	bfs := unixfs_billy.NewBillyFilesystem(ctx, fsHandle, "", time.Now())

	// create test script
	filename := "test.js"
	data := []byte("Hello world!\n")
	err = billy_util.WriteFile(bfs, filename, data, 0o755)
	if err != nil {
		t.Fatal(err.Error())
	}

	// read file size & check
	fi, err := bfs.Stat(filename)
	if err != nil {
		t.Fatal(err.Error())
	}
	if s := int(fi.Size()); s < len(data) {
		t.Fatalf("expected size %d but got %d", len(data), s)
	}
}
