package world_block

import (
	"context"
	"runtime/trace"
	"sync"

	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/world"
	"github.com/aperturerobotics/util/csync"
	"github.com/sirupsen/logrus"
)

// Engine is the world engine instance.
// Uses short-lived block graph transactions internally.
// Reads are against latest state; read txs don't lock.
// Re-tries transaction operations if the underlying transaction is discarded mid-way through.
// Maintains two WorldState objects: one for readers, one for writer.
type Engine struct {
	// le is the logger
	le *logrus.Entry
	// lookupOp looks up a world operation.
	lookupOp world.LookupOp
	// verbose enables verbose logging within world state
	verbose bool
	// wmtx ensures only one write transaction is active at a time
	wmtx csync.Mutex
	// rmtx locks the read-only world instance field & root field & waiters & read/writeTx
	rmtx sync.RWMutex
	// baseRoot is the base root cursor to use.
	// the root cursor is derived with FollowRef from this cursor.
	baseRoot *bucket_lookup.Cursor
	// root is the root cursor in use
	root *bucket_lookup.Cursor
	// readTx is the current read-only world instance
	readTx *Tx
	// writeTx is the current write tx
	// canceled if the state changes mid-write
	writeTx *EngineTx
	// writeTxRel releases wmtx, call when unsetting writeTx
	writeTxRel func()
	// commitFn is a function to be called just before a commit is confirmed.
	// can be nil
	commitFn CommitFn
}

// CommitFn is a function to call with the updated root before confirming it.
// Should be used to write the updated state back to storage.
// Note: engine rmtx is locked while cb is called, do not block or call engine funcs!
// If an error is returned the change will be rolled back.
// Do not change the nrootBcs during this call.
type CommitFn func(nref *bucket.ObjectRef) error

// NewEngine constructs a new world engine.
// commitFn can be nil.
func NewEngine(
	ctx context.Context,
	le *logrus.Entry,
	root *bucket_lookup.Cursor,
	lookupOp world.LookupOp,
	commitFn CommitFn,
	verbose bool,
) (*Engine, error) {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/engine/new")
	defer task.End()

	e := &Engine{
		le:       le,
		baseRoot: root,
		lookupOp: lookupOp,
		root:     root.Clone(),
		commitFn: commitFn,
		verbose:  verbose,
	}
	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/engine/new/update-read-write-txns")
	err := e.updateReadWriteTxns(taskCtx)
	subtask.End()
	if err != nil {
		return nil, err
	}
	return e, nil
}

// GetRootRef gets the current root cursor reference.
func (e *Engine) GetRootRef() *bucket.ObjectRef {
	e.rmtx.RLock()
	ref := e.root.GetRef().Clone()
	e.rmtx.RUnlock()
	return ref
}

// GetGCJournalEntries returns the number of pending GC journal entries.
// Safe to call concurrently. Returns 0 if the read tx or journal is not initialized.
func (e *Engine) GetGCJournalEntries() uint64 {
	e.rmtx.RLock()
	rtx := e.readTx
	e.rmtx.RUnlock()
	if rtx == nil {
		return 0
	}
	return rtx.state.GetGCJournalEntries()
}

// SetRootRef updates the root cursor to point to a new reference.
// Re-creates the internal read transaction with the updated state.
// Cancels any ongoing write tx (to be re-created against new state).
// Can return an error to indicate validation failure.
func (e *Engine) SetRootRef(ctx context.Context, ref *bucket.ObjectRef) error {
	e.rmtx.Lock()
	defer e.rmtx.Unlock()

	return e.setRootRefLocked(ctx, ref)
}

// setRootRefLocked updates the root reference while rmtx is locked.
func (e *Engine) setRootRefLocked(ctx context.Context, ref *bucket.ObjectRef) error {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/engine/set-root-ref")
	defer task.End()

	// if no changes, ignore the call
	if e.root.GetRef().EqualsRef(ref) {
		return nil
	}

	// validate the new root
	if err := ref.Validate(); err != nil {
		return err
	}

	// apply committed changes or rollback
	// oldRoot := e.root.GetRef().Clone()
	oldRoot := e.root
	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/engine/set-root-ref/follow-ref")
	nextRoot, err := e.baseRoot.FollowRef(taskCtx, ref)
	subtask.End()
	if err != nil {
		return err
	}
	e.root = nextRoot
	taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/engine/set-root-ref/update-read-write-txns")
	err = e.updateReadWriteTxns(taskCtx)
	subtask.End()
	if err == nil {
		oldRoot.Release()
	} else {
		e.root = oldRoot
		nextRoot.Release()
	}
	return err
}

// NewTransaction returns a new transaction against the store.
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
// Check GetReadOnly, might not return a write tx if write=true.
func (e *Engine) NewTransaction(ctx context.Context, write bool) (world.Tx, error) {
	return e.NewBlockEngineTransaction(ctx, write)
}

