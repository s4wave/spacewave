package world

import (
	"context"

	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/net/peer"
)

// ReadTrackingEngine records transactions opened with a view's context into
// that evaluation's reads. Wrap the engine before passing it to clients that
// open their own transactions inside WatchView or SendViews.
type ReadTrackingEngine struct {
	// inner supplies transactions, storage, and change notifications.
	inner Engine
}

// NewReadTrackingEngine constructs a tracking engine without opening a transaction.
func NewReadTrackingEngine(engine Engine) *ReadTrackingEngine {
	return &ReadTrackingEngine{inner: engine}
}

// NewTransaction records reads in the context's evaluation, when one is active.
// Commit and Discard retain the underlying transaction's contract.
func (e *ReadTrackingEngine) NewTransaction(ctx context.Context, write bool) (Tx, error) {
	// Open the transaction before attaching the evaluation's read set.
	t, err := e.inner.NewTransaction(ctx, write)
	if err != nil {
		return nil, err
	}

	// Leave transactions outside a view unchanged.
	reads := getViewReads(ctx)
	if reads == nil {
		return t, nil
	}
	return TrackTx(t, reads.reads), nil
}

// OperationAuthor returns the underlying engine's signing device and person.
func (e *ReadTrackingEngine) OperationAuthor(ctx context.Context) (peer.ID, string, error) {
	return e.inner.OperationAuthor(ctx)
}

// Sync fences writes through the underlying engine.
func (e *ReadTrackingEngine) Sync(ctx context.Context) (bool, error) {
	return e.inner.Sync(ctx)
}

// SyncThrough fences writes through seqno via the underlying engine.
func (e *ReadTrackingEngine) SyncThrough(ctx context.Context, seqno uint64) (uint64, error) {
	return e.inner.SyncThrough(ctx, seqno)
}

// BuildStorageCursor builds an empty cursor through the underlying engine.
func (e *ReadTrackingEngine) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	return e.inner.BuildStorageCursor(ctx)
}

// AccessWorldState records a root read whose dependencies are otherwise opaque.
func (e *ReadTrackingEngine) AccessWorldState(ctx context.Context, ref *bucket.ObjectRef, cb func(*bucket_lookup.Cursor) error) error {
	// A root read can depend on any World record; immutable refs cannot change.
	if reads := getViewReads(ctx); reads != nil && ref.GetEmpty() {
		reads.reads.markAll()
	}

	// Keep cursor ownership with the underlying engine.
	return e.inner.AccessWorldState(ctx, ref, cb)
}

// StageWorldState records an opaque storage access and opens its staging scope.
func (e *ReadTrackingEngine) StageWorldState(ctx context.Context) (WorldStage, error) {
	// Staging can read any World record.
	if reads := getViewReads(ctx); reads != nil {
		reads.reads.markAll()
	}

	// Keep the stage's lifetime unchanged.
	return e.inner.StageWorldState(ctx)
}

// GetSeqno returns the current World seqno.
func (e *ReadTrackingEngine) GetSeqno(ctx context.Context) (uint64, error) {
	return e.inner.GetSeqno(ctx)
}

// WaitSeqno waits through the underlying engine.
func (e *ReadTrackingEngine) WaitSeqno(ctx context.Context, seqno uint64) (uint64, error) {
	return e.inner.WaitSeqno(ctx, seqno)
}

// WaitObjectRev waits through the underlying engine's object watch.
func (e *ReadTrackingEngine) WaitObjectRev(ctx context.Context, key string, rev uint64, ignoreNotFound bool) (uint64, error) {
	return e.inner.WaitObjectRev(ctx, key, rev, ignoreNotFound)
}

// WatchChanges forwards the underlying change feed without replacing it.
func (e *ReadTrackingEngine) WatchChanges(ctx context.Context, afterSeqno uint64, touches ChangeFilter, cb func(uint64, *ChangeSet) error) error {
	return WatchTouchingChanges(ctx, e.inner, afterSeqno, touches, cb)
}

// WatchIndexedChanges forwards the underlying indexed change feed.
func (e *ReadTrackingEngine) WatchIndexedChanges(ctx context.Context, afterSeqno uint64, interest *ChangeInterest, cb func(uint64, *ChangeSet) error) error {
	return WatchInterestedChanges(ctx, e.inner, afterSeqno, interest, cb)
}

// _ checks the tracking engine's public contracts.
var (
	_ Engine               = (*ReadTrackingEngine)(nil)
	_ IndexedChangeWatcher = (*ReadTrackingEngine)(nil)
)
