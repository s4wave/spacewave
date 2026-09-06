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
	ctx       context.Context
	ctxCancel context.CancelFunc
	bcs       *block.Cursor

	root *Blob
	// idx is the current read index.
	idx int
	// chunkIdx is the previous chunk we read from.
	// This speeds up seeking for idx during sequential reads.
	chunkIdx int
	chunkSet *sbset.SubBlockSet
	// chunkCache keeps active data and bounded read-ahead. The cursor cache is
	// intentionally bypassed for sequential reads so large HTTP readbacks do not
	// retain every chunk, but repeated small reads inside one chunk must still
	// avoid refetching the same block.
	chunkCache chunkReadCache
}

// NewReader constructs a new reader.
// bcs is located at the root of the blob.
// bcs can have an empty block if needed.
func NewReader(
	ctx context.Context,
	bcs *block.Cursor,
) (*Reader, error) {
	rootBlk, err := UnmarshalBlob(ctx, bcs)
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
func NewRawReader(ctx context.Context, blob *Blob) *Reader {
	return &Reader{
		ctx:       ctx,
		ctxCancel: func() {},
		root:      blob,
	}
}

// Read implements the reader interface.
// Read and Seek must not run concurrently.
func (r *Reader) Read(p []byte) (n int, err error) {
	readStart := r.idx
	if readStart < 0 {
		return 0, io.EOF
	}

	if r.root.GetTotalSize() > math.MaxInt {
		return 0, errors.New("total size exceeds maximum")
	}
	blobSize := int(r.root.GetTotalSize()) //nolint:gosec
	readSize := len(p)
	readEnd := min(r.idx+readSize, blobSize)
	if readStart >= readEnd {
		return 0, io.EOF
	}

	fillZeros := func() int {
		readLen := readEnd - readStart
		for i := range readLen {
			p[i] = 0
		}
		return readLen
	}

	blobType := r.root.GetBlobType()
	switch blobType {
	case BlobType_BlobType_RAW:
		rawBuf := r.root.GetRawData()
		rawBufSize := len(rawBuf)
		if readStart >= rawBufSize {
			// return zeros for the rest of the blob
			return fillZeros(), nil
		}
		if readEnd > rawBufSize {
			readEnd = rawBufSize
		}
		copy(p, rawBuf[readStart:readEnd])
	case BlobType_BlobType_CHUNKED:
		if r.chunkCache.ahead == nil && len(p) >= chunkReadAheadMinRead && r.chunkSet.Len() > 1 {
			r.chunkCache.ahead = newChunkReadAhead(r.ctx, r.chunkSet)
		}
		chkRead, outChkIdx, err := readFromChunks(r.ctx, r.chunkSet, p, readStart, r.chunkIdx, &r.chunkCache)
		if err == io.EOF {
			// readStart must be past the end of the chunks.
			// respect totalSize and fill the remainder with zeros.
			return fillZeros(), nil
		}
		if err != nil {
			return chkRead, err
		}
		readEnd = readStart + chkRead
		r.chunkIdx = outChkIdx
	default:
		return 0, errors.Errorf("unhandled blob type: %s", blobType.String())
	}

	r.idx = readEnd
	return readEnd - readStart, nil
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
	blobSize := r.root.GetTotalSize()
	if blobSize > math.MaxInt64 {
		return 0, errors.New("total size exceeds maximum")
	}
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
	if nextPos != int64(r.idx) && r.chunkCache.ahead != nil {
		r.chunkCache.ahead.close()
		r.chunkCache.ahead = nil
	}
	r.idx = int(nextPos)
	return nextPos, nil
}

// Close cancels the reader and waits for its outstanding chunk reads.
func (r *Reader) Close() error {
	r.ctxCancel()
	if r.chunkCache.ahead != nil {
		r.chunkCache.ahead.close()
	}
	r.chunkCache = chunkReadCache{}
	return nil
}

// _ is a type assertion
var (
	_ io.Reader = (*Reader)(nil)
	_ io.Seeker = (*Reader)(nil)
	_ io.Closer = (*Reader)(nil)
)
