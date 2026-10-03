package world_block

import (
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/sirupsen/logrus"
)

// TestEngineTxReleasesUnreachedWrites checks that a write transaction outside
// a fork releases the blocks its final root does not reach: a stray block of a
// committed transaction and every block of a discarded one.
func TestEngineTxReleasesUnreachedWrites(t *testing.T) {
	// Open a garbage collected testbed volume.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Open a durable-on-write engine on an empty cursor.
	base, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(base.Release)
	engine, err := NewEngine(ctx, le, base, world_mock.LookupMockOp, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := engine.Close(); err != nil {
			t.Error(err)
		}
	})

	// write opens a transaction that writes a stray block and an object root,
	// and creates an object at the root.
	write := func(key string) (*EngineTx, *block.BlockRef, *block.BlockRef) {
		// Open the transaction and its storage cursor.
		t.Helper()
		w, err := engine.NewBlockEngineTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(w.Discard)
		c, err := w.BuildStorageCursor(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Release()

		// Write the stray block and the object root.
		stray, _, err := c.GetBlockStore().PutBlock(ctx, []byte("stray "+key), nil)
		if err != nil {
			t.Fatal(err)
		}
		btx, bcs := c.BuildTransaction(nil)
		bcs.SetBlock(block_mock.NewExample("root "+key), true)
		root, _, err := btx.Write(ctx, true)
		if err != nil {
			t.Fatal(err)
		}

		// Create the object at the root.
		ref := c.GetRef().Clone()
		ref.RootRef = root
		obj, err := w.CreateObject(ctx, key, ref)
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}
		return w, stray, root
	}

	// Commit one transaction and discard another.
	committed, committedStray, committedRoot := write("committed")
	if err := committed.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	discarded, discardedStray, discardedRoot := write("discarded")
	discarded.Discard()

	// Only the committed object root survives a sweep.
	if _, err := block_gc.NewCollector(tb.Volume.GetRefGraph(), tb.Volume, nil).Collect(ctx); err != nil {
		t.Fatal(err)
	}
	want := map[*block.BlockRef]bool{
		committedRoot:  true,
		committedStray: false,
		discardedStray: false,
		discardedRoot:  false,
	}
	for ref, exists := range want {
		if found, err := tb.Volume.GetBlockExists(ctx, ref); err != nil || found != exists {
			t.Fatalf("block %s want=%v found=%v err=%v", ref.MarshalString(), exists, found, err)
		}
	}
}
