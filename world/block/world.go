package world_block

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/cayley"
	"github.com/aperturerobotics/cayley/graph"
	cayley_kv "github.com/aperturerobotics/cayley/graph/kv"
	"github.com/aperturerobotics/hydra/block"
	block_gc "github.com/aperturerobotics/hydra/block/gc"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/kvtx"
	kvtx_block "github.com/aperturerobotics/hydra/kvtx/block"
	kvtx_cayley "github.com/aperturerobotics/hydra/kvtx/cayley"
	kvtx_vlogger "github.com/aperturerobotics/hydra/kvtx/vlogger"
	"github.com/aperturerobotics/hydra/tx"
	"github.com/aperturerobotics/hydra/world"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/sirupsen/logrus"
)

// objectKeyPrefix is the prefix used for object keys in storage
var objectKeyPrefix = "o/"

// WorldState implements world state backed by a block graph.
// Note: GetRoot, WaitSeqno are concurrency safe.
// Note: all other calls are not concurrency safe. Use Tx if you want a mutex.
type WorldState struct {
	le        *logrus.Entry
	btx       *block.Transaction
	bcs       *block.Cursor
	write     bool
	verbose   bool
	discarded atomic.Bool

	// store is the raw block store (unwrapped).
	store block.StoreOps
	// xfrm is the block transformer.
	xfrm block.Transformer
	// onSwept is called for each node swept during GC (optional).
	onSwept func(context.Context, string) error

	objTree   kvtx.BlockTx
	graphTree kvtx.BlockTx
	graphHd   *cayley.Handle
	gcTree    kvtx.BlockTx
	refGraph  *block_gc.RefGraph

	storage  world.WorldStorage
	lookupOp world.LookupOp

	pendingChanges []*block.Cursor // *WorldChange

	// seqnoBcast guards below fields
	seqnoBcast broadcast.Broadcast
	// seqno is the current sequence number of the world state
	seqno uint64
}

// NewWorldState constructs a new world handle.
// btx can be nil to not write during Commit()
// bcs is located at the root of the world (the World block).
// if bcs is empty, creates a new empty world.
// store is the raw block store (for GC wrapping).
// xfrm is the block transformer (may be nil).
// onSwept is called per swept node during GC (may be nil).
// if verbose is true, verbose logging of the graph key/value is enabled.
func NewWorldState(
	ctx context.Context,
	le *logrus.Entry,
	write bool,
	btx *block.Transaction,
	bcs *block.Cursor,
	store block.StoreOps,
	xfrm block.Transformer,
	onSwept func(context.Context, string) error,
	storage world.WorldStorage,
	lookupOp world.LookupOp,
	verbose bool,
) (*WorldState, error) {
	tx := &WorldState{
		btx:     btx,
		bcs:     bcs,
		le:      le,
		write:   write,
		verbose: verbose,

		store:   store,
		xfrm:    xfrm,
		onSwept: onSwept,

		storage:  storage,
		lookupOp: lookupOp,
	}
	if err := tx.SetBlockTransaction(ctx, btx, bcs); err != nil {
		return nil, err
	}
	return tx, nil
}

// BuildWorldStateFromCursor builds a world state from a bucket lookup cursor.
func BuildWorldStateFromCursor(
	ctx context.Context,
	le *logrus.Entry,
	write bool,
	bls *bucket_lookup.Cursor,
	storage world.WorldStorage,
	lookupOp world.LookupOp,
	verbose bool,
) (*WorldState, error) {
	store := bls.GetBucket()
	xfrm := bls.GetTransformer()
	btx, bcs := bls.BuildTransaction(nil)
	return NewWorldState(ctx, le, write, btx, bcs, store, xfrm, nil, storage, lookupOp, verbose)
}

// GetReadOnly returns if the world handle is read-only.
func (t *WorldState) GetReadOnly() bool {
	return !t.write
}

// GetRootRef returns the current root reference.
func (t *WorldState) GetRootRef() *block.BlockRef {
	return t.bcs.GetRef()
}

// GetBcs returns the root block cursor.
func (t *WorldState) GetBcs() *block.Cursor {
	return t.bcs
}

// GetRoot builds the Root object from the block cursor.
//
// Concurrency safe.
func (t *WorldState) GetRoot(ctx context.Context) (*World, error) {
	// bcs uses mutexes internally so this is concurrency safe.
	return UnmarshalWorld(ctx, t.bcs)
}

// GetSeqno returns the current seqno of the world state.
// This is also the sequence number of the most recent change.
// Initializes at 0 for initial world state.
func (t *WorldState) GetSeqno(ctx context.Context) (uint64, error) {
	var currSeqno uint64
	t.seqnoBcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		currSeqno = t.seqno
	})
	return currSeqno, nil
}

