package world_block

import (
	"context"
	"sync/atomic"

	trace "github.com/s4wave/spacewave/db/traceutil"

	"github.com/aperturerobotics/cayley"
	"github.com/aperturerobotics/cayley/graph"
	cayley_kv "github.com/aperturerobotics/cayley/graph/kv"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	kvtx_cayley "github.com/s4wave/spacewave/db/kvtx/cayley"
	kvtx_vlogger "github.com/s4wave/spacewave/db/kvtx/vlogger"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
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

	objTree       kvtx.BlockTx
	graphTree     kvtx.BlockTx
	graphHd       *cayley.Handle
	gcTree        kvtx.BlockTx
	refGraph      *block_gc.RefGraph
	gcJournalTree kvtx.BlockTx
	gcJournal     *gcJournal

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
	ctx, task := trace.NewTask(ctx, "hydra/world-block/world-state/new")
	defer task.End()

	_, subtask := trace.NewTask(ctx, "hydra/world-block/world-state/new/init-struct")
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
	subtask.End()
	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/world-state/new/set-block-transaction")
	err := tx.SetBlockTransaction(taskCtx, btx, bcs)
	subtask.End()
	if err != nil {
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

// SetBufferedStoreSettings overrides the BufferedStore settings used by the
// underlying block Transaction during Commit. Pass nil to reset to defaults.
// No-op if the world state has no write transaction.
func (t *WorldState) SetBufferedStoreSettings(s *block.BufferedStoreSettings) {
	if t == nil || t.btx == nil {
		return
	}
	t.btx.SetBufferedStoreSettings(s)
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
	ctx, task := trace.NewTask(ctx, "hydra/world-block/world-state/set-block-transaction")
	defer task.End()

	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/world-state/set-block-transaction/unmarshal-root")
	root, err := block.UnmarshalBlock[*World](taskCtx, bcs, NewWorldBlock)
	subtask.End()
	if err != nil {
		return err
	}

	taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/world-state/set-block-transaction/build-object-tree")
	objTree, err := t.buildObjectTree(taskCtx, bcs)
	subtask.End()
	if err != nil {
		return err
	}

	taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/world-state/set-block-transaction/build-graph-tree")
	graphTree, graphHandle, err := t.buildGraphTree(taskCtx, bcs)
	subtask.End()
	if err != nil {
		return err
	}

	var refGraphIRIRefKeys map[string]any
	if t.refGraph != nil {
		refGraphIRIRefKeys = t.refGraph.CloneIRIRefKeys()
	}

	// Build GC ref graph for writable transactions with a store.
	var gcTree kvtx.BlockTx
	var refGraph *block_gc.RefGraph
	var initGCRootEdge bool
	if t.write && t.store != nil {
		taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/world-state/set-block-transaction/build-gc-tree")
		gcTree, refGraph, initGCRootEdge, err = t.buildGCTree(taskCtx, bcs)
		subtask.End()
		if err != nil {
			_ = graphHandle.Close()
			graphTree.Discard()
			objTree.Discard()
			return err
		}
		refGraph.ImportIRIRefKeys(refGraphIRIRefKeys)
	}

	// Build the deferred GC journal tree at sub-block 6.
	// Read-side uses it for Entries(); write-side also uses it as a WAL.
	var journalTree kvtx.BlockTx
	var journal *gcJournal
	if t.store != nil {
		taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/world-state/set-block-transaction/build-gc-journal")
		journalTree, err = kvtx_block.BuildKvTransaction(taskCtx, bcs.FollowSubBlock(gcJournalSubBlock), t.write)
		subtask.End()
		if err != nil {
			if refGraph != nil {
				_ = refGraph.Close()
			}
			if gcTree != nil {
				gcTree.Discard()
			}
			_ = graphHandle.Close()
			graphTree.Discard()
			objTree.Discard()
			return err
		}
		journal, err = newGCJournal(journalTree)
		if err != nil {
			journalTree.Discard()
			if refGraph != nil {
				_ = refGraph.Close()
			}
			if gcTree != nil {
				gcTree.Discard()
			}
			_ = graphHandle.Close()
			graphTree.Discard()
			objTree.Discard()
			return err
		}
		// Wrap the transaction's store with GCStoreOps using the journal as WAL (write path only).
		if t.write && btx != nil {
			gcOps := block_gc.NewGCStoreOpsWithTraceTask(t.store, refGraph, block_gc.WorldFlushTask())
			gcOps.SetWALAppender(journal)
			btx.SetStoreOps(gcOps)
		}
	}

	_, subtask = trace.NewTask(ctx, "hydra/world-block/world-state/set-block-transaction/swap-handles")
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
	if t.gcJournalTree != nil {
		t.gcJournalTree.Discard()
	}
	t.objTree, t.graphTree, t.graphHd = objTree, graphTree, graphHandle
	t.gcTree, t.refGraph = gcTree, refGraph
	t.gcJournalTree, t.gcJournal = journalTree, journal
	subtask.End()

	// Initialize the permanent gcroot -> world edge only when the
	// GC graph backing store is first created. Replaying this
	// idempotent Cayley write on every rebuild is expensive.
	if refGraph != nil && initGCRootEdge {
		taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/world-state/set-block-transaction/add-gc-root-ref")
		err := refGraph.AddRef(taskCtx, block_gc.NodeGCRoot, "world")
		subtask.End()
		if err != nil {
			return err
		}
	}

	_, subtask = trace.NewTask(ctx, "hydra/world-block/world-state/set-block-transaction/update-seqno")
	t.updateSeqno(root)
	subtask.End()
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
	if t.gcJournalTree != nil {
		t.gcJournalTree.Discard()
	}
	t.seqnoBcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		broadcast()
	})
}

