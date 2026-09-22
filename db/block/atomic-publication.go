package block

import "context"

// AtomicHeadUpdate describes an in-process metadata compare-and-swap in the
// same transaction as Entries. Key is relative to ObjectStoreID. Replace must
// only inspect the supplied committed record and return its replacement; it
// must not perform I/O or mutate the publication. A failed comparison rejects
// this publication before any of its entries are applied.
//
// The callback allows transformed metadata to compare decoded references rather
// than comparing nondeterministically encrypted encodings. It is not a wire API.
type AtomicHeadUpdate struct {
	// ObjectStoreID scopes the metadata key.
	ObjectStoreID string
	// Key is the metadata key relative to the object store.
	Key []byte
	// Replace maps the committed record to its replacement.
	Replace func(ctx context.Context, current []byte, found bool) ([]byte, error)
}

// AtomicPublication is an immutable, bounded unit of durable publication.
// Entries and everything they reference must remain unchanged until Done.
// Validate sees a read-only overlay containing Entries and the physical write
// transaction, including preceding successful publications in the same group.
// Validation and head comparison run before any mutations for this publication.
// BucketID and TrackGC are populated by the bucket capability, not by the World.
// No independent transaction, reference scan, or application callback may be
// opened by Validate while the physical transaction is held.
type AtomicPublication struct {
	// Entries are the block writes applied by this publication.
	Entries []*PutBatchEntry
	// After is the earlier publication on which this prepared revision depends.
	// It must belong to this writer and precede this request in admission order.
	After *PublicationReceipt
	// Head is the optional metadata compare-and-swap applied with Entries.
	Head *AtomicHeadUpdate
	// RootName and Root transfer the published DAG from bucket staging to a
	// durable named root in the same transaction as Head. Empty RootName keeps
	// the caller's existing retention policy. A submitted named root also stays
	// pinned until the caller releases its PublicationReceipt.
	RootName string
	Root     *BlockRef
	// Validate optionally inspects the prepared overlay before mutation.
	Validate func(ctx context.Context, prepared StoreOps) error
	// BucketID scopes GC ownership; populated by the bucket capability.
	BucketID string
	// TrackGC enables GC reference bookkeeping for the entries.
	TrackGC bool
}
