package block_store_writeback

import (
	"context"
	"slices"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

// SupportsRootRetention reports the inner store's root ownership capability.
// Retiring a local root lets the collector sweep superseded blocks; a marker
// whose block was swept is dropped without upload.
func (m *MarkingStore) SupportsRootRetention() bool {
	return block.SupportsRootRetention(m.store)
}

// KeepsHoldCounts reports whether the inner store keeps shared counts.
func (m *MarkingStore) KeepsHoldCounts() bool { return block.KeepsHoldCounts(m.store) }

// SetRetainedRoots forwards durable root ownership to the inner store and
// marks the blocks it wrote.
func (m *MarkingStore) SetRetainedRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	// Hold the write open until the observer has seen it.
	defer m.ops.end(m.ops.begin(false))

	// Mark in the storing transaction.
	if m.marker != nil {
		ctx, req := kvtx.WithBlockMarker(ctx, m.marker)
		if err := block.SetRetainedRoots(ctx, m.store, entries, roots); err != nil {
			return err
		}
		m.observeEntries(entries, roots)
		if !req.Applied() && slices.ContainsFunc(entries, isMarked) {
			return errUnmarkedWrite
		}
		m.invalidateTombstones(ctx, entries)
		return nil
	}

	// Mark after the write without a marker.
	if err := block.SetRetainedRoots(ctx, m.store, entries, roots); err != nil {
		return err
	}
	m.observeEntries(entries, roots)
	return m.markEntries(ctx, entries)
}

// PinRoot forwards a reader pin to the inner store, then lets the observer see
// the pinned root. A sweep that lists the volume's pins after its log position
// R lists a pin held then; one the observer saw before R was held before R, so
// the listing finds it while it is held, and one seen after R is in the log
// after R. Observing before the pin would leave a pin seen before R and set
// after the listing in neither.
func (m *MarkingStore) PinRoot(ctx context.Context, ref *block.BlockRef) (func(), error) {
	// Pin the root first.
	release, err := block.PinRoot(ctx, m.store, ref)
	if err != nil {
		return nil, err
	}

	// Then log it as a root.
	if o := m.observer.load(); o != nil && !ref.GetEmpty() {
		o.Root(ref.GetHash())
	}
	return release, nil
}

// OpenStage wraps a stage opened on the inner store, so staged writes are
// marked for upload like any other write. The stage is open in the record of
// read operations until its release, as it reads through its own view.
func (m *MarkingStore) OpenStage(ctx context.Context) (block.StoreOps, func(), error) {
	// Record the stage as a read operation, then open it.
	n := m.ops.begin(true)
	store, release, err := block.OpenStage(ctx, m.store)
	if err != nil {
		m.ops.end(n)
		return nil, nil, err
	}
	return m.copyOver(store), m.ops.releaseRead(n, release), nil
}

// ReleaseRoots forwards staging release to the underlying store.
func (m *MarkingStore) ReleaseRoots(ctx context.Context, refs []*block.BlockRef) error {
	return block.ReleaseRoots(ctx, m.store, refs)
}

// MarkRootsComplete forwards the durable World proof.
func (m *MarkingStore) MarkRootsComplete(ctx context.Context, roots []*block.BlockRef) error {
	return block.MarkRootsComplete(ctx, m.store, roots)
}

// RootsComplete checks the inner store's World proofs.
func (m *MarkingStore) RootsComplete(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return block.RootsComplete(ctx, m.store, refs)
}

// HoldCompleteRoots holds completion proofs under the inner stage. A hold
// writes no block, so there is nothing to mark.
func (m *MarkingStore) HoldCompleteRoots(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return block.HoldCompleteRoots(ctx, m.store, refs)
}

// _ is a type assertion
var (
	_ block.RootRetainer       = (*MarkingStore)(nil)
	_ block.CompleteRootHolder = (*MarkingStore)(nil)
)