// WaitSeqno waits for the seqno of the world state to be >= value.
// Returns the seqno when the condition is reached.
// If value == 0, this might return immediately unconditionally.
func (t *WorldState) WaitSeqno(ctx context.Context, value uint64) (uint64, error) {
	for {
		var waitCh <-chan struct{}
		var err error
		var seqno uint64
		t.seqnoBcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			if t.discarded.Load() {
				err = tx.ErrDiscarded
				return
			}

			seqno = t.seqno
			if seqno >= value {
				return
			}

			waitCh = getWaitCh()
		})
		if err != nil {
			return 0, err
		}
		if waitCh == nil {
			return seqno, nil
		}

		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-waitCh:
		}
	}
}

// BuildStorageCursor builds a cursor to the world storage with an empty ref.
// The cursor should be released independently of the WorldState.
// Be sure to call Release on the cursor when done.
func (t *WorldState) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	storage := t.storage
	if storage == nil {
		return nil, world.ErrWorldStorageUnavailable
	}
	return storage.BuildStorageCursor(ctx)
}

// AccessWorldState builds a bucket lookup cursor with an optional ref.
// If the ref is empty, returns empty cursor in the same bucket + volume as the world.
// The lookup cursor will be released after cb returns.
func (t *WorldState) AccessWorldState(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	storage := t.storage
	if storage == nil {
		return world.ErrWorldStorageUnavailable
	}
	return storage.AccessWorldState(ctx, ref, cb)
}

// ApplyWorldOp applies a batch operation at the world level.
// The handling of the operation is operation-type specific.
// Returns the seqno following the operation execution.
// If nil is returned for the error, implies success.
func (t *WorldState) ApplyWorldOp(
	rctx context.Context,
	op world.Operation,
	opSender peer.ID,
) (uint64, bool, error) {
	if op == nil {
		return 0, false, world.ErrEmptyOp
	}
	if t.discarded.Load() {
		return 0, false, tx.ErrDiscarded
	}

	if err := op.Validate(); err != nil {
		return 0, false, err
	}

	ctx, subCtxCancel := context.WithCancel(rctx)
	defer subCtxCancel()

	sysErr, err := op.ApplyWorldOp(ctx, t.le, t, opSender)
	if err != nil {
		return 0, sysErr, err
	}

	seq, err := t.GetSeqno(ctx)
	if err != nil {
		return 0, true, err
	}
	return seq, false, nil
}

// Fork forks the current world state into a completely separate world state.
//
// Creates a new block transaction.
func (t *WorldState) Fork(ctx context.Context) (world.WorldState, error) {
	if t.discarded.Load() {
		return nil, tx.ErrDiscarded
	}

	bcs := t.bcs.DetachTransaction()
	blk, _ := bcs.GetBlock()
	var blkv *World
	if blk != nil {
		var ok bool
		blkv, ok = blk.(*World)
		if !ok {
			return nil, block.ErrUnexpectedType
		}
	}
	if blkv != nil {
		blkv = blkv.CloneVT()
		bcs.SetBlock(blkv, false)
	} else {
		blkv = &World{}
		bcs.SetBlock(blkv, true)
	}
	ows, err := NewWorldState(
		ctx,
		t.le,
		t.write,
		bcs.GetTransaction(),
		bcs,
		t.store,
		t.xfrm,
		t.onSwept,
		t.storage,
		t.lookupOp,
		t.verbose,
	)
	if err != nil {
		return nil, err
	}
	return ows, nil
}

// SetBlockTransaction loads the state from the given block transaction and cursor.
//
// The block transaction store is overridden with one wrapped with the GC store ops.
func (t *WorldState) SetBlockTransaction(ctx context.Context, btx *block.Transaction, bcs *block.Cursor) error {
	root, err := block.UnmarshalBlock[*World](ctx, bcs, NewWorldBlock)
	if err != nil {
		return err
	}
	objTree, err := t.buildObjectTree(ctx, bcs)
	if err != nil {
		return err
	}
	graphTree, graphHandle, err := t.buildGraphTree(ctx, bcs)
	if err != nil {
		return err
	}

	// Build GC ref graph for writable transactions with a store.
	var gcTree kvtx.BlockTx
	var refGraph *block_gc.RefGraph
	if t.write && t.store != nil {
		gcTree, refGraph, err = t.buildGCTree(ctx, bcs)
		if err != nil {
			_ = graphHandle.Close()
			graphTree.Discard()
			objTree.Discard()
			return err
		}
		// Wrap the transaction's store with GCStoreOps.
		if btx != nil {
			gcOps := block_gc.NewGCStoreOps(t.store, refGraph)
			btx.SetStoreOps(gcOps)
		}
	}

	t.btx, t.bcs = btx, bcs
	if t.graphHd != nil {
		_ = t.graphHd.Close()
	}
	if t.graphTree != nil {
		t.graphTree.Discard()
	}
	if t.objTree != nil {
		t.objTree.Discard()
	}
	if t.refGraph != nil {
		_ = t.refGraph.Close()
	}
	if t.gcTree != nil {
		t.gcTree.Discard()
	}
	t.objTree, t.graphTree, t.graphHd = objTree, graphTree, graphHandle
	t.gcTree, t.refGraph = gcTree, refGraph

	// Ensure gcroot -> world edge exists (idempotent).
	if refGraph != nil {
		if err := refGraph.AddRef(ctx, block_gc.NodeGCRoot, "world"); err != nil {
			return err
		}
	}

	t.updateSeqno(root)
	return nil
}

