package blob

import (
	"context"
	"io"

	"github.com/aperturerobotics/hydra/block"
	"github.com/pkg/errors"
)

const (
	// rawHighWaterMark is the default high water mark for a raw blob.
	rawHighWaterMark = 1e6
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

// BuildBlobOpts are options to control the BuildBlob process.
type BuildBlobOpts struct {
	// RawHighWaterMark is the limit for a raw block size.
	// Defaults to 1Mb.
	RawHighWaterMark uint64
}

// BuildBlob constructs a blob chunking / sharding it.
// Blocks will be written to the block transaction.
// The new root Blob block will become the root of bcs.
// TODO: the block cursor should have logic to "flush" to disk.
// Constructs a blob with a known size.
func BuildBlob(
	ctx context.Context,
	dataLen uint64,
	rdr io.Reader,
	bcs *block.Cursor,
	opts BuildBlobOpts,
) (*Blob, error) {
	hwm := opts.RawHighWaterMark
	if hwm == 0 {
		hwm = rawHighWaterMark
	}

	if dataLen <= hwm {
		buf := make([]byte, dataLen)
		if _, err := io.ReadFull(rdr, buf); err != nil {
			return nil, err
		}
		rb := NewRawBlob(buf)
		bcs.SetBlock(rb)
		return rb, nil
	}

	return nil, errors.New("todo: non-raw blobs not implemented")
}
