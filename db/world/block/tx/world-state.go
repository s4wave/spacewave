package world_block_tx

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/s4wave/spacewave/db/block"
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
	// payloads are the payload roots of the operations applied so far,
	// including failed ones
	payloads []*block.BlockRef
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

// Sync fences the block writes made through the underlying world state durable.
func (w *WorldState) Sync(ctx context.Context) (bool, error) {
	return w.world.Sync(ctx)
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

// GetObjectBodiesBatchPage returns one budgeted page from the wrapped state.
func (w *WorldState) GetObjectBodiesBatchPage(ctx context.Context, keys []string, byteBudget int) ([]*world.ObjectBody, uint32, error) {
	// Hold the WorldState lock while reading the page.
	w.mtx.Lock()
	defer w.mtx.Unlock()

	// Reject page reads after the WorldState is discarded.
	if w.discarded {
		return nil, 0, tx.ErrDiscarded
	}

	// Read the object page through the native reader when available.
	if pager, ok := w.world.(world.ObjectBodyPageBatcher); ok {
		return pager.GetObjectBodiesBatchPage(ctx, keys, byteBudget)
	}
	return world.GetObjectBodiesBatchPage(ctx, w.world, keys, byteBudget)
}

// GetObjectBodiesBatchPageWithSeqno returns one budgeted page and its transaction seqno.
func (w *WorldState) GetObjectBodiesBatchPageWithSeqno(ctx context.Context, keys []string, byteBudget int) ([]*world.ObjectBody, uint32, uint64, error) {
	// Hold the WorldState lock while reading the page and sequence number.
	w.mtx.Lock()
	defer w.mtx.Unlock()

	// Reject sequence-aware page reads after the WorldState is discarded.
	if w.discarded {
		return nil, 0, 0, tx.ErrDiscarded
	}

	// Use the native reader when it supplies the page and sequence number together.
	if pager, ok := w.world.(world.ObjectBodyPageSeqnoBatcher); ok {
		return pager.GetObjectBodiesBatchPageWithSeqno(ctx, keys, byteBudget)
	}

	// Read the wrapped World sequence number and its budgeted object page.
	seqno, err := w.world.GetSeqno(ctx)
	if err != nil {
		return nil, 0, 0, err
	}
	bodies, consumed, err := world.GetObjectBodiesBatchPage(ctx, w.world, keys, byteBudget)
	return bodies, consumed, seqno, err
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
	// Reject a read transaction.
	if !w.write {
		return 0, false, tx.ErrNotWrite
	}

	// Build the batch entry that records the op.
	t, err := NewTxApplyWorldOp(op, opSender)
	if err != nil {
		return 0, false, err
	}

	// Hold the transaction open while the op applies.
	w.mtx.Lock()
	defer w.mtx.Unlock()
	if w.discarded {
		return 0, false, tx.ErrDiscarded
	}

	// Own the op's payload even if it fails, then apply and record it.
	w.addPayloadsLocked(op)
	seqno, sysErr, err := w.world.ApplyWorldOp(ctx, op, opSender)
	if err == nil {
		w.txBatch.Txs = append(w.txBatch.Txs, t)
	}
	return seqno, sysErr, err
}

// GetObject looks up an object by key.
// Returns nil, false if not found.
func (w *WorldState) GetObject(ctx context.Context, key string) (world.ObjectState, bool, error) {
	// Hold the WorldState lock while looking up the object.
	w.mtx.Lock()
	defer w.mtx.Unlock()

	// Reject object access after the WorldState is discarded.
	if w.discarded {
		return nil, false, tx.ErrDiscarded
	}

	// Wrap the object handle for transaction recording and release failed lookups.
	objs, objsFound, err := w.world.GetObject(ctx, key)
	if err != nil || !objsFound {
		world.ReleaseObjectState(objs)
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
	// Require a writable WorldState before creating an object.
	if !w.write {
		return nil, tx.ErrNotWrite
	}

	// Prepare the object creation entry for the transaction batch.
	t, err := NewTxCreateObject(key, rootRef)
	if err != nil {
		return nil, err
	}

	// Hold the WorldState lock through object creation.
	w.mtx.Lock()
	defer w.mtx.Unlock()

	// Reject object creation after the WorldState is discarded.
	if w.discarded {
		return nil, tx.ErrDiscarded
	}

	// Create the underlying object and release a failed handle.
	obj, err := w.world.CreateObject(ctx, key, rootRef)
	if err != nil {
		world.ReleaseObjectState(obj)
		return nil, err
	}

	// Record the creation and return the transaction-aware object.
	w.txBatch.Txs = append(w.txBatch.Txs, t)
	return NewObjectState(w, key, obj), nil
}

// RenameObject renames an object key and updates associated graph quads.
func (w *WorldState) RenameObject(ctx context.Context, oldKey, newKey string, descendants bool) (world.ObjectState, error) {
	// Require a writable WorldState before renaming an object.
	if !w.write {
		return nil, tx.ErrNotWrite
	}

	// Hold the WorldState lock through object renaming.
	w.mtx.Lock()
	defer w.mtx.Unlock()

	// Reject object renaming after the WorldState is discarded.
	if w.discarded {
		return nil, tx.ErrDiscarded
	}

	// Collect the object keys that the rename will change.
	renames, err := collectObjectRenames(ctx, w.world, oldKey, newKey, descendants)
	if err != nil {
		return nil, err
	}

	// Rename the underlying object and release a failed handle.
	obj, err := w.world.RenameObject(ctx, oldKey, newKey, descendants)
	if err != nil {
		world.ReleaseObjectState(obj)
		return nil, err
	}

	// Record every renamed object in the transaction batch.
	for _, rename := range renames {
		t, err := NewTxRenameObject(rename.oldKey, rename.newKey)
		if err != nil {
			world.ReleaseObjectState(obj)
			return nil, err
		}
		w.txBatch.Txs = append(w.txBatch.Txs, t)
	}
	return NewObjectState(w, newKey, obj), nil
}

func collectObjectRenames(ctx context.Context, ws world.WorldStateObject, oldKey, newKey string, descendants bool) ([]objectRename, error) {
	// Start with the root object rename and stop when descendants are excluded.
	renames := []objectRename{{oldKey: oldKey, newKey: newKey}}
	if !descendants || oldKey == newKey {
		return renames, nil
	}

	// Open the descendant object iterator for the source key.
	iter := ws.IterateObjects(ctx, oldKey+"/", false)
	defer iter.Close()

	// Collect each descendant object key under its new prefix.
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

	// Order the rename entries so parents replay before children.
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
	// Require a writable WorldState before deleting an object.
	if !w.write {
		return false, tx.ErrNotWrite
	}

	// Prepare the object deletion entry for the transaction batch.
	t, err := NewTxDeleteObject(key)
	if err != nil {
		return false, err
	}

	// Hold the WorldState lock through object deletion.
	w.mtx.Lock()
	defer w.mtx.Unlock()

	// Reject object deletion after the WorldState is discarded.
	if w.discarded {
		return false, tx.ErrDiscarded
	}

	// Delete the underlying object and stop when it is absent.
	deleted, err := w.world.DeleteObject(ctx, key)
	if err != nil || !deleted {
		return false, err
	}

	// Record the successful object deletion in the transaction batch.
	w.txBatch.Txs = append(w.txBatch.Txs, t)
	return true, nil
}

// HasObject reports whether an object exists at key by forwarding to the wrapped
// state, which may answer from its transaction-local object memo.
func (w *WorldState) HasObject(ctx context.Context, key string) (bool, error) {
	w.mtx.Lock()
	defer w.mtx.Unlock()

	if w.discarded {
		return false, tx.ErrDiscarded
	}

	return w.world.HasObject(ctx, key)
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

// LookupGraphQuadsBatch searches for graph quads for each filter in one locked read.
func (w *WorldState) LookupGraphQuadsBatch(ctx context.Context, filters []world.GraphQuad, limitPerFilter uint32) ([][]world.GraphQuad, error) {
	w.mtx.Lock()
	defer w.mtx.Unlock()

	if w.discarded {
		return nil, tx.ErrDiscarded
	}

	return w.world.LookupGraphQuadsBatch(ctx, filters, limitPerFilter)
}

// QueryGraphPath executes a bounded graph traversal.
func (w *WorldState) QueryGraphPath(ctx context.Context, query *world.GraphPathQuery) (*world.GraphPathQueryResult, error) {
	w.mtx.Lock()
	defer w.mtx.Unlock()

	if w.discarded {
		return nil, tx.ErrDiscarded
	}

	return w.world.QueryGraphPath(ctx, query)
}

// SetGraphQuad sets a quad in the graph store.
func (w *WorldState) SetGraphQuad(ctx context.Context, q world.GraphQuad) error {
	// Require a writable WorldState before setting a graph quad.
	if !w.write {
		return tx.ErrNotWrite
	}

	// Prepare the graph quad insertion entry for the transaction batch.
	t, err := NewTxSetGraphQuad(world.GraphQuadToQuad(q))
	if err != nil {
		return err
	}

	// Hold the WorldState lock through graph quad insertion.
	w.mtx.Lock()
	defer w.mtx.Unlock()

	// Reject graph quad insertion after the WorldState is discarded.
	if w.discarded {
		return tx.ErrDiscarded
	}

	// Insert the quad into the underlying World graph.
	if err := w.world.SetGraphQuad(ctx, q); err != nil {
		return err
	}

	// Record the successful graph quad insertion in the transaction batch.
	w.txBatch.Txs = append(w.txBatch.Txs, t)
	return nil
}

// DeleteGraphQuad deletes a quad from the graph store.
// Note: if quad did not exist, returns nil.
func (w *WorldState) DeleteGraphQuad(ctx context.Context, q world.GraphQuad) error {
	// Require a writable WorldState before deleting a graph quad.
	if !w.write {
		return tx.ErrNotWrite
	}

	// Prepare the graph quad deletion entry for the transaction batch.
	t, err := NewTxDeleteGraphQuad(world.GraphQuadToQuad(q))
	if err != nil {
		return err
	}

	// Hold the WorldState lock through graph quad deletion.
	w.mtx.Lock()
	defer w.mtx.Unlock()

	// Reject graph quad deletion after the WorldState is discarded.
	if w.discarded {
		return tx.ErrDiscarded
	}

	// Delete the quad from the underlying World graph.
	if err := w.world.DeleteGraphQuad(ctx, q); err != nil {
		return err
	}

	// Record the successful graph quad deletion in the transaction batch.
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

// TakePayloadRefs returns the payload roots of the operations applied so far
// and forgets them. The caller releases them once the transaction's outcome is
// final; see world.PayloadOperation.
func (w *WorldState) TakePayloadRefs() []*block.BlockRef {
	// Hand the recorded roots to the caller.
	w.mtx.Lock()
	defer w.mtx.Unlock()
	payloads := w.payloads
	w.payloads = nil
	return payloads
}

// addPayloadsLocked records op's payload roots. Caller holds mtx.
func (w *WorldState) addPayloadsLocked(op world.Operation) {
	if pop, ok := op.(world.PayloadOperation); ok {
		w.payloads = append(w.payloads, pop.GetPayloadRefs()...)
	}
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
func (w *WorldState) Commit(ctx context.Context) error {
	// Hold the WorldState lock while closing the transaction.
	w.mtx.Lock()
	defer w.mtx.Unlock()

	// Reject a WorldState that has already been discarded.
	if w.discarded {
		return tx.ErrDiscarded
	}

	// Close the WorldState to further transaction operations.
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
var _ world.Tx = (*WorldState)(nil)
