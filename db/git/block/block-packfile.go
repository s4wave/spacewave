package git_block

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/pkg/errors"
)

// NewPackfileBlock builds a new packfile block.
func NewPackfileBlock() block.Block {
	return &Packfile{}
}

// Validate performs cursory validation of the Packfile.
func (r *Packfile) Validate() error {
	if err := ValidateHash(r.GetPackHash()); err != nil {
		return errors.Wrap(err, "pack_hash")
	}
	if err := r.GetPackBlob().Validate(); err != nil {
		return errors.Wrap(err, "pack_blob")
	}
	if err := r.GetIdxBlob().Validate(); err != nil {
		return errors.Wrap(err, "idx_blob")
	}
	if r.GetObjectCount() == 0 {
		return errors.New("object_count must be greater than zero")
	}
	if r.GetPackSize() == 0 {
		return errors.New("pack_size must be greater than zero")
	}
	if r.GetIdxSize() == 0 {
		return errors.New("idx_size must be greater than zero")
	}
	return nil
}

// FollowPackBlob returns the raw Git packfile blob.
func (r *Packfile) FollowPackBlob(ctx context.Context, bcs *block.Cursor) (*blob.Blob, *block.Cursor, error) {
	cs := bcs.FollowSubBlock(1)
	v, err := cs.Unmarshal(ctx, blob.NewBlobBlock)
	if err != nil {
		return nil, nil, err
	}
	nv, ok := v.(*blob.Blob)
	if !ok {
		return nil, nil, block.ErrUnexpectedType
	}
	return nv, cs, nil
}

// FollowIdxBlob returns the Git pack index blob.
func (r *Packfile) FollowIdxBlob(ctx context.Context, bcs *block.Cursor) (*blob.Blob, *block.Cursor, error) {
	cs := bcs.FollowSubBlock(2)
	v, err := cs.Unmarshal(ctx, blob.NewBlobBlock)
	if err != nil {
		return nil, nil, err
	}
	nv, ok := v.(*blob.Blob)
	if !ok {
		return nil, nil, block.ErrUnexpectedType
	}
	return nv, cs, nil
}

// MarshalBlock marshals the block to binary.
func (r *Packfile) MarshalBlock() ([]byte, error) {
	return r.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (r *Packfile) UnmarshalBlock(data []byte) error {
	return r.UnmarshalVT(data)
}

// ApplySubBlock applies a sub-block change with a field id.
func (r *Packfile) ApplySubBlock(id uint32, next block.SubBlock) error {
	switch id {
	case 1:
		v, ok := next.(*blob.Blob)
		if !ok {
			return block.ErrUnexpectedType
		}
		r.PackBlob = v
	case 2:
		v, ok := next.(*blob.Blob)
		if !ok {
			return block.ErrUnexpectedType
		}
		r.IdxBlob = v
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (r *Packfile) GetSubBlocks() map[uint32]block.SubBlock {
	if r == nil {
		return nil
	}

	v := make(map[uint32]block.SubBlock)
	v[1] = r.GetPackBlob()
	v[2] = r.GetIdxBlob()
	return v
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (r *Packfile) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	if r == nil {
		return nil
	}
	switch id {
	case 1:
		return blob.NewBlobSubBlockCtor(&r.PackBlob)
	case 2:
		return blob.NewBlobSubBlockCtor(&r.IdxBlob)
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*Packfile)(nil))
	_ block.BlockWithSubBlocks = ((*Packfile)(nil))
)
