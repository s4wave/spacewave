package blob

import (
	"bytes"
	"context"
	"io"

	"github.com/aperturerobotics/hydra/block"
)

const (
	// defChunkingMinSize is the default chunk min size.
	defChunkingMinSize = 4096 * 16 // 65536 bytes, 32 unix writes
	// defChunkingMaxSize is the default chunk max size.
	// note: larger chunks have lower storage overhead but worse deduplication.
	// the optimal chunk size is dependent on the content type.
	// set a reasonable default here.
	defChunkingMaxSize = 4096 * 64 // ~262KB, 64 unix writes
	// rawHighWaterMark is the default high water mark for a raw blob.
	// define this to be the max size of a single chunk
	rawHighWaterMark = defChunkingMaxSize
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
		hwm = rawHighWaterMark
	}

	if dataLen <= int64(hwm) {
		buf := make([]byte, dataLen)
		if _, err := io.ReadFull(rdr, buf); err != nil {
			return nil, err
		}
		rb := NewRawBlob(buf)
		bcs.SetBlock(rb, true)
		return rb, nil
	}

	// build a chunked blob
	blob := &Blob{
		BlobType: BlobType_BlobType_CHUNKED,
	}
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