// NewBlockEngineTransaction returns the world-block specific EngineTx type.
func (e *Engine) NewBlockEngineTransaction(ctx context.Context, write bool) (*EngineTx, error) {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/engine/new-block-engine-transaction")
	defer task.End()

	// writeTx is nil if it's a read-only tx
	if !write {
		return newEngineTx(e, nil), nil
	}

	// Released in Discard or Commit
	relLock, err := e.wmtx.Lock(ctx)
	if err != nil {
		return nil, err
	}

	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/engine/new-block-engine-transaction/build-world-state")
	e.rmtx.Lock()
	defer e.rmtx.Unlock()

	world, err := e.buildWorldState(taskCtx, false)
	subtask.End()
	if err != nil {
		relLock()
		return nil, err
	}

	engTx := newEngineTx(e, NewTx(world))
	e.writeTx = engTx
	e.writeTxRel = relLock
	return engTx, nil
}

// ForkBlockTransaction forks the transaction at the current state.
func (e *Engine) ForkBlockTransaction(ctx context.Context, write bool) (*Tx, error) {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/engine/fork-block-transaction")
	defer task.End()

	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/engine/fork-block-transaction/read-lock")
	e.rmtx.RLock()
	subtask.End()
	defer e.rmtx.RUnlock()

	taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/engine/fork-block-transaction/build-world-state")
	ws, err := e.buildWorldState(taskCtx, !write)
	subtask.End()
	if err != nil {
		return nil, err
	}
	taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/engine/fork-block-transaction/new-tx")
	tx := NewTx(ws)
	subtask.End()
	return tx, nil
}

// BuildStorageCursor builds a cursor to the world storage with an empty ref.
// The cursor should be released independently of the WorldState.
// Be sure to call Release on the cursor when done.
func (e *Engine) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	ncs := e.baseRoot.Clone()
	ncs.SetRootRef(nil)
	return ncs, nil
}

// AccessWorldState builds a bucket lookup cursor with an optional ref.
// If the ref Bucket ID is empty, uses the same bucket + volume as the world.
// The lookup cursor will be released after cb returns.
//
// NOTE: this is the implementation of AccessWorldState for the world/block engine.
func (e *Engine) AccessWorldState(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	if ref == nil {
		ncs := e.root.Clone()
		defer ncs.Release()
		return cb(ncs)
	}

	subCtx, subCtxCancel := context.WithCancel(ctx)
	defer subCtxCancel()

	// follow the root block ref
	ncs, err := e.root.FollowRef(subCtx, ref)
	if err != nil {
		return err
	}
	defer ncs.Release()

	return cb(ncs)
}

// GetSeqno returns the current seqno of the world state.
// This is also the sequence number of the most recent change.
// Initializes at 0 for initial world state.
func (e *Engine) GetSeqno(ctx context.Context) (uint64, error) {
	e.rmtx.Lock()
	seqno, err := e.readTx.GetSeqno(ctx)
	e.rmtx.Unlock()
	return seqno, err
}

// WaitSeqno waits for the seqno of the world state to be >= value.
// Returns the seqno when the condition is reached.
// If value == 0, this might return immediately unconditionally.
func (e *Engine) WaitSeqno(ctx context.Context, value uint64) (uint64, error) {
	for {
		e.rmtx.RLock()
		readTx := e.readTx
		e.rmtx.RUnlock()

		seqno, err := readTx.WaitSeqno(ctx, value)
		if readTx.state.discarded.Load() {
			// readTxn was discarded, get the new one.
			continue
		}
		if err != nil {
			return 0, err
		}

		if seqno >= value {
			return seqno, nil
		}
	}
}

// updateReadWriteTxns updates the readTx and cancels writeTx if the state changed
// expects caller to hold rmtx lock
// the state has been affected only if nil is returned
func (e *Engine) updateReadWriteTxns(ctx context.Context) error {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/engine/update-read-write-txns")
	defer task.End()

	// This is the only place readTx might be nil (on first call).
	// If no changes have occurred...
	if e.readTx != nil && e.readTx.state.GetRootRef().EqualsRef(e.root.GetRef().GetRootRef()) {
		return nil
	}

	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/engine/update-read-write-txns/build-world-state")
	world, err := e.buildWorldState(taskCtx, true)
	subtask.End()
	if err != nil {
		return err
	}
	// cancel the old write tx if active
	if e.writeTx != nil {
		e.writeTx.discardLocked()
		e.writeTx = nil // field is checked during Commit() as well
	}
	// swap in the new read tx
	readTx := NewTx(world)
	if e.readTx != nil {
		e.readTx.Discard()
	}
	e.readTx = readTx
	return nil
}

// buildWorldState builds the world state transaction and cursor fields.
// expects caller to hold rmtx
func (e *Engine) buildWorldState(ctx context.Context, readOnly bool) (*WorldState, error) {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/engine/build-world-state")
	defer task.End()

	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/engine/build-world-state/get-bucket")
	store := e.root.GetBucket()
	subtask.End()
	taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/engine/build-world-state/get-transformer")
	xfrm := e.root.GetTransformer()
	subtask.End()
	taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/engine/build-world-state/build-transaction")
	btx, bcs := e.root.BuildTransaction(nil)
	subtask.End()
	if readOnly {
		btx = nil
	}
	taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/engine/build-world-state/new-world-state")
	ws, err := NewWorldState(
		taskCtx,
		e.le,
		!readOnly,
		btx, bcs,
		store,
		xfrm,
		nil, // onSwept: wired in Phase 3
		e,
		e.lookupOp,
		e.verbose,
	)
	subtask.End()
	return ws, err
}

// _ is a type assertion
var _ world.Engine = ((*Engine)(nil))
