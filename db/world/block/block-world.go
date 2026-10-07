package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	block_kvtx "github.com/s4wave/spacewave/db/kvtx/block"
)

// NewWorld constructs a new empty world.
func NewWorld(disableChangelog bool) *World {
	return &World{LastChangeDisable: disableChangelog}
}

// NewWorldBlock constructs a new world state block.
func NewWorldBlock() block.Block {
	return &World{}
}

// DecodedBlockCacheTypeKey returns the decoded-block cache type key.
func (w *World) DecodedBlockCacheTypeKey() string {
	return "db/world/block.World"
}

// UnmarshalWorld unmarshals a world block from a cursor.
// Returns nil, nil if the cursor is empty.
func UnmarshalWorld(ctx context.Context, bcs *block.Cursor) (*World, error) {
	return block.UnmarshalBlock[*World](ctx, bcs, NewWorldBlock)
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (w *World) MarshalBlock() ([]byte, error) {
	return w.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (w *World) UnmarshalBlock(data []byte) error {
	return w.UnmarshalVT(data)
}

// ApplyBlockRef applies a ref change with a field id.
// The reference may be nil if the child block is nil.
func (w *World) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	if id == 7 {
		w.PrevChanges = ptr
	}
	return nil
}

// GetBlockRefs returns all block references by ID.
// Values may be nil. Pending references in a cursor are not included.
func (w *World) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	return map[uint32]*block.BlockRef{7: w.GetPrevChanges()}, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID or unknown.
func (w *World) GetBlockRefCtor(id uint32) block.Ctor {
	if id == 7 {
		return NewChangeLogLLBlock
	}
	return nil
}

// ApplySubBlock applies a sub-block change with a field id.
func (w *World) ApplySubBlock(id uint32, next block.SubBlock) error {
	switch id {
	case 1:
		v, ok := next.(*block_kvtx.KeyValueStore)
		if !ok {
			return block.ErrUnexpectedType
		}
		w.ObjectKeyValue = v
	case 2:
		v, ok := next.(*block_kvtx.KeyValueStore)
		if !ok {
			return block.ErrUnexpectedType
		}
		w.GraphKeyValue = v
	case 3:
		v, ok := next.(*ChangeLogLL)
		if !ok {
			return block.ErrUnexpectedType
		}
		w.LastChange = v
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (w *World) GetSubBlocks() map[uint32]block.SubBlock {
	// Return the object, graph, and last-change sub-blocks.
	m := make(map[uint32]block.SubBlock)
	m[1] = w.GetObjectKeyValue()
	m[2] = w.GetGraphKeyValue()
	m[3] = w.GetLastChange()
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (w *World) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 1:
		return block_kvtx.NewKeyValueStoreSubBlockCtor(&w.ObjectKeyValue)
	case 2:
		return func(create bool) block.SubBlock {
			if w.GraphKeyValue == nil && create {
				w.GraphKeyValue = block_kvtx.NewKeyValueStoreForWorkload(block_kvtx.WorkloadClassGraphPrefixRead)
			}
			if w.GraphKeyValue == nil {
				return nil
			}
			return w.GraphKeyValue
		}
	case 3:
		return NewChangeLogLLSubBlockCtor(&w.LastChange)
	default:
		return nil
	}
}

// _ is a type assertion
var (
	_ block.Block                 = (*World)(nil)
	_ block.DecodedBlockCacheable = (*World)(nil)
	_ block.BlockWithRefs         = (*World)(nil)
	_ block.BlockWithSubBlocks    = (*World)(nil)
)
