package kvtx_block

import (
	"context"
	"testing"

	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/sirupsen/logrus"
)

func TestNextIndexedLogIndexUsesLatestKey(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	btx, bcs := oc.BuildTransaction(nil)
	bcs.SetBlock(NewKeyValueStore(0), true)
	if _, bcs, err = btx.Write(ctx, true); err != nil {
		t.Fatal(err.Error())
	}

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

	tree, err = BuildKvTransaction(ctx, bcs, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tree.Discard()
	next, err := NextIndexedLogIndex(ctx, tree)
	if err != nil {
		t.Fatal(err.Error())
	}
	if next != 10 {
		t.Fatalf("next index: got %d want 10", next)
	}
}

func TestNextIndexedLogIndexEmptyTree(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	btx, bcs := oc.BuildTransaction(nil)
	bcs.SetBlock(NewKeyValueStore(0), true)
	if _, bcs, err = btx.Write(ctx, true); err != nil {
		t.Fatal(err.Error())
	}
	tree, err := BuildKvTransaction(ctx, bcs, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tree.Discard()
	next, err := NextIndexedLogIndex(ctx, tree)
	if err != nil {
		t.Fatal(err.Error())
	}
	if next != 0 {
		t.Fatalf("next index: got %d want 0", next)
	}
}
