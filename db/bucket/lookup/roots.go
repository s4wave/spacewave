package bucket_lookup

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// GetRoot reads the root from the destination store.
func (s *objectCopyStore) GetRoot(ctx context.Context, name string) (*block.BlockRef, error) {
	return s.inner.GetRoot(ctx, name)
}

// ListRoots lists the destination store's roots.
func (s *objectCopyStore) ListRoots(ctx context.Context, prefix string, fn func(block.NamedRoot) error) error {
	return s.inner.ListRoots(ctx, prefix, fn)
}

// SetRoots moves the roots on the destination store.
func (s *objectCopyStore) SetRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	return s.inner.SetRoots(ctx, entries, roots)
}

// GetRoot reports that no name is held: a lookup bucket only reads blocks.
func (l *lookupBucket) GetRoot(context.Context, string) (*block.BlockRef, error) { return nil, nil }

// ListRoots reports no roots.
func (l *lookupBucket) ListRoots(context.Context, string, func(block.NamedRoot) error) error {
	return nil
}

// SetRoots stores the entries and refuses a root that would hold a block:
// this store holds no roots.
func (l *lookupBucket) SetRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	return block.SetRootsUnheld(ctx, l, entries, roots)
}
