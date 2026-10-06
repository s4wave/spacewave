package block

import "context"

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
	// Heads are the metadata compare-and-swaps applied with Entries. At least
	// one is required, and every key is distinct.
	Heads []*AtomicHeadUpdate
	// RootName and Root transfer the published DAG from bucket staging to a
	// durable named root in the same transaction as Heads. Empty RootName keeps
	// the caller's existing retention policy. A submitted named root also stays
	// pinned until the caller releases its PublicationReceipt.
	RootName string
	// Root is the published DAG root RootName holds.
	Root *BlockRef
	// Roots are further named roots set in the same transaction, without a
	// reader pin. They apply only with TrackGC, as RootName does.
	Roots []NamedRoot
	// Ordered lets the physical commit be ordered rather than durable: applied
	// when the receipt resolves, and durable no later than the volume's next
	// durable commit. A group commits ordered only when every member allows it.
	Ordered bool
	// Validate optionally inspects the prepared overlay before mutation.
	Validate func(ctx context.Context, prepared StoreOps) error
	// BucketID scopes GC ownership; populated by the bucket capability.
	BucketID string
	// TrackGC enables GC reference bookkeeping for the entries.
	TrackGC bool
}
