package blob

import (
	"context"
	"io"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/sbset"
	"github.com/pkg/errors"
)

// Reader reads from a blob.
type Reader struct {
	ctx       context.Context
	ctxCancel context.CancelFunc
	bcs       *block.Cursor

	root          *Blob
	idx, chunkIdx int
	chunkSet      *sbset.SubBlockSet
}

// NewReader constructs a new reader.
// bcs is located at the root of the blob.
// bcs can have an empty block if needed.
func NewReader(
	ctx context.Context,
	bcs *block.Cursor,
) (*Reader, error) {
	rootBlk, err := UnmarshalBlob(bcs)
	if err != nil {
		return nil, err
	}
	if rootBlk == nil {
		rootBlk = &Blob{}
		bcs.SetBlock(rootBlk, false)
	}
	rdr := &Reader{bcs: bcs}
	rdr.root = rootBlk
	if rootBlk.GetBlobType() == BlobType_BlobType_CHUNKED {
		rdr.chunkSet = rootBlk.
			GetChunkIndex().
			GetChunkSet(bcs.FollowSubBlock(4))
	}
	rdr.ctx, rdr.ctxCancel = context.WithCancel(ctx)
	return rdr, nil
}

// NewRawReader reads blobs of type raw only.
func NewRawReader(blob *Blob) *Reader {
	return &Reader{
		ctx: context.Background(),
		ctxCancel: func() {
			// no-op
		},
		root: blob,
	}
}

// Read implements the reader interface.
// Read and Seek are not concurrent safe.
func (r *Reader) Read(p []byte) (n int, err error) {
	blobSize := int(r.root.GetTotalSize())
	readSize := len(p)
	readStart := r.idx
	readEnd := r.idx + readSize
	if readEnd > blobSize {
		readEnd = blobSize
	}
	if readStart >= readEnd {
		return 0, io.EOF
	}
	blobType := r.root.GetBlobType()
	switch blobType {
	case BlobType_BlobType_RAW:
		rawBuf := r.root.GetRawData()
		rawBufSize := len(rawBuf)
		if readEnd > rawBufSize {
			readEnd = rawBufSize
			if readStart >= readEnd {
				return 0, io.EOF
			}
		}
		copy(p, rawBuf[readStart:readEnd])
	case BlobType_BlobType_CHUNKED:
		chkRead, outChkIdx, err := ReadFromChunks(r.chunkSet, p, readStart, r.chunkIdx)
		if err != nil {
			// returns io.EOF only if readStart is past the end of the chunks.
			return 0, err
		}
		readEnd = readStart + chkRead
		r.chunkIdx = outChkIdx
	default:
		return 0, errors.Errorf("unhandled blob type: %s", blobType.String())
	}

	r.idx = readEnd
	return int(readEnd) - int(readStart), nil
}

// Seek implements the seeking interface.
// Seek sets the offset for the next Read or Write to offset,
// interpreted according to whence:
// SeekStart means relative to the start of the file,
// SeekCurrent means relative to the current offset, and
// SeekEnd means relative to the end.
// Seek returns the new offset relative to the start of the
// file and an error, if any.
//
// Seeking past the end of the blob does NOT immediately trigger EOF.
//
// Seeking to an offset before the start of the file is an error.
// Seeking to any positive offset is legal, but the behavior of subsequent
// I/O operations on the underlying object is implementation-dependent.
// Read and Seek are not concurrent safe.
func (r *Reader) Seek(offset int64, whence int) (int64, error) {
	blobSize := r.root.GetTotalSize()
	nextPos := offset
	switch whence {
	case io.SeekCurrent:
		nextPos += int64(r.idx)
	case io.SeekEnd:
		nextPos += int64(blobSize)
	}
	if nextPos < 0 {
		return 0, errors.New("seek to before start of blob")
	}
	r.idx = int(nextPos)
	return nextPos, nil
}

// Close closes the reader, canceling the context.
func (r *Reader) Close() error {
	r.ctxCancel()
	return nil
}

// _ is a type assertion
var (
	_ io.Reader = ((*Reader)(nil))
	_ io.Seeker = ((*Reader)(nil))
	_ io.Closer = ((*Reader)(nil))
	// _ io.WriterTo = ((*Reader)(nil))
)
