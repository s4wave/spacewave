package world_block

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
)

// NewNestedWorld stores roots from the enclosing World's bucket as local edges.
// bucketID identifies the enclosing bucket. An explicit bucket on either
// reference must match it; transform settings remain attached to each reference.
func NewNestedWorld(bucketID string, worldRef, payloadRef *bucket.ObjectRef) (*NestedWorld, error) {
	if (worldRef.GetBucketId() != "" && worldRef.GetBucketId() != bucketID) ||
		(payloadRef.GetBucketId() != "" && payloadRef.GetBucketId() != bucketID) {
		return nil, errors.New("nested World root belongs to another bucket")
	}
	local := func(ref *bucket.ObjectRef) *bucket.ObjectRef {
		if ref == nil {
			return nil
		}
		ref = ref.Clone()
		ref.BucketId = ""
		return ref
	}
	return &NestedWorld{WorldRef: local(worldRef), PayloadRef: local(payloadRef)}, nil
}

// NewNestedWorldBlock constructs a nested World block.
func NewNestedWorldBlock() block.Block {
	return &NestedWorld{}
}

// DecodedBlockCacheTypeKey returns the decoded-block cache type key.
func (n *NestedWorld) DecodedBlockCacheTypeKey() string {
	return "db/world/block.NestedWorld"
}

// UnmarshalNestedWorld unmarshals a nested World block from a cursor.
func UnmarshalNestedWorld(ctx context.Context, cursor *block.Cursor) (*NestedWorld, error) {
	return block.UnmarshalBlock[*NestedWorld](ctx, cursor, NewNestedWorldBlock)
}

// MarshalBlock marshals the block to binary.
func (n *NestedWorld) MarshalBlock() ([]byte, error) {
	return n.MarshalVT()
}

// UnmarshalBlock unmarshals the block from binary.
func (n *NestedWorld) UnmarshalBlock(data []byte) error {
	return n.UnmarshalVT(data)
}

// ApplySubBlock replaces the nested World or app-specific payload reference.
func (n *NestedWorld) ApplySubBlock(id uint32, next block.SubBlock) error {
	ref, ok := next.(*bucket.ObjectRef)
	if !ok {
		return block.ErrUnexpectedType
	}
	switch id {
	case 1:
		n.WorldRef = ref
	case 2:
		n.PayloadRef = ref
	}
	return nil
}

// GetSubBlocks returns both retained references for block graph traversal.
func (n *NestedWorld) GetSubBlocks() map[uint32]block.SubBlock {
	return map[uint32]block.SubBlock{1: n.GetWorldRef(), 2: n.GetPayloadRef()}
}

// GetSubBlockCtor returns the constructor for either retained reference.
func (n *NestedWorld) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 1:
		return bucket.NewObjectRefSubBlockCtor(&n.WorldRef)
	case 2:
		return bucket.NewObjectRefSubBlockCtor(&n.PayloadRef)
	default:
		return nil
	}
}

// _ is a type assertion
var (
	_ block.Block                 = (*NestedWorld)(nil)
	_ block.DecodedBlockCacheable = (*NestedWorld)(nil)
	_ block.BlockWithSubBlocks    = (*NestedWorld)(nil)
)
