package git_block

import (
	"context"

	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/blob"
	"google.golang.org/protobuf/proto"
)

// NewEncodedObjectBlock builds a new encoded object block.
func NewEncodedObjectBlock() block.Block {
	return &EncodedObject{}
}

// Validate performs cursory validation of the EncodedObject.
func (r *EncodedObject) Validate() error {
	if r.GetDataHash().GetHashType() != hash.HashType_HashType_SHA1 ||
		len(r.GetDataHash().GetHash()) != 20 {
		return ErrHashTypeInvalid
	}
	if err := r.GetDataBlob().Validate(); err != nil {
		return err
	}
	return nil
}

// ValidateFull performs full validation of the EncodedObject.
// This fetches all the data.
// Note: this does not check the hash of the data, just block graph validity.
func (r *EncodedObject) ValidateFull(ctx context.Context, bcs *block.Cursor) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if err := r.GetDataBlob().ValidateFull(ctx, bcs); err != nil {
		return err
	}
	return nil
}

// FollowDataBlob attempts to follow the blob field.
func (r *EncodedObject) FollowDataBlob(bcs *block.Cursor) (*blob.Blob, *block.Cursor, error) {
	dataBlobBcs := bcs.FollowSubBlock(1)
	blobi, err := dataBlobBcs.Unmarshal(blob.NewBlobBlock)
	if err != nil {
		return nil, nil, err
	}
	bl, blOk := blobi.(*blob.Blob)
	if !blOk {
		return nil, nil, block.ErrUnexpectedType
	}
	return bl, dataBlobBcs, nil
}

// BuildDataBlobReader builds the data blob reader.
func (r *EncodedObject) BuildDataBlobReader(ctx context.Context, bcs *block.Cursor) (*blob.Reader, error) {
	return blob.NewReader(ctx, bcs.FollowSubBlock(1))
}

// MarshalBlock marshals the block to binary.
func (r *EncodedObject) MarshalBlock() ([]byte, error) {
	return proto.Marshal(r)
}

// UnmarshalBlock unmarshals the block to the object.
func (r *EncodedObject) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, r)
}

// ApplySubBlock applies a sub-block change with a field id.
func (r *EncodedObject) ApplySubBlock(id uint32, next block.SubBlock) error {
	switch id {
	case 1:
		v, ok := next.(*blob.Blob)
		if !ok {
			return block.ErrUnexpectedType
		}
		r.DataBlob = v
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (r *EncodedObject) GetSubBlocks() map[uint32]block.SubBlock {
	v := make(map[uint32]block.SubBlock)
	v[1] = r.GetDataBlob()
	return v
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (r *EncodedObject) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 1:
		return func(create bool) block.SubBlock {
			v := r.GetDataBlob()
			if create && v == nil {
				r.DataBlob = &blob.Blob{}
				v = r.DataBlob
			}
			return v
		}
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*EncodedObject)(nil))
	_ block.BlockWithSubBlocks = ((*EncodedObject)(nil))
)
