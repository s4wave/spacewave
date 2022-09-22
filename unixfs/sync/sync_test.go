package unixfs_sync

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_world "github.com/aperturerobotics/hydra/unixfs/world"
	memfs "github.com/go-git/go-billy/v5/memfs"
	billy_util "github.com/go-git/go-billy/v5/util"
)

// TestSync tests syncing a UnixFS to the disk.
func TestSync(t *testing.T) {
	objKey := "fs/test/1"
	ctx := context.Background()
	wfs, wtb, err := unixfs_world.BuildTestbed(ctx, objKey, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer wtb.Release()

	rref, err := wfs.AddRootReference(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rref.Release()

	ts := time.Now()
	bfs := unixfs.NewBillyFS(ctx, rref, "", ts)

	testFile := "test.txt"
	testData := []byte("Hello world!")
	err = billy_util.WriteFile(bfs, testFile, testData, 0755)
	if err != nil {
		t.Fatal(err.Error())
	}

	// TODO: requires a slight delay for the fscursors to update
	// TODO: This is a bug that currently is being fixed
	time.Sleep(time.Millisecond * 50)

	outFs := memfs.New()
	err = billy_util.WriteFile(outFs, testFile, []byte("Incorrect data to be overwritten by sync"), 0755)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = billy_util.WriteFile(outFs, "deleteme.txt", []byte("This file should be deleted"), 0755)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = SyncToBilly(ctx, outFs, rref, DeleteMode_DeleteMode_DURING)
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
