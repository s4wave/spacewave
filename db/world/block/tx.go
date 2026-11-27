package world_block

import (
	"context"
	"slices"

	"github.com/aperturerobotics/util/csync"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
)

// Tx implements the World transaction interface. A mutex makes its calls
// concurrency safe.
type Tx struct {
	// state is the underlying root state object.
	state *WorldState
	// rmtx guards the world operations, single-writer multi-reader.
	// Not used for WaitSeqno.
	rmtx csync.RWMutex
}

// NewTx constructs a new transaction from a world state.
// Guards the calls with a RWMutex (concurrency safe).
// Prevents operations after the Tx was discarded or committed.
func NewTx(state *WorldState) *Tx {
	return &Tx{state: state}
}

// Fork forks the current tx into a completely separate tx.
//
// Creates a new block transaction.
func (t *Tx) Fork(ctx context.Context) (world.WorldState, error) {
	return lockedValue(ctx, t, false, func() (world.WorldState, error) {
		forkedState, err := t.state.Fork(ctx)
		if err != nil {
			return nil, err
		}
		return NewTx(forkedState.(*WorldState)), nil
	})
}

// GetReadOnly returns if the tx is read-only.
func (t *Tx) GetReadOnly() bool {
	return t.state.GetReadOnly()
}

// Sync fences the block writes made through this tx's world state durable.
func (t *Tx) Sync(ctx context.Context) (bool, error) {
	return t.state.Sync(ctx)
}

// Flush writes the block writes made through this tx's world state without a
// durability fence. See WorldState.Flush.
func (t *Tx) Flush(ctx context.Context) error {
	return t.state.Flush(ctx)
}

// TakePending borrows the block writes buffered by this tx's world state, so
// an atomic publication can write them. See WorldState.TakePending.
func (t *Tx) TakePending(ctx context.Context) (*block.PendingBatch, error) {
	return t.state.TakePending(ctx)
}

// GetSeqno returns the current seqno of the world state.
// This is also the sequence number of the most recent change.
// Initializes at 0 for initial world state.
func (t *Tx) GetSeqno(ctx context.Context) (uint64, error) {
	return lockedValue(ctx, t, false, func() (uint64, error) {
		return t.state.GetSeqno(ctx)
	})
}

// GetObjectBodiesBatchPage reads one page of object bodies and the
// transaction seqno under one read lock.
func (t *Tx) GetObjectBodiesBatchPage(
	ctx context.Context,
	keys []string,
	byteBudget int,
) (bodies []*world.ObjectBody, consumed uint32, seqno uint64, err error) {
	err = t.locked(ctx, false, func() error {
		bodies, consumed, seqno, err = t.state.GetObjectBodiesBatchPage(ctx, keys, byteBudget)
		return err
	})
	return bodies, consumed, seqno, err
}

// WaitSeqno waits for the seqno of the world state to be >= value.
// Returns the seqno when the condition is reached.
// If value == 0, this might return immediately unconditionally.
func (t *Tx) WaitSeqno(ctx context.Context, value uint64) (uint64, error) {
	return t.state.WaitSeqno(ctx, value)
}

// BuildStorageCursor builds a cursor to the world storage with an empty ref.
// The cursor should be released independently of the WorldState.
// Be sure to call Release on the cursor when done.
func (t *Tx) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	return t.state.BuildStorageCursor(ctx)
}

// StageWorldState returns a stage over the transaction's storage.
func (t *Tx) StageWorldState(ctx context.Context) (world.WorldStage, error) {
	return t.state.StageWorldState(ctx)
}

// AccessWorldState builds a bucket lookup cursor with an optional ref.
// If the ref is empty, returns empty cursor in the same bucket + volume as the world.
// The lookup cursor will be released after cb returns.
func (t *Tx) AccessWorldState(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	return t.state.AccessWorldState(ctx, ref, cb)
}

// ApplyWorldOp applies a batch operation at the world level.
// The handling of the operation is operation-type specific.
// Returns the seqno following the operation execution.
// If nil is returned for the error, implies success.
func (t *Tx) ApplyWorldOp(
	ctx context.Context,
	op world.Operation,
	opSender peer.ID,
) (seqno uint64, sysErr bool, err error) {
	err = t.locked(ctx, true, func() error {
		seqno, sysErr, err = t.state.ApplyWorldOp(ctx, op, opSender)
		return err
	})
	return seqno, sysErr, err
}

// PrepareCommit folds the pending changes into the World tree without writing
// a block. See WorldState.PrepareCommit.
func (t *Tx) PrepareCommit(ctx context.Context) error {
	return t.locked(ctx, true, func() error {
		return t.state.PrepareCommit(ctx)
	})
}

// commitChanges captures the ChangeSet before committing rebuilds the state
// onto its written root. The engine owns the write transaction exclusively.
func (t *Tx) commitChanges(ctx context.Context) (*block.BlockRef, *world.ChangeSet, error) {
	// Capture changes before entering the commit's trace scope.
	changes := t.state.takeChanges()

	// Seal the transaction's blocks under its new root.
	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/engine-tx/commit-block-transaction/write-tx-commit")
	nroot, err := t.CommitBlockTransaction(taskCtx)
	subtask.End()
	return nroot, changes, err
}

