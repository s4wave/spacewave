package bucket_mock

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// GetRoot reads the root from the backing store.
func (b *mockBucket) GetRoot(ctx context.Context, name string) (*block.BlockRef, error) {
	return b.store.GetRoot(ctx, name)
}

// ListRoots lists the backing store's roots.
func (b *mockBucket) ListRoots(ctx context.Context, prefix string, fn func(block.NamedRoot) error) error {
	return b.store.ListRoots(ctx, prefix, fn)
}

// SetRoots moves the roots on the backing store.
func (b *mockBucket) SetRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	return b.store.SetRoots(ctx, entries, roots)
}
