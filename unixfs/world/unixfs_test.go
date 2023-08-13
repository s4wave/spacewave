package unixfs_world

import (
	"context"
	"testing"
	"time"

	hydra_testbed "github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_e2e "github.com/aperturerobotics/hydra/unixfs/e2e"
	world_testbed "github.com/aperturerobotics/hydra/world/testbed"
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
	bfs := unixfs.NewBillyFilesystem(ctx, fsHandle, "", time.Now())

	// create test script
	filename := "test.js"
	data := []byte("Hello world!\n")
	err = billy_util.WriteFile(bfs, filename, data, 0755)
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
