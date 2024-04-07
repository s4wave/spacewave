package unixfs_sync

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/hydra/testbed"
	unixfs_billy "github.com/aperturerobotics/hydra/unixfs/billy"
	unixfs_world_testbed "github.com/aperturerobotics/hydra/unixfs/world/testbed"
	world_testbed "github.com/aperturerobotics/hydra/world/testbed"
	memfs "github.com/go-git/go-billy/v5/memfs"
	billy_util "github.com/go-git/go-billy/v5/util"
	"github.com/sirupsen/logrus"
)

// TestSync tests syncing a UnixFS to the disk.
func TestSync(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	btb, err := testbed.NewTestbed(ctx, le, testbed.WithVerbose(true))
	if err != nil {
		t.Fatal(err.Error())
	}

	objKey := "test-fs"
	rref, _, err := unixfs_world_testbed.BuildTestbed(
		btb,
		objKey,
		true,
		world_testbed.WithWorldVerbose(true),
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	ts := time.Now()
	bfs := unixfs_billy.NewBillyFS(ctx, rref, "", ts)

	testFile := "test.txt"
	testData := []byte("Hello world!")
	err = billy_util.WriteFile(bfs, testFile, testData, 0o755)
	if err != nil {
		t.Fatal(err.Error())
	}

	outFs := memfs.New()
	err = billy_util.WriteFile(outFs, testFile, []byte("Incorrect data to be overwritten by sync"), 0o755)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = billy_util.WriteFile(outFs, "deleteme.txt", []byte("This file should be deleted"), 0o755)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = SyncToBilly(ctx, outFs, rref, DeleteMode_DeleteMode_DURING, nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	readData, err := billy_util.ReadFile(outFs, testFile)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(testData, readData) {
		t.Fatalf("data mismatch: %v != %v", testData, readData)
	}

	outFiles, err := outFs.ReadDir("")
	if err != nil {
		t.Fatal(err.Error())
	}

	if len(outFiles) != 1 {
		t.Fatalf("expected 1 file but got %d", len(outFiles))
	}
}
