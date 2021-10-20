package world_block_tx

import (
	"context"
	"sync"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/tx"
	"github.com/aperturerobotics/hydra/world"
)

// WorldState implements a WorldState backed by a read state & a forked write
// state. Buffers applied operations into TxBatch objects.
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
	// seqno is the current write seqno
	seqno uint64
}

// NewWorldState constructs a new world state without forking it.
func NewWorldState(ctx context.Context, world world.WorldState, write bool) (*WorldState, error) {
	var seqno uint64
	if write {
		var err error
		seqno, err = world.GetSeqno()
		if err != nil {
			return nil, err
		}
	}
	return &WorldState{
		ctx:     ctx,
		world:   world,
		write:   write,
		txBatch: &TxBatch{},
		seqno:   seqno,
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
func (w *WorldState) GetSeqno() (uint64, error) {
	w.mtx.Lock()
	readSeqno, err := w.world.GetSeqno()
	if err == nil {
		if readSeqno > w.seqno {
			w.seqno = readSeqno
		}
	}
	seqno := w.seqno
	w.mtx.Unlock()
	return seqno, err
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

	seqno, sysErr, err := w.world.ApplyWorldOp(op, opSender)
	if err == nil {
		w.txBatch.Txs = append(w.txBatch.Txs, t)
		if seqno > w.seqno {
			w.seqno = seqno
		} else {
			w.seqno++
			seqno = w.seqno
		}
	}
	return seqno, sysErr, err
}

// CreateObject creates a object with a key and initial root ref.
func (w *WorldState) CreateObject(key string, rootRef *bucket.ObjectRef) (world.ObjectState, error) {
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

	obj, err := w.world.CreateObject(key, rootRef)
	if err != nil {
		return nil, err
	}

	w.txBatch.Txs = append(w.txBatch.Txs, t)
	w.seqno++
	return NewObjectState(w, key, obj), nil
}

// GetObject looks up an object by key.
// Returns nil, false if not found.
func (w *WorldState) GetObject(key string) (world.ObjectState, bool, error) {
	w.mtx.Lock()
	defer w.mtx.Unlock()

	if w.discarded {
		return nil, false, tx.ErrDiscarded
	}

	objs, objsFound, err := w.world.GetObject(key)
	if err != nil || !objsFound {
		return nil, false, err
	}
	return NewObjectState(w, key, objs), true, nil
}

// DeleteObject deletes an object and associated graph quads by ID.
// Calls DeleteGraphObject internally.
// Returns false, nil if not found.
func (w *WorldState) DeleteObject(key string) (bool, error) {
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

	deleted, err := w.world.DeleteObject(key)
	if err != nil || !deleted {
		return false, err
	}

	w.txBatch.Txs = append(w.txBatch.Txs, t)
	w.seqno++
	return true, nil
}

// AccessCayleyGraph calls a callback with a temporary Cayley graph handle.
// All accesses of the handle should complete before returning cb.
// Try to make access (queries) as short as possible.
// Write operations will fail if the store is read-only.
func (w *WorldState) AccessCayleyGraph(write bool, cb func(h world.CayleyHandle) error) error {
	w.mtx.Lock()
	defer w.mtx.Unlock()

	if w.discarded {
		return tx.ErrDiscarded
	}

	// note: force write to false, we only allow ApplyObjectOp and ApplyWorldOp here.
	return w.world.AccessCayleyGraph(false, cb)
}

// LookupGraphQuads searches for graph quads in the store.
func (w *WorldState) LookupGraphQuads(filter world.GraphQuad, limit uint32) ([]world.GraphQuad, error) {
	w.mtx.Lock()
	defer w.mtx.Unlock()

	if w.discarded {
		return nil, tx.ErrDiscarded
	}

	return w.world.LookupGraphQuads(filter, limit)
}

// SetGraphQuad sets a quad in the graph store.
func (w *WorldState) SetGraphQuad(q world.GraphQuad) error {
	w.mtx.Lock()
	defer w.mtx.Unlock()

	if w.discarded {
		return tx.ErrDiscarded
	}

	return w.world.SetGraphQuad(q)
}

// DeleteGraphQuad deletes a quad from the graph store.
// Note: if quad did not exist, returns nil.
func (w *WorldState) DeleteGraphQuad(q world.GraphQuad) error {
	w.mtx.Lock()
	defer w.mtx.Unlock()

	if w.discarded {
		return tx.ErrDiscarded
	}

	return w.world.DeleteGraphQuad(q)
}

// DeleteGraphObject deletes all quads with Subject or Object set to value.
// May also remove objects with <predicate> or <value> set to the value.
// Returns number of removed quads and any error.
func (w *WorldState) DeleteGraphObject(value string) error {
	w.mtx.Lock()
	defer w.mtx.Unlock()

	if w.discarded {
		return tx.ErrDiscarded
	}

	return w.world.DeleteGraphObject(value)
}

// GetTxBatch returns the transaction batch.
func (w *WorldState) GetTxBatch() *TxBatch {
	w.mtx.Lock()
	b := &TxBatch{Txs: w.txBatch.GetTxs()}
	w.mtx.Unlock()
	return b
}

// GetTxBatchTx returns the transaction batch as a tx.
// May return nil if there are no tx in the batch.
func (w *WorldState) GetTxBatchTx() (*Tx, error) {
	return NewTxBatch(w.GetTxBatch())
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
