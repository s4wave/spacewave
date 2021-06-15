package world_block

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/golang/protobuf/proto"
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

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (o *Object) MarshalBlock() ([]byte, error) {
	return proto.Marshal(o)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (o *Object) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, o)
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
