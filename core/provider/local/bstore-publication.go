package provider_local

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// publisher returns the atomic publisher of the account's own storage, or nil
// when that storage cannot publish atomically.
func (b *BlockStore) publisher() block.AtomicPublisher {
	if b.placement == nil {
		return nil
	}
	p, ok := b.placement.local.(block.AtomicPublisher)
	if !ok || !p.SupportsAtomicPublication() {
		return nil
	}
	return p
}

// SupportsAtomicPublication reports whether the account's own storage
// publishes atomically. SubmitAtomic still rejects a publication while the
// store queues writes for upload, since the publication would skip their
// upload markers.
func (b *BlockStore) SupportsAtomicPublication() bool {
	return b.publisher() != nil
}

// AtomicPublicationVolumeID returns the account storage's publication domain.
func (b *BlockStore) AtomicPublicationVolumeID() string {
	if p := b.publisher(); p != nil {
		return p.AtomicPublicationVolumeID()
	}
	return ""
}

// SubmitAtomic admits a publication to the account's own storage. It returns
// block.ErrAtomicPublicationUnsupported while writes queue for upload; a
// storage backend chosen after admission backfills the published blocks.
func (b *BlockStore) SubmitAtomic(ctx context.Context, p *block.AtomicPublication) (*block.PublicationReceipt, error) {
	// Leave writes queued for upload to the marking store.
	publisher := b.publisher()
	if publisher == nil {
		return nil, block.ErrAtomicPublicationUnsupported
	}
	if status, _ := b.placement.wb.GetStatus(); status.Enabled {
		return nil, block.ErrAtomicPublicationUnsupported
	}

	// Admit the publication and drop decoded blocks its tombstones remove.
	receipt, err := publisher.SubmitAtomic(ctx, p)
	if err != nil {
		return nil, err
	}
	b.invalidateBatchTombstones(ctx, p.Entries)
	return receipt, nil
}

// PublishAtomic submits a publication and waits for its durable result with
// cancellation disabled after admission.
func (b *BlockStore) PublishAtomic(ctx context.Context, p *block.AtomicPublication) error {
	receipt, err := b.SubmitAtomic(ctx, p)
	if err != nil {
		return err
	}
	defer receipt.Release()
	return receipt.Wait(context.WithoutCancel(ctx))
}

// _ is a type assertion
var _ block.AtomicPublisher = (*BlockStore)(nil)
