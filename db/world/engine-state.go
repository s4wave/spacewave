package world

import (
	"context"

	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/tx"
)

// engineWorldState implements a WorldState on top of an Engine.
// Short-lived transactions are created for each operation.
type engineWorldState struct {
	e     Engine
	write bool
}

// NewEngineWorldState constructs a WorldState with an Engine.
func NewEngineWorldState(e Engine, write bool) WorldState {
	return &engineWorldState{e: e, write: write}
}

// GetReadOnly returns if the state is read-only.
func (e *engineWorldState) GetReadOnly() bool {
	return !e.write
}

// GetSeqno returns the current seqno of the world state.
// This is also the sequence number of the most recent change.
// Initializes at 0 for initial world state.
func (e *engineWorldState) GetSeqno(ctx context.Context) (uint64, error) {
	return e.e.GetSeqno(ctx)
}

// WaitSeqno waits for the seqno of the world state to be >= value.
// Returns nil when the condition is reached.
// If value == 0, this might return immediately unconditionally.
func (e *engineWorldState) WaitSeqno(ctx context.Context, value uint64) (uint64, error) {
	return e.e.WaitSeqno(ctx, value)
}

// BuildStorageCursor builds a cursor to the world storage with an empty ref.
// The cursor should be released independently of the WorldState.
// Be sure to call Release on the cursor when done.
func (e *engineWorldState) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	return e.e.BuildStorageCursor(ctx)
}

// AccessWorldState builds a bucket lookup cursor with an optional ref.
// If the ref is empty, returns empty cursor in the same bucket + volume as the world.
// The lookup cursor will be released after cb returns.
func (e *engineWorldState) AccessWorldState(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	return e.e.AccessWorldState(ctx, ref, cb)
}

// ApplyWorldOp applies a batch operation at the world level.
// The handling of the operation is operation-type specific.
// Returns the seqno following the operation execution.
// If nil is returned for the error, implies success.
func (e *engineWorldState) ApplyWorldOp(
	ctx context.Context,
	op Operation,
	sender peer.ID,
) (uint64, bool, error) {
	var outSeqno uint64
	var outSysErr bool
	err := e.performOp(ctx, true, func(tx Tx) error {
		var berr error
		outSeqno, outSysErr, berr = tx.ApplyWorldOp(ctx, op, sender)
		return berr
	})
	return outSeqno, outSysErr, err
}

// CreateObject creates a object with a key and initial root ref.
// Returns ErrObjectExists if the object already exists.
func (e *engineWorldState) CreateObject(ctx context.Context, key string, rootRef *bucket.ObjectRef) (ObjectState, error) {
	var outState ObjectState
	err := e.performOp(ctx, true, func(tx Tx) error {
		_, err := tx.CreateObject(ctx, key, rootRef)
		if err != nil {
			return err
		}
		outState = newEngineWorldStateObject(e, key)
		return nil
	})
	return outState, err
}

// IterateObjects returns an iterator with the given object key prefix.
// The prefix is NOT clipped from the output keys.
// Keys are returned in sorted order.
// Must call Next() or Seek() before valid.
// Call Close when done with the iterator.
// Any init errors will be available via the iterator's Err() method.
func (e *engineWorldState) IterateObjects(ctx context.Context, prefix string, reversed bool) ObjectIterator {
	return NewEngineObjectIterator(ctx, e.e, prefix, reversed)
}

// GetObject looks up an object by key.
// Returns nil, false if not found.
func (e *engineWorldState) GetObject(ctx context.Context, key string) (ObjectState, bool, error) {
	var found bool
	err := e.performOp(ctx, false, func(tx Tx) error {
		var nerr error
		_, found, nerr = tx.GetObject(ctx, key)
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
func (e *engineWorldState) DeleteObject(ctx context.Context, key string) (bool, error) {
	var found bool
	err := e.performOp(ctx, true, func(tx Tx) error {
		var nerr error
		found, nerr = tx.DeleteObject(ctx, key)
		return nerr
	})
	return found, err
}

// AccessCayleyGraph calls a callback with a temporary Cayley graph handle.
// All accesses of the handle should complete before returning cb.
// Try to make access (queries) as short as possible.
// Write operations will fail if the store is read-only.
func (e *engineWorldState) AccessCayleyGraph(ctx context.Context, write bool, cb func(ctx context.Context, h CayleyHandle) error) error {
	return e.performOp(ctx, write, func(tx Tx) error {
		return tx.AccessCayleyGraph(ctx, write, cb)
	})
}

// LookupGraphQuads searches for graph quads in the store.
func (e *engineWorldState) LookupGraphQuads(ctx context.Context, filter GraphQuad, limit uint32) ([]GraphQuad, error) {
	var quads []GraphQuad
	err := e.performOp(ctx, false, func(tx Tx) error {
		var berr error
		quads, berr = tx.LookupGraphQuads(ctx, filter, limit)
		return berr
	})
	return quads, err
}

// SetGraphQuad sets a quad in the graph store.
// Subject: must be an existing object IRI: <object-key>
// Predicate: a predicate string, e.x. IRI: <ref>
// Object: an existing object IRI: <object-key>
// If already exists, returns nil.
func (e *engineWorldState) SetGraphQuad(ctx context.Context, q GraphQuad) error {
	return e.performOp(ctx, true, func(tx Tx) error {
		return tx.SetGraphQuad(ctx, q)
	})
}

// DeleteGraphQuad deletes a quad from the graph store.
// Note: if quad did not exist, returns nil.
func (e *engineWorldState) DeleteGraphQuad(ctx context.Context, q GraphQuad) error {
	return e.performOp(ctx, true, func(tx Tx) error {
		return tx.DeleteGraphQuad(ctx, q)
	})
}

// DeleteGraphObject deletes all quads with Subject or Object set to value.
// May also remove objects with <predicate> or <value> set to the value.
func (e *engineWorldState) DeleteGraphObject(ctx context.Context, value string) error {
	return e.performOp(ctx, true, func(tx Tx) error {
		return tx.DeleteGraphObject(ctx, value)
	})
}

// performOp performs an operation.
func (e *engineWorldState) performOp(ctx context.Context, write bool, cb func(tx Tx) error) error {
	if !e.write && write {
		return tx.ErrNotWrite
	}

	if err := ctx.Err(); err != nil {
		return ctx.Err()
	}

	tx, err := e.e.NewTransaction(ctx, write)
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
