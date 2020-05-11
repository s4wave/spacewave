package file

import (
	"bytes"
	"io"
	"sort"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/blob"
	"github.com/aperturerobotics/hydra/bucket/event"
	"github.com/aperturerobotics/hydra/cid"
)

// Writer is a handle that can write to a handle.
type Writer struct {
	*Handle

	btx           *block.Transaction
	buildBlobOpts blob.BuildBlobOpts
}

// NewWriter builds a new writer handle.
// btx can be nil
func NewWriter(
	h *Handle,
	btx *block.Transaction,
	buildBlobOpts blob.BuildBlobOpts,
) *Writer {
	return &Writer{
		Handle:        h,
		btx:           btx,
		buildBlobOpts: buildBlobOpts,
	}
}

// CommitWriter commits any pending writes using a block transaction.
// Note: the block transaction must match the handle's block cursor.
func CommitWriter(w *Writer, btx *block.Transaction) ([]*bucket_event.Event, *block.Cursor, error) {
	w.clearReadState()
	eves, ncs, err := btx.Write()
	if err == nil {
		w.bcs = ncs
	}
	return eves, ncs, err
}

// Write writes to the handle, immediately flushing if btx is set.
func (w *Writer) Write(p []byte) (n int, err error) {
	idx := w.idx
	if err := w.WriteBytes(idx, p); err != nil {
		return 0, err
	}
	w.idx += uint64(len(p))
	if w.btx != nil {
		_, _, err = CommitWriter(w, w.btx)
		if err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

// WriteBlob writes a blob to an index in a new range.
// Implies removing any ranges which are completely occluded.
func (w *Writer) WriteBlob(index, size uint64, ref *cid.BlockRef) error {
	if err := w.moveRootBlobToRange(); err != nil {
		return err
	}
	nonce := w.root.GetRangeNonce()
	w.root.RangeNonce += 1
	rlen := len(w.root.Ranges)
	rrefID := NewFileRangeRefId(rlen)
	w.clearReadState()
	w.bcs.ClearRef(rrefID)
	rcs := w.bcs.FollowRef(rrefID, ref)
	w.bcs.SetRef(rrefID, rcs)
	w.root.Ranges = append(w.root.Ranges, &Range{
		Nonce:  nonce,
		Start:  index,
		Length: size,
		Ref:    ref,
	})
	w.sortRanges()

	oldSize := w.root.GetTotalSize()
	nextSize := index + size
	if nextSize > oldSize {
		w.root.TotalSize = nextSize
		w.bcs.SetBlock(w.root)
	}
	return nil
}

// WriteBytes writes bytes to a blob and then to an index.
func (w *Writer) WriteBytes(index uint64, buf []byte) error {
	if err := w.moveRootBlobToRange(); err != nil {
		return err
	}
	nonce := w.root.GetRangeNonce()
	w.root.RangeNonce += 1
	rlen := len(w.root.Ranges)
	rrefID := NewFileRangeRefId(rlen)
	w.bcs.ClearRef(rrefID)
	rcs := w.bcs.FollowRef(rrefID, nil)

	bblob, err := blob.BuildBlob(
		w.ctx,
		uint64(len(buf)),
		bytes.NewReader(buf),
		rcs,
		w.buildBlobOpts,
	)
	if err != nil {
		return err
	}
	_ = bblob // has already been set into rcs

	size := bblob.GetTotalSize()
	w.root.Ranges = append(w.root.Ranges, &Range{
		Nonce:  nonce,
		Start:  index,
		Length: size,
		Ref:    nil, // will be filled by writer
	})
	w.sortRanges()
	w.clearReadState()

	oldSize := w.root.GetTotalSize()
	nextSize := index + size
	if nextSize > oldSize {
		w.root.TotalSize = nextSize
		w.bcs.SetBlock(w.root)
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

	nonce := w.root.GetRangeNonce()
	w.root.RangeNonce += 1
	rrefID := NewFileRangeRefId(len(w.root.Ranges))
	w.root.Ranges = append(w.root.Ranges, &Range{
		Nonce:  nonce,
		Start:  0,
		Length: rblobSize,
		Ref:    nil, // will be filled by writer
	})
	rcs := w.bcs.FollowRef(rrefID, nil)
	w.root.RootBlob = nil
	rcs.SetBlock(rblob)
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
