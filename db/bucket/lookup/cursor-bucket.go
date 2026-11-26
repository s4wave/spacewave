package bucket_lookup

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// GetStoredBlock reads encoded bytes and stored references from the bucket.
// It bypasses the cursor's transformer and transaction-store overrides.
func (c *Cursor) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	return c.bkt.GetStoredBlock(ctx, ref)
}

// HasStoredBlock checks the bucket without transaction-store overrides.
func (c *Cursor) HasStoredBlock(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return c.bkt.GetBlockExists(ctx, ref)
}

// ReadRoot reads a named root of the cursor's underlying bucket.
func (c *Cursor) ReadRoot(ctx context.Context, name string) (*block.BlockRef, error) {
	return c.bkt.GetRoot(ctx, name)
}

// ReleaseRoot removes the current named root if it exists.
// The compare-and-set preserves a root another writer changed after the read.
func (c *Cursor) ReleaseRoot(ctx context.Context, name string) error {
	// Read the retained root before issuing a removal.
	root, err := c.bkt.GetRoot(ctx, name)
	if err != nil || root.GetEmpty() {
		return err
	}

	// Release exactly the root observed by this operation.
	return c.bkt.SetRoots(ctx, nil, []block.NamedRoot{{Name: name, Previous: root}})
}
