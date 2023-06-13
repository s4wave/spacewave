package world_block

import (
	"context"
	"sync"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/tx"
	"github.com/aperturerobotics/hydra/world"
)

// Tx implements the hydra world transaction interface.
// uses a mutex for concurrent-safe calls
type Tx struct {
	// state is the underlying root state object
	state *WorldState
	// rmtx guards the world operations, single-writer multi-reader
	// not used for WaitSeqno
	rmtx sync.RWMutex
	// discarded indicates the tx was discarded already
	discarded bool
}

// NewTx constructs a new transaction from a world state.
// Guards the calls with a RWMutex (concurrency safe).
// Prevents operations after the Tx was discarded or committed.
func NewTx(state *WorldState) *Tx {
	return &Tx{state: state}
}

// Fork forks the current tx into a completely separate tx.
//
// Creates a new block transaction.
func (t *Tx) Fork(ctx context.Context) (world.WorldState, error) {
	t.rmtx.Lock()
	defer t.rmtx.Unlock()

	if t.discarded {
		return nil, tx.ErrDiscarded
	}

	forkedState, err := t.state.Fork(ctx)
	if err != nil {
		return nil, err
	}
	return NewTx(forkedState.(*WorldState)), nil
}

// GetReadOnly returns if the tx is read-only.
func (t *Tx) GetReadOnly() bool {
	return t.state.GetReadOnly()
}

// GetSeqno returns the current seqno of the world state.
// This is also the sequence number of the most recent change.
// Initializes at 0 for initial world state.
func (t *Tx) GetSeqno(ctx context.Context) (uint64, error) {
	t.rmtx.RLock()
	defer t.rmtx.RUnlock()

	return t.state.GetSeqno(ctx)
}

// WaitSeqno waits for the seqno of the world state to be >= value.
// Returns the seqno when the condition is reached.
// If value == 0, this might return immediately unconditionally.
func (t *Tx) WaitSeqno(ctx context.Context, value uint64) (uint64, error) {
	return t.state.WaitSeqno(ctx, value)
}

// BuildStorageCursor builds a cursor to the world storage with an empty ref.
// The cursor should be released independently of the WorldState.
// Be sure to call Release on the cursor when done.
func (t *Tx) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	return t.state.BuildStorageCursor(ctx)
}

// AccessWorldState builds a bucket lookup cursor with an optional ref.
// If the ref is empty, returns empty cursor in the same bucket + volume as the world.
// The lookup cursor will be released after cb returns.
func (t *Tx) AccessWorldState(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	return t.state.AccessWorldState(ctx, ref, cb)
}

// ApplyWorldOp applies a batch operation at the world level.
// The handling of the operation is operation-type specific.
// Returns the seqno following the operation execution.
// If nil is returned for the error, implies success.
func (t *Tx) ApplyWorldOp(
	ctx context.Context,
	op world.Operation,
	opSender peer.ID,
) (uint64, bool, error) {
	t.rmtx.Lock()
	defer t.rmtx.Unlock()

	if t.discarded {
		return 0, false, tx.ErrDiscarded
	}

	return t.state.ApplyWorldOp(ctx, op, opSender)
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
func (t *Tx) Commit(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return context.Canceled
	}
	t.rmtx.Lock()
	discarded := t.discarded
	var err error
	if !discarded {
		t.discarded = true
		err = t.state.Commit(ctx)
	}
	t.rmtx.Unlock()
	if discarded {
		return tx.ErrDiscarded
	}
	return err
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
func (t *Tx) Discard() {
	t.rmtx.Lock()
	discarded := t.discarded
	if !discarded {
		t.discarded = true
	}
	t.rmtx.Unlock()
}

// _ is a type assertion
var (
	_ world.ForkableWorldState = (*Tx)(nil)
	_ world.WorldState         = (*Tx)(nil)
	_ world.Tx                 = (*Tx)(nil)
)
