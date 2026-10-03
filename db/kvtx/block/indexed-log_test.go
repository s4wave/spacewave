package kvtx_block

import (
	"context"
	"testing"

	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/sirupsen/logrus"
)

func TestNextIndexedLogIndexUsesLatestKey(t *testing.T) {
	for _, impl := range []KVImplType{
		KVImplType_KV_IMPL_TYPE_IAVL,
		KVImplType_KV_IMPL_TYPE_OKRA,
	} {
		t.Run(impl.String(), func(t *testing.T) {
			testNextIndexedLogIndexUsesLatestKey(t, impl)
		})
	}
}

func testNextIndexedLogIndexUsesLatestKey(t *testing.T, impl KVImplType) {
	// Create a logger for the indexed log testbed.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start a testbed for the selected indexed log backend.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Persist an empty indexed log root in the testbed bucket.
	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	btx, bcs := oc.BuildTransaction(nil)
	bcs.SetBlock(NewKeyValueStore(impl), true)
	if _, bcs, err = btx.Write(ctx, true); err != nil {
		t.Fatal(err.Error())
	}

	// Write and commit sparse indexed log entries.
	tree, err := BuildKvTransaction(ctx, bcs, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	for _, index := range []uint64{0, 3, 9} {
		valueCursor := bcs.Detach(false)
		valueCursor.ClearAllRefs()
		valueCursor.SetBlock(block_mock.NewExample("indexed log"), true)
		if err := tree.SetCursorAtKey(ctx, IndexedLogKey(index), valueCursor, false); err != nil {
			t.Fatal(err.Error())
		}
	}
	if err := tree.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	// Open the saved indexed log for readback.
	tree, err = BuildKvTransaction(ctx, bcs, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tree.Discard()

	// Verify the reopened log advances past its latest index.
	next, err := NextIndexedLogIndex(ctx, tree)
	if err != nil {
		t.Fatal(err.Error())
	}
	if next != 10 {
		t.Fatalf("next index: got %d want 10", next)
	}
}

func TestNextIndexedLogIndexEmptyTree(t *testing.T) {
	for _, impl := range []KVImplType{
		KVImplType_KV_IMPL_TYPE_IAVL,
		KVImplType_KV_IMPL_TYPE_OKRA,
	} {
		t.Run(impl.String(), func(t *testing.T) {
			testNextIndexedLogIndexEmptyTree(t, impl)
		})
	}
}

func testNextIndexedLogIndexEmptyTree(t *testing.T, impl KVImplType) {
	// Create a logger for the empty indexed log testbed.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start a testbed for the selected empty log backend.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Persist an empty indexed log root in the testbed bucket.
	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	btx, bcs := oc.BuildTransaction(nil)
	bcs.SetBlock(NewKeyValueStore(impl), true)
	if _, bcs, err = btx.Write(ctx, true); err != nil {
		t.Fatal(err.Error())
	}

	// Open the empty indexed log for readback.
	tree, err := BuildKvTransaction(ctx, bcs, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tree.Discard()

	// Verify an empty indexed log starts at index zero.
	next, err := NextIndexedLogIndex(ctx, tree)
	if err != nil {
		t.Fatal(err.Error())
	}
	if next != 0 {
		t.Fatalf("next index: got %d want 0", next)
	}
}
