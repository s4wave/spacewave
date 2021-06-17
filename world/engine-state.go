package world

import (
	"context"

	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/tx"
)

// engineWorldState implements a WorldState on top of an Engine.
// Short-lived transactions are created for each operation.
type engineWorldState struct {
	ctx   context.Context
	e     Engine
	write bool
}

// NewEngineWorldState constructs a WorldState with an Engine.
func NewEngineWorldState(ctx context.Context, e Engine, write bool) WorldState {
	return &engineWorldState{ctx: ctx, e: e, write: write}
}

// GetReadOnly returns if the state is read-only.
func (e *engineWorldState) GetReadOnly() bool {
	return !e.write
}

// CreateObject creates an empty object with a key.
// Returns ErrObjectExists if the object already exists.
func (e *engineWorldState) CreateObject(key string, rootRef *bucket.ObjectRef) (ObjectState, error) {
	var outState ObjectState
	err := e.performOp(true, func(tx Tx) error {
		_, err := tx.CreateObject(key, rootRef)
		if err != nil {
			return err
		}
		outState = newEngineWorldStateObject(e, key)
		return nil
	})
	return outState, err
}

// GetObject looks up an object by key.
// Returns nil, false if not found.
func (e *engineWorldState) GetObject(key string) (ObjectState, bool, error) {
	var found bool
	err := e.performOp(false, func(tx Tx) error {
		var nerr error
		_, found, nerr = tx.GetObject(key)
		return nerr
	})
	var outState ObjectState
	if err == nil && found {
		outState = newEngineWorldStateObject(e, key)
	}
	return outState, found, err
}

// DeleteObject deletes an object and associated graph quads by ID.
// Calls DeleteGraphObject internally.
// Returns false, nil if not found.
func (e *engineWorldState) DeleteObject(key string) (bool, error) {
	var found bool
	err := e.performOp(true, func(tx Tx) error {
		var nerr error
		found, nerr = tx.DeleteObject(key)
		return nerr
	})
	return found, err
}

// AccessCayleyGraph calls a callback with a temporary Cayley graph handle.
// All accesses of the handle should complete before returning cb.
// Try to make access (queries) as short as possible.
// Write operations will fail if the store is read-only.
func (e *engineWorldState) AccessCayleyGraph(write bool, cb func(h CayleyHandle) error) error {
	return e.performOp(write, func(tx Tx) error {
		return tx.AccessCayleyGraph(write, cb)
	})
}

// LookupGraphQuads searches for graph quads in the store.
func (e *engineWorldState) LookupGraphQuads(filter GraphQuad, limit uint32) ([]GraphQuad, error) {
	var quads []GraphQuad
	err := e.performOp(false, func(tx Tx) error {
		var berr error
		quads, berr = tx.LookupGraphQuads(filter, limit)
		return berr
	})
	return quads, err
}

// SetGraphQuad sets a quad in the graph store.
// Subject: must be an existing object IRI: <object-id>
// Predicate: a predicate string, e.x. IRI: <ref>
// Object: an existing object IRI: <object-id>
// If already exists, returns nil.
func (e *engineWorldState) SetGraphQuad(q GraphQuad) error {
	return e.performOp(true, func(tx Tx) error {
		return tx.SetGraphQuad(q)
	})
}

// DeleteGraphQuad deletes a quad from the graph store.
// Note: if quad did not exist, returns nil.
func (e *engineWorldState) DeleteGraphQuad(q GraphQuad) error {
	return e.performOp(true, func(tx Tx) error {
		return tx.DeleteGraphQuad(q)
	})
}

// DeleteGraphObject deletes all quads with Subject or Object set to value.
// May also remove objects with <predicate> or <value> set to the value.
// Returns number of removed quads and any error.
func (e *engineWorldState) DeleteGraphObject(value string) error {
	return e.performOp(true, func(tx Tx) error {
		return tx.DeleteGraphObject(value)
	})
}

// performOp performs an operation.
func (e *engineWorldState) performOp(write bool, cb func(tx Tx) error) error {
	if !e.write && write {
		return tx.ErrNotWrite
	}

	ctx := e.ctx
	select {
	case <-ctx.Done():
		return context.Canceled
	default:
	}

	tx, err := e.e.NewTransaction(write)
	if err != nil {
		return err
	}
	defer tx.Discard() // catches panic cases

	err = cb(tx)
	if err == nil && write {
		err = tx.Commit(ctx)
	}
	return err
}

// _ is a type assertion
var _ WorldState = ((*engineWorldState)(nil))
