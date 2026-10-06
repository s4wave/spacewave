package sobject_world_engine

import (
	"context"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/bstore"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/bucket"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/db/world"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/sirupsen/logrus"
)

// hidingBlockStore withholds one block while hidden is set, as a store does
// when no peer supplies the block.
type hidingBlockStore struct {
	bstore.BlockStore
	ref    *block.BlockRef
	hidden atomic.Bool
}

// GetBlock reads a block, reporting the hidden block as not found.
func (s *hidingBlockStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	if s.hidden.Load() && ref.EqualsRef(s.ref) {
		return nil, false, nil
	}
	return s.BlockStore.GetBlock(ctx, ref)
}

// opCounter counts the operations replay starts to apply.
type opCounter struct {
	n atomic.Int32
}

// Levels returns the level processOp logs a starting operation at.
func (c *opCounter) Levels() []logrus.Level {
	return []logrus.Level{logrus.DebugLevel}
}

// Fire counts a starting operation.
func (c *opCounter) Fire(entry *logrus.Entry) error {
	if entry.Message == "processing op" {
		c.n.Add(1)
	}
	return nil
}

// TestEngineStopsReplayAtMissingBlock replays a log whose last operation needs
// a block that is not available. The initial replay and each later one must
// stop at that operation without holding the write lock, serve the World before
// it, fail writers with the missing block, and resume at that operation, never
// applying an earlier operation again, once the block arrives.
func TestEngineStopsReplayAtMissingBlock(t *testing.T) {
	// Build a Space whose operations create three objects, create one whose
	// root block is missing, and then edit it.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	privA, pidA := newReplayTestKey(t)
	space := newReplayTestSpace(t, pidA)
	space.c.SetStaticLookupOp(world_mock.LookupMockObjectOp)

	// Write the block the fourth operation's object uses as its root.
	transformConf := space.genesis.GetHeadRef().GetTransformConf()
	xfrm, err := block_transform.NewTransformer(controller.ConstructOpts{}, space.c.sfs, transformConf)
	if err != nil {
		t.Fatal(err.Error())
	}
	btx, bcs := block.NewTransaction(space.so.blockStore, xfrm, nil, nil)
	bcs.SetBlock(&block_mock.Example{Msg: "payload"}, true)
	payload, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Build the fourth operation's object on that root and the fifth's edit
	// of it.
	ghost, err := world_block_tx.NewTxCreateObject("ghost", &bucket.ObjectRef{RootRef: payload, TransformConf: transformConf})
	if err != nil {
		t.Fatal(err.Error())
	}
	edit, err := world_block_tx.NewTxApplyObjectOp(world_mock.MockObjectOpId, world_mock.NewMockObjectOp("edited"), "ghost", pidA)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Sign the five operations as one chain.
	o1 := space.sign("o1", privA, "object-1", &sobject.SOOperationLink{Nonce: 1})
	o2 := space.sign("o2", privA, "object-2", &sobject.SOOperationLink{Nonce: 2, PrevOpHash: o1.Hash()})
	o3 := space.sign("o3", privA, "object-3", &sobject.SOOperationLink{Nonce: 3, PrevOpHash: o2.Hash()})
	o4 := space.signTx("ghost", privA, ghost, &sobject.SOOperationLink{Nonce: 4, PrevOpHash: o3.Hash()})
	o5 := space.signTx("edit", privA, edit, &sobject.SOOperationLink{Nonce: 5, PrevOpHash: o4.Hash()})

	// Encode the genesis World as the checkpoint state.
	genesisData, err := space.genesis.MarshalVT()
	if err != nil {
		t.Fatal(err.Error())
	}
	checkpoint := &sobject.SOCheckpointInner{StateData: genesisData}

	// Order the operations above the checkpoint.
	set := sobject.NewSOOperationSet(replayTestObjectID, checkpoint)
	for _, op := range []*sobject.SOOperation{o1, o2, o3, o4, o5} {
		if _, err := set.Add(op); err != nil {
			t.Fatal(err.Error())
		}
	}

	// Build the engine over a store that hides the root block.
	snap := &replayTestSnapshot{config: space.config, checkpoint: checkpoint, set: set}
	store := &hidingBlockStore{BlockStore: space.so.blockStore, ref: payload}
	store.hidden.Store(true)
	so := &testSharedObject{peerID: pidA, blockStore: store, localStore: store_kvtx_inmem.NewStore(), snapshot: snap}

	// The initial replay serves the World before the missing block.
	stateCtr := ccontainer.NewCContainer[sobject.SharedObjectStateSnapshot](snap)
	if _, err := space.c.waitWorldInit(ctx, so, stateCtr, newReplayer(space.c, so)); err != nil {
		t.Fatalf("initial replay with a missing block = %v; want the World before it", err)
	}

	// Build the engine over the same store.
	blk, err := buildBlockEngine(ctx, space.c.le, space.c.bus, space.c.sfs, so, space.genesis.GetHeadRef(), transformConf, space.c.buildLookupWorldOp(space.c.le), false)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(blk.Release)
	engine := newSoEngine(space.c, so, blk.bengine, newReplayer(space.c, so))

	// Count the operations replay applies.
	applied := &opCounter{}
	space.c.le.Logger.AddHook(applied)
	space.c.le.Logger.SetLevel(logrus.DebugLevel)
	space.c.le.Logger.SetOutput(io.Discard)
	wantApplied := func(want int32) {
		t.Helper()
		if got := applied.n.Load(); got != want {
			t.Fatalf("replay started %d operations; want %d", got, want)
		}
	}

	// writeLockFree fails the test when the write lock stays held.
	writeLockFree := func() {
		// Take and drop the lock within two seconds.
		t.Helper()
		lockCtx, lockCancel := context.WithTimeout(ctx, 2*time.Second)
		defer lockCancel()
		unlock, err := space.c.writeMtx.Lock(lockCtx)
		if err != nil {
			t.Fatalf("write lock is held: %v", err)
		}
		unlock()
	}

	// A writer fails with the missing block after replay applied the four
	// operations before it and tried the fifth.
	if tx, err := engine.NewTransaction(ctx, true); !errors.Is(err, block.ErrNotFound) {
		if err == nil {
			tx.Discard()
		}
		t.Fatalf("write with a missing block = %v; want block.ErrNotFound", err)
	} else if want := world_mock.MockObjectOpId + " on ghost"; !strings.Contains(err.Error(), want) {
		t.Fatalf("write with a missing block = %v; want it to name %q", err, want)
	}
	wantApplied(5)
	writeLockFree()

	// Readers see the World before the missing block.
	read, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	for _, key := range []string{"object-1", "object-2", "object-3", "ghost"} {
		objectState, found, err := read.GetObject(ctx, key)
		world.ReleaseObjectState(objectState)
		if err != nil || !found {
			t.Fatalf("object %s before the missing block: found %v, %v", key, found, err)
		}
	}
	read.Discard()

	// The watcher keeps running through a state change and retries only the
	// operation that failed, as does the next writer.
	if err := space.c.executeWatchSOStateOnce(ctx, snap, engine); err != nil {
		t.Fatalf("watcher ended on a missing block: %v", err)
	}
	wantApplied(6)
	if tx, err := engine.NewTransaction(ctx, true); !errors.Is(err, block.ErrNotFound) {
		if err == nil {
			tx.Discard()
		}
		t.Fatalf("second write with a missing block = %v; want block.ErrNotFound", err)
	}
	wantApplied(7)
	writeLockFree()

	// Once the block arrives, a writer resumes at the operation that stopped.
	store.hidden.Store(false)
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("write after the block arrived: %v", err)
	}
	tx.Discard()
	wantApplied(8)
}
