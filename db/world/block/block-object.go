package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
)

// NewObject constructs a new Object block from a key and root ref.
func NewObject(key string, rootRef *bucket.ObjectRef) *Object {
	return &Object{
		Key:     key,
		RootRef: rootRef,
		Rev:     1,
	}
}

// NewObjectBlock constructs a new object block.
func NewObjectBlock() block.Block {
	return &Object{}
}

// UnmarshalObject unmarshals a Object block from a cursor.
// If empty, returns nil, nil
func UnmarshalObject(ctx context.Context, bcs *block.Cursor) (*Object, error) {
	return block.UnmarshalBlock[*Object](ctx, bcs, NewObjectBlock)
}

// Clone clones the Object.
func (o *Object) Clone() *Object {
	return o.CloneVT()
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (o *Object) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (o *Object) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// ApplySubBlock applies a sub-block change with a field id.
func (o *Object) ApplySubBlock(id uint32, next block.SubBlock) error {
	switch id {
	case 2:
		v, ok := next.(*bucket.ObjectRef)
		if !ok {
			return block.ErrUnexpectedType
		}
		o.RootRef = v
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (o *Object) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	m[2] = o.GetRootRef()
	return nil
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (o *Object) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 2:
		return bucket.NewObjectRefSubBlockCtor(&o.RootRef)
	default:
		return nil
	}
}

// _ is a type assertion
var (
	_ block.Block              = ((*Object)(nil))
	_ block.BlockWithSubBlocks = ((*Object)(nil))
)
