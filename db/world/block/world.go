package world_block

import (
	"context"
	"sync/atomic"

	trace "github.com/s4wave/spacewave/db/traceutil"

	"github.com/aperturerobotics/cayley"
	"github.com/aperturerobotics/cayley/graph"
	cayley_kv "github.com/aperturerobotics/cayley/graph/kv"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/s4wave/spacewave/db/block"
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
	le          *logrus.Entry
	btx         *block.Transaction
	bcs         *block.Cursor
	write       bool
	verbose     bool
	discarded   atomic.Bool
	readRelease func()

	// store is the block store the World writes through. The volume behind it
	// owns physical reachability.
	store block.StoreOps
	// ownedStore is the write buffer this state created, nil for forks that
	// share their parent's. It records the blocks it writes.
	ownedStore *block.BufferedStore
	// releaseOwned reports that ownedStore writes to the engine store, so
	// Discard releases the blocks it wrote that the final root does not reach.
	// A buffer over a staged session store leaves them to the publication,
	// since releasing through that store would publish it early.
	releaseOwned bool
	// keepRoots are written roots outside the World tree whose blocks Discard
	// keeps owned with the final root's. See KeepRoots.
	keepRoots []*block.BlockRef

	objTree   kvtx.BlockTx
	graphTree kvtx.BlockTx
	graphHd   *cayley.Handle

	storage  world.WorldStorage
	lookupOp world.LookupOp
	// localBucketID marks same-bucket object roots as local DAG edges.
	localBucketID string

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
// store is the block store the World writes through.
// if verbose is true, verbose logging of the graph key/value is enabled.
func NewWorldState(
	ctx context.Context,
	le *logrus.Entry,
	write bool,
	btx *block.Transaction,
	bcs *block.Cursor,
	store block.StoreOps,
	storage world.WorldStorage,
	lookupOp world.LookupOp,
	verbose bool,
) (*WorldState, error) {
	// Start the World-state trace.
	ctx, task := trace.NewTask(ctx, "hydra/world-block/world-state/new")
	defer task.End()

	// Build the World state and load it from the block transaction.
	_, subtask := trace.NewTask(ctx, "hydra/world-block/world-state/new/init-struct")
	tx := &WorldState{
		btx:     btx,
		bcs:     bcs,
		le:      le,
		write:   write,
		verbose: verbose,
		store:   store,

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
	// Pin the cursor's root for the life of the state.
	store := bls.GetBucket()
	btx, bcs := bls.BuildTransaction(nil)
	releaseRoot, err := block.PinRoot(ctx, store, bls.GetRef().GetRootRef())
	if err != nil {
		return nil, err
	}

	// Build the state, which releases the pin on Discard.
	state, err := NewWorldState(ctx, le, write, btx, bcs, store, storage, lookupOp, verbose)
	if err != nil {
		releaseRoot()
		return nil, err
	}
	state.readRelease = releaseRoot
	return state, nil
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
	fenced, err := t.store.Sync(ctx)
	if err == nil && fenced && t.write {
		err = block.MarkRootComplete(ctx, t.store, t.GetRootRef())
	}
	return fenced, err
}

// Flush writes the block writes made through this state to the backing store
// without its durability fence, then records the root's completion proof. The
// caller guarantees that a later durable commit makes both durable in order
// (see volume.WriteOrderer). States without a buffered store have nothing to
// write.
func (t *WorldState) Flush(ctx context.Context) error {
	// Write the buffered blocks.
	store, ok := t.store.(*block.BufferedStore)
	if !ok {
		return nil
	}
	if err := store.Flush(ctx); err != nil {
		return err
	}

	// Record the written root's completion proof.
	if t.write {
		return block.MarkRootComplete(ctx, t.store, t.GetRootRef())
	}
	return nil
}

// TakePending borrows the block writes buffered by this state, so an atomic
// publication can write them in place of Flush. The caller completes the batch
// with the publication's result; a failed batch returns to the buffer. A state
// without a write buffer returns an empty batch.
func (t *WorldState) TakePending(ctx context.Context) (*block.PendingBatch, error) {
	if t.ownedStore == nil {
		return &block.PendingBatch{}, nil
	}
	return t.ownedStore.TakePending(ctx)
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
	// Build a storage cursor and attach the transaction store.
	storage := t.storage
	if storage == nil {
		return nil, world.ErrWorldStorageUnavailable
	}
	cursor, err := storage.BuildStorageCursor(ctx)
	if err != nil {
		return nil, err
	}
	t.setCursorStore(ctx, cursor)
	return cursor, nil
}

// StageWorldState returns a stage over this write state's storage.
func (t *WorldState) StageWorldState(ctx context.Context) (world.WorldStage, error) {
	if !t.write {
		return nil, tx.ErrNotWrite
	}
	return world.NewTransactionStage(t), nil
}

// setCursorStore routes the cursor's blocks through the transaction store.
// A read-only state's cursor rejects writes.
func (t *WorldState) setCursorStore(ctx context.Context, cursor *bucket_lookup.Cursor) {
	if t.store != nil {
		cursor.SetTransactionStore(t.store)
	}
	if !t.write {
		setReadOnlyCursor(ctx, cursor)
	}
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
	return storage.AccessWorldState(ctx, ref, func(cursor *bucket_lookup.Cursor) error {
		t.setCursorStore(ctx, cursor)
		return cb(cursor)
	})
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
	// Reject an empty or discarded operation.
	if op == nil {
		return 0, false, world.ErrEmptyOp
	}
	if t.discarded.Load() {
		return 0, false, tx.ErrDiscarded
	}

	// Validate the world operation.
	if err := op.Validate(); err != nil {
		return 0, false, err
	}

	// Cancel the apply context when the operation returns.
	ctx, subCtxCancel := context.WithCancel(rctx)
	defer subCtxCancel()

	// Apply the world operation.
	sysErr, err := op.ApplyWorldOp(ctx, t.le, t, opSender)
	if err != nil {
		return 0, sysErr, err
	}

	// Read the sequence number after the operation.
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
	// A discarded state cannot fork.
	if t.discarded.Load() {
		return nil, tx.ErrDiscarded
	}

	// Detach a copy of the World root the fork owns.
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

	// Pin the root and build the fork on it.
	release, err := block.PinRoot(ctx, t.store, t.GetRootRef())
	if err != nil {
		return nil, err
	}
	ows, err := NewWorldState(
		ctx,
		t.le,
		t.write,
		bcs.GetTransaction(),
		bcs,
		t.store,
		t.storage,
		t.lookupOp,
		t.verbose,
	)
	if err != nil {
		release()
		return nil, err
	}
	ows.readRelease = release
	ows.localBucketID = t.localBucketID
	return ows, nil
}

// SetBlockTransaction loads the state from the given block transaction and cursor.
func (t *WorldState) SetBlockTransaction(ctx context.Context, btx *block.Transaction, bcs *block.Cursor) error {
	return t.setBlockTransaction(ctx, btx, bcs)
}

// setBlockTransaction rebuilds the world state onto btx and bcs.
func (t *WorldState) setBlockTransaction(
	ctx context.Context,
	btx *block.Transaction,
	bcs *block.Cursor,
) error {
	// Start the block-transaction trace.
	ctx, task := trace.NewTask(ctx, "hydra/world-block/world-state/set-block-transaction")
	defer task.End()

	// Unmarshal the World root.
	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/world-state/set-block-transaction/unmarshal-root")
	root, err := block.UnmarshalBlock[*World](taskCtx, bcs, NewWorldBlock)
	subtask.End()
	if err != nil {
		return err
	}

	// Build the object tree.
	taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/world-state/set-block-transaction/build-object-tree")
	objTree, err := t.buildObjectTree(taskCtx, bcs)
	subtask.End()
	if err != nil {
		return err
	}

	// Build the graph tree.
	taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/world-state/set-block-transaction/build-graph-tree")
	graphTree, graphHandle, err := t.buildGraphTree(taskCtx, bcs)
	subtask.End()
	if err != nil {
		return err
	}

	// Swap in the rebuilt trees and drop the object memo.
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
	t.objTree, t.graphTree, t.graphHd = objTree, graphTree, graphHandle

	// The rebuilt block state supersedes any transaction-local object memo.
	t.objectExistsMemo = nil
	subtask.End()

	// Update the sequence number from the new root.
	_, subtask = trace.NewTask(ctx, "hydra/world-block/world-state/set-block-transaction/update-seqno")
	t.updateSeqno(root)
	subtask.End()
	return nil
}

// KeepRoots keeps the blocks this state wrote that roots reach owned when
// Discard releases the blocks the final root does not reach. Use it for blocks
// that outlive the state outside the World tree, such as the payload of an
// operation that is replayed later.
func (t *WorldState) KeepRoots(roots ...*block.BlockRef) {
	t.keepRoots = append(t.keepRoots, roots...)
}

// Discard discards the resources in the WorldState.
func (t *WorldState) Discard() {
	// Discard only once.
	if t.discarded.Swap(true) {
		return
	}

	// Drop the trees and the unpublished staged writes.
	if t.objTree != nil {
		t.objTree.Discard()
	}
	if t.graphTree != nil {
		t.graphTree.Discard()
	}
	t.btx.DiscardStagedWrites()

	// Release the written blocks neither the final root nor a kept root reaches.
	if t.releaseOwned {
		roots := append([]*block.BlockRef{t.GetRootRef()}, t.keepRoots...)
		if err := t.ownedStore.ReleaseUnreached(context.Background(), roots...); err != nil {
			t.le.WithError(err).Warn("unable to release unreached world blocks")
		}
	}

	// Release the base root pin and wake sequence waiters.
	if t.readRelease != nil {
		t.readRelease()
		t.readRelease = nil
	}
	t.objectExistsMemo = nil
	t.seqnoBcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		broadcast()
	})
}

