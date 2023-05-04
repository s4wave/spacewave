package world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/tx"
	"golang.org/x/sync/semaphore"
)

// BusEngine uses a directive lookup to access the Engine.
// Returns ErrTxDiscarded if directive expires internally.
// Use EngineWorldState to manage retrying automatically.
// Call Release when done with the engine.
type BusEngine struct {
	// ctx is the root context
	ctx context.Context
	// c cancels the context
	c context.CancelFunc
	// b is the bus to query
	b bus.Bus
	// engineID is the engine id to look up
	engineID string
	// buildSema guards below fields
	buildSema *semaphore.Weighted
	// handle is the current engine handle
	handle Engine
	// rel is the current release func
	rel func()
}

// NewBusEngine constructs a new BusEngine instance.
func NewBusEngine(ctx context.Context, b bus.Bus, engineID string) *BusEngine {
	e := &BusEngine{
		b:         b,
		engineID:  engineID,
		buildSema: semaphore.NewWeighted(1),
	}
	e.ctx, e.c = context.WithCancel(ctx)
	return e
}

// NewTransaction returns a new transaction against the store.
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
// Check GetReadOnly, might not return a write tx if write=true.
func (e *BusEngine) NewTransaction(write bool) (Tx, error) {
	handle, err := e.getOrBuildHandle()
	if err != nil {
		return nil, err
	}
	return handle.NewTransaction(write)
}

// BuildStorageCursor builds a cursor to the world storage with an empty ref.
// The cursor should be released independently of the WorldState.
// Be sure to call Release on the cursor when done.
func (e *BusEngine) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	handle, err := e.getOrBuildHandle()
	if err != nil {
		return nil, err
	}
	return handle.BuildStorageCursor(ctx)
}

// AccessWorldState builds a bucket lookup cursor with an optional ref.
// If the ref Bucket ID is empty, uses the same bucket + volume as the world.
// The lookup cursor will be released after cb returns.
func (e *BusEngine) AccessWorldState(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	handle, err := e.getOrBuildHandle()
	if err != nil {
		return err
	}
	return handle.AccessWorldState(ctx, ref, cb)
}

// GetSeqno returns the current seqno of the world state.
// This is also the sequence number of the most recent change.
// Initializes at 0 for initial world state.
func (e *BusEngine) GetSeqno() (uint64, error) {
	tx, err := e.NewTransaction(false)
	if err != nil {
		return 0, err
	}
	defer tx.Discard()

	return tx.GetSeqno()
}

// WaitSeqno waits for the seqno of the world state to be >= value.
// Returns nil when the condition is reached.
// If value == 0, this might return immediately unconditionally.
func (e *BusEngine) WaitSeqno(ctx context.Context, value uint64) (uint64, error) {
	handle, err := e.getOrBuildHandle()
	if err != nil {
		return 0, err
	}
	return handle.WaitSeqno(ctx, value)
}

// Close closes the bus engine.
func (e *BusEngine) Close() {
	e.c()
	err := e.buildSema.Acquire(context.Background(), 1)
	if err != nil {
		e.handle = nil
	}
}

// getOrBuildHandle gets or builds the handle.
func (e *BusEngine) getOrBuildHandle() (Engine, error) {
	// lookup the engine
	ctx := e.ctx
	err := e.buildSema.Acquire(ctx, 1)
	if err != nil {
		return nil, err
	}
	defer e.buildSema.Release(1)

	var rel func()
	handle := e.handle
	if handle == nil {
		lookupVal, _, lookupRef, err := ExLookupWorldEngine(e.ctx, e.b, false, e.engineID, func() {
			go func() {
				err := e.buildSema.Acquire(context.Background(), 1)
				if err != nil {
					return
				}
				if e.handle == handle {
					e.handle = nil
					if e.rel != nil {
						e.rel()
						e.rel = nil
					}
				}
				e.buildSema.Release(1)
			}()
		})
		if err != nil {
			return nil, err
		}
		handle = lookupVal
		rel = lookupRef.Release
	}
	if handle == nil {
		return nil, tx.ErrDiscarded
	}
	if e.handle != handle {
		if e.rel != nil {
			e.rel()
		}
		e.handle = handle
		e.rel = rel
	}
	return handle, nil
}

// _ is a type assertion
var _ Engine = ((*BusEngine)(nil))
