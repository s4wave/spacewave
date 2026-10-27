package kvtx

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	store_kvtx "github.com/s4wave/spacewave/db/kvtx"
	store_kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
)

// refGraphPrefix holds the Volume's reference graph records.
var refGraphPrefix = []byte{2}

// openRefGraph opens the Volume's reference graph over store. Opening reads
// nothing, so each transaction opens its own.
func openRefGraph(store store_kvtx.Store) *block_gc.RefGraph {
	return block_gc.NewRefGraph(store_kvtx_prefixer.NewPrefixer(store, refGraphPrefix))
}

// SupportsAtomicPublication reports whether the Volume's store commits
// transactions atomically, which atomic publication needs.
func (v *Volume) SupportsAtomicPublication() bool {
	return v.publications != nil
}

// SubmitAtomic admits an atomic publication and returns its completion receipt.
// Admission is not durability: await the receipt for the durable result.
func (v *Volume) SubmitAtomic(ctx context.Context, p *block.AtomicPublication) (*block.PublicationReceipt, error) {
	// Require the Volume atomic publication domain, and refuse new data below
	// the disk reserve before the publication takes a queue slot.
	if !v.SupportsAtomicPublication() {
		return nil, block.ErrAtomicPublicationUnsupported
	}
	if err := v.checkReserve(ctx, 0); err != nil {
		return nil, err
	}

	// Reserve a reader pin for a published named root.
	var owner string
	var release func()
	if p != nil && p.TrackGC && p.RootName != "" && !p.Root.GetEmpty() {
		var err error
		owner, release, err = v.pinRoot(ctx, p.Root, true)
		if err != nil {
			return nil, err
		}
	}

	// Submit the publication and release its pin if admission fails.
	receipt, err := v.publications.submit(ctx, p, owner, release)
	if err != nil && release != nil {
		release()
	}
	return receipt, err
}

// PublishAtomic submits an atomic publication and waits for its durable result
// with cancellation disabled after admission.
func (v *Volume) PublishAtomic(ctx context.Context, p *block.AtomicPublication) error {
	receipt, err := v.SubmitAtomic(ctx, p)
	if err != nil {
		return err
	}
	defer receipt.Release()
	return receipt.Wait(context.WithoutCancel(ctx))
}

// PublicationStats reports bounded writer occupancy and actual grouped commits.
// Counters describe this handle, not other processes sharing the volume.
type PublicationStats struct {
	// Accepted counts publications admitted to the queue.
	Accepted uint64
	// Completed counts publications resolved by the drain goroutine.
	Completed uint64
	// PhysicalCommits counts physical transactions the writer committed.
	PhysicalCommits uint64
	// Rejected counts completed publications whose result failed.
	Rejected uint64
	// Pending is the number of publications currently queued.
	Pending int
	// PendingBytes is the computed admission cost of queued publications.
	PendingBytes int
	// PendingEntries is the number of queued entries.
	PendingEntries int
}

// GetPublicationStats returns a snapshot of this handle's writer occupancy.
func (v *Volume) GetPublicationStats() PublicationStats {
	// Return empty statistics when the Volume has no publication writer.
	if v.publications == nil {
		return PublicationStats{}
	}

	// Snapshot publication writer statistics under its broadcast lock.
	w := v.publications
	var stats PublicationStats
	w.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		stats = w.stats
	})
	return stats
}

// AtomicPublicationVolumeID identifies the write domain's shared blocks/metadata namespace.
func (v *Volume) AtomicPublicationVolumeID() string { return v.GetID() }

// _ is a type assertion.
var _ block.AtomicPublisher = (*Volume)(nil)
