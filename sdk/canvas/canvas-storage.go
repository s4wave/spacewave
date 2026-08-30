package s4wave_canvas

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_kvtx "github.com/s4wave/spacewave/db/kvtx/block"
)

// NewCanvasStorageBlock constructs a Canvas storage root block.
func NewCanvasStorageBlock() block.Block {
	return &CanvasStorage{}
}

// NewCanvasStorage constructs an empty Canvas storage DAG.
func NewCanvasStorage() *CanvasStorage {
	return &CanvasStorage{
		Nodes: block_kvtx.NewKeyValueStoreForWorkload(block_kvtx.WorkloadClassWriteChurn),
	}
}

// DecodedBlockCacheTypeKey returns the decoded-block cache type key.
func (s *CanvasStorage) DecodedBlockCacheTypeKey() string {
	return "sdk/canvas.CanvasStorage"
}

// Validate checks the Canvas node index.
func (s *CanvasStorage) Validate() error {
	if s.GetNodes() == nil {
		return errors.New("Canvas storage node index is missing")
	}
	return s.GetNodes().Validate()
}

// MarshalBlock marshals the storage root to binary.
func (s *CanvasStorage) MarshalBlock() ([]byte, error) {
	return s.MarshalVT()
}

// UnmarshalBlock unmarshals the storage root from binary.
func (s *CanvasStorage) UnmarshalBlock(data []byte) error {
	return s.UnmarshalVT(data)
}

// ApplySubBlock applies a Canvas storage sub-block change.
func (s *CanvasStorage) ApplySubBlock(id uint32, next block.SubBlock) error {
	if id != 1 {
		return nil
	}
	nodes, ok := next.(*block_kvtx.KeyValueStore)
	if !ok {
		return block.ErrUnexpectedType
	}
	s.Nodes = nodes
	return nil
}

// GetSubBlocks returns the Canvas storage sub-blocks by field ID.
func (s *CanvasStorage) GetSubBlocks() map[uint32]block.SubBlock {
	return map[uint32]block.SubBlock{1: s.GetNodes()}
}

// GetSubBlockCtor returns the constructor for a Canvas storage sub-block.
func (s *CanvasStorage) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	if id != 1 {
		return nil
	}
	return func(create bool) block.SubBlock {
		if s.Nodes == nil && create {
			s.Nodes = block_kvtx.NewKeyValueStoreForWorkload(block_kvtx.WorkloadClassWriteChurn)
		}
		if s.Nodes == nil {
			return nil
		}
		return s.Nodes
	}
}

// UnmarshalCanvasStorage unmarshals a Canvas storage root from a cursor.
func UnmarshalCanvasStorage(ctx context.Context, bcs *block.Cursor) (*CanvasStorage, error) {
	return block.UnmarshalBlock[*CanvasStorage](ctx, bcs, NewCanvasStorageBlock)
}

var (
	_ block.Block                 = (*CanvasStorage)(nil)
	_ block.BlockWithSubBlocks    = (*CanvasStorage)(nil)
	_ block.DecodedBlockCacheable = (*CanvasStorage)(nil)
)
