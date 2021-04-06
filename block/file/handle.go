package file

import (
	"context"
	"io"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/blob"
	"github.com/pkg/errors"
)

// Handle is a open file handle, using a block transaction and a
// Not concurrency safe.
type Handle struct {
	ctx       context.Context
	ctxCancel context.CancelFunc
	bcs       *block.Cursor

	root         *File
	idx          uint64
	nextEval     uint64
	lastStartIdx int

	currentRangeIdx int
	currentRange    *Range
	currentBlob     *blob.Reader
}

// NewHandle constructs a new reader.
// bcs is located at the root of the file.
func NewHandle(
	ctx context.Context,
	bcs *block.Cursor,
	root *File,
) *Handle {
	rdr := &Handle{bcs: bcs, root: root}
	rdr.ctx, rdr.ctxCancel = context.WithCancel(ctx)
	return rdr
}

// GetRef returns the root reference.
func (r *Handle) GetRef() *block.BlockRef {
	return r.bcs.GetRef()
}

// Size returns the total size of the file.
func (r *Handle) Size() uint64 {
	return r.root.GetTotalSize()
}

// Read implements the reader interface.
// Read and Seek are not concurrent safe.
func (r *Handle) Read(p []byte) (n int, err error) {
	totalSize := r.root.GetTotalSize()
	readSize := uint64(len(p))

	if err := r.evaluateCurrentRange(); err != nil {
		return 0, err
	}

	idx := r.idx
	// readEnd is the index after the one we will read to.
	readEnd := idx + readSize
	// zeros if current == nil
	if r.currentRange == nil || r.currentBlob == nil {
		// zeroEnd is the index after the zeros.
		zeroEnd := r.nextEval
		if zeroEnd > totalSize {
			zeroEnd = totalSize
		}
		if zeroEnd < readEnd {
			readEnd = zeroEnd
		}
		// read up to min(readEnd, zeroEnd)
		readN := readEnd - idx
		// this is optimized by compiler to memset
		for i := 0; i < int(readN); i++ {
			p[i] = 0
		}
		r.idx += readN
		return int(readN), nil
	}

	// otherwise we are reading from a blob...
	// nextEval will be at or before end of next blob.
	blobEnd := r.nextEval
	if readEnd > blobEnd {
		readEnd = blobEnd
	}
	readN := readEnd - idx
	blobReadN, err := r.currentBlob.Read(p[:readN])
	if err != nil {
		r.currentBlob = nil
		r.currentRange = nil
		r.currentRangeIdx = 0
		r.nextEval = 0
		return 0, err
	}

	nextIdx := r.idx + uint64(blobReadN)
	if nextIdx > blobEnd {
		nextIdx = blobEnd
	}
	r.idx = nextIdx
	return blobReadN, nil
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
// Seeking to an offset before the start of the file is an error.
// Seeking to any positive offset is legal, but the behavior of subsequent
// Read and Seek are not concurrent safe.
func (r *Handle) Seek(offset int64, whence int) (int64, error) {
	nextIdx := offset
	switch whence {
	case io.SeekCurrent:
		nextIdx += int64(r.idx)
	case io.SeekEnd:
		nextIdx += int64(r.root.GetTotalSize())
	}
	if nextIdx < 0 {
		return 0, errors.New("seek to before start of file")
	}
	// clear current fetched state if we move too much
	if nextIdx < int64(r.idx) ||
		(r.nextEval != 0 && int64(r.nextEval) < nextIdx) {
		r.clearReadState()
	}
	r.idx = uint64(nextIdx)
	return nextIdx, nil
}

// Close closes the reader, canceling the context.
// Not concurrency safe.
func (r *Handle) Close() error {
	r.ctxCancel()
	r.clearReadState()
	return nil
}

// evaluateCurrentRange updates the currentRange for the idx.
func (r *Handle) evaluateCurrentRange() error {
	select {
	case <-r.ctx.Done():
		return r.ctx.Err()
	default:
	}
	if r.nextEval > r.idx {
		return nil
	} else if r.nextEval == 0 || r.idx >= r.nextEval {
		r.nextEval = 0
		r.currentRange = nil
		r.currentRangeIdx = 0
		if r.currentBlob != nil {
			r.currentBlob.Close()
			r.currentBlob = nil
		}
	}
	totalSize := r.root.GetTotalSize()
	if r.idx >= totalSize {
		return io.EOF
	}
	ranges := r.root.GetRanges()
	idx := r.idx
	if len(ranges) == 0 {
		rootBlob := r.root.GetRootBlob()
		rootBlobSize := rootBlob.GetTotalSize()
		if idx >= rootBlobSize {
			return io.EOF
		}
		r.currentRange = &Range{
			Start:  0,
			Length: rootBlobSize,
		}
		r.currentRangeIdx = 0
		r.currentBlob = blob.NewReader(r.ctx, r.bcs, rootBlob)
		r.nextEval = rootBlobSize
		return nil
	}

	// Find ranges where start < idx and start + len > idx.
	// Locate the range with the highest nonce (newest)
	var bestNonce uint64
	r.currentRange = nil
	r.currentRangeIdx = 0
	if r.currentBlob != nil {
		r.currentBlob.Close()
		r.currentBlob = nil
	}
	for i := r.lastStartIdx; i < len(ranges); i++ {
		st := ranges[i].GetStart()
		if st > idx {
			if st < r.nextEval || r.nextEval == 0 {
				r.nextEval = st
			}
			break
		}

		// end is the last index + 1
		end := st + ranges[i].GetLength()
		if end <= idx {
			// if the end is less than the current idx
			// only increment this by 1, sometimes we may backtrack
			if r.lastStartIdx == i-1 {
				r.lastStartIdx = i
			}
			continue
		}

		// this blob is in range, take it if the nonce is higher than the current best.
		rangeNonce := ranges[i].GetNonce()
		if bestNonce == 0 || rangeNonce > bestNonce {
			if end < r.nextEval || r.nextEval == 0 {
				r.nextEval = end
			}
			r.currentRange = ranges[i]
			r.currentRangeIdx = i
			bestNonce = rangeNonce
		}
	}

	if r.currentRange == nil {
		// there is no range for this index (zeros, sparse file)
		// nextEval is set to the next block we will encounter
		// if there is no nextEval:
		if r.nextEval == 0 {
			r.nextEval = totalSize
		}
		return nil
	}

	// lookup the blob at this index.
	// this might trigger a network fetch.
	blobRoot, blobCs, err := r.followRootRangeBlobRef(
		r.currentRangeIdx,
		r.currentRange.GetRef(),
	)
	if err != nil {
		return err
	}
	r.currentBlob = blob.NewReader(r.ctx, blobCs, blobRoot)

	// say we are at index 100
	// blob might start at index 50
	// we need to seek 100-50 = 50 past the start
	seekPos := int64(r.idx) - int64(r.currentRange.GetStart())
	if seekPos < 0 {
		r.clearReadState()
		return errors.New("inconsistent range start position")
	}
	if seekPos != 0 {
		_, err := r.currentBlob.Seek(seekPos, io.SeekStart)
		if err != nil {
			return err
		}
	}
	return nil
}

// clearReadState clears the read state.
func (r *Handle) clearReadState() {
	if r.currentBlob != nil {
		r.currentBlob.Close()
		r.currentBlob = nil
	}
	r.currentRange = nil
	r.currentRangeIdx = 0
	r.nextEval = 0
	r.lastStartIdx = 0
}

// followRootRangeBlobRef follows a block reference in a Range in the root
func (r *Handle) followRootRangeBlobRef(
	idx int,
	blobRef *block.BlockRef,
) (*blob.Blob, *block.Cursor, error) {
	refID := NewFileRangeRefId(idx)
	ncs := r.bcs.FollowRef(refID, blobRef)
	blobi, err := ncs.Unmarshal(blob.NewBlobBlock)
	if err != nil {
		return nil, nil, err
	}
	if blobi == nil {
		return nil, nil, errors.Errorf("blob at index %d reference empty", idx)
	}
	return blobi.(*blob.Blob), ncs, nil
}

// _ is a type assertion
var (
	_ io.Reader = ((*Handle)(nil))
	_ io.Seeker = ((*Handle)(nil))
	_ io.Closer = ((*Handle)(nil))
	// _ io.WriterTo = ((*Handle)(nil))
)