// Commit commits the current pending changes to the block cursor.
// updates the WorldState with the new root
func (t *WorldState) Commit(ctx context.Context) error {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/world-state/commit")
	defer task.End()

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
	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/world-state/commit/get-root")
	w, err := t.GetRoot(taskCtx)
	subtask.End()
	if err != nil {
		return err
	}

	taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/world-state/commit/flush-world-changes")
	err = t.flushWorldChanges(taskCtx, w)
	subtask.End()
	if err != nil || t.btx == nil {
		return err
	}

	// Defer bucket-level GC flushes during the block write so they
	// accumulate and flush once at the end instead of per-PutBlock.
	t.store.BeginDeferFlush()

	var bcs *block.Cursor
	taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/world-state/commit/block-write")
	_, bcs, err = t.btx.Write(taskCtx, true)
	subtask.End()
	if err != nil {
		// End the deferred scope even on error to flush any partial work.
		if endErr := t.store.EndDeferFlush(ctx); endErr != nil {
			return errors.Wrap(endErr, err.Error())
		}
		return err
	}

	// End the deferred bucket-level flush scope: one batched flush.
	taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/world-state/commit/flush-gc-pending/bucket-batch")
	err = t.store.EndDeferFlush(taskCtx)
	subtask.End()
	if err != nil {
		return err
	}

	// Flush buffered world-level GC ref graph operations after Write
	// releases the cursor mutex. With the deferred journal wired,
	// FlushPending appends to the journal instead of mutating the
	// Cayley graph directly.
	if gcOps, ok := t.btx.GetStoreOps().(*block_gc.GCStoreOps); ok {
		taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/world-state/commit/flush-gc-pending")
		err := gcOps.FlushPending(taskCtx)
		subtask.End()
		if err != nil {
			return err
		}
	}
	// Reconcile the journal if it exceeds the threshold.
	if t.gcJournal != nil && t.gcJournal.Entries() >= gcJournalReconcileThreshold {
		taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/world-state/commit/reconcile-gc-journal")
		_, err := t.ReconcileGCJournal(taskCtx)
		subtask.End()
		if err != nil {
			return err
		}
	}
	taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/world-state/commit/set-block-transaction")
	err = t.SetBlockTransaction(taskCtx, t.btx, bcs)
	subtask.End()
	return err
}

// GetRefGraph returns the GC reference graph, or nil if not initialized.
func (t *WorldState) GetRefGraph() block_gc.RefGraphOps {
	if t.refGraph == nil {
		return nil
	}
	return t.refGraph
}

// GetGCJournalEntries returns the number of pending GC journal entries.
// Returns 0 if the journal is not initialized.
func (t *WorldState) GetGCJournalEntries() uint64 {
	if t.gcJournal == nil {
		return 0
	}
	return t.gcJournal.Entries()
}

// gcJournalReconcileThreshold is the journal entry count that triggers
// automatic reconciliation during Commit.
const gcJournalReconcileThreshold = 64

// GarbageCollect sweeps unreferenced nodes from the GC ref graph.
// Only valid on writable WorldState instances with GC enabled.
// Returns nil stats if GC is not enabled.
// Reconciles any pending GC journal entries before collecting.
func (t *WorldState) GarbageCollect(ctx context.Context) (*block_gc.Stats, error) {
	if t.refGraph == nil {
		return nil, nil
	}
	// Reconcile deferred journal before collecting.
	if _, err := t.ReconcileGCJournal(ctx); err != nil {
		return nil, errors.Wrap(err, "reconcile gc journal before collect")
	}
	c := block_gc.NewCollector(t.refGraph, t.store, t.onSwept)
	return c.Collect(ctx)
}

