package file

import (
	"context"
	"io"
	"math"
	"runtime/trace"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/db/block/sbset"
)

// Handle is a open file handle, using a block transaction and a
// Not concurrency safe.
type Handle struct {
	ctx       context.Context
	ctxCancel context.CancelFunc
	bcs       *block.Cursor

	root     *File
	idx      uint64
	nextEval uint64

	rangeSet        *sbset.SubBlockSet
	currentRangeIdx int
	currentRange    *Range
	currentBlob     *blob.Reader // nil if currentRange is zeros
}

// NewHandle constructs a new reader.
// bcs is located at the root of the file.
func NewHandle(
	ctx context.Context,
	bcs *block.Cursor,
	root *File,
) *Handle {
	if root == nil {
		root = &File{}
		bcs.SetBlock(root, false)
	}
	rdr := &Handle{
		bcs:      bcs,
		root:     root,
		rangeSet: NewRangeSet(&root.Ranges, bcs.FollowSubBlock(4)),
	}
	rdr.ctx, rdr.ctxCancel = context.WithCancel(ctx)
	return rdr
}

// GetCursor returns the underlying cursor pointing to *File.
func (r *Handle) GetCursor() *block.Cursor {
	return r.bcs
}

// GetRef returns the root reference.
func (r *Handle) GetRef() *block.BlockRef {
	return r.bcs.GetRef()
}

// Size returns the total size of the file.
func (r *Handle) Size() uint64 {
	return r.root.GetTotalSize()
}

// ComputeStorageSize computes the total size of all blocks making up the File.
//
// note: not accurate until the btx has been committed.
func (r *Handle) ComputeStorageSize(ctx context.Context) (uint64, error) {
	var storageSize uint64

	// add the size of the root block
	rootData, _, err := r.bcs.Fetch(ctx)
	if err != nil {
		return 0, err
	}
	storageSize += uint64(len(rootData))

	ranges := r.root.GetRanges()
	if len(ranges) == 0 {
		// use root blob
		rootBlobBcs := r.bcs.FollowSubBlock(2)
		blobStorageSize, _, err := r.root.GetRootBlob().ComputeStorageSize(ctx, rootBlobBcs)
		if err != nil {
			return 0, err
		}
		storageSize += blobStorageSize
		return storageSize, nil
	}

	// iterate over ranges
	rangesBcs := r.bcs.FollowSubBlock(4)
	for i, r := range ranges {
		rangeBcs := rangesBcs.FollowSubBlock(uint32(i)) //nolint:gosec
		blobBcs := r.FollowBlob(rangeBcs)
		bl, err := blob.UnmarshalBlob(ctx, blobBcs)
		if err != nil {
			return 0, err
		}
		blobStorageSize, _, err := bl.ComputeStorageSize(ctx, blobBcs)
		if err != nil {
			return 0, err
		}
		storageSize += blobStorageSize
	}

	return storageSize, nil
}

