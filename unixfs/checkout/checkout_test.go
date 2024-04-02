package unixfs_checkout

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/hydra/testbed"
	unixfs_billy "github.com/aperturerobotics/hydra/unixfs/billy"
	unixfs_world "github.com/aperturerobotics/hydra/unixfs/world"
	memfs "github.com/go-git/go-billy/v5/memfs"
	billy_util "github.com/go-git/go-billy/v5/util"
	"github.com/sirupsen/logrus"
)

// TestCheckout tests checking out a UnixFS to the disk.
func TestCheckout(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	objKey := "fs/test"
	wfs, wtb, err := unixfs_world.BuildTestbed(tb, objKey, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer wtb.Release()

	ts := time.Now()
	bfs := unixfs_billy.NewBillyFS(ctx, wfs, "", ts)

	testFile := "test.txt"
	testData := []byte("Hello world!")
	err = billy_util.WriteFile(bfs, testFile, testData, 0o755)
	if err != nil {
		t.Fatal(err.Error())
	}

	outFs := memfs.New()
	err = CheckoutToBilly(ctx, outFs, wfs, nil)
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
