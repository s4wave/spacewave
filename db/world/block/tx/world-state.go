package world_block_tx

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
)

// WorldState implements a WorldState which tracks applied operations as a TxBatch.
type WorldState struct {
	// ctx is the world state context
	ctx context.Context
	// world is the temporary write world
	world world.WorldState
	// write indicates if the world state allows writes
	write bool

	// mtx guards below fields
	mtx sync.Mutex
	// discarded indicates the state is discarded
	discarded bool
	// txBatch is the batch of applied txs so far
	txBatch *TxBatch
}

// NewWorldState constructs a new world state without forking it.
func NewWorldState(ctx context.Context, world world.WorldState, write bool) (*WorldState, error) {
	return &WorldState{
		ctx:     ctx,
		world:   world,
		write:   write,
		txBatch: &TxBatch{},
	}, nil
}

// ForkWorldState forks a world state and constructs a write tx.
//
// Note: this shares the same block transaction, careful not to commit/discard it too soon.
func ForkWorldState(ctx context.Context, world world.ForkableWorldState, write bool) (*WorldState, error) {
	// fork the world -> write world
	// note: this uses the same block transaction
	forkedState, err := world.Fork(ctx)
	if err != nil {
		return nil, err
	}
	return NewWorldState(ctx, forkedState, write)
}

// GetReadOnly returns if the state is read-only.
func (w *WorldState) GetReadOnly() bool {
	return !w.write
}

// GetSeqno returns the current seqno of the world state.
// This is also the sequence number of the most recent change.
// Initializes at 0 for initial world state.
// Note: this will be an estimate ONLY of the final seqno.
func (w *WorldState) GetSeqno(ctx context.Context) (uint64, error) {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	return w.world.GetSeqno(ctx)
}

// WaitSeqno waits for the seqno of the world state to be >= value.
// Returns the seqno when the condition is reached.
// If value == 0, this might return immediately unconditionally.
func (w *WorldState) WaitSeqno(ctx context.Context, value uint64) (uint64, error) {
	return w.world.WaitSeqno(ctx, value)
}

// BuildStorageCursor builds a cursor to the world storage with an empty ref.
// The cursor should be released independently of the WorldState.
// Be sure to call Release on the cursor when done.
func (w *WorldState) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	return w.world.BuildStorageCursor(ctx)
}

// AccessWorldState builds a bucket lookup cursor with an optional ref.
// If the ref is empty, returns empty cursor in the same bucket + volume as the world.
// The lookup cursor will be released after cb returns.
func (w *WorldState) AccessWorldState(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	return w.world.AccessWorldState(ctx, ref, cb)
}

// ApplyWorldOp applies a batch operation at the world level.
// The handling of the operation is operation-type specific.
// Returns the seqno following the operation execution.
// If nil is returned for the error, implies success.
func (w *WorldState) ApplyWorldOp(
	ctx context.Context,
	op world.Operation,
	opSender peer.ID,
) (uint64, bool, error) {
	if !w.write {
		return 0, false, tx.ErrNotWrite
	}

	t, err := NewTxApplyWorldOp(op)
	if err != nil {
		return 0, false, err
	}

	w.mtx.Lock()
	defer w.mtx.Unlock()

	if w.discarded {
		return 0, false, tx.ErrDiscarded
	}

	seqno, sysErr, err := w.world.ApplyWorldOp(ctx, op, opSender)
	if err == nil {
		w.txBatch.Txs = append(w.txBatch.Txs, t)
	}

	return seqno, sysErr, err
}

// GetObject looks up an object by key.
// Returns nil, false if not found.
func (w *WorldState) GetObject(ctx context.Context, key string) (world.ObjectState, bool, error) {
	w.mtx.Lock()
	defer w.mtx.Unlock()

	if w.discarded {
		return nil, false, tx.ErrDiscarded
	}

	objs, objsFound, err := w.world.GetObject(ctx, key)
	if err != nil || !objsFound {
		return nil, false, err
	}
	return NewObjectState(w, key, objs), true, nil
}

