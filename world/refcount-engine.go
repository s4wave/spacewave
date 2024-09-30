package world

import (
	"context"

	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/refcount"
)

// RefCountEngine is an engine backed by a reference counter.
type RefCountEngine struct {
	// rc contains the engine reference counter
	rc *refcount.RefCount[*Engine]
}

// NewRefCountEngine constructs a new refcount engine.
//
// keepUnref sets if the engine should be kept if there are zero references.
// ctx is used to resolve the value when a reference is added.
// ctx can be nil and updated with SetContext or ClearContext
func NewRefCountEngine(ctx context.Context, keepUnref bool, resolver EngineResolver) *RefCountEngine {
	return NewRefCountEngineWithCtr(ctx, keepUnref, resolver, nil, nil)
}

// NewRefCountEngineWithCtr builds a new refcount engine that stores the engine
// in the target ccontainers. Either of the ccontainers can be nil.
//
// keepUnref sets if the engine should be kept if there are zero references.
func NewRefCountEngineWithCtr(
	ctx context.Context,
	keepUnref bool,
	resolver EngineResolver,
	target *ccontainer.CContainer[*Engine],
	targetErr *ccontainer.CContainer[*error],
) *RefCountEngine {
	return &RefCountEngine{
		rc: refcount.NewRefCount(ctx, keepUnref, target, targetErr, resolver),
	}
}

// SetContext updates the context used for fetching the bus engine.
//
// Can be nil to prevent / cancel looking up the engine.
func (e *RefCountEngine) SetContext(ctx context.Context) {
	e.rc.SetContext(ctx)
}

// ClearContext clears the context used for fetching the bus engine.
func (e *RefCountEngine) ClearContext() {
	e.rc.ClearContext()
}

// NewTransaction returns a new transaction against the store.
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
// Check GetReadOnly, might not return a write tx if write=true.
func (e *RefCountEngine) NewTransaction(ctx context.Context, write bool) (Tx, error) {
	engine, ref, err := e.rc.Wait(ctx)
	if err != nil {
		return nil, err
	}
	tx, err := (*engine).NewTransaction(ctx, write)
	if err != nil {
		ref.Release()
		return nil, err
	}
	return NewRefCountTx(tx, ref), nil
}

// BuildStorageCursor builds a cursor to the world storage with an empty ref.
// The cursor should be released independently of the WorldState.
// Be sure to call Release on the cursor when done.
func (e *RefCountEngine) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	var bls *bucket_lookup.Cursor
	err := e.rc.Access(ctx, func(ctx context.Context, val *Engine) error {
		var err error
		bls, err = (*val).BuildStorageCursor(ctx)
		return err
	})
	if err != nil {
		if bls != nil {
			bls.Release()
		}
		return nil, err
	}
	return bls, nil
}

// AccessWorldState builds a bucket lookup cursor with an optional ref.
// If the ref Bucket ID is empty, uses the same bucket + volume as the world.
// The lookup cursor will be released after cb returns.
func (e *RefCountEngine) AccessWorldState(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	return e.rc.Access(ctx, func(ctx context.Context, val *Engine) error {
		return (*val).AccessWorldState(ctx, ref, cb)
	})
}

// GetSeqno returns the current seqno of the world state.
// This is also the sequence number of the most recent change.
// Initializes at 0 for initial world state.
func (e *RefCountEngine) GetSeqno(ctx context.Context) (uint64, error) {
	tx, err := e.NewTransaction(ctx, false)
	if err != nil {
		return 0, err
	}
	defer tx.Discard()

	return tx.GetSeqno(ctx)
}

// WaitSeqno waits for the seqno of the world state to be >= value.
// Returns nil when the condition is reached.
// If value == 0, this might return immediately unconditionally.
func (e *RefCountEngine) WaitSeqno(ctx context.Context, value uint64) (uint64, error) {
	var seqno uint64
	err := e.rc.Access(ctx, func(ctx context.Context, val *Engine) error {
		var err error
		seqno, err = (*val).WaitSeqno(ctx, value)
		return err
	})
	return seqno, err
}

// _ is a type assertion
var _ Engine = ((*RefCountEngine)(nil))
