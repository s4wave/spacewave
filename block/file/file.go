package file

import (
	"bytes"
	"context"
	"io"
	"sort"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/blob"
	"github.com/aperturerobotics/hydra/cid"
	"github.com/golang/protobuf/proto"
)

// NewFileBlock builds a new file root block.
func NewFileBlock() block.Block {
	return &File{}
}

// NewFileRangeRefId returns the reference index for a range index in a File.
func NewFileRangeRefId(idx int) uint32 {
	return uint32(4 + idx)
}

// IdxFromFileRangeRefId returns the Range index from the reference id.
func IdxFromFileRangeRefId(refID uint32) int {
	return int(refID) - 4
}

// FetchToBuffer fetches a full File to a buffer.
func FetchToBuffer(ctx context.Context, bcs *block.Cursor, buf *bytes.Buffer) error {
	rootBlock, err := bcs.Unmarshal(NewFileBlock)
	if err != nil || rootBlock == nil {
		return err
	}
	root := rootBlock.(*File)
	if root.GetTotalSize() == 0 {
		// empty file
		return nil
	}

	rootRanges := root.GetRanges()
	if len(rootRanges) == 0 {
		rootBlob := root.GetRootBlob()
		bcs.SetBlock(rootBlob)
		return blob.FetchToBuffer(ctx, bcs, buf)
	}

	rseeker := NewHandle(ctx, bcs, root)
	defer rseeker.Close()

	_, err = io.Copy(buf, rseeker)
	return err
}

// FetchToBytes fetches to a bytes slice.
func FetchToBytes(ctx context.Context, bcs *block.Cursor) ([]byte, error) {
	var buf bytes.Buffer
	if err := FetchToBuffer(ctx, bcs, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (f *File) MarshalBlock() ([]byte, error) {
	return proto.Marshal(f)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (f *File) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, f)
}

// ApplyBlockRef applies a ref change with a field id.
// The reference may be nil if the child block is nil.
func (f *File) ApplyBlockRef(id uint32, ptr *cid.BlockRef) error {
	// ref id is based on field number
	if id >= 4 {
		idx := IdxFromFileRangeRefId(id)
		ranges := f.GetRanges()
		if len(ranges) > idx {
			rn := ranges[idx]
			if rn.GetLength() != 0 {
				rn.Ref = ptr
			}
		}
	}

	return nil
}

// GetBlockRefs returns all block references by ID.
// May return nil, and values may also be nil.
// Note: this does not include pending references (in a cursor)
func (f *File) GetBlockRefs() (map[uint32]*cid.BlockRef, error) {
	refs := make(map[uint32]*cid.BlockRef)
	for i, r := range f.GetRanges() {
		refs[NewFileRangeRefId(i)] = r.GetRef()
	}
	return refs, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID or unknown.
func (f *File) GetBlockRefCtor(id uint32) block.Ctor {
	if id >= 4 {
		return blob.NewBlobBlock
	}
	return nil
}

// SortRanges sorts the ranges slice.
// note: this is not used anywhere internally.
func (f *File) SortRanges() {
	sort.Sort(RangeSlice(f.Ranges))
}

// _ is a type assertion
var _ block.Block = ((*File)(nil))
