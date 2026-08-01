package world_block

import (
	"context"
	"slices"
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
	kvtx_block_okra "github.com/s4wave/spacewave/db/kvtx/block/okra"
	kvtx_cayley "github.com/s4wave/spacewave/db/kvtx/cayley"
	kvtx_vlogger "github.com/s4wave/spacewave/db/kvtx/vlogger"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// objectKeyPrefix is the prefix used for object keys in storage
var objectKeyPrefix = "o/"

const (
	defaultGCJournalReconcileEntryLimit uint64 = 64
	defaultGCJournalReconcileEdgeLimit  uint64 = 4096
)

type worldRefGraph interface {
	block_gc.RefGraphOps
}

// WorldState implements world state backed by a block graph.
// Note: GetRoot, WaitSeqno are concurrency safe.
// Note: all other calls are not concurrency safe. Use Tx if you want a mutex.
type WorldState struct {
	le          *logrus.Entry
	btx         *block.Transaction
	bcs         *block.Cursor
	write       bool
	verbose     bool
	discarded   atomic.Bool
	readRelease func()

	// store is the raw block store (unwrapped).
	store block.StoreOps
	// xfrm is the block transformer.
	xfrm block.Transformer
	// onSwept is called for each node swept during GC (optional).
	onSwept func(context.Context, string) error

	objTree        kvtx.BlockTx
	graphTree      kvtx.BlockTx
	graphHd        *cayley.Handle
	gcTree         kvtx.BlockTx
	gcTreeIsolated bool
	refGraph       worldRefGraph
	gcJournalTree  kvtx.BlockTx
	gcJournal      *gcJournal
	gcJournalDirty bool

	storage  world.WorldStorage
	lookupOp world.LookupOp

	// objectExistsMemo remembers object keys known to exist during the current
	// transaction so repeated HasObject calls skip redundant object-tree reads.
	// Reset whenever the transaction rebuilds its block state
	// (SetBlockTransaction, Discard). Not guarded by its own lock: the world
	// state is single-threaded per its contract and callers serialize through
	// Tx.
	objectExistsMemo map[string]struct{}

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

// Sync fences the block writes made through this state durable.
// A bare world state fences only its block store; the durable head is advanced
// by the owning Engine (see Engine.Sync). Returns (true, nil) when there is no
// store to fence (read-only or coordinator-backed states).
func (t *WorldState) Sync(ctx context.Context) (bool, error) {
	if t.store == nil {
		return true, nil
	}
	return t.store.Sync(ctx)
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
	return t.setBlockTransaction(ctx, btx, bcs)
}

// setBlockTransaction rebuilds the world state onto btx and bcs.
func (t *WorldState) setBlockTransaction(
	ctx context.Context,
	btx *block.Transaction,
	bcs *block.Cursor,
) error {
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

	// Build GC ref graph for writable transactions with a store.
	var gcTree kvtx.BlockTx
	var refGraph *block_gc.RefGraph
	var initGCRootEdge bool
	var gcTreeIsolated bool
	if t.write && t.store != nil {
		taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/world-state/set-block-transaction/build-gc-tree")
		gcTree, refGraph, initGCRootEdge, gcTreeIsolated, err = t.buildGCTree(taskCtx, bcs)
		subtask.End()
		if err != nil {
			_ = graphHandle.Close()
			graphTree.Discard()
			objTree.Discard()
			return err
		}
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
		journal, err = newGCJournal(ctx, journalTree, t.write)
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
	var activeRefGraph worldRefGraph
	if refGraph != nil {
		activeRefGraph = refGraph
	}
	t.objTree, t.graphTree, t.graphHd = objTree, graphTree, graphHandle
	t.gcTree, t.gcTreeIsolated, t.refGraph = gcTree, gcTreeIsolated, activeRefGraph
	t.gcJournalTree, t.gcJournal = journalTree, journal
	// The rebuilt block state supersedes any transaction-local object memo.
	t.objectExistsMemo = nil
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
	if t.readRelease != nil {
		t.readRelease()
		t.readRelease = nil
	}
	t.objectExistsMemo = nil
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
	block.BeginDeferFlush(t.store)

	journalEntriesBefore := t.GetGCJournalEntries()
	var bcs *block.Cursor
	if t.gcTreeIsolated && t.gcTree != nil && t.gcTree.GetCursor().IsDirty() {
		taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/world-state/commit/commit-isolated-gc-tree")
		err = t.gcTree.Commit(taskCtx)
		subtask.End()
		if err != nil {
			if endErr := block.EndDeferFlush(ctx, t.store); endErr != nil {
				return errors.Wrap(endErr, err.Error())
			}
			return err
		}
	}
	taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/world-state/commit/block-write")
	_, bcs, err = t.btx.Write(taskCtx, false)
	subtask.End()
	if err != nil {
		// End the deferred scope even on error to flush any partial work.
		if endErr := block.EndDeferFlush(ctx, t.store); endErr != nil {
			return errors.Wrap(endErr, err.Error())
		}
		return err
	}

	// End the deferred bucket-level flush scope: one batched flush.
	taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/world-state/commit/flush-gc-pending/bucket-batch")
	err = block.EndDeferFlush(taskCtx, t.store)
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
	journalTreeDirty := false
	if t.gcJournalTree != nil && t.gcJournalTree.GetCursor() != nil {
		journalTreeDirty = t.gcJournalTree.GetCursor().IsDirty()
	}
	journalChanged := t.gcJournalTree != nil && (t.GetGCJournalEntries() != journalEntriesBefore || journalTreeDirty || t.gcJournalDirty)
	if journalChanged {
		// The world GC flush appends to the journal after the primary write.
		// Persist that journal update through the inner store so it does not
		// recursively append another WAL entry.
		prevStore := t.btx.GetStoreOps()
		t.btx.SetStoreOps(t.store)
		taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/world-state/commit/persist-gc-journal")
		err = t.gcJournalTree.Commit(taskCtx)
		if err == nil {
			_, bcs, err = t.btx.Write(taskCtx, true)
		}
		subtask.End()
		t.btx.SetStoreOps(prevStore)
		if err != nil {
			return err
		}
		t.gcJournalDirty = false
	}
	if !journalChanged {
		taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/world-state/commit/clear-block-tree")
		_, bcs, err = t.btx.Write(taskCtx, true)
		subtask.End()
		if err != nil {
			return err
		}
	}
	taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/world-state/commit/set-block-transaction")
	err = t.setBlockTransaction(taskCtx, t.btx, bcs)
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

// GarbageCollect sweeps unreferenced nodes from the GC ref graph.
// Only valid on writable WorldState instances with GC enabled.
// Returns nil stats if GC is not enabled.
// Reconciles any pending GC journal entries before collecting.
func (t *WorldState) GarbageCollect(ctx context.Context) (*block_gc.Stats, error) {
	if t.refGraph == nil {
		return nil, nil
	}
	release, err := t.pinCurrentWorldRootForGC(ctx)
	if err != nil {
		return nil, err
	}
	if release != nil {
		defer release()
	}
	// Reconcile one bounded deferred-journal chunk before collecting.
	reconcile, err := t.reconcileGCJournal(ctx, defaultGCJournalReconcileEntryLimit, defaultGCJournalReconcileEdgeLimit)
	if err != nil {
		return nil, errors.Wrap(err, "reconcile gc journal before collect")
	}
	if reconcile.remainingEntries != 0 {
		trace.Logf(
			ctx,
			"gc-journal",
			"defer collect: remaining_entries=%d applied_entries=%d applied_edges=%d",
			reconcile.remainingEntries,
			reconcile.appliedEntries,
			reconcile.appliedEdges,
		)
		return &block_gc.Stats{}, nil
	}
	if err := t.removeCurrentWorldRootUnreferenced(ctx); err != nil {
		return nil, errors.Wrap(err, "mark pinned world root for gc")
	}
	// A World ref graph covers one retained root, while the physical store is
	// shared by engine snapshots, transactions, forks, and coordinated heads.
	// Only a store-scoped reachability owner may authorize physical deletion.
	c := block_gc.NewCollector(t.refGraph, t.store, t.onSwept)
	return c.CollectGraphOnly(ctx)
}

func (t *WorldState) pinCurrentWorldRootForGC(ctx context.Context) (func(), error) {
	if t.refGraph == nil || t.bcs == nil {
		return nil, nil
	}
	rootRef := t.bcs.GetRef()
	if rootRef == nil || rootRef.GetEmpty() {
		return nil, nil
	}
	rootIRI := block_gc.BlockIRI(rootRef)
	if rootIRI == "" {
		return nil, nil
	}
	if err := t.refGraph.AddRef(ctx, "world", rootIRI); err != nil {
		return nil, errors.Wrap(err, "pin world root for gc")
	}
	return func() {
		_ = t.refGraph.RemoveRef(context.Background(), "world", rootIRI)
	}, nil
}

func (t *WorldState) removeCurrentWorldRootUnreferenced(ctx context.Context) error {
	if t.refGraph == nil || t.bcs == nil {
		return nil
	}
	rootRef := t.bcs.GetRef()
	if rootRef == nil || rootRef.GetEmpty() {
		return nil
	}
	rootIRI := block_gc.BlockIRI(rootRef)
	if rootIRI == "" {
		return nil
	}
	return t.refGraph.RemoveRef(ctx, block_gc.NodeUnreferenced, rootIRI)
}

// ReconcileGCJournal applies one bounded pending GC journal chunk to the Cayley
// ref graph. Call during idle periods or forced checkpoints. The caller must
// commit the world state afterward to persist the reconciled graph and journal
// cursor.
//
// Returns the number of journal entries applied, or 0 if the journal was empty
// or GC is not enabled.
func (t *WorldState) ReconcileGCJournal(ctx context.Context) (int, error) {
	result, err := t.reconcileGCJournal(ctx, defaultGCJournalReconcileEntryLimit, defaultGCJournalReconcileEdgeLimit)
	return result.appliedEntries, err
}

func (t *WorldState) reconcileGCJournal(ctx context.Context, maxEntries, maxEdges uint64) (gcJournalReconcileResult, error) {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/world-state/reconcile-gc-journal")
	defer task.End()

	if t.refGraph == nil || t.gcJournal == nil {
		return gcJournalReconcileResult{}, nil
	}

	var allAdds, allRemoves []block_gc.RefEdge
	entries, err := t.gcJournal.Take(ctx, maxEntries, maxEdges)
	if err != nil {
		return gcJournalReconcileResult{}, errors.Wrap(err, "iterate gc journal")
	}
	if len(entries) == 0 {
		return gcJournalReconcileResult{}, nil
	}
	for _, entry := range entries {
		allAdds = append(allAdds, entry.adds...)
		allRemoves = append(allRemoves, entry.removes...)
	}

	if err := t.refGraph.ApplyRefBatch(ctx, allAdds, allRemoves); err != nil {
		return gcJournalReconcileResult{}, errors.Wrap(err, "apply gc journal batch")
	}
	if err := t.gcJournal.DeleteApplied(ctx, entries); err != nil {
		return gcJournalReconcileResult{}, errors.Wrap(err, "delete applied gc journal entries")
	}
	t.gcJournalDirty = true
	result := gcJournalReconcileResult{
		appliedEntries:   len(entries),
		appliedEdges:     len(allAdds) + len(allRemoves),
		remainingEntries: t.gcJournal.Entries(),
	}
	trace.Logf(
		ctx,
		"gc-journal",
		"applied_entries=%d applied_edges=%d remaining_entries=%d",
		result.appliedEntries,
		result.appliedEdges,
		result.remainingEntries,
	)
	return result, nil
}

type gcJournalReconcileResult struct {
	appliedEntries   int
	appliedEdges     int
	remainingEntries uint64
}

// buildObjectTree builds the object tree handle.
func (t *WorldState) buildObjectTree(ctx context.Context, bcs *block.Cursor) (kvtx.BlockTx, error) {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/world-state/build-object-tree")
	defer task.End()
	return kvtx_block.BuildKvTransaction(ctx, bcs.FollowSubBlock(1), true)
}

// buildGCTree builds the GC reference graph tree and RefGraph handle.
// Returns whether the caller should initialize the gcroot -> world edge and
// whether the tree commits through an isolated block transaction.
func (t *WorldState) buildGCTree(
	ctx context.Context,
	bcs *block.Cursor,
) (kvtx.BlockTx, *block_gc.RefGraph, bool, bool, error) {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/world-state/build-gc-tree")
	defer task.End()

	gcTreeBcs := bcs.FollowSubBlock(5)
	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/world-state/build-gc-tree/load-kv-store")
	kvs, err := kvtx_block.LoadKeyValueStore(taskCtx, gcTreeBcs)
	subtask.End()
	if err != nil {
		return nil, nil, false, false, err
	}

	taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/world-state/build-gc-tree/build-kv-transaction")
	ktx, isolated, err := t.buildGCKvTransaction(taskCtx, kvs, gcTreeBcs)
	subtask.End()
	if err != nil {
		return nil, nil, false, false, err
	}
	taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/world-state/build-gc-tree/size")
	gcTreeSize, err := ktx.Size(taskCtx)
	subtask.End()
	if err != nil {
		ktx.Discard()
		return nil, nil, false, false, err
	}
	initGCRootEdge := gcTreeSize == 0
	if initGCRootEdge && t.refGraph != nil {
		outgoing, err := t.refGraph.GetOutgoingRefs(ctx, block_gc.NodeGCRoot)
		if err != nil {
			ktx.Discard()
			return nil, nil, false, false, err
		}
		if slices.Contains(outgoing, "world") {
			initGCRootEdge = false
		}
	}

	taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/world-state/build-gc-tree/new-ref-graph")
	rg, err := block_gc.NewRefGraph(taskCtx, kvtx.NewTxStore(ktx), nil)
	subtask.End()
	if err != nil {
		ktx.Discard()
		return nil, nil, false, false, err
	}
	return ktx, rg, initGCRootEdge, isolated, nil
}

func (t *WorldState) buildGCKvTransaction(
	ctx context.Context,
	kvs *kvtx_block.KeyValueStore,
	gcTreeBcs *block.Cursor,
) (kvtx.BlockTx, bool, error) {
	if kvs.GetImplType() != kvtx_block.KVImplType_KV_IMPL_TYPE_OKRA || t.btx == nil || t.store == nil {
		ktx, err := kvs.BuildKvTransaction(ctx, gcTreeBcs, true)
		return ktx, false, err
	}

	isolatedTx, isolatedRoot := block.NewTransaction(t.store, t.btx.GetTransformer(), nil, t.btx.GetPutOpts())
	isolatedKVS := kvs.CloneVT()
	isolatedRoot.SetBlock(isolatedKVS, false)
	treeBcs := isolatedRoot.FollowSubBlock(3)
	ktx, err := kvtx_block_okra.NewTx(ctx, treeBcs, isolatedTx, true, func(ncs *block.Cursor) {
		_ = ncs.SetAsSubBlock(3, isolatedRoot)
		gcTreeBcs.SetBlock(isolatedKVS.CloneVT(), true)
	})
	if err != nil {
		return nil, false, err
	}
	return ktx, true, nil
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
