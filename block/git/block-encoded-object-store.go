package git

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/iavl"
	"github.com/golang/protobuf/proto"
	"github.com/restic/chunker"
)

// NewEncodedObjectStoreBlock builds a new object store block.
func NewEncodedObjectStoreBlock() block.Block {
	return &EncodedObjectStore{}
}

// MarshalBlock marshals the block to binary.
func (r *EncodedObjectStore) MarshalBlock() ([]byte, error) {
	return proto.Marshal(r)
}

// UnmarshalBlock unmarshals the block to the object.
func (r *EncodedObjectStore) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, r)
}

// BuildObjectTree builds the iavl tree.
//
// Bcs should be located at EncodedObjectStore.
func (r *EncodedObjectStore) BuildObjectTree(bcs *block.Cursor) (*iavl.Tx, error) {
	return buildIavlSubBlockTree(1, bcs, r)
}

// ApplySubBlock applies a sub-block change with a field id.
func (r *EncodedObjectStore) ApplySubBlock(id uint32, next block.SubBlock) error {
	switch id {
	case 1:
		v, ok := next.(*iavl.Node)
		if !ok {
			return block.ErrUnexpectedType
		}
		r.IavlRoot = v
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (r *EncodedObjectStore) GetSubBlocks() map[uint32]block.SubBlock {
	if r == nil {
		return nil
	}

	v := make(map[uint32]block.SubBlock)
	v[1] = r.GetIavlRoot
	return v
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (r *EncodedObjectStore) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	if r == nil {
		return nil
	}
	switch id {
	case 1:
		return iavl.NewAVLTreeSubBlockCtor(&r.IavlRoot)
	}
	return nil
}

// getOrGenerateChunkerPoly gets or generates the chunking polynomial.
func (r *EncodedObjectStore) getOrGenerateChunkerPoly() (uint64, error) {
	if v := r.GetChunkingPol(); v != 0 {
		return v, nil
	}

	p, err := chunker.RandomPolynomial()
	if err != nil {
		return 0, err
	}
	pl := uint64(p)
	r.ChunkingPol = pl
	return pl, err
}

// _ is a type assertion
var (
	_ block.Block              = (*EncodedObjectStore)(nil)
	_ block.BlockWithSubBlocks = (*EncodedObjectStore)(nil)
)