// Discard discards the resources in the WorldState.
func (t *WorldState) Discard() {
	if t.discarded.Swap(true) {
		return
	}
	if t.objTree != nil {
		t.objTree.Discard()
	}
	if t.graphTree != nil {
		t.graphTree.Discard()
	}
	if t.refGraph != nil {
		_ = t.refGraph.Close()
	}
	if t.gcTree != nil {
		t.gcTree.Discard()
	}
	t.seqnoBcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		broadcast()
	})
}

// Commit commits the current pending changes to the block cursor.
// updates the WorldState with the new root
func (t *WorldState) Commit(ctx context.Context) error {
	if !t.write {
		return tx.ErrNotWrite
	}
	// Note: we do NOT discard after commit in WorldState.
	// We can re-use the state immediately after Commit.
	if t.discarded.Load() {
		return tx.ErrDiscarded
	}
	if err := ctx.Err(); err != nil {
		return context.Canceled
	}
	w, err := t.GetRoot(ctx)
	if err != nil {
		return err
	}
	err = t.flushWorldChanges(ctx, w)
	if err != nil || t.btx == nil {
		return err
	}
	_, bcs, err := t.btx.Write(ctx, true)
	if err != nil {
		return err
	}
	// Flush buffered GC ref graph operations after Write releases the cursor mutex.
	if gcOps, ok := t.btx.GetStoreOps().(*block_gc.GCStoreOps); ok {
		if err := gcOps.FlushPending(ctx); err != nil {
			return err
		}
	}
	return t.SetBlockTransaction(ctx, t.btx, bcs)
}

// GetRefGraph returns the GC reference graph, or nil if not initialized.
func (t *WorldState) GetRefGraph() *block_gc.RefGraph {
	return t.refGraph
}

// GarbageCollect sweeps unreferenced nodes from the GC ref graph.
// Only valid on writable WorldState instances with GC enabled.
// Returns nil stats if GC is not enabled.
func (t *WorldState) GarbageCollect(ctx context.Context) (*block_gc.Stats, error) {
	if t.refGraph == nil {
		return nil, nil
	}
	c := block_gc.NewCollector(t.refGraph, t.store, t.onSwept)
	return c.Collect(ctx)
}

// buildObjectTree builds the object tree handle.
func (t *WorldState) buildObjectTree(ctx context.Context, bcs *block.Cursor) (kvtx.BlockTx, error) {
	return kvtx_block.BuildKvTransaction(ctx, bcs.FollowSubBlock(1), true)
}

// buildGCTree builds the GC reference graph tree and RefGraph handle.
func (t *WorldState) buildGCTree(ctx context.Context, bcs *block.Cursor) (kvtx.BlockTx, *block_gc.RefGraph, error) {
	ktx, err := kvtx_block.BuildKvTransaction(ctx, bcs.FollowSubBlock(5), true)
	if err != nil {
		return nil, nil, err
	}
	rg, err := block_gc.NewRefGraph(ctx, kvtx.NewTxStore(ktx), nil)
	if err != nil {
		ktx.Discard()
		return nil, nil, err
	}
	return ktx, rg, nil
}

// buildGraphTree builds the graph tree (kv storage) handle.
func (t *WorldState) buildGraphTree(ctx context.Context, bcs *block.Cursor) (kvtx.BlockTx, *cayley.Handle, error) {
	ktx, err := kvtx_block.BuildKvTransaction(ctx, bcs.FollowSubBlock(2), true)
	if err != nil {
		return nil, nil, err
	}

	if t.verbose {
		ktx = kvtx_vlogger.NewBlockTx(t.le, ktx)
	}

	// makes frequent NewTx() Get() Discard() calls
	// back it all w/ a single transaction
	graphOpts := make(graph.Options, 1)
	// disable bloom filter: very slow to allocate during tx processing
	graphOpts[cayley_kv.OptBloom] = false
	// disable custom indexes: use the default set
	// reduces the number of Get calls to zero
	graphOpts[cayley_kv.OptAssumeDefaultIdx] = true
	// NOTE: the ctx is used here for internal hidalgo k/v transactions!
	// it must not be canceled while WorldState is in use!
	graphHd, err := kvtx_cayley.NewGraph(ctx, kvtx.NewTxStore(ktx), graphOpts)
	if err != nil {
		ktx.Discard()
		return nil, nil, err
	}

	return ktx, graphHd, nil
}

// _ is a type assertion
var (
	_ world.WorldState         = ((*WorldState)(nil))
	_ world.ForkableWorldState = ((*WorldState)(nil))
)
