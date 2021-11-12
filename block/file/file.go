package file

import (
	"bytes"
	"context"
	"io"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/blob"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

// NewFileBlock builds a new file root block.
func NewFileBlock() block.Block {
	return &File{}
}

// UnmarshalFile unmarshals the File block.
// Returns nil, nil if empty
func UnmarshalFile(bcs *block.Cursor) (*File, error) {
	exi, err := bcs.Unmarshal(NewFileBlock)
	if err != nil {
		return nil, err
	}
	if exi == nil {
		return nil, nil
	}
	ex, ok := exi.(*File)
	if !ok {
		return nil, block.ErrUnexpectedType
	}
	return ex, nil
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
		bcs.SetBlock(rootBlob, true)
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

// Validate performs cursory validation of the file object.
func (f *File) Validate() error {
	if err := f.GetRootBlob().Validate(); err != nil {
		return errors.Wrap(err, "root_blob")
	}
	if len(f.GetRanges()) != 0 {
		if f.GetRootBlob().GetTotalSize() != 0 {
			return errors.New("expected root_blob to be empty if ranges are set")
		}
	}
	for i, r := range f.GetRanges() {
		if r.GetLength() == 0 {
			return errors.Errorf("range with zero length is invalid: %d", i)
		}
		if !r.GetRef().GetEmpty() {
			if err := r.GetRef().Validate(); err != nil {
				return errors.Wrapf(err, "ranges[%d]", i)
			}
		}
	}
	return nil
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

// ApplySubBlock applies a sub-block change with a field id.
func (f *File) ApplySubBlock(id uint32, next block.SubBlock) error {
	var ok bool
	switch id {
	case 2:
		f.RootBlob, ok = next.(*blob.Blob)
		if !ok {
			return block.ErrUnexpectedType
		}
	case 4:
		// no-op for ranges set.
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (f *File) GetSubBlocks() map[uint32]block.SubBlock {
	if f == nil {
		return nil
	}
	m := make(map[uint32]block.SubBlock)
	m[2] = f.GetRootBlob()
	m[4] = NewRangeSet(&f.Ranges, nil)
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (f *File) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 2:
		return func(create bool) block.SubBlock {
			v := f.RootBlob
			if create && v == nil {
				v = &blob.Blob{}
				f.RootBlob = v
			}
			return v
		}
	case 4:
		return func(create bool) block.SubBlock {
			return NewRangeSet(&f.Ranges, nil)
		}
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*File)(nil))
	_ block.BlockWithSubBlocks = ((*File)(nil))
)
