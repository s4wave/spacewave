package unixfs_checkout

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/hydra/testbed"
	unixfs_billy "github.com/aperturerobotics/hydra/unixfs/billy"
	unixfs_world_testbed "github.com/aperturerobotics/hydra/unixfs/world/testbed"
	testbed0 "github.com/aperturerobotics/hydra/world/testbed"
	memfs "github.com/go-git/go-billy/v5/memfs"
	billy_util "github.com/go-git/go-billy/v5/util"
	"github.com/sirupsen/logrus"

	// TestCheckout tests checking out a UnixFS to the disk.
	"github.com/aperturerobotics/hydra/unixfs"
)

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
	wfs, wtb, err := func() (*unixfs.FSHandle, *testbed0.Testbed, error) {
		wtb, err := testbed0.NewTestbed(tb, []testbed0.Option{}...)
		if err != nil {
			return nil, nil, err
		}
		ufs, err := unixfs_world_testbed.InitTestbed(wtb, objKey, true)
		if err != nil {
			return nil, wtb, err
		}
		return ufs, wtb, nil
	}()
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
