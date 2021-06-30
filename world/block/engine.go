package world_block

import (
	"context"
	"sync"

	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/world"
)

// Engine is the world engine instance.
// Uses short-lived block graph transactions internally.
// Reads are against latest state; read txs don't lock.
// Re-tries transaction operations if the underlying transaction is discarded mid-way through.
// Maintains two WorldState objects: one for readers, one for writer.
type Engine struct {
	// ctx is the context
	ctx context.Context
	// wmtx ensures only one write transaction is active at a time
	wmtx sync.Mutex
	// rmtx locks the read-only world instance field & root field & waiters
	rmtx sync.RWMutex
	// root is the root cursor in use
	root *bucket_lookup.Cursor
	// readTx is the current read-only world instance
	readTx *Tx
	// waiters are callbacks that should be called when seqno changes
	waiters []func(seqno uint64)

	// TODO handle w/ the following
	worldOpHandlers  []world.ApplyWorldOpFunc
	objectOpHandlers []world.ApplyObjectOpFunc
}

// NewEngine constructs a new world engine.
func NewEngine(
	ctx context.Context,
	root *bucket_lookup.Cursor,
	worldOpHandlers []world.ApplyWorldOpFunc,
	objectOpHandlers []world.ApplyObjectOpFunc,
) (*Engine, error) {
	e := &Engine{
		ctx:  ctx,
		root: root,

		worldOpHandlers:  worldOpHandlers,
		objectOpHandlers: objectOpHandlers,
	}
	if err := e.updateReadTx(); err != nil {
		return nil, err
	}
	return e, nil
}

// NewTransaction returns a new transaction against the store.
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
// If the store is not the latest HEAD block, it will be read-only.
// Check GetReadOnly, might not return a write tx if write=true.
func (e *Engine) NewTransaction(write bool) (world.Tx, error) {
	var world *WorldState
	var err error
	var writeTx *Tx
	if write {
		e.wmtx.Lock() // unlocked in Commit or Discard
		world, err = e.buildWorldState(false)
		if err != nil {
			e.wmtx.Unlock()
			return nil, err
		}
		writeTx = NewTx(world)
	}
	return newEngineTx(e, writeTx), nil
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

// updateReadTx updates the current read-only world state handle.
// expects caller to hold rmtx lock
func (e *Engine) updateReadTx() error {
	world, err := e.buildWorldState(true)
	if err != nil {
		return err
	}
	if e.readTx != nil {
		e.readTx.Discard()
	}
	e.readTx = NewTx(world)
	var nseqno uint64
	if len(e.waiters) != 0 {
		nseqno, err = e.readTx.GetSeqno()
		if err == nil {
			e.procWaiters(nseqno)
		}
	}
	return err
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
func (e *Engine) buildWorldState(readOnly bool) (*WorldState, error) {
	btx, bcs := e.root.BuildTransaction(nil)
	if readOnly {
		btx = nil
	}
	return NewWorldState(
		e.ctx,
		btx, bcs,
		e.AccessWorldState,
		e.worldOpHandlers,
		e.objectOpHandlers,
	)
}

// _ is a type assertion
var _ world.Engine = ((*Engine)(nil))
