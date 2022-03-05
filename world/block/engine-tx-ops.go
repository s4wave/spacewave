package world_block

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/tx"
	"github.com/aperturerobotics/hydra/world"
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
	op world.Operation,
	opSender peer.ID,
) (uint64, bool, error) {
	var outSeqno uint64
	var outSysErr bool
	err := e.performOp(func(tx *Tx) error {
		var berr error
		outSeqno, outSysErr, berr = tx.ApplyWorldOp(op, opSender)
		return berr
	})
	return outSeqno, outSysErr, err
}

// CreateObject creates a object with a key and initial root ref.
// Returns ErrObjectExists if the object already exists.
func (e *EngineTx) CreateObject(key string, rootRef *bucket.ObjectRef) (world.ObjectState, error) {
	if err := e.performOp(func(tx *Tx) error {
		_, err := tx.CreateObject(key, rootRef)
		return err
	}); err != nil {
		return nil, err
	}

	return newEngineTxObjectState(e, key), nil
}

// GetObject looks up an object by key.
// Returns nil, false if not found.
func (e *EngineTx) GetObject(key string) (world.ObjectState, bool, error) {
	// check if object exists
	var found bool
	err := e.performOp(func(tx *Tx) error {
		var nerr error
		_, found, nerr = tx.GetObject(key)
		return nerr
	})
	if err != nil || !found {
		return nil, found, err
	}

	return newEngineTxObjectState(e, key), true, nil
}

// DeleteObject deletes an object and associated graph quads by ID.
// Calls DeleteGraphObject internally.
// Returns false, nil if not found.
func (e *EngineTx) DeleteObject(key string) (bool, error) {
	var deleted bool
	err := e.performOp(func(tx *Tx) error {
		var nerr error
		deleted, nerr = tx.DeleteObject(key)
		return nerr
	})
	return deleted, err
}

// AccessCayleyGraph calls a callback with a temporary Cayley graph handle.
// All accesses of the handle should complete before returning cb.
// Try to make access (queries) as short as possible.
// Write operations will fail if the store is read-only.
func (e *EngineTx) AccessCayleyGraph(write bool, cb func(h world.CayleyHandle) error) error {
	return e.performOp(func(tx *Tx) error {
		return tx.AccessCayleyGraph(write, cb)
	})
}

// LookupGraphQuads searches for graph quads in the store.
func (e *EngineTx) LookupGraphQuads(filter world.GraphQuad, limit uint32) ([]world.GraphQuad, error) {
	var quads []world.GraphQuad
	err := e.performOp(func(tx *Tx) error {
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
func (e *EngineTx) SetGraphQuad(q world.GraphQuad) error {
	return e.performOp(func(tx *Tx) error {
		return tx.SetGraphQuad(q)
	})
}

// DeleteGraphQuad deletes a quad from the graph store.
// Note: if quad did not exist, returns nil.
func (e *EngineTx) DeleteGraphQuad(q world.GraphQuad) error {
	return e.performOp(func(tx *Tx) error {
		return tx.DeleteGraphQuad(q)
	})
}

// DeleteGraphObject deletes all quads with Subject or Object set to value.
// May also remove objects with <predicate> or <value> set to the value.
func (e *EngineTx) DeleteGraphObject(value string) error {
	return e.performOp(func(tx *Tx) error {
		return tx.DeleteGraphObject(value)
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
