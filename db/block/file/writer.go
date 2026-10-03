package file

import (
	"bytes"
	"errors"
	"io"
	"math"
	"slices"
	"sort"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/db/tx"
)

// Writer is a handle that can write to a handle.
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
	// Discard the file read state and require a writable block transaction.
	w.clearReadState()
	if w.btx == nil {
		return nil, nil, tx.ErrNotWrite
	}

	// Commit the file blocks and retain the resulting cursor on success.
	ref, ncs, err := w.btx.Write(w.ctx, true)
	if err == nil {
		w.bcs = ncs
	}
	return ref, ncs, err
}

// Write writes to the handle, immediately flushing if btx is set.
func (w *Writer) Write(p []byte) (n int, err error) {
	// Write the bytes at the current file position.
	idx := w.idx
	if err := w.WriteBytes(idx, p); err != nil {
		return 0, err
	}

	// Advance the file position and flush an attached block transaction.
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
	// Reject empty writes before preparing blob or range state.
	if dataLen <= 0 {
		return nil
	}

	// Reuse the append operation for an existing root or range blob.
	// appendToBlob appends to an existing blob
	appendToBlob := func(rcsBlob *blob.Blob, rblobCs *block.Cursor) error {
		return rcsBlob.AppendData(w.ctx, dataLen, dataRdr, rblobCs, w.buildBlobOpts)
	}

	// Replace the file root when this write covers the existing contents.
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
		return w.normalize()
	}

	// Append directly to an un-ranged root blob when the write is contiguous.
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
		return w.normalize()
	}

	// Move the root blob into a range before handling partial writes.
	// XXX: optimization: if index==0, check root blob has same len and contents
	if err := w.moveRootBlobToRange(); err != nil {
		return err
	}

	// Extend a contiguous range when no higher-nonce range would overlap.
	// optimization: extend the range at the location
	// to do this properly, need to assert that:
	// - the range is the highest nonce for that position
	// - there is no range following the range w/ a higher nonce within the write len
	// Locate a candidate range and inspect its overlap boundary.
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
					w.compactOccludedRanges()
					w.clearReadState()

					return w.normalize()
				}
			}
		}
	}

	// Create a new range for data that cannot extend an existing range.
	nonce := w.root.GetRangeNonce()
	w.root.RangeNonce += 1
	w.root.Ranges = append(w.root.Ranges, &Range{
		Nonce:  nonce,
		Start:  index,
		Length: uint64(dataLen),
		Ref:    nil, // will be filled by writer
	})

	// Replace the range's blob reference with the newly built blob.
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
		w.rangeSet.GetCursor().ClearRef(uint32(rlen)) //nolint:gosec
		w.root.Ranges = w.root.Ranges[:len(w.root.Ranges)-1]
		w.root.RangeNonce -= 1
		return err
	}

	// Sort and compact ranges, then grow the file size when necessary.
	size := bblob.GetTotalSize()
	w.sortRanges()
	w.compactOccludedRanges()
	w.clearReadState()

	// Extend the file size to include the newly written range.
	oldSize := w.root.GetTotalSize()
	nextSize := index + size
	if nextSize > oldSize {
		w.root.TotalSize = nextSize
		w.bcs.MarkDirty()
	}

	// Fold the new range into the root blob when it can be embedded.
	return w.normalize()
}

// WriteBytes writes bytes to a blob and then to an index.
func (w *Writer) WriteBytes(index uint64, data []byte) error {
	return w.WriteFrom(index, int64(len(data)), bytes.NewReader(data))
}

// WriteBlob writes a blob to an index in a new range.
// Implies removing any ranges which are completely occluded.
func (w *Writer) WriteBlob(index, size uint64, ref *block.BlockRef) error {
	// Move the root blob into a range before adding the referenced blob.
	if err := w.moveRootBlobToRange(); err != nil {
		return err
	}

	// Allocate a new range and connect it to the referenced blob.
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

	// Order the file ranges and remove ranges hidden by newer writes.
	w.sortRanges() // TODO: faster sorted insert
	w.compactOccludedRanges()

	// Extend the file size to include the referenced blob.
	oldSize := w.root.GetTotalSize()
	nextSize := index + size
	if nextSize > oldSize {
		w.root.TotalSize = nextSize
		w.bcs.MarkDirty()
	}

	// Fold the range into the root blob when it can be embedded.
	return w.normalize()
}

