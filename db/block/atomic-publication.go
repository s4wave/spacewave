package block

import (
	"context"
	"errors"
	"sync"
)

// ErrAtomicPublicationUnsupported is returned before admission, with no side
// effects, when a store cannot atomically persist blocks, ownership and metadata.
// It is the only publication error for which a caller may use a legacy path.
var ErrAtomicPublicationUnsupported = errors.New("atomic publication unsupported")

// ErrPublicationClosed means the publication writer no longer accepts work.
var ErrPublicationClosed = errors.New("publication writer closed")

// AtomicHeadUpdate describes an in-process metadata compare-and-swap in the
// same transaction as Entries. Key is relative to ObjectStoreID. Replace must
// only inspect the supplied committed record and return its replacement; it
// must not perform I/O or mutate the publication. A failed comparison rejects
// this publication before any of its entries are applied.
//
// The callback allows transformed metadata to compare decoded references rather
// than comparing nondeterministically encrypted encodings. It is not a wire API.
type AtomicHeadUpdate struct {
	ObjectStoreID string
	Key           []byte
	Replace       func(ctx context.Context, current []byte, found bool) ([]byte, error)
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
	Entries []*PutBatchEntry
	// After is the earlier publication on which this prepared revision depends.
	// It must belong to this writer and precede this request in admission order.
	After    *PublicationReceipt
	Head     *AtomicHeadUpdate
	Validate func(ctx context.Context, prepared StoreOps) error
	BucketID string
	TrackGC  bool
}

// AtomicPublisher is the optional, shared in-process publication capability.
// SupportsAtomicPublication is stable during the publisher's lifetime. A
// successful SubmitAtomic transfers an immutable borrow until receipt.Done;
// admission is NOT durability. Once admitted, cancellation of a waiter does not
// cancel the publication or imply that it failed. Close must join accepted work.
// PublishAtomic is SubmitAtomic followed by an uncancelled durability wait after
// admission, preserving the legacy Commit contract of an unambiguous result.
type AtomicPublisher interface {
	SupportsAtomicPublication() bool
	SubmitAtomic(ctx context.Context, publication *AtomicPublication) (*PublicationReceipt, error)
	PublishAtomic(ctx context.Context, publication *AtomicPublication) error
}

// PublicationReceipt distinguishes admission from completion. Err is meaningful
// only after Done closes. Wait cancellation means the outcome is still unknown;
// the receipt remains usable for obtaining the eventual durable result.
type PublicationReceipt struct {
	done chan struct{}
	once sync.Once
	err  error
}

// NewPublicationReceipt constructs an unresolved completion owned by a writer.
func NewPublicationReceipt() *PublicationReceipt {
	return &PublicationReceipt{done: make(chan struct{})}
}

func (r *PublicationReceipt) Done() <-chan struct{} { return r.done }

// Resolve completes a receipt exactly once. Only its owning publisher calls it.
func (r *PublicationReceipt) Resolve(err error) {
	r.once.Do(func() {
		r.err = err
		close(r.done)
	})
}

func (r *PublicationReceipt) Wait(ctx context.Context) error {
	// Prefer an already-known outcome over simultaneous waiter cancellation.
	select {
	case <-r.done:
		return r.err
	default:
	}
	select {
	case <-r.done:
		return r.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ErrPublicationTooLarge rejects a publication that cannot fit the writer's
// bounded admission budget. Large block bodies should be prepared durably first.
var ErrPublicationTooLarge = errors.New("publication exceeds bounded admission budget")

// ErrPublicationDependency means a predecessor failed or was not submitted to
// this writer before its dependent publication.
var ErrPublicationDependency = errors.New("publication predecessor did not succeed")