// takeCandidate borrows the written root's reachable blocks and recorded payload
// roots, then reads its sequence number and decoded root. A failed read returns
// the batch to the buffer. The engine owns the write transaction exclusively.
func (t *Tx) takeCandidate(ctx context.Context, root *block.BlockRef, recorded []*block.BlockRef) (*block.PendingBatch, uint64, *World, error) {
	// Borrow the reachable blocks before reading the candidate metadata.
	batch, err := t.state.takeReachable(ctx, slices.Concat([]*block.BlockRef{root}, recorded)...)
	if err != nil {
		return nil, 0, nil, err
	}
	seqno, err := t.state.GetSeqno(ctx)
	var rootBlock *World
	if err == nil {
		rootBlock, err = t.state.GetRoot(ctx)
	}
	if err != nil {
		batch.Complete(err)
		return nil, 0, nil, err
	}
	return batch, seqno, rootBlock, nil
}

// matchesRoot tests the transaction's current root without taking its lock.
// The caller owns the transaction or holds the Engine publication lock.
func (t *Tx) matchesRoot(ref *block.BlockRef) bool {
	return t.state.GetRootRef().EqualsRef(ref)
}

// readRootAt reads the immutable head block only when the transaction matches
// ref. The caller holds the Engine publication lock; the state guards its cursor.
func (t *Tx) readRootAt(ctx context.Context, ref *block.BlockRef) (*World, error) {
	if !t.matchesRoot(ref) {
		return nil, nil
	}
	return t.state.GetRoot(ctx)
}

// readSeqno reads the state's guarded sequence number without the transaction
// lock. The caller owns the transaction before sharing it.
func (t *Tx) readSeqno(ctx context.Context) (uint64, error) {
	return t.state.GetSeqno(ctx)
}

// setChangelogDisabled changes the history setting under the transaction lock.
func (t *Tx) setChangelogDisabled(ctx context.Context, disable bool) (bool, error) {
	return lockedValue(ctx, t, true, func() (bool, error) {
		return t.state.SetChangelogDisabled(ctx, disable)
	})
}

// end records the error that operations report after the retirement drains the
// transaction. Like the state's atomic marking, it must not wait for its users.
func (t *Tx) end(err error) {
	t.state.end(err)
}

// CommitBlockTransaction implements Commit but additionally returns the updated BlockRef.
// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
func (t *Tx) CommitBlockTransaction(ctx context.Context) (*block.BlockRef, error) {
	// Start the commit trace.
	ctx, task := trace.NewTask(ctx, "hydra/world-block/tx/commit-block-transaction")
	defer task.End()

	// Commit the transaction state under the write lock and return its root ref.
	return lockedValue(ctx, t, true, func() (*block.BlockRef, error) {
		// Commit the state.
		taskCtx, task := trace.NewTask(ctx, "hydra/world-block/tx/commit-block-transaction/state-commit")
		err := t.state.Commit(taskCtx)
		task.End()
		if err != nil {
			return nil, err
		}

		// Read the committed root ref.
		_, task = trace.NewTask(ctx, "hydra/world-block/tx/commit-block-transaction/get-root-ref")
		ref := t.state.GetRootRef()
		task.End()
		return ref, nil
	})
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
func (t *Tx) Commit(ctx context.Context) error {
	_, err := t.CommitBlockTransaction(ctx)
	return err
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
func (t *Tx) Discard() {
	lkr := t.rmtx.Locker()
	lkr.Lock()
	t.state.Discard()
	lkr.Unlock()
}

// locked runs fn under the read lock, or under the write lock when write is
// set. The single-threaded state is read and changed only inside fn.
func (t *Tx) locked(ctx context.Context, write bool, fn func() error) error {
	unlock, err := t.rmtx.Lock(ctx, write)
	if err != nil {
		return err
	}
	defer unlock()
	return fn()
}

// lockedValue runs fn under the transaction lock, as locked does, and returns
// its value.
func lockedValue[T any](ctx context.Context, t *Tx, write bool, fn func() (T, error)) (T, error) {
	var v T
	err := t.locked(ctx, write, func() error {
		var err error
		v, err = fn()
		return err
	})
	return v, err
}

// lockedLive runs fn under the read lock once the state is not discarded, for
// the object handles and iterators the state does not check on its own.
func (t *Tx) lockedLive(ctx context.Context, fn func() error) error {
	return t.locked(ctx, false, func() error {
		if err := t.state.discardErr(); err != nil {
			return err
		}
		return fn()
	})
}

// _ is a type assertion
var (
	_ world.ForkableWorldState = (*Tx)(nil)
	_ world.WorldState         = (*Tx)(nil)
	_ world.Tx                 = (*Tx)(nil)
)
