package block_store_writeback

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// supportsAtomicPublication reports whether store publishes atomically.
func supportsAtomicPublication(store block.StoreOps) bool {
	publisher, ok := store.(block.AtomicPublisher)
	return ok && publisher.SupportsAtomicPublication()
}

// AtomicPublicationVolumeID returns the local store's publication domain.
func (s *PackedStore) AtomicPublicationVolumeID() string {
	if !supportsAtomicPublication(s.local) {
		return ""
	}
	return s.local.(block.AtomicPublisher).AtomicPublicationVolumeID()
}

// SupportsAtomicPublication reports whether the local store publishes
// atomically.
func (s *PackedStore) SupportsAtomicPublication() bool {
	return supportsAtomicPublication(s.local)
}

// SubmitAtomic appends the publication's values to the Log, then admits it to
// the local store with its entries external, so the Volume commits their
// child lists, holders and heads without the bytes. The Log keeps the copies
// of the blocks the publication tombstones: the publication commits after
// admission, or never, and a value kept past its tombstone only uploads as
// garbage.
func (s *PackedStore) SubmitAtomic(ctx context.Context, p *block.AtomicPublication) (*block.PublicationReceipt, error) {
	// Require a local store that publishes atomically.
	if !supportsAtomicPublication(s.local) {
		return nil, block.ErrAtomicPublicationUnsupported
	}
	publisher := s.local.(block.AtomicPublisher)
	if !s.divert {
		return publisher.SubmitAtomic(ctx, p)
	}

	// Append the values, then admit the publication.
	landed, _, err := s.appendValues(ctx, p.Entries)
	if err != nil {
		return nil, err
	}
	diverted := *p
	diverted.Entries = landed
	return publisher.SubmitAtomic(ctx, &diverted)
}

// PublishAtomic submits a publication and waits for its durable result with
// cancellation disabled after admission.
func (s *PackedStore) PublishAtomic(ctx context.Context, p *block.AtomicPublication) error {
	receipt, err := s.SubmitAtomic(ctx, p)
	if err != nil {
		return err
	}
	defer receipt.Release()
	return receipt.Wait(context.WithoutCancel(ctx))
}

// BeginDeferFlush forwards the GC defer-flush scope to the local store.
func (s *PackedStore) BeginDeferFlush() {
	block.BeginDeferFlush(s.local)
}

// EndDeferFlush forwards closing the GC defer-flush scope to the local store.
func (s *PackedStore) EndDeferFlush(ctx context.Context) error {
	return block.EndDeferFlush(ctx, s.local)
}

// _ is a type assertion
var _ block.AtomicPublisher = (*PackedStore)(nil)
