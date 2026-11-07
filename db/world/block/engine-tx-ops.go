package world_block

import (
	"context"
	stderrors "errors"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
)

// maxEngineTxTries bounds retries after a transaction is discarded.
const maxEngineTxTries = 10

// BuildStorageCursor builds a cursor to the world storage with an empty ref.
// The cursor should be released independently of the WorldState.
// Be sure to call Release on the cursor when done.
// A write transaction's cursor writes through the transaction's store.
func (e *EngineTx) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	if e.writeTx != nil {
		return e.writeTx.BuildStorageCursor(ctx)
	}
	return e.engine.BuildStorageCursor(ctx)
}

// StageWorldState returns a stage over the write transaction's storage.
func (e *EngineTx) StageWorldState(ctx context.Context) (world.WorldStage, error) {
	if e.writeTx == nil {
		return nil, tx.ErrNotWrite
	}
	return e.writeTx.StageWorldState(ctx)
}

// AccessWorldState builds a bucket lookup cursor with an optional ref.
// If the ref is empty, returns empty cursor in the same bucket + volume as the world.
// The lookup cursor will be released after cb returns.
func (e *EngineTx) AccessWorldState(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	if e.writeTx != nil {
		return e.writeTx.AccessWorldState(ctx, ref, cb)
	}

	// A read snapshot reads from the head it holds, without the Engine lock
	// every reader would otherwise queue on for each object. The read holds
	// the head so a concurrent detach cannot release it mid-clone.
	if held := e.readHead.Load(); held != nil && !e.rel.Load() && held.tryHold() {
		defer held.release()
		root := held.root.Clone()
		defer root.Release()
		return e.engine.accessFrom(ctx, root, ref, func(cursor *bucket_lookup.Cursor) error {
			setReadOnlyCursor(ctx, cursor)
			return cb(cursor)
		})
	}
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
) (seqno uint64, sysErr bool, err error) {
	err = e.performOp(ctx, func(tx *Tx) error {
		seqno, sysErr, err = tx.ApplyWorldOp(ctx, op, opSender)
		return err
	})
	return seqno, sysErr, err
}

// CreateObject creates an object with a key and initial root ref.
// Returns ErrObjectExists if the object already exists.
func (e *EngineTx) CreateObject(ctx context.Context, key string, rootRef *bucket.ObjectRef) (world.ObjectState, error) {
	obj, err := performOpValue(ctx, e, func(tx *Tx) (world.ObjectState, error) {
		return tx.CreateObject(ctx, key, rootRef)
	})
	if err != nil {
		world.ReleaseObjectState(obj)
		return nil, err
	}
	return newEngineTxObjectState(e, key, obj), nil
}

