package sobject_world_engine

import (
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/db/tx"
	world_block "github.com/s4wave/spacewave/db/world/block"
)

// TestReadCheckpointFreezesWorld checks the read checkpoint serves the World
// replayed from the held state and rejects every write.
func TestReadCheckpointFreezesWorld(t *testing.T) {
	// Hold one operation above the genesis checkpoint. A later operation the
	// departed device never received stays out of its World.
	ctx := t.Context()
	priv, pid := newReplayTestKey(t)
	space := newReplayTestSpace(t, pid)
	space.so.localStore = store_kvtx_inmem.NewStore()
	known := space.sign("known", priv, "known-object", &sobject.SOOperationLink{Nonce: 1})
	_ = space.sign("later", priv, "later-object", &sobject.SOOperationLink{Nonce: 2, PrevOpHash: known.Hash()})
	snap := newHeldTestSnapshot(t, space, known)

	// The checkpoint reads the replayed World only.
	engine, release, err := OpenReadCheckpoint(ctx, space.c.le, space.c.bus, space.so, "test-read-checkpoint", snap)
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

// heldTestSnapshot is a held state: the genesis World as its checkpoint and
// the operations above it.
type heldTestSnapshot struct {
	*replayTestSnapshot
	checkpoint *sobject.SOCheckpointInner
	set        *sobject.SOOperationSet
}

// newHeldTestSnapshot holds ops above the genesis World of space.
func newHeldTestSnapshot(t *testing.T, space *replayTestSpace, ops ...*sobject.SOOperation) *heldTestSnapshot {
	// Encode the genesis World as the checkpoint state.
	t.Helper()
	stateData, err := space.genesis.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := &sobject.SOCheckpointInner{StateData: stateData}

	// Hold the operations above it.
	set := sobject.NewSOOperationSet(replayTestObjectID, checkpoint)
	for _, op := range ops {
		if _, err := set.Add(op); err != nil {
			t.Fatal(err)
		}
	}
	return &heldTestSnapshot{
		replayTestSnapshot: &replayTestSnapshot{config: space.config},
		checkpoint:         checkpoint,
		set:                set,
	}
}

// GetCheckpoint returns the genesis checkpoint.
func (s *heldTestSnapshot) GetCheckpoint(context.Context) (*sobject.SOCheckpointInner, error) {
	return s.checkpoint, nil
}

// GetOperationSet returns the held operations.
func (s *heldTestSnapshot) GetOperationSet(context.Context) (*sobject.SOOperationSet, error) {
	return s.set, nil
}
