package sobject_world_engine

import (
	"errors"
	"testing"

	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/db/tx"
	world_block "github.com/s4wave/spacewave/db/world/block"
)

// TestReadCheckpointFreezesWorld checks the read checkpoint serves the last
// installed World head and rejects every write.
func TestReadCheckpointFreezesWorld(t *testing.T) {
	// Install a World holding one object, then build a later World.
	ctx := t.Context()
	c, so, head := newProcessTestWorld(t, ctx)
	so.localStore = store_kvtx_inmem.NewStore()
	known := applyTransactionTestObject(t, c, so, head, "known-object")
	if err := writeWorldHead(ctx, so, known.GetHeadRef()); err != nil {
		t.Fatal(err)
	}
	_ = applyTransactionTestObject(t, c, so, known, "later-object")

	// The checkpoint reads the installed World only.
	engine, release, err := OpenReadCheckpoint(ctx, c.le, c.bus, so)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		release()
		if _, err := engine.NewTransaction(ctx, false); !errors.Is(err, world_block.ErrEngineClosed) {
			t.Fatalf("released checkpoint retained its engine: %v", err)
		}
	}()

	// A read transaction sees the known object only.
	read, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer read.Discard()
	if found, err := read.HasObject(ctx, "known-object"); err != nil || !found {
		t.Fatalf("checkpoint lost known object: found=%v err=%v", found, err)
	}
	if found, err := read.HasObject(ctx, "later-object"); err != nil || found {
		t.Fatalf("checkpoint exposed later object: found=%v err=%v", found, err)
	}

	// It refuses writes.
	if _, err := engine.NewTransaction(ctx, true); !errors.Is(err, tx.ErrNotWrite) {
		t.Fatalf("checkpoint accepted write transaction: %v", err)
	}

	// Both of its cursors refuse writes.
	cursor, err := engine.BuildStorageCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cursor.Release()
	if _, _, err := cursor.PutBlock(ctx, []byte("unreferenced history write"), nil); !errors.Is(err, tx.ErrNotWrite) {
		t.Fatalf("checkpoint storage cursor accepted a write: %v", err)
	}
	if err := engine.AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
		_, _, err := cursor.PutBlock(ctx, []byte("root history write"), nil)
		return err
	}); !errors.Is(err, tx.ErrNotWrite) {
		t.Fatalf("checkpoint World cursor accepted a write: %v", err)
	}
}
