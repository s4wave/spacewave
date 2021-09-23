package world_block

import (
	"context"
	"sync"

	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/world"
	"golang.org/x/sync/semaphore"
)

// Engine is the world engine instance.
// Uses short-lived block graph transactions internally.
// Reads are against latest state; read txs don't lock.
// Re-tries transaction operations if the underlying transaction is discarded mid-way through.
// Maintains two WorldState objects: one for readers, one for writer.
type Engine struct {
	// ctx is the context
	ctx context.Context
	// lookupOp looks up a world operation.
	lookupOp world.LookupOp
	// wmtx ensures only one write transaction is active at a time
	wmtx *semaphore.Weighted
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
	// waiters are callbacks that should be called when seqno changes
	waiters []func(seqno uint64)
}

// NewEngine constructs a new world engine.
func NewEngine(
	ctx context.Context,
	root *bucket_lookup.Cursor,
	lookupOp world.LookupOp,
) (*Engine, error) {
	e := &Engine{
		ctx:      ctx,
		baseRoot: root,
		lookupOp: lookupOp,
		root:     root.Clone(),

		wmtx: semaphore.NewWeighted(1),
	}
	if err := e.updateReadWriteTxns(); err != nil {
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

// SetRootRef updates the root cursor to point to a new reference.
// Re-creates the internal read transaction with the updated state.
// Cancels any ongoing write tx (to be re-created against new state).
// Can return an error to indicate validation failure.
func (e *Engine) SetRootRef(ctx context.Context, ref *bucket.ObjectRef) error {
	e.rmtx.Lock()
	defer e.rmtx.Unlock()

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
	nextRoot, err := e.baseRoot.FollowRef(ctx, ref)
	if err != nil {
		return err
	}
	e.root = nextRoot
	err = e.updateReadWriteTxns()
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
func (e *Engine) NewTransaction(write bool) (world.Tx, error) {
	return e.NewBlockEngineTransaction(write)
}

// NewBlockEngineTransaction returns the world-block specific EngineTx type.
func (e *Engine) NewBlockEngineTransaction(write bool) (*EngineTx, error) {
	// writeTx is nil if it's a read-only tx
	if !write {
		return newEngineTx(e, nil), nil
	}

	// Released in Discard or Commit
	if err := e.wmtx.Acquire(e.ctx, 1); err != nil {
		return nil, err
	}

	e.rmtx.Lock()
	defer e.rmtx.Unlock()

	world, err := e.buildWorldState(false)
	if err != nil {
		e.wmtx.Release(1)
		return nil, err
	}

	engTx := newEngineTx(e, NewTx(world))
	e.writeTx = engTx
	return engTx, nil
}

// ForkBlockTransaction forks the transaction at the current state.
func (e *Engine) ForkBlockTransaction(write bool) (*Tx, error) {
	e.rmtx.RLock()
	defer e.rmtx.RUnlock()

	ws, err := e.buildWorldState(!write)
	if err != nil {
		return nil, err
	}
	return NewTx(ws), nil
}

// AccessWorldState builds a bucket lookup cursor with an optional ref.
// If the ref Bucket ID is empty, uses the same bucket + volume as the world.
// The lookup cursor will be released after cb returns.
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
	ncs, err := e.root.FollowRef(subCtx, ref)
	if err != nil {
		return err
	}
	defer ncs.Release()
	return cb(ncs)
}

// WaitSeqno waits for the seqno of the world state to be >= value.
// Returns the seqno when the condition is reached.
// If value == 0, this might return immediately unconditionally.
func (e *Engine) WaitSeqno(ctx context.Context, value uint64) (uint64, error) {
	for {
		e.rmtx.Lock()
		seqno, err := e.readTx.GetSeqno()
		var waitCh chan uint64
		tooOld := seqno < value
		if err == nil && tooOld {
			waitCh = make(chan uint64, 1)
			e.waiters = append(e.waiters, func(seqno uint64) {
				select {
				case waitCh <- seqno:
				default:
				}
			})
		}
		e.rmtx.Unlock()
		if err != nil {
			return 0, err
		}
		if !tooOld {
			return seqno, nil
		}

		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case seqno = <-waitCh:
			// seqno updated
			if seqno >= value {
				return seqno, nil
			}
		}
	}
}

// updateReadWriteTxns updates the readTx and cancels writeTx if the state changed
// expects caller to hold rmtx lock
// the state has been affected only if nil is returned
func (e *Engine) updateReadWriteTxns() error {
	// If no changes have occurred...
	if e.readTx != nil &&
		e.readTx.state.GetRootRef().EqualsRef(e.root.GetRef().GetRootRef()) {
		return nil
	}

	world, err := e.buildWorldState(true)
	if err != nil {
		return err
	}
	readTx := NewTx(world)
	var nseqno uint64
	if len(e.waiters) != 0 {
		nseqno, err = readTx.GetSeqno()
		if err == nil {
			e.procWaiters(nseqno)
		}
	}
	if err != nil {
		readTx.Discard()
		world.Close()
		return err
	}
	// cancel the old write tx if active
	if e.writeTx != nil {
		e.writeTx.Discard()
		e.writeTx = nil // field is checked during Commit() as well
	}
	// swap in the new read tx
	if e.readTx != nil {
		e.readTx.Discard()
	}
	e.readTx = readTx
	return nil
}

// procWaiters calls all waiters.
// expects rmtx to be locked
func (e *Engine) procWaiters(nseqno uint64) {
	waiters := e.waiters
	e.waiters = nil
	for _, w := range waiters {
		w(nseqno)
	}
}

// buildWorldState builds the world state transaction and cursor fields.
// expects caller to hold rmtx
func (e *Engine) buildWorldState(readOnly bool) (*WorldState, error) {
	btx, bcs := e.root.BuildTransaction(nil)
	if readOnly {
		btx = nil
	}
	return NewWorldState(
		e.ctx,
		!readOnly,
		btx, bcs,
		e.AccessWorldState,
		e.lookupOp,
	)
}

// _ is a type assertion
var _ world.Engine = ((*Engine)(nil))
