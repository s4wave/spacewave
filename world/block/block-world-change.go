package world_block

import (
	"github.com/aperturerobotics/hydra/block"
)

// NewWorldChangeBlock is the world change block constructor.
func NewWorldChangeBlock() block.Block {
	return &WorldChange{}
}

// NewWorldChangeSubBlockCtor returns the sub-block constructor.
func NewWorldChangeSubBlockCtor(r **WorldChange) block.SubBlockCtor {
	if r == nil {
		return nil
	}
	return func(create bool) block.SubBlock {
		v := *r
		if create && v == nil {
			v = &WorldChange{}
			*r = v
		}
		return v
	}
}

// UnmarshalWorldChange unmarshals a world change from a cursor.
// If empty, returns nil, nil
func UnmarshalWorldChange(bcs *block.Cursor) (*WorldChange, error) {
	return block.UnmarshalBlock[*WorldChange](bcs, NewWorldChangeBlock)
}

// IsEmpty checks if the world change is empty.
func (w *WorldChange) IsEmpty() bool {
	return w.GetChangeType() == WorldChangeType_WorldChange_INVALID
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (w *WorldChange) MarshalBlock() ([]byte, error) {
	return w.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (w *WorldChange) UnmarshalBlock(data []byte) error {
	return w.UnmarshalVT(data)
}

// ApplyBlockRef applies a ref change with a field id.
// The reference may be nil if the child block is nil.
func (w *WorldChange) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	switch id {
	case 4:
		w.TransactionRef = ptr
	case 5:
		w.ObjectRef = ptr
	case 6:
		w.PrevObjectRef = ptr
	}
	return nil
}

// GetBlockRefs returns all block references by ID.
// May return nil, and values may also be nil.
// Note: this does not include pending references (in a cursor)
func (w *WorldChange) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	m := make(map[uint32]*block.BlockRef, 4)
	m[4] = w.GetTransactionRef()
	m[5] = w.GetObjectRef()
	m[6] = w.GetPrevObjectRef()
	return m, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID or unknown.
func (w *WorldChange) GetBlockRefCtor(id uint32) block.Ctor {
	switch id {
	case 4:
		// unknown: could be any block type
	case 5:
		return NewObjectBlock
	case 6:
		return NewObjectBlock
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block         = ((*WorldChange)(nil))
	_ block.BlockWithRefs = ((*WorldChange)(nil))
)