// Commit commits the current pending changes to the block cursor and updates
// the WorldState with the new root.
func (t *WorldState) Commit(ctx context.Context) error {
	// Trace the commit.
	ctx, task := trace.NewTask(ctx, "hydra/world-block/world-state/commit")
	defer task.End()

	// Commit needs a live write state. It does not discard the state, which
	// stays usable after Commit.
	if !t.write {
		return tx.ErrNotWrite
	}
	if t.discarded.Load() {
		return tx.ErrDiscarded
	}
	if err := ctx.Err(); err != nil {
		return context.Canceled
	}

	// Load the World root the pending changes apply to.
	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/world-state/commit/get-root")
	w, err := t.GetRoot(taskCtx)
	subtask.End()
	if err != nil {
		return err
	}

	// Record the pending changes in the World root.
	taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/world-state/commit/flush-world-changes")
	err = t.flushWorldChanges(taskCtx, w)
	subtask.End()
	if err != nil || t.btx == nil {
		return err
	}

	// Write the dirty blocks and release the tree data. Write batches the
	// volume's reference updates into one flush.
	taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/world-state/commit/block-write")
	_, bcs, err := t.btx.Write(taskCtx, true)
	subtask.End()
	if err != nil {
		return err
	}

	// Rebuild the state on the written root.
	taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/world-state/commit/set-block-transaction")
	err = t.setBlockTransaction(taskCtx, t.btx, bcs)
	subtask.End()
	return err
}

// buildObjectTree builds the object tree handle.
func (t *WorldState) buildObjectTree(ctx context.Context, bcs *block.Cursor) (kvtx.BlockTx, error) {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/world-state/build-object-tree")
	defer task.End()
	return kvtx_block.BuildKvTransaction(ctx, bcs.FollowSubBlock(1), true)
}

// buildGraphTree builds the graph tree (kv storage) handle.
func (t *WorldState) buildGraphTree(ctx context.Context, bcs *block.Cursor) (kvtx.BlockTx, *cayley.Handle, error) {
	// Start the graph-tree trace.
	ctx, task := trace.NewTask(ctx, "hydra/world-block/world-state/build-graph-tree")
	defer task.End()

	// Build the graph key-value transaction.
	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/world-state/build-graph-tree/build-kv-transaction")
	ktx, err := kvtx_block.BuildKvTransaction(taskCtx, bcs.FollowSubBlock(2), true)
	subtask.End()
	if err != nil {
		return nil, nil, err
	}

	// Wrap the graph transaction in a verbose logger when requested.
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
	_ world.WorldState         = (*WorldState)(nil)
	_ world.ForkableWorldState = (*WorldState)(nil)
)
