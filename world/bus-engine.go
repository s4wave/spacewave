package world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
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
	handle EngineHandle
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
// If the store is not the latest HEAD block, it will be read-only.
// Check GetReadOnly, might not return a write tx if write=true.
func (e *BusEngine) NewTransaction(write bool) (Tx, error) {
	// lookup the engine
	ctx := e.ctx
	err := e.buildSema.Acquire(ctx, 1)
	if err != nil {
		return nil, err
	}
	defer e.buildSema.Release(1)

	handle := e.handle
	if handle != nil {
		select {
		case <-handle.GetContext().Done():
			handle = nil
		default:
		}
	}
	var rel func()
	if handle == nil {
		lookupVal, lookupRef, err := ExLookupWorldEngine(e.ctx, e.b, e.engineID)
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
	return handle.NewTransaction(write)
}

// Close closes the bus engine.
func (e *BusEngine) Close() {
	e.c()
	err := e.buildSema.Acquire(context.Background(), 1)
	if err != nil {
		e.handle = nil
	}
}

// _ is a type assertion
var _ Engine = ((*BusEngine)(nil))