// GetObject looks up an object by key.
// Returns nil, false if not found.
func (e *EngineTx) GetObject(ctx context.Context, key string) (obj world.ObjectState, found bool, err error) {
	// Look up the object in the transaction.
	err = e.performOp(ctx, func(tx *Tx) error {
		obj, found, err = tx.GetObject(ctx, key)
		return err
	})
	if err != nil || !found {
		return nil, found, err
	}

	// Keep the handle only for a write transaction: a read snapshot's Tx can
	// be replaced, so its object handles load the object for each call.
	if e.writeTx == nil {
		world.ReleaseObjectState(obj)
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
	return &engineTxObjectIterator{e: e, ctx: ctx, prefix: prefix, reversed: reversed}
}

// DeleteObject deletes an object and associated graph quads by ID.
// Calls DeleteGraphObject internally.
// Returns false, nil if not found.
func (e *EngineTx) DeleteObject(ctx context.Context, key string) (bool, error) {
	return performOpValue(ctx, e, func(tx *Tx) (bool, error) {
		return tx.DeleteObject(ctx, key)
	})
}

// PurgeObject removes the object entry and its graph quads without reading
// the object's record. Returns false, nil if not found.
func (e *EngineTx) PurgeObject(ctx context.Context, key string) (bool, error) {
	return performOpValue(ctx, e, func(tx *Tx) (bool, error) {
		return tx.PurgeObject(ctx, key)
	})
}

// RenameObject renames an object key and updates associated graph quads.
func (e *EngineTx) RenameObject(ctx context.Context, oldKey, newKey string, descendants bool) (world.ObjectState, error) {
	obj, err := performOpValue(ctx, e, func(tx *Tx) (world.ObjectState, error) {
		return tx.RenameObject(ctx, oldKey, newKey, descendants)
	})
	if err != nil {
		world.ReleaseObjectState(obj)
		return nil, err
	}
	return newEngineTxObjectState(e, newKey, obj), nil
}

// AccessCayleyGraph calls a callback with a temporary Cayley graph handle.
// All accesses of the handle should complete before returning cb.
// Try to make access (queries) as short as possible.
// Write operations will fail if the store is read-only.
func (e *EngineTx) AccessCayleyGraph(ctx context.Context, write bool, cb func(ctx context.Context, h world.CayleyHandle) error) error {
	return e.performOp(ctx, func(tx *Tx) error {
		return tx.AccessCayleyGraph(ctx, write, cb)
	})
}

// LookupGraphQuads searches for graph quads in the store.
func (e *EngineTx) LookupGraphQuads(ctx context.Context, filter world.GraphQuad, limit uint32) ([]world.GraphQuad, error) {
	return performOpValue(ctx, e, func(tx *Tx) ([]world.GraphQuad, error) {
		return tx.LookupGraphQuads(ctx, filter, limit)
	})
}

// LookupGraphQuadsBatch searches for graph quads for each filter in one transaction operation.
func (e *EngineTx) LookupGraphQuadsBatch(ctx context.Context, filters []world.GraphQuad, limitPerFilter uint32) ([][]world.GraphQuad, error) {
	return performOpValue(ctx, e, func(tx *Tx) ([][]world.GraphQuad, error) {
		return tx.LookupGraphQuadsBatch(ctx, filters, limitPerFilter)
	})
}

// QueryGraphPath executes a bounded graph traversal.
func (e *EngineTx) QueryGraphPath(ctx context.Context, query *world.GraphPathQuery) (*world.GraphPathQueryResult, error) {
	return performOpValue(ctx, e, func(tx *Tx) (*world.GraphPathQueryResult, error) {
		return tx.QueryGraphPath(ctx, query)
	})
}

// ListObjectsWithType reads one typed subject page through the transaction.
func (e *EngineTx) ListObjectsWithType(ctx context.Context, typeID, startAfter string, limit uint32) (keys []string, more bool, err error) {
	err = e.performOp(ctx, func(tx *Tx) error {
		keys, more, err = tx.ListObjectsWithType(ctx, typeID, startAfter, limit)
		return err
	})
	return keys, more, err
}

// SetGraphQuad sets a quad in the graph store.
// Subject: must be an existing object IRI: <object-key>
// Predicate: a predicate string, e.x. IRI: <ref>
// Object: an existing object IRI: <object-key>
// If already exists, returns nil.
func (e *EngineTx) SetGraphQuad(ctx context.Context, q world.GraphQuad) error {
	return e.performOp(ctx, func(tx *Tx) error {
		return tx.SetGraphQuad(ctx, q)
	})
}

// DeleteGraphQuad deletes a quad from the graph store.
// Note: if quad did not exist, returns nil.
func (e *EngineTx) DeleteGraphQuad(ctx context.Context, q world.GraphQuad) error {
	return e.performOp(ctx, func(tx *Tx) error {
		return tx.DeleteGraphQuad(ctx, q)
	})
}

// DeleteGraphObject deletes all quads with Subject or Object set to value.
// May also remove objects with <predicate> or <value> set to the value.
func (e *EngineTx) DeleteGraphObject(ctx context.Context, value string) error {
	return e.performOp(ctx, func(tx *Tx) error {
		return tx.DeleteGraphObject(ctx, value)
	})
}

// HasObject reports whether an object exists at key. It routes through the
// transaction like GetObject, so a write transaction can answer from its
// transaction-local object memo.
func (e *EngineTx) HasObject(ctx context.Context, key string) (bool, error) {
	return performOpValue(ctx, e, func(tx *Tx) (bool, error) {
		return tx.HasObject(ctx, key)
	})
}

// performOp preserves the read revision across storage retries and invalidates
// coordinated writes whose backing snapshot is stale.
func (e *EngineTx) performOp(ctx context.Context, cb func(tx *Tx) error) error {
	if e.writeTx != nil {
		err := cb(e.writeTx)
		if isCoordinatedWriteSnapshotError(err) && e.engine.authority != nil {
			// Another writer remapped the storage under the base: rebase.
			e.Discard()
			return stderrors.Join(errors.Wrap(world.ErrSuperseded, "coordinated write snapshot"), err)
		}
		return err
	}

	// Reopening storage preserves the reader's immutable World root. A concurrent
	// refresh can retire the captured transaction, so retry against its replacement.
	var err error
	for tries := 0; tries <= maxEngineTxTries; tries++ {
		reader := e.readTx.Load()
		if e.rel.Load() || reader == nil {
			return e.endedErr()
		}
		err = cb(reader)
		if errors.Is(err, tx.ErrDiscarded) {
			continue
		}
		if !isCoordinatedWriteSnapshotError(err) {
			return err
		}
		if refreshErr := e.refreshReadSnapshot(ctx, reader); refreshErr != nil {
			return refreshErr
		}
	}
	return err
}

// refreshReadSnapshot reopens storage for the same revision after invalidation.
func (e *EngineTx) refreshReadSnapshot(ctx context.Context, previous *Tx) error {
	// Reopen storage at the transaction's captured read root.
	locked := e.engine.bcast.Lock()
	if e.engine.closed || e.rel.Load() {
		locked.Unlock()
		return e.endedErr()
	}
	if e.readTx.Load() != previous {
		locked.Unlock()
		return nil
	}
	root, err := e.engine.baseRoot.FollowRef(ctx, e.readRoot)
	if err != nil {
		locked.Unlock()
		return err
	}
	state, err := e.engine.buildWorldStateForRoot(ctx, true, root, nil)
	root.Release()
	if err != nil {
		locked.Unlock()
		return err
	}

	// Swap the reader under snapMu, which a concurrent Discard detaches it
	// under, unless that Discard already ran.
	e.engine.snapMu.Lock()
	if _, ok := e.engine.snapshotTxs[e]; !ok || e.readTx.Load() != previous {
		e.engine.snapMu.Unlock()
		locked.Unlock()
		state.Discard()
		return nil
	}

	// Retire the replaced reader and the head it held outside the Engine
	// lock. The held head belongs to the storage it closes, so later object
	// reads go through the Engine.
	retirement := e.engine.beginRetirementLocked(engineRetirement{readTx: previous, readHead: e.readHead.Swap(nil)})
	e.readTx.Store(NewTx(state))
	e.engine.snapMu.Unlock()
	locked.Unlock()
	e.engine.drainRetirement(ctx, retirement)
	return nil
}

// performOpValue runs fn through performOp and returns the value of its last
// attempt.
func performOpValue[T any](ctx context.Context, e *EngineTx, fn func(tx *Tx) (T, error)) (T, error) {
	var v T
	err := e.performOp(ctx, func(tx *Tx) error {
		var err error
		v, err = fn(tx)
		return err
	})
	return v, err
}

// isCoordinatedWriteSnapshotError reports whether err says the storage
// snapshot under the transaction was invalidated by another writer.
func isCoordinatedWriteSnapshotError(err error) bool {
	return err != nil && errors.Is(err, kvtx.ErrInvalidSnapshot)
}