// ReconcileGCJournal applies pending GC journal entries to the Cayley ref graph
// and clears the journal. Call during idle periods or forced checkpoints. The
// caller must commit the world state afterward to persist the reconciled graph
// and cleared journal.
//
// Returns the number of journal entries applied, or 0 if the journal was empty
// or GC is not enabled.
func (t *WorldState) ReconcileGCJournal(ctx context.Context) (int, error) {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/world-state/reconcile-gc-journal")
	defer task.End()

	if t.refGraph == nil || t.gcJournal == nil {
		return 0, nil
	}

	// Collect all journal entries into one merged batch.
	var allAdds, allRemoves []block_gc.RefEdge
	count := 0
	err := t.gcJournal.Iterate(ctx, func(adds, removes []block_gc.RefEdge) error {
		allAdds = append(allAdds, adds...)
		allRemoves = append(allRemoves, removes...)
		count++
		return nil
	})
	if err != nil {
		return 0, errors.Wrap(err, "iterate gc journal")
	}
	if count == 0 {
		return 0, nil
	}

	// Apply the merged batch to the Cayley ref graph.
	if err := t.refGraph.ApplyRefBatch(ctx, allAdds, allRemoves); err != nil {
		return 0, errors.Wrap(err, "apply gc journal batch")
	}

	// Clear the journal.
	if err := t.gcJournal.Clear(ctx); err != nil {
		return 0, errors.Wrap(err, "clear gc journal")
	}
	return count, nil
}

// buildObjectTree builds the object tree handle.
func (t *WorldState) buildObjectTree(ctx context.Context, bcs *block.Cursor) (kvtx.BlockTx, error) {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/world-state/build-object-tree")
	defer task.End()
	return kvtx_block.BuildKvTransaction(ctx, bcs.FollowSubBlock(1), true)
}

// buildGCTree builds the GC reference graph tree and RefGraph handle.
// Returns whether the caller should initialize the gcroot -> world edge.
func (t *WorldState) buildGCTree(ctx context.Context, bcs *block.Cursor) (kvtx.BlockTx, *block_gc.RefGraph, bool, error) {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/world-state/build-gc-tree")
	defer task.End()

	gcTreeBcs := bcs.FollowSubBlock(5)
	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/world-state/build-gc-tree/load-kv-store")
	kvs, err := kvtx_block.LoadKeyValueStore(taskCtx, gcTreeBcs)
	subtask.End()
	if err != nil {
		return nil, nil, false, err
	}
	initGCRootEdge := kvs.GetIavlRoot() == nil

	taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/world-state/build-gc-tree/build-kv-transaction")
	ktx, err := kvs.BuildKvTransaction(taskCtx, gcTreeBcs, true)
	subtask.End()
	if err != nil {
		return nil, nil, false, err
	}
	taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/world-state/build-gc-tree/new-ref-graph")
	rg, err := block_gc.NewRefGraph(taskCtx, kvtx.NewTxStore(ktx), nil)
	subtask.End()
	if err != nil {
		ktx.Discard()
		return nil, nil, false, err
	}
	return ktx, rg, initGCRootEdge, nil
}

// buildGraphTree builds the graph tree (kv storage) handle.
func (t *WorldState) buildGraphTree(ctx context.Context, bcs *block.Cursor) (kvtx.BlockTx, *cayley.Handle, error) {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/world-state/build-graph-tree")
	defer task.End()

	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/world-state/build-graph-tree/build-kv-transaction")
	ktx, err := kvtx_block.BuildKvTransaction(taskCtx, bcs.FollowSubBlock(2), true)
	subtask.End()
	if err != nil {
		return nil, nil, err
	}

	if t.verbose {
		ktx = kvtx_vlogger.NewBlockTx(t.le, ktx)
	}

	// makes frequent NewTx() Get() Discard() calls
	// back it all w/ a single transaction
	graphOpts := make(graph.Options, 1)
	// disable custom indexes: use the default set
	// reduces the number of Get calls to zero
	graphOpts[cayley_kv.OptAssumeDefaultIdx] = true
	// NOTE: the ctx is used here for internal hidalgo k/v transactions!
	// it must not be canceled while WorldState is in use!
	taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/world-state/build-graph-tree/new-graph-handle")
	graphHd, err := kvtx_cayley.NewGraph(taskCtx, kvtx.NewTxStore(ktx), graphOpts)
	subtask.End()
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
