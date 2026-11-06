package block_gc

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// IsHole reports whether the audit recorded ref as a hole under a live root.
func (r *Records) IsHole(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return r.tx.Exists(ctx, r.key(kindHole, BlockKey(ref)))
}

// holeReader reads the recorded holes for the puts of one transaction. It
// first reads, once, whether the Volume records any hole, so a put into a
// Volume without one, the usual case, reads no hole record per block.
type holeReader struct {
	// rec are the liveness records.
	rec *Records
	// read reports that any was read.
	read bool
	// any reports that the Volume records a hole.
	any bool
}

// isHole reports whether the audit recorded ref as a hole.
func (h *holeReader) isHole(ctx context.Context, ref *block.BlockRef) (bool, error) {
	// Read whether any hole is recorded, once.
	if !h.read {
		any, err := h.rec.anyKey(ctx, h.rec.key(kindHole))
		if err != nil {
			return false, err
		}
		h.read, h.any = true, any
	}

	// Read the block's record only when one may exist.
	if !h.any {
		return false, nil
	}
	return h.rec.IsHole(ctx, ref)
}

// SetHole records ref as a hole under a live root.
func (r *Records) SetHole(ctx context.Context, ref *block.BlockRef) error {
	return r.tx.Set(ctx, r.key(kindHole, BlockKey(ref)), nil)
}

// DeleteHole deletes the hole record of ref.
func (r *Records) DeleteHole(ctx context.Context, ref *block.BlockRef) error {
	return r.tx.Delete(ctx, r.key(kindHole, BlockKey(ref)))
}
