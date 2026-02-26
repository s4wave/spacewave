package file

import (
	"bytes"
	"errors"
	"io"
	"math"
	"sort"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/blob"
	"github.com/aperturerobotics/hydra/tx"
)

// Writer is a handle that can write to a handle.
// TODO: drop any ranges which are fully occluded by new ranges.
type Writer struct {
	*Handle

	btx           *block.Transaction
	buildBlobOpts *blob.BuildBlobOpts
}

// NewWriter builds a new writer handle.
// btx can be nil
// buildBlobOpts can be nil
func NewWriter(
	h *Handle,
	btx *block.Transaction,
	buildBlobOpts *blob.BuildBlobOpts,
) *Writer {
	if buildBlobOpts == nil {
		buildBlobOpts = &blob.BuildBlobOpts{}
	}
	return &Writer{
		Handle:        h,
		btx:           btx,
		buildBlobOpts: buildBlobOpts,
	}
}

// CommitWriter commits any pending writes using a block transaction.
// Note: the block transaction must match the handle's block cursor.
func CommitWriter(w *Writer) (*block.BlockRef, *block.Cursor, error) {
	w.clearReadState()
	if w.btx == nil {
		return nil, nil, tx.ErrNotWrite
	}
	ref, ncs, err := w.btx.Write(w.ctx, true)
	if err == nil {
		w.bcs = ncs
	}
	return ref, ncs, err
}

