package blob

import (
	"bytes"
	"context"
	"io"

	"github.com/s4wave/spacewave/db/block"
)

const (
	// DefChunkingMinSize is the default chunk min size.
	DefChunkingMinSize = 2048 * 125 // 256000 bytes
	// DefChunkingTargetSize is the default chunk target size.
	// Only used by the JC chunker.
	DefChunkingTargetSize = 512000 // 512000 bytes
	// DefChunkingMaxSize is the default chunk max size.
	DefChunkingMaxSize = 4096 * (64 * 3) // 786432 bytes
	// DefRawHighWaterMark is the default high water mark for a raw blob.
	// define this to be the max size of a single chunk
	DefRawHighWaterMark = DefChunkingMaxSize
)

// NewRawBlob constructs a new Raw blob.
// Data is not copied.
func NewRawBlob(data []byte) *Blob {
	return &Blob{
		BlobType:  BlobType_BlobType_RAW,
		TotalSize: uint64(len(data)),
		RawData:   data,
	}
}

// BuildBlob constructs a blob chunking / sharding it.
// Blocks will be written to the block transaction.
// The new root Blob block will become the root of bcs.
// Constructs a blob with a known size.
func BuildBlob(
	ctx context.Context,
	dataLen int64,
	rdr io.Reader,
	bcs *block.Cursor,
	opts *BuildBlobOpts,
) (*Blob, error) {
	hwm := opts.GetRawHighWaterMark()
	if hwm == 0 {
		hwm = DefRawHighWaterMark
	}

	if dataLen <= int64(hwm) { //nolint:gosec
		buf := make([]byte, dataLen)
		if _, err := io.ReadFull(rdr, buf); err != nil {
			return nil, err
		}
		rb := NewRawBlob(buf)
		bcs.SetBlock(rb, true)
		return rb, nil
	}

	// build a chunked blob
	blob := &Blob{BlobType: BlobType_BlobType_CHUNKED}
	bcs.SetBlock(blob, true)
	err := blob.WriteChunkIndex(ctx, bcs, opts, io.LimitReader(rdr, dataLen))
	if err != nil {
		return nil, err
	}
	return blob, nil
}

// BuildBlobWithBytes is a shortcut to build a blob from a byte slice.
func BuildBlobWithBytes(ctx context.Context, data []byte, bcs *block.Cursor) (*Blob, error) {
	return BuildBlob(ctx, int64(len(data)), bytes.NewReader(data), bcs, nil)
}

// BuildBlobWithReader constructs a blob chunking / sharding it.
// Blocks will be written to the block transaction.
// The new root Blob block will become the root of bcs.
// Constructs a blob with an unknown size.
// If you know the size ahead of time, use BuildBlob.
func BuildBlobWithReader(
	ctx context.Context,
	rdr io.Reader,
	bcs *block.Cursor,
	opts *BuildBlobOpts,
) (*Blob, error) {
	hwm := opts.GetRawHighWaterMark()
	if hwm == 0 {
		hwm = DefRawHighWaterMark
	}

	// Read at least the high water mark from the reader first.
	var buf bytes.Buffer
	nn, err := buf.ReadFrom(io.LimitReader(rdr, int64(hwm))) //nolint:gosec
	if err != nil {
		return nil, err
	}

	// If we read less than high water mark, we can use a single block.
	if nn < int64(hwm) { //nolint:gosec
		rb := NewRawBlob(buf.Bytes())
		bcs.SetBlock(rb, true)
		return rb, nil
	}

	// Otherwise: build a chunked blob
	// Tee the existing read data with rdr
	blob := &Blob{BlobType: BlobType_BlobType_CHUNKED}
	bcs.SetBlock(blob, true)
	err = blob.WriteChunkIndex(ctx, bcs, opts, io.MultiReader(&buf, rdr))
	if err != nil {
		return nil, err
	}
	return blob, nil
}