// Reset completely clears the contents of the file.
func (w *Writer) Reset() {
	// Clear the file contents, range references, and size together.
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
	// Keep the file state when its size already matches the request.
	if size == w.root.GetTotalSize() {
		return nil
	}

	// Discard cached reads and clear all contents for a zero-length file.
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
		for i, v := range slices.Backward(w.root.Ranges) {
			irange := v
			irangeStart := irange.GetStart()
			if irangeStart >= size {
				removeFrom = i

				rangesBcs.ClearRef(uint32(i)) //nolint:gosec
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

		// drop the root blob contents past the new end of file
		if err := w.trimRootBlob(size); err != nil {
			return err
		}
	} else {
		// when adding size to the file:
		// - lookup the last range in the file
		// - create a new range filled with zeros over the portion of the range that
		//   extends past the end of the new file length.
		// alternatively: reduce the len of the ranges using the same code as above
		var zeroFrom, zeroTo uint64
		var zeroRange bool
		if len(w.root.Ranges) == 0 {
			// ensure that the root blob is shorter than total size
			if err := w.trimRootBlob(oldSize); err != nil {
				return err
			}
		} else {
			lastRange := w.root.Ranges[len(w.root.Ranges)-1]
			lastRangeStart := lastRange.GetStart()
			lastRangeEnd := lastRangeStart + lastRange.GetLength()
			if lastRangeEnd > oldSize {
				if oldSize > math.MaxInt || lastRangeEnd > math.MaxInt {
					return errors.New("file size exceeds maximum")
				}
				zeroFrom = oldSize
				zeroTo = lastRangeEnd
				zeroRange = true
			}
		}

		if zeroRange && zeroTo > zeroFrom {
			// write a zeroed range
			err := w.WriteBlob(zeroFrom, zeroTo-zeroFrom, nil)
			if err != nil {
				return err
			}
		}
	}

	// set the filesize to the new size
	w.root.TotalSize = size

	// Place the truncated contents in their canonical shape.
	return w.normalize()
}

// trimRootBlob truncates the root blob to at most size bytes.
func (w *Writer) trimRootBlob(size uint64) error {
	// Keep the root blob when it already fits within the requested size.
	rootBlob := w.root.GetRootBlob()
	if rootBlob.GetTotalSize() <= size {
		return nil
	}
	if size > math.MaxInt64 {
		return errors.New("total size exceeds maximum")
	}

	// Truncate the root blob and mark the file block dirty.
	rootBlobBcs := w.bcs.FollowSubBlock(2)
	if err := rootBlob.Truncate(w.ctx, rootBlobBcs, w.buildBlobOpts, int64(size)); err != nil {
		return err
	}
	w.bcs.MarkDirty()
	return nil
}

// moveRootBlobToRange moves the root blob if it is set to a range.
func (w *Writer) moveRootBlobToRange() error {
	// Keep the root blob when file ranges already exist.
	if len(w.root.Ranges) != 0 {
		return nil
	}

	// the root blob may extend past the end of the file: drop those bytes.
	if err := w.trimRootBlob(w.root.GetTotalSize()); err != nil {
		return err
	}

	// Read the trimmed root blob size and skip an empty blob.
	rblob := w.root.GetRootBlob()
	rblobSize := rblob.GetTotalSize()
	if rblobSize == 0 {
		return nil
	}

	// Locate the root blob cursor and allocate its range nonce.
	rblobBcs := w.bcs.FollowSubBlock(2)
	nonce := w.root.GetRangeNonce()

	// Create a range spanning the former root blob.
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

	// Order and compact file ranges and discard the former read state.
	w.sortRanges()
	w.compactOccludedRanges()
	w.clearReadState()
	return nil
}

// maxInlineRawBlobSize is the largest raw blob a file block embeds as its root
// blob. A larger raw blob is stored as its own block and referenced by a range,
// so a file written from an already stored blob does not copy its bytes.
const maxInlineRawBlobSize = 4096

// inlineBlob returns whether the file block embeds b as its root blob.
// A chunked blob embeds its chunk index, which stays small.
func inlineBlob(b *blob.Blob) bool {
	return b.GetBlobType() != blob.BlobType_BlobType_RAW || b.GetTotalSize() <= maxInlineRawBlobSize
}

// normalize places the file contents in their canonical shape, which depends
// only on the contents and not on the writes that produced them. A root blob
// that inlineBlob rejects moves to a range, a single range at zero holding an
// inline blob moves to the root blob, and a remaining single range gets the
// first nonce.
func (w *Writer) normalize() error {
	if !inlineBlob(w.root.GetRootBlob()) {
		if err := w.moveRootBlobToRange(); err != nil {
			return err
		}
	}
	if err := w.moveRangeToRootBlob(); err != nil {
		return err
	}
	if len(w.root.GetRanges()) == 1 {
		w.root.Ranges[0].Nonce = 0
		w.root.RangeNonce = 1
		w.bcs.MarkDirty()
	}
	return nil
}

// moveRangeToRootBlob embeds the blob of a single range starting at zero into
// the file block when inlineBlob allows it. Otherwise it does nothing.
func (w *Writer) moveRangeToRootBlob() error {
	// Require a single range starting at the file origin.
	ranges := w.root.GetRanges()
	if len(ranges) != 1 || ranges[0].GetStart() != 0 {
		return nil
	}

	// Load the range blob and require that it can be embedded.
	rootRange := ranges[0]
	_, rangeBcs := w.rangeSet.Get(0)
	rangeBlobBcs := rangeBcs.FollowRef(4, rootRange.GetRef())
	nrootBlob, err := blob.UnmarshalBlob(w.ctx, rangeBlobBcs)
	if err != nil {
		return err
	}
	if !inlineBlob(nrootBlob) {
		return nil
	}

	// Replace the range references with the embedded root blob.
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

	// Discard read state that refers to the former range.
	w.clearReadState()
	return nil
}

// sortRanges sorts the root ranges stitching the block graph.
func (w *Writer) sortRanges() {
	hrs := NewHandleRangeSlice(w.Handle)
	sort.Sort(hrs)
}

func (w *Writer) compactOccludedRanges() {
	ranges := w.root.GetRanges()
	if len(ranges) <= 1 {
		return
	}

	for i := range slices.Backward(ranges) {
		if rangeCoveredByHigherNonce(ranges, i) {
			w.deleteRange(i)
			ranges = w.root.GetRanges()
		}
	}
}

func (w *Writer) deleteRange(idx int) {
	// Shift following file ranges over the deleted range.
	ranges := w.root.GetRanges()
	lastIdx := len(ranges) - 1
	for i := idx; i < lastIdx; i++ {
		w.rangeSet.Swap(i, i+1)
	}

	// Remove the final range slot after shifting the remaining ranges.
	w.root.Ranges[lastIdx] = nil
	w.root.Ranges = w.root.Ranges[:lastIdx]

	// Clear the deleted range reference and mark the file block dirty.
	w.rangeSet.GetCursor().ClearRef(uint32(lastIdx)) //nolint:gosec
	w.bcs.MarkDirty()
}

// rangeCoveredByHigherNonce returns true if the range at idx is fully
// covered by a later range with a higher nonce.
func rangeCoveredByHigherNonce(ranges []*Range, idx int) bool {
	// Establish the span whose coverage by newer ranges must be checked.
	rng := ranges[idx]
	start := rng.GetStart()
	end := start + rng.GetLength()
	coveredEnd := start

	// Extend the covered span until newer ranges cover it or make no progress.
	for {
		var advanced bool
		for i, other := range ranges {
			if i == idx || other.GetNonce() <= rng.GetNonce() {
				continue
			}
			otherStart := other.GetStart()
			otherEnd := otherStart + other.GetLength()
			if otherStart > coveredEnd || otherEnd <= coveredEnd {
				continue
			}
			coveredEnd = otherEnd
			advanced = true
			if coveredEnd >= end {
				return true
			}
		}
		if !advanced {
			return false
		}
	}
}

// _ is a type assertion
var _ io.Writer = (*Writer)(nil)
