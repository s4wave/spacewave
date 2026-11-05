package block_store_http

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// GetRoot reports that no name is held: this store holds no roots.
func (b *HTTPBlock) GetRoot(context.Context, string) (*block.BlockRef, error) { return nil, nil }

// ListRoots reports no roots.
func (b *HTTPBlock) ListRoots(context.Context, string, func(block.NamedRoot) error) error { return nil }

// SetRoots stores the entries and refuses a root that would hold a block:
// this store holds no roots.
func (b *HTTPBlock) SetRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	return block.SetRootsUnheld(ctx, b, entries, roots)
}
