package blob

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/byteslice"
	"github.com/golang/protobuf/proto"
)

// NewChunk constructs a new chunk.
func NewChunk(dataRef *block.BlockRef, size, start uint64) *Chunk {
	return &Chunk{DataRef: dataRef, Size: size, Start: start}
}

// NewChunkBlock builds a new repo ref block.
func NewChunkBlock() block.Block {
	return &Chunk{}
}

// Validate checks the reference.
func (r *Chunk) Validate() error {
	if r.GetDataRef().GetEmpty() || r.GetSize() == 0 {
		return ErrEmptyChunk
	}
	if err := r.GetDataRef().Validate(); err != nil {
		return err
	}

	return nil
}

// FollowDataRef follows the data reference.
func (r *Chunk) FollowDataRef(bcs *block.Cursor) *block.Cursor {
	return bcs.FollowRef(1, r.GetDataRef())
}

// MarshalBlock marshals the block to binary.
func (r *Chunk) MarshalBlock() ([]byte, error) {
	return proto.Marshal(r)
}

// UnmarshalBlock unmarshals the block to the object.
func (r *Chunk) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, r)
}

// ApplyBlockRef applies a ref change with a field id.
// The reference may be nil if the child block is nil.
func (r *Chunk) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	switch id {
	case 1:
		r.DataRef = ptr
	}
	return nil
}

// GetBlockRefs returns all block references by ID.
// May return nil, and values may also be nil.
// Note: this does not include pending references (in a cursor)
func (r *Chunk) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	if r == nil {
		return nil, nil
	}
	m := make(map[uint32]*block.BlockRef)
	m[1] = r.GetDataRef()
	return m, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID or unknown.
func (r *Chunk) GetBlockRefCtor(id uint32) block.Ctor {
	switch id {
	case 1:
		return byteslice.NewByteSliceBlock
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block         = ((*Chunk)(nil))
	_ block.BlockWithRefs = ((*Chunk)(nil))
)
