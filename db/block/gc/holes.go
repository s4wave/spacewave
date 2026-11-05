package block_gc

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// IsHole reports whether the audit recorded ref as a hole under a live root.
func (r *Records) IsHole(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return r.tx.Exists(ctx, r.key(kindHole, BlockKey(ref)))
}

// SetHole records ref as a hole under a live root.
func (r *Records) SetHole(ctx context.Context, ref *block.BlockRef) error {
	return r.tx.Set(ctx, r.key(kindHole, BlockKey(ref)), nil)
}

// DeleteHole deletes the hole record of ref.
func (r *Records) DeleteHole(ctx context.Context, ref *block.BlockRef) error {
	return r.tx.Delete(ctx, r.key(kindHole, BlockKey(ref)))
}
