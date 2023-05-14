package blob

import (
	"context"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/util/rcompare"
)

// CompareBlobs compares the contents of two blobs for equality.
func CompareBlobs(ctx context.Context, bcs1, bcs2 *block.Cursor) (bool, error) {
	bl1, err := UnmarshalBlob(ctx, bcs1)
	if err != nil {
		return false, err
	}
	bl2, err := UnmarshalBlob(ctx, bcs2)
	if err != nil {
		return false, err
	}
	if bl1.GetTotalSize() != bl2.GetTotalSize() {
		return false, nil
	}
	if bl1 == bl2 || bl1.EqualVT(bl2) {
		return true, nil
	}
	// compare
	r1, err := NewReader(ctx, bcs1)
	if err != nil {
		return false, err
	}
	r2, err := NewReader(ctx, bcs2)
	if err != nil {
		return false, err
	}
	return rcompare.CompareReadersEqual(r1, r2)
}
