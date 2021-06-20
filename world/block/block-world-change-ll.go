package world_block

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

// NewWorldChangeLLBlock constructs a new WorldChangeLL block.
func NewWorldChangeLLBlock() block.Block {
	return &WorldChangeLL{}
}

// AppendWorldChangeLL appends a world change set to the list.
// nextBcs should point to the location to write the new WorldChangeLL.
// prevBcs should point to the previous *WorldChangeLL.
// prevBcs can be nil to indicate a brand-new linked list.
// if prevBcs is a sub-block, it will be detached before it is referenced
// prevBcs and nextBcs can be the same block cursor, if both are sub-block
// all world changes must have the same change type
// if prevBcs is set, it will be checked to ensure same WorldChange type
// returns the new world change ll node
func AppendWorldChangeLL(
	nextBcs *block.Cursor,
	prevBcs *block.Cursor,
	worldChangesBcs []*block.Cursor,
) (*WorldChangeLL, error) {
	// handling of previous entry
	var err error
	var prevWorldChangeLL *WorldChangeLL
	if prevBcs != nil {
		if prevBcs.IsSubBlock() {
			// cannot blockref to a sub-block
			// detach prevBcs, maintaining refs
			prevBcs = prevBcs.Detach(true)
		}
		// unmarshal previous block
		prevWorldChangeLL, err = UnmarshalWorldChangeLL(prevBcs)
		if err != nil {
			return nil, err
		}
	}

	// build next entry
	w := &WorldChangeLL{
		TotalSize: uint32(len(worldChangesBcs)),
		Changes:   make([]*WorldChange, len(worldChangesBcs)),
	}
	nextBcs.SetBlock(w, true)
	nextBcs.ClearAllRefs()
	changesBcs := nextBcs.FollowSubBlock(4)
	for i, chBcs := range worldChangesBcs {
		err = chBcs.SetAsSubBlock(uint32(i), changesBcs)
		if err != nil {
			return nil, err
		}
	}
	// perform some initial checks
	if err := w.Validate(); err != nil {
		return nil, err
	}
	if !prevWorldChangeLL.IsEmpty() {
		w.Height = prevWorldChangeLL.GetHeight() + 1
		w.TotalSize += prevWorldChangeLL.GetTotalSize()
		nextBcs.SetRef(2, prevBcs)
	}
	return w, nil
}

// UnmarshalWorldChangeLL unmarshals a world change ll from a cursor.
// If empty, returns nil, nil
func UnmarshalWorldChangeLL(bcs *block.Cursor) (*WorldChangeLL, error) {
	if bcs == nil {
		return nil, nil
	}
	blk, err := bcs.Unmarshal(NewWorldChangeLLBlock)
	if err != nil {
		return nil, err
	}
	if blk == nil {
		return nil, nil
	}
	bv, ok := blk.(*WorldChangeLL)
	if !ok {
		return nil, block.ErrUnexpectedType
	}
	return bv, nil
}

// Validate performs checks on the world change ll block.
func (w *WorldChangeLL) Validate() error {
	changes := w.GetChanges()
	if len(changes) == 0 {
		return world.ErrEmptyOp
	}
	if int(w.GetTotalSize()) < len(changes) {
		return errors.New("total size must be at least number of changes")
	}

	// ensure changes are all of the same type
	seenChangeType := changes[0].GetChangeType()
	for i, wch := range changes {
		if wch.IsEmpty() {
			return world.ErrEmptyChange
		}
		if cht := wch.GetChangeType(); cht != seenChangeType {
			return errors.Wrapf(
				world.ErrUnexpectedChangeType,
				"worldChanges[%d]: expected %v got %v",
				i,
				seenChangeType,
				wch.GetChangeType(),
			)
		}
	}

	return nil
}

// IsEmpty checks if the world change is empty.
func (w *WorldChangeLL) IsEmpty() bool {
	return w.GetTotalSize() == 0 || len(w.GetChanges()) == 0
}

// AppendWorldChange appends a world change to the batch entry.
// bcs should be located at WorldChangeLL
// returns cursor containing ch within the linked-list node
// returns nil if bcs was nil
func (w *WorldChangeLL) AppendWorldChange(ch *WorldChange, bcs *block.Cursor) *block.Cursor {
	if bcs != nil {
		bcs = bcs.FollowSubBlock(4)
	}
	subBlock := NewWorldChangeSet(&w.Changes, bcs)
	nidx := len(w.Changes)
	w.Changes = append(w.Changes, ch)
	_, nbcs := subBlock.Get(nidx)
	return nbcs
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (w *WorldChangeLL) MarshalBlock() ([]byte, error) {
	return proto.Marshal(w)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (w *WorldChangeLL) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, w)
}

// ApplyBlockRef applies a ref change with a field id.
// The reference may be nil if the child block is nil.
func (w *WorldChangeLL) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	switch id {
	case 2:
		w.PrevRef = ptr
	}
	return nil
}

// GetBlockRefs returns all block references by ID.
// May return nil, and values may also be nil.
// Note: this does not include pending references (in a cursor)
func (w *WorldChangeLL) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	m := make(map[uint32]*block.BlockRef, 4)
	m[2] = w.GetPrevRef()
	return m, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID or unknown.
func (w *WorldChangeLL) GetBlockRefCtor(id uint32) block.Ctor {
	switch id {
	case 2:
		return NewWorldChangeLLBlock
	}
	return nil
}

// ApplySubBlock applies a sub-block change with a field id.
func (w *WorldChangeLL) ApplySubBlock(id uint32, next block.SubBlock) error {
	// field 4: no-op (is a sub-block set)
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (w *WorldChangeLL) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	m[4] = NewWorldChangeSet(&w.Changes, nil)
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (w *WorldChangeLL) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 4:
		return func(create bool) block.SubBlock {
			return NewWorldChangeSet(&w.Changes, nil)
		}
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*WorldChangeLL)(nil))
	_ block.BlockWithRefs      = ((*WorldChangeLL)(nil))
	_ block.BlockWithSubBlocks = ((*WorldChangeLL)(nil))
)
