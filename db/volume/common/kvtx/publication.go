package kvtx

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// The GC key prefix is shared by ordinary volume opening and atomic publication.
// This intentionally preserves the existing durable representation.
func volumeRefGraphPrefix() []byte { return []byte("gc/") }

// SupportsAtomicPublication excludes custom block domains, deferred Commit
// wrappers and WAL-backed GC. Those configurations keep their existing path.
func (v *Volume) SupportsAtomicPublication() bool {
	return v.publications != nil && v.walAppender == nil && v.gcManagerHooks == nil
}

// SubmitAtomic admits an atomic publication and returns its completion receipt.
// Admission is not durability: await the receipt for the durable result.
func (v *Volume) SubmitAtomic(ctx context.Context, p *block.AtomicPublication) (*block.PublicationReceipt, error) {
	if !v.SupportsAtomicPublication() {
		return nil, block.ErrAtomicPublicationUnsupported
	}
	var owner string
	var release func()
	if p != nil && p.TrackGC && p.RootName != "" && !p.Root.GetEmpty() {
		var err error
		owner, release, err = v.pinRoot(ctx, p.Root, true)
		if err != nil {
			return nil, err
		}
	}
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
	if v.publications == nil {
		return PublicationStats{}
	}
	w := v.publications
	var stats PublicationStats
	w.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		stats = w.stats
	})
	return stats
}

var _ block.AtomicPublisher = (*Volume)(nil)

// AtomicPublicationVolumeID identifies the write domain's shared blocks/metadata namespace.
func (v *Volume) AtomicPublicationVolumeID() string { return v.GetID() }
