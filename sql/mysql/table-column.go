package mysql

import (
	"context"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/blob"
	"github.com/aperturerobotics/hydra/block/msgpack"
	"github.com/golang/protobuf/proto"
)

// TableColumnMaxSize is the maximum size of the data field, if larger than
// this size, data will be stored in a block referenced by "data_ref".
const TableColumnMaxSize = 2e4 // 20kb

// BuildTableColumn constructs a TableColumn by marshaling a col with msgpack.
//
// bcs must be set.
func BuildTableColumn(
	ctx context.Context,
	bcs *block.Cursor,
	opts *blob.BuildBlobOpts,
	col interface{},
) (*TableColumn, error) {
	ntc := &TableColumn{}
	bcs.ClearAllRefs()
	bcs.SetBlock(ntc)

	// create the data container
	cbcs := bcs.FollowSubBlock(1)
	mblob, err := msgpack.BuildMsgpackBlob(ctx, cbcs, opts, col)
	if err != nil {
		return nil, err
	}
	cbcs.SetBlock(mblob)
	ntc.MsgpackBlob = mblob

	return ntc, nil
}

// FetchSqlColumn converts the row back into a sql column.
func (t *TableColumn) FetchSqlColumn(ctx context.Context, bcs *block.Cursor) (interface{}, error) {
	// msgpack contains type information
	var out interface{}

	// follow the data container
	cbcs := bcs.FollowSubBlock(1)
	err := t.GetMsgpackBlob().UnmarshalMsgpack(ctx, cbcs, &out)
	if err != nil {
		return nil, err
	}

	return out, nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (t *TableColumn) MarshalBlock() ([]byte, error) {
	return proto.Marshal(t)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (t *TableColumn) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, t)
}

// ApplySubBlock applies a sub-block change with a field id.
func (t *TableColumn) ApplySubBlock(id uint32, next block.SubBlock) error {
	switch id {
	case 1:
		v, ok := next.(*msgpack.MsgpackBlob)
		if !ok {
			return block.ErrUnexpectedType
		}
		t.MsgpackBlob = v
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (t *TableColumn) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	m[1] = t.GetMsgpackBlob()
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (t *TableColumn) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 1:
		return func(create bool) block.SubBlock {
			v := t.GetMsgpackBlob()
			if create && v == nil {
				v = &msgpack.MsgpackBlob{}
			}
			return v
		}
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*TableColumn)(nil))
	_ block.BlockWithSubBlocks = ((*TableColumn)(nil))
	_ block.SubBlock           = ((*TableColumn)(nil))
)
