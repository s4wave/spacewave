package packfile

import (
	"context"
	"io"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
)

// ScanPack calls fn with each entry of the packfile of size bytes in ra, in
// key order, reading one value at a time after the index tail. It only reads
// ra. Memory is the index tail plus one value of at most maxValue bytes. A
// larger entry fails before its value is allocated. fn must copy what it keeps.
func ScanPack(ctx context.Context, ra io.ReaderAt, size uint64, maxValue uint64, fn func(key, value []byte) error) error {
	// Read the index tail with at most three reads from the end.
	tail, err := ReadIndexTail(ctx, size, IndexTailFirstRead, func(_ context.Context, off int64, length int) ([]byte, error) {
		buf := make([]byte, length)
		n, err := ra.ReadAt(buf, off)
		if n == length {
			return buf, nil
		}
		return nil, err
	})
	if err != nil {
		return err
	}

	// Read each value its entry places, bounded before allocation.
	return ScanIndexTail(tail, size, func(ie *kvfile.IndexEntry) error {
		// Stop on cancellation, then bound the entry before allocating
		// its value.
		if err := ctx.Err(); err != nil {
			return err
		}
		if ie.GetSize() > maxValue || ie.GetOffset() > size || ie.GetSize() > size-ie.GetOffset() {
			return errors.Errorf("packfile entry of %d bytes at %d exceeds its bounds", ie.GetSize(), ie.GetOffset())
		}
		value := make([]byte, ie.GetSize())
		if _, err := ra.ReadAt(value, int64(ie.GetOffset())); err != nil { //nolint:gosec // bounded by size above.
			return errors.Wrap(err, "read packfile value")
		}
		return fn(ie.GetKey(), value)
	})
}