// Read implements the reader interface.
// Read and Seek are not concurrent safe.
// It fills p across range boundaries until p is full, EOF is reached, or a
// selected blob reader stops making progress.
func (r *Handle) Read(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	traceEnabled := trace.IsEnabled()
	readCtx := r.ctx
	if traceEnabled {
		var task *trace.Task
		readCtx, task = trace.NewTask(readCtx, "db/block/file/handle/read")
		defer task.End()
	}
	totalSize := r.root.GetTotalSize()
	if r.idx >= totalSize {
		return 0, io.EOF
	}

	for n < len(p) && r.idx < totalSize {
		var evalErr error
		if traceEnabled {
			_, task := trace.NewTask(readCtx, "db/block/file/handle/read/evaluate-range")
			evalErr = r.evaluateCurrentRange()
			task.End()
		} else {
			evalErr = r.evaluateCurrentRange()
		}
		if evalErr != nil {
			if n > 0 && evalErr == io.EOF {
				return n, nil
			}
			return n, evalErr
		}

		idx := r.idx
		if idx >= totalSize {
			break
		}

		readLen := min(uint64(len(p)-n), totalSize-idx)
		readEnd := idx + readLen
		if r.nextEval != 0 && r.nextEval < readEnd {
			readEnd = r.nextEval
		}
		if readEnd <= idx {
			r.clearReadState()
			if n > 0 {
				return n, nil
			}
			return 0, errors.New("invalid file read range")
		}

		readN := int(readEnd - idx) //nolint:gosec
		if r.currentRange == nil || r.currentBlob == nil {
			// this is optimized by compiler to memset
			for i := range readN {
				p[n+i] = 0
			}
			r.idx += uint64(readN) //nolint:gosec
			n += readN
			continue
		}

		var blobReadN int
		if traceEnabled {
			_, task := trace.NewTask(readCtx, "db/block/file/handle/read/blob-read")
			blobReadN, err = r.currentBlob.Read(p[n : n+readN])
			task.End()
		} else {
			blobReadN, err = r.currentBlob.Read(p[n : n+readN])
		}
		if blobReadN > 0 {
			nextIdx := min(r.idx+uint64(blobReadN), readEnd) //nolint:gosec
			r.idx = nextIdx
			n += blobReadN
		}
		if err != nil {
			if r.currentBlob != nil {
				r.currentBlob.Close()
				r.currentBlob = nil
			}
			r.currentRange = nil
			r.currentRangeIdx = 0
			r.nextEval = 0
			if err == io.EOF && blobReadN > 0 && r.idx == readEnd {
				// EOF with bytes read at the planned boundary continues the
				// loop so evaluateCurrentRange selects the next covering span.
				continue
			}
			if n > 0 {
				return n, nil
			}
			return 0, err
		}
		if blobReadN == 0 {
			break
		}
	}
	if n == 0 && r.idx >= totalSize {
		return 0, io.EOF
	}
	return n, nil
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
	if r.idx > math.MaxInt64 {
		return 0, errors.New("file position exceeds maximum")
	}
	if r.root.GetTotalSize() > math.MaxInt64 {
		return 0, errors.New("total size exceeds maximum")
	}
	nextIdx := offset
	switch whence {
	case io.SeekCurrent:
		nextIdx += int64(r.idx)
	case io.SeekEnd:
		nextIdx += int64(r.root.GetTotalSize()) //nolint:gosec
	}
	if nextIdx < 0 {
		return 0, errors.New("seek to before start of file")
	}
	currIdx := int64(r.idx)
	if nextIdx == currIdx {
		return nextIdx, nil
	}
	nextEval := int64(r.nextEval) //nolint:gosec
	if nextIdx < currIdx || (nextEval != 0 && nextEval <= nextIdx) {
		// if rewinding or if next idx > nextEval, clear read state.
		r.clearReadState()
	} else if nextIdx > currIdx {
		// fast-forward the blob reader if necessary
		if r.currentBlob != nil {
			if r.currentRange.GetStart() > math.MaxInt64 || r.currentRange.GetLength() > math.MaxInt64 {
				r.clearReadState()
				return 0, errors.New("range bounds exceed maximum")
			}
			rangeStart := int64(r.currentRange.GetStart()) //nolint:gosec
			rangeLen := int64(r.currentRange.GetLength())  //nolint:gosec
			if rangeStart+rangeLen <= nextIdx {
				// passed end of blob
				r.clearReadState()
			} else {
				// seek
				blobPos := nextIdx - rangeStart
				if _, err := r.currentBlob.Seek(blobPos, io.SeekStart); err != nil {
					// if any issue seeking, clear read state
					r.clearReadState()
				}
			}
		}
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

	seekBlob := func() error {
		if r.currentBlob == nil {
			return nil
		}
		// say we are at index 100
		// blob might start at index 50
		// we need to seek 100-50 = 50 past the start
		if r.idx > math.MaxInt64 || r.currentRange.GetStart() > math.MaxInt64 {
			r.clearReadState()
			return errors.New("position exceeds maximum")
		}
		seekPos := int64(r.idx) - int64(r.currentRange.GetStart()) //nolint:gosec
		if seekPos < 0 {
			r.clearReadState()
			return errors.New("inconsistent range start position")
		}
		_, err := r.currentBlob.Seek(seekPos, io.SeekStart)
		return err
	}

	if r.nextEval > r.idx {
		return seekBlob()
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
	var err error
	if len(ranges) == 0 {
		rootBlob := r.root.GetRootBlob()
		rootBlobSize := rootBlob.GetTotalSize()
		if idx >= rootBlobSize {
			// zeros
			r.currentRange = &Range{
				Start:  rootBlobSize,
				Length: totalSize - rootBlobSize,
			}
			r.currentBlob = nil
			r.nextEval = totalSize
			return nil
		}
		r.currentRange = &Range{
			Start:  0,
			Length: rootBlobSize,
		}
		r.currentRangeIdx = 0
		r.currentBlob, err = blob.NewReader(r.ctx, r.bcs.FollowSubBlock(2))
		if err != nil {
			return err
		}
		r.nextEval = rootBlobSize
		return seekBlob()
	}

	// Find every range covering idx and choose the highest nonce (newest).
	// Ranges are sorted by start, not by end. A small overwrite can end before a
	// lower-nonce base range that still covers the next byte, so correctness
	// requires checking all earlier starts until the first future start.
	var bestNonce uint64
	var foundRange bool
	r.currentRange = nil
	r.currentRangeIdx = 0
	if r.currentBlob != nil {
		r.currentBlob.Close()
		r.currentBlob = nil
	}
	for i := range ranges {
		st := ranges[i].GetStart()
		if st > idx {
			// evaluate at the next range with start > idx
			if st < r.nextEval || r.nextEval == 0 {
				r.nextEval = st
			}
			break
		}

		// end is the last index + 1
		end := st + ranges[i].GetLength()
		if end <= idx {
			continue
		}

		// this blob is in range, take it if the nonce is higher than the current best.
		rangeNonce := ranges[i].GetNonce()
		if !foundRange || rangeNonce > bestNonce {
			if end < r.nextEval || r.nextEval == 0 {
				// NOTE: if there's a range that starts after idx, but before
				// the end of this range, with a higher Nonce, then nextEval
				// will be adjusted at the beginning of the next loop iteration.
				r.nextEval = end
			}
			r.currentRange = ranges[i]
			r.currentRangeIdx = i
			bestNonce = rangeNonce
			foundRange = true
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
	_, blobCs, err := r.followRootRangeBlobRef(
		r.currentRangeIdx,
		r.currentRange.GetRef(),
	)
	if err != nil {
		return err
	}
	if blobCs != nil {
		r.currentBlob, err = blob.NewReader(r.ctx, blobCs)
		if err != nil {
			return err
		}
	} else {
		// read zeros
		r.currentBlob = nil
	}
	return seekBlob()
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
}

// followRootRangeBlobRef follows a block reference in a Range in the root.
func (r *Handle) followRootRangeBlobRef(
	idx int,
	blobRef *block.BlockRef,
) (*blob.Blob, *block.Cursor, error) {
	rangeCs := r.bcs.FollowSubBlock(4).FollowSubBlock(uint32(idx)) //nolint:gosec
	var ncs *block.Cursor
	if blobRef == nil {
		ncs = rangeCs.GetExistingRef(4)
		if ncs == nil {
			return nil, nil, nil
		}
	} else {
		ncs = rangeCs.FollowRef(4, blobRef)
	}
	blobi, err := ncs.Unmarshal(r.ctx, blob.NewBlobBlock)
	if err != nil {
		return nil, nil, err
	}
	if blobi == nil {
		return nil, nil, nil
	}
	return blobi.(*blob.Blob), ncs, nil
}

// _ is a type assertion
var (
	_ io.Reader = (*Handle)(nil)
	_ io.Seeker = (*Handle)(nil)
	_ io.Closer = (*Handle)(nil)
	// _ io.WriterTo = ((*Handle)(nil))
)
