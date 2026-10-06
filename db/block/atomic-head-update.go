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