// Write writes to the handle, immediately flushing if btx is set.
func (w *Writer) Write(p []byte) (n int, err error) {
	idx := w.idx
	if err := w.WriteBytes(idx, p); err != nil {
		return 0, err
	}
	w.idx += uint64(len(p))
	if w.btx != nil {
		_, _, err = CommitWriter(w)
		if err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

// WriteFrom writes data from a reader to a blob and then an index.
func (w *Writer) WriteFrom(index uint64, dataLen int64, dataRdr io.Reader) error {
	if dataLen <= 0 {
		return nil
	}

	// appendToBlob appends to an existing blob
	appendToBlob := func(rcsBlob *blob.Blob, rblobCs *block.Cursor) error {
		return rcsBlob.AppendData(w.ctx, dataLen, dataRdr, rblobCs, w.buildBlobOpts)
	}

	// optimization: if start=0 and len >= size, fully overwrite the entire file
	rootTotalSize := w.root.GetTotalSize()
	if rootTotalSize > math.MaxInt64 {
		return errors.New("total size exceeds maximum")
	}
	if index == 0 && dataLen >= int64(rootTotalSize) {
		// clear file contents
		w.Reset()
		// set root blob to the new contents
		rootBlobCs := w.bcs.FollowSubBlock(2)
		b1, err := blob.BuildBlob(w.ctx, dataLen, dataRdr, rootBlobCs, w.buildBlobOpts)
		if err != nil {
			return err
		}
		w.root.RootBlob = b1
		// set total size to new size
		w.root.TotalSize = uint64(dataLen)
		w.bcs.MarkDirty()
		return nil
	}

	// optimization: if root blob is set and len == index, append to it
	rlen := len(w.root.Ranges)
	rootBlobSize := w.root.GetRootBlob().GetTotalSize()
	if rlen == 0 && rootBlobSize <= w.root.GetTotalSize() && rootBlobSize == index {
		rootBlobCs := w.bcs.FollowSubBlock(2)
		if err := appendToBlob(w.root.GetRootBlob(), rootBlobCs); err != nil {
			return err
		}
		rootBlobEnd := w.root.GetRootBlob().GetTotalSize()
		if rootBlobEnd > w.root.TotalSize {
			w.root.TotalSize = rootBlobEnd
			w.bcs.MarkDirty()
		}
		return nil
	}

	// XXX: optimization: if index==0, check root blob has same len and contents
	if err := w.moveRootBlobToRange(); err != nil {
		return err
	}

	// optimization: extend the range at the location
	// to do this properly, need to assert that:
	// - the range is the highest nonce for that position
	// - there is no range following the range w/ a higher nonce within the write len
	ranges := w.root.Ranges
	rlen = len(ranges)
	if rlen != 0 && index != 0 {
		if index > math.MaxInt {
			return errors.New("write index exceeds maximum")
		}
		rangeSlice := RangeSlice(ranges)
		rng, rngIdx, rngFound := rangeSlice.LocatePosition(int(index) - 1)
		writeEnd := index + uint64(dataLen)
		// if the range covers index-1 and ends at the write index...
		if rngFound && rng.GetStart()+rng.GetLength() == index {
			// scan forward
			// make sure there are no ranges covering [pos, writeEnd) w/ higher nonce
			var found bool
			for i := rngIdx + 1; i < rlen; i++ {
				rrng := ranges[i]
				rrngStart := rrng.GetStart()
				if rrngStart >= writeEnd {
					// sorted by start pos, there will be no more ranges covering
					break
				}
				rrngEnd := rrngStart + rrng.GetLength()
				if rrngEnd < index {
					continue
				}
				// ignore nonce < rng.Nonce
				if rrng.GetNonce() > rng.GetNonce() {
					found = true
					break
				}
			}
			// if found = true, we can't extend this range, it will collide with another.
			if !found {
				// append to the range
				_, rcs := w.rangeSet.Get(rngIdx)
				rblobCs := rng.FollowBlob(rcs)
				rcsBlob, err := blob.UnmarshalBlob(w.ctx, rblobCs)
				if err != nil {
					return err
				}
				// only append to blob with length filling entire range
				rcsBlobSize := rcsBlob.GetTotalSize()
				if rcsBlobSize != 0 && rcsBlobSize == rng.GetLength() {
					err = appendToBlob(rcsBlob, rblobCs)
					if err != nil {
						return err
					}
					rng.Length = rcsBlob.GetTotalSize()
					lastRangeEnd := rng.Start + rng.Length
					if lastRangeEnd > w.root.TotalSize {
						w.root.TotalSize = lastRangeEnd
					}
					rcs.MarkDirty()
					w.clearReadState()

					if err := w.moveRangeToRootBlob(); err != nil {
						return err
					}
					return nil
				}
			}
		}
	}

	nonce := w.root.GetRangeNonce()
	w.root.RangeNonce += 1
	w.root.Ranges = append(w.root.Ranges, &Range{
		Nonce:  nonce,
		Start:  index,
		Length: uint64(dataLen),
		Ref:    nil, // will be filled by writer
	})

	_, rangeCs := w.rangeSet.Get(rlen)
	rangeCs.ClearRef(4)
	rcs := rangeCs.FollowRef(4, nil)
	// rcs.SetBlock() and MarkDirty() will be called
	bblob, err := blob.BuildBlob(
		w.ctx,
		dataLen,
		dataRdr,
		rcs,
		w.buildBlobOpts,
	)
	if err != nil {
		w.rangeSet.GetCursor().ClearRef(uint32(rlen))
		w.root.Ranges = w.root.Ranges[:len(w.root.Ranges)-1]
		w.root.RangeNonce -= 1
		return err
	}

	size := bblob.GetTotalSize()
	w.sortRanges()
	w.clearReadState()

	oldSize := w.root.GetTotalSize()
	nextSize := index + size
	if nextSize > oldSize {
		w.root.TotalSize = nextSize
		w.bcs.MarkDirty()
	}

	// move range to root blob if possible
	if err := w.moveRangeToRootBlob(); err != nil {
		return err
	}

	return nil
}

// WriteBytes writes bytes to a blob and then to an index.
func (w *Writer) WriteBytes(index uint64, data []byte) error {
	return w.WriteFrom(index, int64(len(data)), bytes.NewReader(data))
}

// WriteBlob writes a blob to an index in a new range.
// Implies removing any ranges which are completely occluded.
func (w *Writer) WriteBlob(index, size uint64, ref *block.BlockRef) error {
	if err := w.moveRootBlobToRange(); err != nil {
		return err
	}
	nonce := w.root.GetRangeNonce()
	w.root.RangeNonce += 1
	rlen := len(w.root.Ranges)
	w.clearReadState()
	w.root.Ranges = append(w.root.Ranges, &Range{
		Nonce:  nonce,
		Start:  index,
		Length: size,
		Ref:    ref,
	})
	_, rcs := w.rangeSet.Get(rlen)
	rcs.ClearRef(4)
	w.sortRanges() // TODO: faster sorted insert

	oldSize := w.root.GetTotalSize()
	nextSize := index + size
	if nextSize > oldSize {
		w.root.TotalSize = nextSize
		w.bcs.MarkDirty()
	}

	// move the range to the root blob if possible
	if err := w.moveRangeToRootBlob(); err != nil {
		return err
	}

	return nil
}

// Reset completely clears the contents of the file.
func (w *Writer) Reset() {
	rangesBcs := w.bcs.FollowSubBlock(4)
	w.root.RootBlob = nil
	w.bcs.ClearRef(2)
	w.root.Ranges = nil
	rangesBcs.ClearAllRefs()
	w.root.RangeNonce = 0
	w.root.TotalSize = 0
	w.bcs.MarkDirty()
}

// Truncate shrinks or extends the file handle to the given size.
func (w *Writer) Truncate(size uint64) error {
	if size == w.root.GetTotalSize() {
		return nil
	}

	w.clearReadState()
	rangesBcs := w.bcs.FollowSubBlock(4)
	if size == 0 {
		// special case: rapidly clear the file contents
		w.Reset()
		return nil
	}

	// when reducing size from file:
	oldSize := w.root.GetTotalSize()
	if size < oldSize {
		// drop/trim any ranges that are outside the new file
		removeFrom := -1
		for i := len(w.root.Ranges) - 1; i >= 0; i-- {
			irange := w.root.Ranges[i]
			irangeStart := irange.GetStart()
			if irangeStart >= size {
				removeFrom = i
				rangesBcs.ClearRef(uint32(i)) //nolint:gosec
				w.root.Ranges[i] = nil
				continue
			}

			irangeLen := irange.GetLength()
			irangeEnd := irangeStart + irangeLen
			if irangeEnd <= size {
				continue
			}
			// shorten range to end of file
			irangeBcs := rangesBcs.FollowSubBlock(uint32(i)) //nolint:gosec
			if irangeEnd > size {
				// truncate the range + blob
				irangeBlobBcs := irange.FollowBlob(irangeBcs)
				irangeBlob, err := blob.UnmarshalBlob(w.ctx, irangeBlobBcs)
				if err != nil {
					return err
				}
				irangeLen = size - irangeStart
				if irangeBlob != nil {
					if irangeLen > math.MaxInt64 {
						return errors.New("range length exceeds maximum")
					}
					err = irangeBlob.Truncate(w.ctx, irangeBlobBcs, w.buildBlobOpts, int64(irangeLen))
					if err != nil {
						return err
					}
				}
				irange.Length = irangeLen
				irangeBcs.MarkDirty()
			}
		}
		if removeFrom == 0 {
			// fast path: clear file and set new size
			w.Reset()
		} else if removeFrom != -1 {
			w.root.Ranges = w.root.Ranges[:removeFrom]
			w.bcs.MarkDirty()
		}
	} else {
		// when adding size to the file:
		// - lookup the last range in the file
		// - create a new range filled with zeros over the portion of the range that
		//   extends past the end of the new file length.
		// alternatively: reduce the len of the ranges using the same code as above
		zeroFrom, zeroTo := -1, -1
		if len(w.root.Ranges) == 0 {
			// ensure that the root blob is shorter than total size
			rootBlob := w.root.GetRootBlob()
			rootBlobSize := rootBlob.GetTotalSize()
			if rootBlobSize > oldSize {
				rootBlobBcs := w.bcs.FollowSubBlock(2)
				if oldSize > math.MaxInt64 {
					return errors.New("total size exceeds maximum")
				}
				err := rootBlob.Truncate(w.ctx, rootBlobBcs, w.buildBlobOpts, int64(oldSize))
				if err != nil {
					return err
				}
			}
		} else {
			lastRange := w.root.Ranges[len(w.root.Ranges)-1]
			lastRangeStart := lastRange.GetStart()
			lastRangeEnd := lastRangeStart + lastRange.GetLength()
			if lastRangeEnd > oldSize {
				if oldSize > math.MaxInt || lastRangeEnd > math.MaxInt {
					return errors.New("file size exceeds maximum")
				}
				zeroFrom = int(oldSize)
				zeroTo = int(lastRangeEnd)
			}
		}

		if zeroFrom >= 0 && zeroTo > zeroFrom {
			// write a zeroed range
			err := w.WriteBlob(uint64(zeroFrom), uint64(zeroTo-zeroFrom), nil) //nolint:gosec
			if err != nil {
				return err
			}
		}
	}

	// set the filesize to the new size
	w.root.TotalSize = size

	// move range to root blob if applicable
	if err := w.moveRangeToRootBlob(); err != nil {
		return err
	}
	return nil
}

// moveRootBlobToRange moves the root blob if it is set to a range.
func (w *Writer) moveRootBlobToRange() error {
	rblob := w.root.GetRootBlob()
	rblobSize := rblob.GetTotalSize()
	if len(w.root.Ranges) != 0 || rblobSize == 0 {
		return nil
	}

	rblobBcs := w.bcs.FollowSubBlock(2)
	nonce := w.root.GetRangeNonce()

	w.root.RangeNonce += 1
	w.root.Ranges = append(w.root.Ranges, &Range{
		Nonce:  nonce,
		Start:  0,
		Length: rblobSize,
		Ref:    nil, // will be filled by writer
	})

	// set range -> blob to old root Blob
	_, rcs := w.rangeSet.Get(len(w.root.Ranges) - 1)
	rcs.ClearRef(4)
	rcs.SetRef(4, rblobBcs)

	// clear root blob subblock
	w.bcs.ClearRef(2)
	w.root.RootBlob = nil

	w.sortRanges()
	w.clearReadState()
	return nil
}

// moveRangeToRootBlob moves data to a root blob if there is only a single range
// with start == 0 or containing zeros only. otherwise does nothing.
func (w *Writer) moveRangeToRootBlob() error {
	ranges := w.root.GetRanges()
	// note: can probably remove the ranges[0].getstart check here.
	if len(ranges) != 1 || ranges[0].GetStart() != 0 {
		return nil
	}

	rootRange := ranges[0]
	_, rangeBcs := w.rangeSet.Get(0)
	rangeBlobBcs := rangeBcs.FollowRef(4, rootRange.GetRef())
	nrootBlob, err := blob.UnmarshalBlob(w.ctx, rangeBlobBcs)
	if err != nil {
		return err
	}

	// skip moving range if it is non-zeros and starts at an offset
	if nrootBlob != nil && rootRange.GetStart() != 0 {
		return nil
	}

	w.root.RangeNonce = 0
	w.root.Ranges = nil
	w.rangeSet.GetCursor().ClearAllRefs()
	if nrootBlob != nil {
		if err := rangeBlobBcs.SetAsSubBlock(2, w.bcs); err != nil {
			return err
		}
		w.root.RootBlob = nrootBlob
	} else {
		w.root.RootBlob = nil
		w.bcs.ClearRef(2)
	}

	w.clearReadState()
	return nil
}

// sortRanges sorts the root ranges stitching the block graph.
func (w *Writer) sortRanges() {
	hrs := NewHandleRangeSlice(w.Handle)
	sort.Sort(hrs)
}

// _ is a type assertion
var _ io.Writer = ((*Writer)(nil))
