package file

import (
	"bytes"
	"io"
	"sort"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/blob"
	"github.com/aperturerobotics/hydra/tx"
	"github.com/restic/chunker"
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
	// ensure chunking polynomial is set
	if poly := h.root.GetChunkingPol(); poly != 0 {
		buildBlobOpts.ChunkingPol = poly
	} else if poly := h.root.GetRootBlob().GetChunkIndex().GetPol(); poly != 0 {
		buildBlobOpts.ChunkingPol = poly
	} else if buildBlobOpts.ChunkingPol == 0 {
		np, _ := chunker.RandomPolynomial()
		buildBlobOpts.ChunkingPol = uint64(np)
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
	ref, ncs, err := w.btx.Write(true)
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
	return nil
}

// WriteBytes writes bytes to a blob and then to an index.
func (w *Writer) WriteBytes(index uint64, buf []byte) error {
	// XXX: possible optimization: read the range first and check if identical
	if err := w.moveRootBlobToRange(); err != nil {
		return err
	}

	nonce := w.root.GetRangeNonce()
	totalSize := len(buf)
	rlen := len(w.root.Ranges)
	w.root.RangeNonce += 1
	w.root.Ranges = append(w.root.Ranges, &Range{
		Nonce:  nonce,
		Start:  index,
		Length: uint64(totalSize),
		Ref:    nil, // will be filled by writer
	})

	_, rangeCs := w.rangeSet.Get(rlen)
	rangeCs.ClearRef(4)
	rcs := rangeCs.FollowRef(4, nil)
	bblob, err := blob.BuildBlob(
		w.ctx,
		int64(len(buf)),
		bytes.NewReader(buf),
		rcs,
		w.buildBlobOpts,
	)
	if err != nil {
		w.rangeSet.GetCursor().ClearRef(uint32(rlen))
		w.root.Ranges = w.root.Ranges[:len(w.root.Ranges)-1]
		w.root.RangeNonce -= 1
		return err
	}
	if rootPol := w.root.GetChunkingPol(); rootPol != 0 && bblob.GetChunkIndex().GetPol() == rootPol {
		bblob.ChunkIndex.Pol = 0
	}
	_ = bblob // rcs.SetBlock() has been called
	// rcs.MarkDirty() -- unnecesary due to SetBlock

	size := bblob.GetTotalSize()
	w.sortRanges()
	w.clearReadState()

	oldSize := w.root.GetTotalSize()
	nextSize := index + size
	if nextSize > oldSize {
		w.root.TotalSize = nextSize
		w.bcs.SetBlock(w.root, true)
	}

	return nil
}

// Truncate shrinks or extends the file handle to the given size.
func (w *Writer) Truncate(size uint64) error {
	if size == w.root.GetTotalSize() {
		return nil
	}

	w.clearReadState()
	rangesBcs := w.bcs.FollowSubBlock(4)
	clearFile := func() {
		w.root.RootBlob = nil
		w.bcs.ClearRef(2)
		w.root.Ranges = nil
		rangesBcs.ClearAllRefs()
		w.bcs.ClearRef(4)
		w.root.RangeNonce = 0
		w.root.TotalSize = 0
	}
	if size == 0 {
		// special case: rapidly clear the file contents
		clearFile()
		return nil
	}

	// when reducing size from file:
	oldSize := w.root.GetTotalSize()
	if size < oldSize {
		// drop any ranges that start outside the new file
		removeFrom := -1
		for i := len(w.root.Ranges) - 1; i >= 0; i-- {
			irange := w.root.Ranges[i]
			if irange.GetStart() >= size {
				removeFrom = i
				rangesBcs.ClearRef(uint32(i))
			} else {
				break
			}
		}
		if removeFrom == 0 {
			// fast path: clear file and set new size
			clearFile()
		} else if removeFrom != -1 {
			w.root.Ranges = w.root.Ranges[:removeFrom]
		}
	} else {
		// when adding size to the file:
		// - lookup the last range in the file
		// - create a new range filled with zeros over the portion of the range that
		//   extends past the end of the new file length.
		zeroFrom, zeroTo := -1, -1
		if len(w.root.Ranges) == 0 {
			// ensure that the root blob is shorter than total size
			rootBlobSize := w.root.GetRootBlob().GetTotalSize()
			if rootBlobSize > oldSize {
				zeroFrom = int(oldSize)
				zeroTo = int(rootBlobSize)
				if err := w.moveRootBlobToRange(); err != nil {
					return err
				}
			}
		} else {
			lastRange := w.root.Ranges[len(w.root.Ranges)-1]
			lastRangeStart := lastRange.GetStart()
			lastRangeEnd := lastRangeStart + lastRange.GetLength()
			if lastRangeEnd > size {
				zeroFrom = int(oldSize)
				zeroTo = int(lastRangeEnd)
			}
		}

		if zeroFrom >= 0 && zeroTo > zeroFrom {
			// write a zeroed range
			err := w.WriteBlob(uint64(zeroFrom), uint64(zeroTo-zeroFrom), nil)
			if err != nil {
				return err
			}
		}
	}

	// set the filesize to the new size
	w.root.TotalSize = size
	return nil
}

// moveRootBlobToRange moves the root blob if it is set to a range.
func (w *Writer) moveRootBlobToRange() error {
	rblob := w.root.GetRootBlob()
	rblobSize := rblob.GetTotalSize()
	if len(w.root.Ranges) != 0 || rblobSize == 0 {
		return nil
	}

	nonce := w.root.GetRangeNonce()
	w.root.RangeNonce += 1
	w.root.Ranges = append(w.root.Ranges, &Range{
		Nonce:  nonce,
		Start:  0,
		Length: rblobSize,
		Ref:    nil, // will be filled by writer
	})
	_, rcs := w.rangeSet.Get(len(w.root.Ranges) - 1)
	rcs.ClearRef(4)
	bcs := rcs.FollowRef(4, nil)
	bcs.SetBlock(rblob, true)
	w.root.RootBlob = nil
	w.bcs.ClearRef(2)
	w.sortRanges()
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
