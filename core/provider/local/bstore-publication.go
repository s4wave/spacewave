package provider_local

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

// publisher returns the atomic publisher of the account's own storage, or nil
// when that storage cannot publish atomically. A store whose SQL backend is
// primary publishes its heads there, after its SQL publisher drains the blocks.
func (b *BlockStore) publisher() block.AtomicPublisher {
	if b.placement == nil || (b.placement.sqlPrimary && b.placement.sqlPublish == nil) {
		return nil
	}
	p, ok := b.placement.local.(block.AtomicPublisher)
	if !ok || !p.SupportsAtomicPublication() {
		return nil
	}
	return p
}

// SupportsAtomicPublication reports whether the account's own storage
// publishes atomically.
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

// ReservePublication claims SQL-primary admission capacity without writer locks.
// Other placements keep their existing publisher path.
func (b *BlockStore) ReservePublication(ctx context.Context, size int64) (block.PublicationReservation, error) {
	if !b.SupportsPublicationReservations() {
		return nil, nil
	}
	reservation, err := b.placement.sqlPublish.reserve(ctx, size)
	if reservation == nil {
		return nil, err
	}
	return reservation, err
}

// SupportsPublicationReservations reports whether SQL-primary pre-lock claims are enabled.
func (b *BlockStore) SupportsPublicationReservations() bool {
	return b.placement != nil && b.placement.sqlPrimary && b.placement.sqlPublish != nil && sqlReservationsEnabled.Load()
}

// TryReservePublication claims SQL-primary capacity without a queue wait.
func (b *BlockStore) TryReservePublication(ctx context.Context, size int64) (block.PublicationReservation, error) {
	if !b.SupportsPublicationReservations() {
		return nil, nil
	}
	r, err := b.placement.sqlPublish.reserveCapacity(ctx, size, false)
	if r == nil {
		return nil, err
	}
	return r, err
}

// AcquirePublicationPreparation bounds unpublished candidates to one per SQL store.
func (b *BlockStore) AcquirePublicationPreparation(ctx context.Context) (func(), error) {
	if !b.SupportsPublicationReservations() {
		return func() {}, nil
	}
	return b.placement.sqlPublish.prepare.Lock(ctx)
}

// SubmitAtomic admits a publication to the account's own storage. While writes
// queue for upload, the publication's blocks are marked in the transaction that
// commits them, so the publication and its upload markers are atomic; a storage
// backend chosen after admission backfills the published blocks.
// A store whose SQL backend is primary admits it to its SQL publisher.
func (b *BlockStore) SubmitAtomic(ctx context.Context, p *block.AtomicPublication) (*block.PublicationReceipt, error) {
	// Require storage that publishes atomically.
	publisher := b.publisher()
	if publisher == nil {
		return nil, block.ErrAtomicPublicationUnsupported
	}
	if b.placement.sqlPrimary {
		return b.placement.sqlPublish.submit(ctx, p)
	}

	// Mark the publication's blocks in its transaction while writes queue for
	// upload. The writer commits the markers with the blocks, or neither.
	if status, _ := b.placement.wb.GetStatus(); status.Enabled {
		ctx, _ = kvtx.WithBlockMarker(ctx, b.placement.wb)
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
