package blob

import (
	"context"
	"io"
	"math"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/sbset"
)

// Reader reads from a blob. Streaming reads retain a bounded window of chunk
// data. Read, Seek, and Close must not be called concurrently.
type Reader struct {
	// ctx cancels chunk fetches when the caller cancels or the reader closes.
	ctx context.Context
	// ctxCancel releases this reader's derived context.
	ctxCancel context.CancelFunc
	// bcs is the existing cursor at the blob root.
	bcs *block.Cursor
	// root supplies the representation and declared logical size.
	root *Blob
	// idx is the current read index.
	idx int
	// chunkIdx is the previous chunk we read from.
	// This speeds up seeking for idx during sequential reads.
	chunkIdx int
	// chunkSet supplies the existing chunk references for a chunked blob.
	chunkSet *sbset.SubBlockSet
	// chunkCache keeps recently read chunks and bounded read-ahead. The cursor
	// cache is intentionally bypassed so large HTTP readbacks do not retain
	// every chunk, but repeated small reads inside one chunk must still avoid
	// refetching the same block.
	chunkCache chunkReadCache
}

// NewReader constructs a new reader.
// bcs is located at the root of the blob.
// bcs can have an empty block if needed.
func NewReader(
	ctx context.Context,
	bcs *block.Cursor,
) (*Reader, error) {
	// Resolve the blob through the caller's existing cursor.
	rootBlk, err := UnmarshalBlob(ctx, bcs)
	if err != nil {
		return nil, err
	}
	if rootBlk == nil {
		rootBlk = &Blob{}
		bcs.SetBlock(rootBlk, false)
	}

	// Bind chunk access and cancellation to the returned reader.
	rdr := &Reader{bcs: bcs}
	rdr.root = rootBlk
	if rootBlk.GetBlobType() == BlobType_BlobType_CHUNKED {
		rdr.chunkSet = rootBlk.
			GetChunkIndex().
			GetChunkSet(bcs.FollowSubBlock(4))
	}

	// Release this derived context when the reader closes.
	rdr.ctx, rdr.ctxCancel = context.WithCancel(ctx)
	return rdr, nil
}

// NewRawReader reads blobs of type raw only.
func NewRawReader(ctx context.Context, blob *Blob) *Reader {
	return &Reader{
		ctx:       ctx,
		ctxCancel: func() {},
		root:      blob,
	}
}

// SetChunkCacheLimit retains up to limit bytes of recently read chunks, so
// random access that revisits chunks does not fetch them again. The latest
// chunk is always retained. The default limit of zero suits streaming reads.
func (r *Reader) SetChunkCacheLimit(limit int) {
	r.chunkCache.limit = limit
}

// Read implements the reader interface.
// Read and Seek must not run concurrently.
func (r *Reader) Read(p []byte) (n int, err error) {
	// Bound the request by the declared size without overflowing the position.
	if r.root.GetTotalSize() > math.MaxInt {
		return 0, errors.New("total size exceeds maximum")
	}
	blobSize := int(r.root.GetTotalSize()) //nolint:gosec // the size bound above protects this conversion.
	if r.idx < 0 || r.idx >= blobSize || len(p) == 0 {
		return 0, io.EOF
	}
	readSize := min(len(p), blobSize-r.idx)
	p = p[:readSize]

	// Read stored bytes or the implicit zero tail through the selected representation.
	switch blobType := r.root.GetBlobType(); blobType {
	case BlobType_BlobType_RAW:
		if r.idx >= len(r.root.RawData) {
			clear(p)
			n = len(p)
		} else {
			n = copy(p, r.root.RawData[r.idx:])
		}
	case BlobType_BlobType_CHUNKED:
		// Enable bounded read-ahead only for streaming reads over several chunks.
		if r.chunkCache.ahead == nil && len(p) >= chunkReadAheadMinRead && r.chunkSet.Len() > 1 {
			r.chunkCache.ahead = newChunkReadAhead(r.ctx, r.chunkSet)
		}

		// Reads past the stored chunks fill the remaining logical range with zeros.
		var chunkIdx int
		n, chunkIdx, err = readFromChunks(r.ctx, r.chunkSet, p, r.idx, r.chunkIdx, &r.chunkCache)
		switch err {
		case io.EOF:
			clear(p)
			n, err = len(p), nil
		case nil:
			r.chunkIdx = chunkIdx
		}
	default:
		return 0, errors.Errorf("unhandled blob type: %s", blobType.String())
	}

	// Every returned byte advances the reader, including zeros and partial reads.
	r.idx += n
	return n, err
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
// Read and Seek must not run concurrently.
func (r *Reader) Seek(offset int64, whence int) (int64, error) {
	// Require a logical size representable by the seeking interface.
	blobSize := r.root.GetTotalSize()
	if blobSize > math.MaxInt64 {
		return 0, errors.New("total size exceeds maximum")
	}

	// Resolve the requested origin and reject a position before the blob.
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

	// Stop speculative reads before publishing a different cursor position.
	if nextPos != int64(r.idx) && r.chunkCache.ahead != nil {
		r.chunkCache.ahead.close()
		r.chunkCache.ahead = nil
	}
	r.idx = int(nextPos)
	return nextPos, nil
}

// Close cancels the reader and waits for its outstanding chunk reads.
func (r *Reader) Close() error {
	// Cancel and join chunk reads before releasing the retained buffers.
	r.ctxCancel()
	if r.chunkCache.ahead != nil {
		r.chunkCache.ahead.close()
	}
	r.chunkCache = chunkReadCache{}
	return nil
}

// _ checks that Reader implements the standard streaming interfaces.
var (
	_ io.Reader = (*Reader)(nil)
	_ io.Seeker = (*Reader)(nil)
	_ io.Closer = (*Reader)(nil)
)
