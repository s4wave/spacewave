package blob

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/sbset"
	"github.com/pkg/errors"
)

// NewChunkIndex constructs a new chunk index.
func NewChunkIndex(chunks []*Chunk) *ChunkIndex {
	return &ChunkIndex{Chunks: chunks}
}

// NewChunkIndexBlock builds a new repo ref block.
func NewChunkIndexBlock() block.Block {
	return &ChunkIndex{}
}

// UnmarshalChunkIndex unmarshals a chunk index from a cursor.
// If empty, returns nil, nil
func UnmarshalChunkIndex(bcs *block.Cursor) (*ChunkIndex, error) {
	if bcs == nil {
		return nil, nil
	}
	blk, err := bcs.Unmarshal(NewChunkIndexBlock)
	if err != nil {
		return nil, err
	}
	if blk == nil {
		return nil, nil
	}
	bv, ok := blk.(*ChunkIndex)
	if !ok {
		return nil, block.ErrUnexpectedType
	}
	return bv, nil
}

// Validate checks the reference.
func (r *ChunkIndex) Validate() error {
	if len(r.GetChunks()) == 0 {
		return ErrEmptyChunk
	}
	var totalSize uint64
	for i, c := range r.GetChunks() {
		if err := c.Validate(); err != nil {
			return errors.Wrapf(err, "chunks[%d]", i)
		}
		chunkSize := c.GetSize()
		if st := c.GetStart(); st != totalSize {
			return errors.Wrapf(
				ErrOutOfSequenceChunk,
				"expected start %d but got %d",
				totalSize, st,
			)
		}
		totalSize += chunkSize
	}
	return nil
}

// MarshalBlock marshals the block to binary.
func (r *ChunkIndex) MarshalBlock() ([]byte, error) {
	return r.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (r *ChunkIndex) UnmarshalBlock(data []byte) error {
	return r.UnmarshalVT(data)
}

// ApplySubBlock applies a sub-block change with a field id.
func (r *ChunkIndex) ApplySubBlock(id uint32, next block.SubBlock) error {
	// no-op here
	return nil
}

// GetChunkSet returns the chunk set sub-block.
func (r *ChunkIndex) GetChunkSet(bcs *block.Cursor) *sbset.SubBlockSet {
	if r == nil {
		return NewChunkSet(nil, nil)
	}
	if bcs != nil {
		bcs = bcs.FollowSubBlock(1)
	}
	return NewChunkSet(&r.Chunks, bcs)
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (r *ChunkIndex) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	m[1] = r.GetChunkSet(nil)
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (r *ChunkIndex) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 1:
		return func(create bool) block.SubBlock {
			return r.GetChunkSet(nil)
		}
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*ChunkIndex)(nil))
	_ block.BlockWithSubBlocks = ((*ChunkIndex)(nil))
)
