package world_block

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/golang/protobuf/proto"
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

// IsEmpty checks if the world change is empty.
func (w *WorldChange) IsEmpty() bool {
	return w.GetSeqno() == 0 ||
		w.GetChangeType() == WorldChangeType_WorldChange_INVALID
}

// AppendChange appends a world change to the list, change becomes the latest.
// Updates seqno, prevrev.
// wBcs can be empty, but should point to w - the old WorldChange - (not as a sub-block)
// if wBcs points to a sub-block, it will be ignored
// changeBcs should point to the new block cursor containing change.
// Returns new block cursor pointing to w.
func (w *WorldChange) AppendChange(wBcs, changeBcs *block.Cursor, change *WorldChange) *block.Cursor {
	if !w.IsEmpty() {
		if wBcs != nil {
			blk, isSubBlock := wBcs.GetBlock()
			if isSubBlock || blk != w {
				// cannot blockref to a sub-block
				wBcs = nil
			}
		}
		if wBcs == nil {
			wBcs = changeBcs.Detach(false)
			wBcs.ClearAllRefs()
			wBcs.SetBlock(w, true)
		}
		change.PrevRef = wBcs.GetRef()
		change.Seqno = w.GetSeqno() + 1
		changeBcs.SetBlock(change, true)
		changeBcs.SetRef(2, wBcs) // update block graph
	} else {
		// first change
		change.PrevRef = nil
		change.Seqno = 1
		changeBcs.ClearRef(2)
	}
	return wBcs
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (w *WorldChange) MarshalBlock() ([]byte, error) {
	return proto.Marshal(w)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (w *WorldChange) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, w)
}

// ApplyBlockRef applies a ref change with a field id.
// The reference may be nil if the child block is nil.
func (w *WorldChange) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	switch id {
	case 2:
		w.PrevRef = ptr
	case 5:
		w.TransactionRef = ptr
	case 6:
		w.ObjectRef = ptr
	case 7:
		w.PrevObjectRef = ptr
	}
	return nil
}

// GetBlockRefs returns all block references by ID.
// May return nil, and values may also be nil.
// Note: this does not include pending references (in a cursor)
func (w *WorldChange) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	m := make(map[uint32]*block.BlockRef, 4)
	m[2] = w.GetPrevRef()
	m[5] = w.GetTransactionRef()
	m[6] = w.GetObjectRef()
	m[7] = w.GetPrevObjectRef()
	return m, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID or unknown.
func (w *WorldChange) GetBlockRefCtor(id uint32) block.Ctor {
	switch id {
	case 2:
		return NewWorldChangeBlock
	case 5:
		// unknown: could be any block type
	case 6:
		return NewObjectBlock
	case 7:
		return NewObjectBlock
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block         = ((*WorldChange)(nil))
	_ block.BlockWithRefs = ((*WorldChange)(nil))
)
