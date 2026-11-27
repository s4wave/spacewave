package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
)

// NewObject constructs a new Object block from a key and root ref.
//
// A nil root ref is normalized to an empty ObjectRef so that an object created
// with no explicit root carries an empty root sub-block, identical to one
// created with an explicit empty ref. Readers that strictly decode the object
// root (the SQL store opener) then see an empty root rather than a missing
// sub-block that decodes as the wrong block type.
func NewObject(key string, rootRef *bucket.ObjectRef) *Object {
	if rootRef == nil {
		rootRef = &bucket.ObjectRef{}
	}
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

// DecodedBlockCacheTypeKey returns the decoded-block cache type key.
func (o *Object) DecodedBlockCacheTypeKey() string {
	return "db/world/block.Object"
}

// UnmarshalObject unmarshals a Object block from a cursor.
// If empty, returns nil, nil.
func UnmarshalObject(ctx context.Context, bcs *block.Cursor) (*Object, error) {
	return block.UnmarshalBlock[Object](ctx, bcs)
}

// Clone clones the Object.
func (o *Object) Clone() *Object {
	return o.CloneVT()
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
	return m
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
	_ block.Block                 = (*Object)(nil)
	_ block.DecodedBlockCacheable = (*Object)(nil)
	_ block.BlockWithSubBlocks    = (*Object)(nil)
)