// IterateObjects returns an iterator with the given object key prefix.
// The prefix is NOT clipped from the output keys.
// Keys are returned in sorted order.
// Must call Next() or Seek() before valid.
// Call Close when done with the iterator.
// Any init errors will be available via the iterator's Err() method.
func (w *WorldState) IterateObjects(ctx context.Context, prefix string, reversed bool) world.ObjectIterator {
	return NewObjectIterator(w, ctx, prefix, reversed)
}

// CreateObject creates a object with a key and initial root ref.
func (w *WorldState) CreateObject(ctx context.Context, key string, rootRef *bucket.ObjectRef) (world.ObjectState, error) {
	if !w.write {
		return nil, tx.ErrNotWrite
	}

	t, err := NewTxCreateObject(key, rootRef)
	if err != nil {
		return nil, err
	}

	w.mtx.Lock()
	defer w.mtx.Unlock()

	if w.discarded {
		return nil, tx.ErrDiscarded
	}

	obj, err := w.world.CreateObject(ctx, key, rootRef)
	if err != nil {
		return nil, err
	}

	w.txBatch.Txs = append(w.txBatch.Txs, t)
	return NewObjectState(w, key, obj), nil
}

// RenameObject renames an object key and updates associated graph quads.
func (w *WorldState) RenameObject(ctx context.Context, oldKey, newKey string, descendants bool) (world.ObjectState, error) {
	if !w.write {
		return nil, tx.ErrNotWrite
	}

	w.mtx.Lock()
	defer w.mtx.Unlock()

	if w.discarded {
		return nil, tx.ErrDiscarded
	}

	renames, err := collectObjectRenames(ctx, w.world, oldKey, newKey, descendants)
	if err != nil {
		return nil, err
	}

	obj, err := w.world.RenameObject(ctx, oldKey, newKey, descendants)
	if err != nil {
		return nil, err
	}

	for _, rename := range renames {
		t, err := NewTxRenameObject(rename.oldKey, rename.newKey)
		if err != nil {
			return nil, err
		}
		w.txBatch.Txs = append(w.txBatch.Txs, t)
	}
	return NewObjectState(w, newKey, obj), nil
}

