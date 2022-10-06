package unixfs_checkout

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

// TestCheckout tests checking out a UnixFS to the disk.
func TestCheckout(t *testing.T) {
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
	err = CheckoutToBilly(ctx, outFs, rref, nil)
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
}
