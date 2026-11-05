package block_store_kvtx

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// GetRoot reports that no name is held: this store holds no roots.
func (k *KVTxBlock) GetRoot(context.Context, string) (*block.BlockRef, error) { return nil, nil }

// ListRoots reports no roots.
func (k *KVTxBlock) ListRoots(context.Context, string, func(block.NamedRoot) error) error { return nil }

// SetRoots stores the entries and drops the roots.
func (k *KVTxBlock) SetRoots(ctx context.Context, entries []*block.PutBatchEntry, _ []block.NamedRoot) error {
	return block.SetRootsUnheld(ctx, k, entries)
}

// GetRoot reports that no name is held: this store holds no roots.
func (r *readOperation) GetRoot(context.Context, string) (*block.BlockRef, error) { return nil, nil }

// ListRoots reports no roots.
func (r *readOperation) ListRoots(context.Context, string, func(block.NamedRoot) error) error {
	return nil
}

// SetRoots stores the entries and drops the roots.
func (r *readOperation) SetRoots(ctx context.Context, entries []*block.PutBatchEntry, _ []block.NamedRoot) error {
	return block.SetRootsUnheld(ctx, r, entries)
}
