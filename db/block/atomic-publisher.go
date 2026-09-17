package block

import "context"

// AtomicPublisher is the optional, shared in-process publication capability.
// SupportsAtomicPublication is stable during the publisher's lifetime. A
// successful SubmitAtomic transfers an immutable borrow until receipt.Done;
// admission is NOT durability. Once admitted, cancellation of a waiter does not
// cancel the publication or imply that it failed. Close must join accepted work.
// PublishAtomic is SubmitAtomic followed by an uncancelled durability wait after
// admission, preserving the legacy Commit contract of an unambiguous result.
type AtomicPublisher interface {
	// AtomicPublicationVolumeID identifies the shared blocks/metadata domain.
	AtomicPublicationVolumeID() string
	// SupportsAtomicPublication reports whether atomic publication is available.
	SupportsAtomicPublication() bool
	// SubmitAtomic admits a publication and returns its completion receipt.
	SubmitAtomic(ctx context.Context, publication *AtomicPublication) (*PublicationReceipt, error)
	// PublishAtomic submits a publication and waits for its durable result.
	PublishAtomic(ctx context.Context, publication *AtomicPublication) error
}

// AtomicBlockPreparer persists construction overflow in the same transaction as
// its bucket ownership, without changing a World head. Calls are synchronous;
// the caller retains its buffers until return and no oversized entry is queued.
// An empty bucketID disables GC bookkeeping. Unsupported is returned before I/O.
// This capability complements, rather than bypasses, bounded publication admission.
type AtomicBlockPreparer interface {
	// PrepareOwnedBlock durably prepares one oversized body with bucket ownership.
	PrepareOwnedBlock(ctx context.Context, bucketID string, data []byte, opts *PutOpts) (*BlockRef, bool, error)
	// PrepareOwnedBlockBatch durably prepares a batch with bucket ownership.
	PrepareOwnedBlockBatch(ctx context.Context, bucketID string, entries []*PutBatchEntry) error
}
