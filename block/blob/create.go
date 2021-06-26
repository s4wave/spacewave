package blob

import (
	"context"
	"io"

	"github.com/aperturerobotics/hydra/block"
	"github.com/restic/chunker"
)

const (
	// defChunkingMinSize is the default chunk min size.
	defChunkingMinSize = 10e3 // 10KB
	// defChunkingMaxSize is the default chunk max size.
	// most systems have a max block size of 1MiB: use 512KB
	defChunkingMaxSize = 512e3 // 512 KB
	// rawHighWaterMark is the default high water mark for a raw blob.
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
	chkIdxBcs := bcs.FollowSubBlock(4)
	var err error
	blob.ChunkIndex, blob.TotalSize, err = BuildChunkIndex(
		ctx,
		rdr,
		chkIdxBcs,
		chunker.Pol(opts.GetChunkingPol()),
		opts.GetChunkingMinSize(), opts.GetChunkingMaxSize(),
	)
	if err != nil {
		return nil, err
	}
	return blob, nil
}
