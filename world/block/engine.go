package world_block

import (
	"context"
	"sync"

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
	// rmtx locks the read-only world instance field & root field
	rmtx sync.RWMutex
	// root is the root cursor in use
	root *bucket_lookup.Cursor
	// readTx is the current read-only world instance
	readTx *Tx
}

// NewEngine constructs a new world engine.
func NewEngine(ctx context.Context, root *bucket_lookup.Cursor) (*Engine, error) {
	e := &Engine{ctx: ctx, root: root}
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
		e.wmtx.Lock()
		world, err = e.buildWorldState(false)
		if err != nil {
			e.wmtx.Unlock()
			return nil, err
		}
		writeTx = NewTx(world)
	}
	return newEngineTx(e, writeTx), nil
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
	return nil
}

// buildWorldState builds the world state transaction and cursor fields.
func (e *Engine) buildWorldState(readOnly bool) (*WorldState, error) {
	btx, bcs := e.root.BuildTransaction(nil)
	if readOnly {
		btx = nil
	}
	return NewWorldState(e.ctx, btx, bcs)
}

// _ is a type assertion
var _ world.Engine = ((*Engine)(nil))
