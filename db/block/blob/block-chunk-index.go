package blob

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/sbset"
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
func UnmarshalChunkIndex(ctx context.Context, bcs *block.Cursor) (*ChunkIndex, error) {
	return block.UnmarshalBlock[*ChunkIndex](ctx, bcs, NewChunkIndexBlock)
}

// IsNil returns if the object is nil.
func (r *ChunkIndex) IsNil() bool {
	return r == nil
}

// Validate checks the reference.
func (r *ChunkIndex) Validate() error {
	// Require at least one chunk.
	if len(r.GetChunks()) == 0 {
		return ErrEmptyChunk
	}

	// Check the chunks are valid and contiguous, and find the tail start.
	tailStart := r.GetTailStart()
	var totalSize uint64
	var tailFound bool
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
		tailFound = tailFound || totalSize == tailStart
		totalSize += chunkSize
	}

	// Require the tail to start at a chunk boundary.
	if !tailFound && tailStart != totalSize {
		return errors.Errorf("tail start %d is not a chunk boundary", tailStart)
	}
	return nil
}

// GetEnd returns the end of the last chunk.
func (r *ChunkIndex) GetEnd() uint64 {
	chunks := r.GetChunks()
	if len(chunks) == 0 {
		return 0
	}
	last := chunks[len(chunks)-1]
	return last.GetStart() + last.GetSize()
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
	_ block.Block              = (*ChunkIndex)(nil)
	_ block.BlockWithSubBlocks = (*ChunkIndex)(nil)
)
