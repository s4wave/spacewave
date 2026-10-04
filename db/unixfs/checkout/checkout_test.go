package unixfs_checkout

import (
	"bytes"
	"context"
	"testing"
	"time"

	memfs "github.com/go-git/go-billy/v6/memfs"
	billy_util "github.com/go-git/go-billy/v6/util"
	"github.com/s4wave/spacewave/db/testbed"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	unixfs_world_testbed "github.com/s4wave/spacewave/db/unixfs/world/testbed"
	testbed0 "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/sirupsen/logrus"

	"github.com/s4wave/spacewave/db/unixfs"
)

// TestCheckout verifies that a UnixFS file survives checkout to a BillyFS.
func TestCheckout(t *testing.T) {
	// Prepare a logger and context for the checkout test.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the storage testbed.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create a test filesystem in the World testbed.
	objKey := "fs/test"
	wfs, wtb, err := func() (*unixfs.FSHandle, *testbed0.Testbed, error) {
		// Start the World testbed that owns the filesystem object.
		wtb, err := testbed0.NewTestbed(tb, []testbed0.Option{}...)
		if err != nil {
			return nil, nil, err
		}

		// Initialize the UnixFS test object.
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

	// Build a BillyFS view of the initialized UnixFS.
	ts := time.Now()
	bfs := unixfs_billy.NewBillyFS(ctx, wfs, "", ts)

	// Write sample contents through the BillyFS adapter.
	testFile := "test.txt"
	testData := []byte("Hello world!")
	err = billy_util.WriteFile(bfs, testFile, testData, 0o755)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Check out the UnixFS into an in-memory BillyFS.
	outFs := memfs.New()
	err = CheckoutToBilly(ctx, outFs, wfs, nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the copied file bytes match the source.
	readData, err := billy_util.ReadFile(outFs, testFile)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(testData, readData) {
		t.Fatalf("data mismatch: %v != %v", testData, readData)
	}
}
