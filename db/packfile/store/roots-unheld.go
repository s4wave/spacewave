package store

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// GetRoot reports that no name is held: this store holds no roots.
func (s *PackfileStore) GetRoot(context.Context, string) (*block.BlockRef, error) { return nil, nil }

// ListRoots reports no roots.
func (s *PackfileStore) ListRoots(context.Context, string, func(block.NamedRoot) error) error {
	return nil
}

// SetRoots stores the entries and drops the roots.
func (s *PackfileStore) SetRoots(ctx context.Context, entries []*block.PutBatchEntry, _ []block.NamedRoot) error {
	return block.SetRootsUnheld(ctx, s, entries)
}
