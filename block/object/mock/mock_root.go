package object_mock

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/cid"
	"github.com/golang/protobuf/proto"
)

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (r *Root) MarshalBlock() ([]byte, error) {
	return proto.Marshal(r)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (r *Root) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, r)
}

// ApplyBlockRef applies a ref change with a field id.
func (r *Root) ApplyBlockRef(id uint32, ptr *cid.BlockRef) error {
	switch id {
	case 1:
		r.ExamplePtr.RootRef = ptr
	}
	return nil
}

// GetBlockRefs returns all block references by ID.
// May return nil, and values may also be nil.
// Note: this does not include pending references (in a cursor)
func (r *Root) GetBlockRefs() (map[uint32]*cid.BlockRef, error) {
	return map[uint32]*cid.BlockRef{
		1: r.GetExamplePtr().GetRootRef(),
	}, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID.
func (r *Root) GetBlockRefCtor(id uint32) block.Ctor {
	switch id {
	case 1:
		return func() block.Block { return &Root{} }
	}
	return nil
}

// _ is a type assertion
var _ block.Block = ((*Root)(nil))
