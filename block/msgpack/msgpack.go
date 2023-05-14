package msgpack

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/blob"
	"github.com/vmihailenco/msgpack/v5"
)

// NewMsgpackBlobBlock constructs a new db root block.
func NewMsgpackBlobBlock() block.Block {
	return &MsgpackBlob{}
}

// BuildMsgpackBlob packs an object into a blob, storing the object into the cursor.
//
// opts can be nil
func BuildMsgpackBlob(
	ctx context.Context,
	bcs *block.Cursor,
	opts *blob.BuildBlobOpts,
	obj interface{},
) (*MsgpackBlob, error) {
	nobj := &MsgpackBlob{}
	bcs.ClearAllRefs()
	bcs.SetBlock(nobj, true)
	dat, err := msgpack.Marshal(obj)
	if err != nil {
		return nil, err
	}
	nobj.Blob, err = blob.BuildBlob(
		ctx,
		int64(len(dat)),
		bytes.NewReader(dat),
		bcs.FollowSubBlock(1),
		opts,
	)
	if err != nil {
		return nil, err
	}
	return nobj, nil
}

// UnmarshalMsgpackBlob loads a msgpack blob at a cursor.
// may return nil
func UnmarshalMsgpackBlob(ctx context.Context, bcs *block.Cursor) (*MsgpackBlob, error) {
	return block.UnmarshalBlock[*MsgpackBlob](ctx, bcs, NewMsgpackBlobBlock)
}

// IsNil returns if the object is nil.
func (m *MsgpackBlob) IsNil() bool {
	return m == nil
}

// Validate performs cursory validation of the msgpack blob.
func (m *MsgpackBlob) Validate() error {
	if err := m.GetBlob().Validate(); err != nil {
		return err
	}
	return nil
}

// IsEmpty checks if the blob is empty.
func (m *MsgpackBlob) IsEmpty() bool {
	return m.GetBlob().IsEmpty()
}

// Decode: blob.UnmarshalObject() -> object interface{}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (m *MsgpackBlob) MarshalBlock() ([]byte, error) {
	return m.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (m *MsgpackBlob) UnmarshalBlock(data []byte) error {
	return m.UnmarshalVT(data)
}

// BuildMsgpackDecoder builds a streaming decoder for the blob.
//
// bcs must be located at the MsgpackBlob object.
func (m *MsgpackBlob) BuildMsgpackDecoder(ctx context.Context, bcs *block.Cursor) (*msgpack.Decoder, error) {
	if m.GetBlob().GetTotalSize() == 0 {
		return msgpack.NewDecoder(bytes.NewReader(nil)), nil
	}
	// streaming msgpack decoding from the block graph.
	br, err := blob.NewReader(ctx, bcs.FollowSubBlock(1))
	if err != nil {
		return nil, err
	}
	dec := msgpack.NewDecoder(br)
	return dec, nil
}

// UnmarshalMsgpack unmarshals the msgpack data to an object.
//
// bcs must be located at the MsgpackBlob object.
func (m *MsgpackBlob) UnmarshalMsgpack(ctx context.Context, bcs *block.Cursor, obj interface{}) error {
	dec, err := m.BuildMsgpackDecoder(ctx, bcs)
	if err != nil {
		return err
	}
	return dec.Decode(obj)
}

// ApplySubBlock applies a sub-block change with a field id.
func (m *MsgpackBlob) ApplySubBlock(id uint32, next block.SubBlock) error {
	switch id {
	case 1:
		blk, ok := next.(*blob.Blob)
		if !ok {
			return block.ErrUnexpectedType
		}
		m.Blob = blk
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (m *MsgpackBlob) GetSubBlocks() map[uint32]block.SubBlock {
	mm := make(map[uint32]block.SubBlock)
	mm[1] = m.GetBlob()
	return mm
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (m *MsgpackBlob) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 1:
		return func(create bool) block.SubBlock {
			b := m.GetBlob()
			if create && b == nil {
				b = &blob.Blob{}
			}
			return b
		}
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*MsgpackBlob)(nil))
	_ block.BlockWithSubBlocks = ((*MsgpackBlob)(nil))
)