func collectObjectRenames(ctx context.Context, ws world.WorldStateObject, oldKey, newKey string, descendants bool) ([]objectRename, error) {
	renames := []objectRename{{oldKey: oldKey, newKey: newKey}}
	if !descendants || oldKey == newKey {
		return renames, nil
	}

	iter := ws.IterateObjects(ctx, oldKey+"/", false)
	defer iter.Close()
	for iter.Next() {
		key := iter.Key()
		next, ok := rewriteObjectKeyPrefix(key, oldKey, newKey)
		if !ok {
			continue
		}
		renames = append(renames, objectRename{oldKey: key, newKey: next})
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	slices.SortFunc(renames, func(a, b objectRename) int {
		return len(a.oldKey) - len(b.oldKey)
	})
	return renames, nil
}

func rewriteObjectKeyPrefix(key, oldKey, newKey string) (string, bool) {
	if key == oldKey {
		return newKey, true
	}
	prefix := oldKey + "/"
	if !strings.HasPrefix(key, prefix) {
		return key, false
	}
	return newKey + key[len(oldKey):], true
}

type objectRename struct {
	oldKey string
	newKey string
}

// DeleteObject deletes an object and associated graph quads by ID.
// Calls DeleteGraphObject internally.
// Returns false, nil if not found.
func (w *WorldState) DeleteObject(ctx context.Context, key string) (bool, error) {
	if !w.write {
		return false, tx.ErrNotWrite
	}

	t, err := NewTxDeleteObject(key)
	if err != nil {
		return false, err
	}

	w.mtx.Lock()
	defer w.mtx.Unlock()

	if w.discarded {
		return false, tx.ErrDiscarded
	}

	deleted, err := w.world.DeleteObject(ctx, key)
	if err != nil || !deleted {
		return false, err
	}

	w.txBatch.Txs = append(w.txBatch.Txs, t)
	return true, nil
}

// AccessCayleyGraph calls a callback with a temporary Cayley graph handle.
// All accesses of the handle should complete before returning cb.
// Try to make access (queries) as short as possible.
// Write operations will fail if the store is read-only.
func (w *WorldState) AccessCayleyGraph(ctx context.Context, write bool, cb func(ctx context.Context, h world.CayleyHandle) error) error {
	w.mtx.Lock()
	defer w.mtx.Unlock()

	if w.discarded {
		return tx.ErrDiscarded
	}

	// note: force write to false, we only allow ApplyObjectOp and ApplyWorldOp here.
	return w.world.AccessCayleyGraph(ctx, false, cb)
}

// LookupGraphQuads searches for graph quads in the store.
func (w *WorldState) LookupGraphQuads(ctx context.Context, filter world.GraphQuad, limit uint32) ([]world.GraphQuad, error) {
	w.mtx.Lock()
	defer w.mtx.Unlock()

	if w.discarded {
		return nil, tx.ErrDiscarded
	}

	return w.world.LookupGraphQuads(ctx, filter, limit)
}

// SetGraphQuad sets a quad in the graph store.
func (w *WorldState) SetGraphQuad(ctx context.Context, q world.GraphQuad) error {
	if !w.write {
		return tx.ErrNotWrite
	}

	t, err := NewTxSetGraphQuad(world.GraphQuadToQuad(q))
	if err != nil {
		return err
	}

	w.mtx.Lock()
	defer w.mtx.Unlock()

	if w.discarded {
		return tx.ErrDiscarded
	}

	if err := w.world.SetGraphQuad(ctx, q); err != nil {
		return err
	}

	w.txBatch.Txs = append(w.txBatch.Txs, t)
	return nil
}

// DeleteGraphQuad deletes a quad from the graph store.
// Note: if quad did not exist, returns nil.
func (w *WorldState) DeleteGraphQuad(ctx context.Context, q world.GraphQuad) error {
	if !w.write {
		return tx.ErrNotWrite
	}

	t, err := NewTxDeleteGraphQuad(world.GraphQuadToQuad(q))
	if err != nil {
		return err
	}

	w.mtx.Lock()
	defer w.mtx.Unlock()

	if w.discarded {
		return tx.ErrDiscarded
	}

	if err := w.world.DeleteGraphQuad(ctx, q); err != nil {
		return err
	}

	w.txBatch.Txs = append(w.txBatch.Txs, t)
	return nil
}

// DeleteGraphObject deletes all quads with Subject or Object set to value.
// May also remove objects with <predicate> or <value> set to the value.
func (w *WorldState) DeleteGraphObject(ctx context.Context, value string) error {
	w.mtx.Lock()
	defer w.mtx.Unlock()

	if w.discarded {
		return tx.ErrDiscarded
	}

	return w.world.DeleteGraphObject(ctx, value)
}

// GetTxBatch returns the transaction batch.
// NOTE: call this after Commit or Discard!
func (w *WorldState) GetTxBatch() *TxBatch {
	w.mtx.Lock()
	defer w.mtx.Unlock()

	return w.txBatch
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
func (w *WorldState) Commit(ctx context.Context) error {
	w.mtx.Lock()
	defer w.mtx.Unlock()

	if w.discarded {
		return tx.ErrDiscarded
	}

	w.discarded = true
	return nil
}

// Discard cancels the transaction and discards all txs.
func (w *WorldState) Discard() {
	// note: mark the tx as discarded
	w.mtx.Lock()
	defer w.mtx.Unlock()

	if !w.discarded {
		w.discarded = true
		w.txBatch.Txs = nil
	}
}

// _ is a type assertion
var _ world.Tx = ((*WorldState)(nil))
