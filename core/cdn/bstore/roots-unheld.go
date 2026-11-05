package cdn_bstore

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// GetRoot reports that no name is held: this store holds no roots.
func (s *CdnBlockStore) GetRoot(context.Context, string) (*block.BlockRef, error) { return nil, nil }

// ListRoots reports no roots.
func (s *CdnBlockStore) ListRoots(context.Context, string, func(block.NamedRoot) error) error {
	return nil
}

// SetRoots stores the entries and drops the roots.
func (s *CdnBlockStore) SetRoots(ctx context.Context, entries []*block.PutBatchEntry, _ []block.NamedRoot) error {
	return block.SetRootsUnheld(ctx, s, entries)
}

// GetRoot reports that no name is held: this store holds no roots.
func (s *SuppliedBlockStore) GetRoot(context.Context, string) (*block.BlockRef, error) {
	return nil, nil
}

// ListRoots reports no roots.
func (s *SuppliedBlockStore) ListRoots(context.Context, string, func(block.NamedRoot) error) error {
	return nil
}

// SetRoots stores the entries and drops the roots.
func (s *SuppliedBlockStore) SetRoots(ctx context.Context, entries []*block.PutBatchEntry, _ []block.NamedRoot) error {
	return block.SetRootsUnheld(ctx, s, entries)
}
