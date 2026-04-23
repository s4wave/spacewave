package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
)

// maxEngineTxTries is the maximum number of times to retry after discarded
const maxEngineTxTries = 10

// BuildStorageCursor builds a cursor to the world storage with an empty ref.
// The cursor should be released independently of the WorldState.
// Be sure to call Release on the cursor when done.
func (e *EngineTx) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	return e.engine.BuildStorageCursor(ctx)
}

// AccessWorldState builds a bucket lookup cursor with an optional ref.
// If the ref is empty, returns empty cursor in the same bucket + volume as the world.
// The lookup cursor will be released after cb returns.
func (e *EngineTx) AccessWorldState(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	return e.engine.AccessWorldState(ctx, ref, cb)
}

// ApplyWorldOp applies a batch operation at the world level.
// The handling of the operation is operation-type specific.
// Returns the seqno following the operation execution.
// If nil is returned for the error, implies success.
func (e *EngineTx) ApplyWorldOp(
	ctx context.Context,
	op world.Operation,
	opSender peer.ID,
) (uint64, bool, error) {
	var outSeqno uint64
	var outSysErr bool
	err := e.performOp(func(tx *Tx) error {
		var berr error
		outSeqno, outSysErr, berr = tx.ApplyWorldOp(ctx, op, opSender)
		return berr
	})
	return outSeqno, outSysErr, err
}

// CreateObject creates a object with a key and initial root ref.
// Returns ErrObjectExists if the object already exists.
func (e *EngineTx) CreateObject(ctx context.Context, key string, rootRef *bucket.ObjectRef) (world.ObjectState, error) {
	var obj world.ObjectState
	if err := e.performOp(func(tx *Tx) error {
		var err error
		obj, err = tx.CreateObject(ctx, key, rootRef)
		return err
	}); err != nil {
		return nil, err
	}

	return newEngineTxObjectState(e, key, obj), nil
}

// GetObject looks up an object by key.
// Returns nil, false if not found.
func (e *EngineTx) GetObject(ctx context.Context, key string) (world.ObjectState, bool, error) {
	// check if object exists
	var found bool
	var obj world.ObjectState
	err := e.performOp(func(tx *Tx) error {
		var nerr error
		obj, found, nerr = tx.GetObject(ctx, key)
		return nerr
	})
	if err != nil || !found {
		return nil, found, err
	}

	if e.writeTx == nil {
		obj = nil
	}
	return newEngineTxObjectState(e, key, obj), true, nil
}

// IterateObjects returns an iterator with the given object key prefix.
// The prefix is NOT clipped from the output keys.
// Keys are returned in sorted order.
// Must call Next() or Seek() before valid.
// Call Close when done with the iterator.
// Any init errors will be available via the iterator's Err() method.
func (e *EngineTx) IterateObjects(ctx context.Context, prefix string, reversed bool) world.ObjectIterator {
	return NewEngineTxObjectIterator(e, ctx, prefix, reversed)
}

// DeleteObject deletes an object and associated graph quads by ID.
// Calls DeleteGraphObject internally.
// Returns false, nil if not found.
func (e *EngineTx) DeleteObject(ctx context.Context, key string) (bool, error) {
	var deleted bool
	err := e.performOp(func(tx *Tx) error {
		var nerr error
		deleted, nerr = tx.DeleteObject(ctx, key)
		return nerr
	})
	return deleted, err
}

// RenameObject renames an object key and updates associated graph quads.
func (e *EngineTx) RenameObject(ctx context.Context, oldKey, newKey string) (world.ObjectState, error) {
	var obj world.ObjectState
	if err := e.performOp(func(tx *Tx) error {
		var err error
		obj, err = tx.RenameObject(ctx, oldKey, newKey)
		return err
	}); err != nil {
		return nil, err
	}

	return newEngineTxObjectState(e, newKey, obj), nil
}

// AccessCayleyGraph calls a callback with a temporary Cayley graph handle.
// All accesses of the handle should complete before returning cb.
// Try to make access (queries) as short as possible.
// Write operations will fail if the store is read-only.
func (e *EngineTx) AccessCayleyGraph(ctx context.Context, write bool, cb func(ctx context.Context, h world.CayleyHandle) error) error {
	return e.performOp(func(tx *Tx) error {
		return tx.AccessCayleyGraph(ctx, write, cb)
	})
}

// LookupGraphQuads searches for graph quads in the store.
func (e *EngineTx) LookupGraphQuads(ctx context.Context, filter world.GraphQuad, limit uint32) ([]world.GraphQuad, error) {
	var quads []world.GraphQuad
	err := e.performOp(func(tx *Tx) error {
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
func (e *EngineTx) SetGraphQuad(ctx context.Context, q world.GraphQuad) error {
	return e.performOp(func(tx *Tx) error {
		return tx.SetGraphQuad(ctx, q)
	})
}

// DeleteGraphQuad deletes a quad from the graph store.
// Note: if quad did not exist, returns nil.
func (e *EngineTx) DeleteGraphQuad(ctx context.Context, q world.GraphQuad) error {
	return e.performOp(func(tx *Tx) error {
		return tx.DeleteGraphQuad(ctx, q)
	})
}

// DeleteGraphObject deletes all quads with Subject or Object set to value.
// May also remove objects with <predicate> or <value> set to the value.
func (e *EngineTx) DeleteGraphObject(ctx context.Context, value string) error {
	return e.performOp(func(tx *Tx) error {
		return tx.DeleteGraphObject(ctx, value)
	})
}

// GarbageCollect sweeps unreferenced nodes from the GC ref graph.
// Only valid on writable EngineTx instances with GC enabled.
func (e *EngineTx) GarbageCollect(ctx context.Context) error {
	return e.performOp(func(tx *Tx) error {
		_, err := tx.state.GarbageCollect(ctx)
		return err
	})
}

// performOp performs an operation while retrying if the read tx was discarded
// if ErrTxDiscarded is returned, retries against the updated txn
func (e *EngineTx) performOp(cb func(tx *Tx) error) error {
	if e.writeTx != nil {
		return cb(e.writeTx)
	}

	tries := 0
	var err error
	for {
		e.engine.rmtx.RLock()
		rtx := e.engine.readTx
		e.engine.rmtx.RUnlock()
		if rtx == nil {
			return context.Canceled
		}
		err = cb(rtx)
		if err == nil || err != tx.ErrDiscarded {
			// complete
			break
		}

		// retry
		tries++
		if tries > maxEngineTxTries {
			break
		}
	}
	return err
}

// _ is a type assertion
var _ world.WorldState = ((*EngineTx)(nil))
