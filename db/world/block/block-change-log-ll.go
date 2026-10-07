package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	filters "github.com/s4wave/spacewave/db/block/filters"
	"github.com/s4wave/spacewave/db/world"
)

const (
	minChangeLogLLBloomCapacity = 64
	maxChangeLogLLBloomCapacity = 500000

	// HeadChangeCountLimit is the most changes stored inline in an entry.
	HeadChangeCountLimit = 5
	// NodeChangeCountLimit is the most changes in one linked-list node.
	NodeChangeCountLimit = 2048

	// ChangeLogSegmentLen is the number of entries in a changelog segment.
	// The entry with seqno%ChangeLogSegmentLen == 1 starts a new segment.
	ChangeLogSegmentLen = 1024
)

// NewChangeLogLLBlock constructs a new ChangeLogLL block.
func NewChangeLogLLBlock() block.Block {
	return &ChangeLogLL{}
}

// NewChangeLogLLSubBlockCtor returns the sub-block constructor.
func NewChangeLogLLSubBlockCtor(r **ChangeLogLL) block.SubBlockCtor {
	return block.NewSubBlockCtor(r, func() *ChangeLogLL { return &ChangeLogLL{} })
}

// UnmarshalChangeLogLL unmarshals a changelog entry from a cursor.
// Returns nil, nil if the cursor is empty.
func UnmarshalChangeLogLL(ctx context.Context, bcs *block.Cursor) (*ChangeLogLL, error) {
	return block.UnmarshalBlock[*ChangeLogLL](ctx, bcs, NewChangeLogLLBlock)
}

// AppendChangeLogLL appends world changes as the changelog entry at seqno,
// respecting HeadChangeCountLimit and NodeChangeCountLimit.
//
// nextBcs is where the new entry is written. prevBcs is the previous entry in
// the segment, or nil to start a new segment. A sub-block prevBcs is detached
// before it is referenced, so prevBcs and nextBcs may be the same cursor. All
// changes must have the same change type.
//
// Returns the new entry, which is also set into nextBcs.
func AppendChangeLogLL(
	ctx context.Context,
	storeKeyCount uint64,
	seqno uint64,
	nextBcs *block.Cursor,
	prevBcs *block.Cursor,
	worldChangesBcs []*block.Cursor,
) (*ChangeLogLL, error) {
	// Reject an empty change list.
	if len(worldChangesBcs) == 0 {
		return nil, world.ErrEmptyChange
	}

	// Detach a sub-block previous entry, which a block ref cannot target.
	if prevBcs != nil && prevBcs.IsSubBlock() {
		prevBcs = prevBcs.Detach(true)
	}

	// Start the next changelog node from the first change and link the previous node.
	firstChange, err := UnmarshalWorldChange(ctx, worldChangesBcs[0])
	if err != nil {
		return nil, err
	}
	cll := &ChangeLogLL{
		Seqno: seqno,
		// all in the changelog node must be of same type
		ChangeType: firstChange.GetChangeType(),
	}
	nextBcs.ClearAllRefs()
	nextBcs.SetBlock(cll, true)
	if prevBcs != nil {
		nextBcs.SetRef(2, prevBcs)
	}

	// build bloom filter if necessary
	bloomCapacity := int(storeKeyCount) //nolint:gosec
	if len(worldChangesBcs) <= HeadChangeCountLimit {
		bloomCapacity = 0
	} else if bloomCapacity > maxChangeLogLLBloomCapacity {
		bloomCapacity = maxChangeLogLLBloomCapacity
	} else if bloomCapacity < minChangeLogLLBloomCapacity {
		bloomCapacity = minChangeLogLLBloomCapacity
	}

	// Build a key filter when the batch is large enough to need one.
	var kfb *filters.KeyFiltersBuilder
	if bloomCapacity != 0 {
		kfb = filters.NewKeyFiltersBuilder(bloomCapacity)
	}

	// changeBatchBcs located at world-change linked list HEAD sub-block
	changeBatchBcs := nextBcs.FollowSubBlock(3)
	i := 0
	for {
		changeBatch := worldChangesBcs[i:]
		if len(changeBatch) == 0 {
			break
		}
		// limit size of linked-list node
		maxBatchLen := NodeChangeCountLimit
		if maxBatchLen != 0 && len(changeBatch) > maxBatchLen {
			// batch at most NodeChangeCountLimit together
			changeBatch = changeBatch[:maxBatchLen]
		} else if len(changeBatch) > HeadChangeCountLimit {
			// make sure the final batch is <= HeadChangeCountLimit long.
			changeBatch = changeBatch[:len(changeBatch)-HeadChangeCountLimit]
		}

		// update the key filters
		if kfb != nil {
			for _, chBcs := range changeBatch {
				ch, err := UnmarshalWorldChange(ctx, chBcs)
				if err != nil {
					return nil, err
				}
				ApplyWorldChangeToKeyFilters(kfb, ch)
			}
		}

		// update HEAD of linked list by pushing a node
		// internally, detaches the previous WorldChangeLL into a new cursor
		cll.ChangeBatch, err = AppendWorldChangeLL(ctx, changeBatchBcs, changeBatchBcs, changeBatch)
		if err != nil {
			return nil, err
		}
		i += len(changeBatch)
	}

	// Store the built key filter on the changelog node.
	if kfb != nil {
		cll.KeyFilters = kfb.BuildKeyFilters()
	}

	return cll, nil
}

// IsNil returns if the object is nil.
func (w *ChangeLogLL) IsNil() bool {
	return w == nil
}

// IsEmpty checks if the world change is empty.
func (w *ChangeLogLL) IsEmpty() bool {
	return w.GetSeqno() == 0
}

// Clone clones the changelog ll object.
// Note: references the same ChangeBatch object.
// Note: clones the KeyFilters object.
func (w *ChangeLogLL) Clone() *ChangeLogLL {
	if w == nil {
		return nil
	}
	return &ChangeLogLL{
		Seqno:       w.Seqno,
		PrevRef:     w.PrevRef,
		ChangeBatch: w.ChangeBatch,
		ChangeType:  w.ChangeType,
		KeyFilters:  w.KeyFilters.Clone(),
	}
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (w *ChangeLogLL) MarshalBlock() ([]byte, error) {
	return w.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (w *ChangeLogLL) UnmarshalBlock(data []byte) error {
	return w.UnmarshalVT(data)
}

// ApplyBlockRef applies a ref change with a field id.
// The reference may be nil if the child block is nil.
func (w *ChangeLogLL) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	switch id {
	case 2:
		w.PrevRef = ptr
	}
	return nil
}

// GetBlockRefs returns all block references by ID.
// Values may be nil. Pending references in a cursor are not included.
func (w *ChangeLogLL) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	m := make(map[uint32]*block.BlockRef, 4)
	m[2] = w.GetPrevRef()
	return m, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID or unknown.
func (w *ChangeLogLL) GetBlockRefCtor(id uint32) block.Ctor {
	switch id {
	case 2:
		return NewChangeLogLLBlock
	}
	return nil
}

// ApplySubBlock applies a sub-block change with a field id.
func (w *ChangeLogLL) ApplySubBlock(id uint32, next block.SubBlock) error {
	switch id {
	case 3:
		v, ok := next.(*WorldChangeLL)
		if !ok {
			return block.ErrUnexpectedType
		}
		w.ChangeBatch = v
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (w *ChangeLogLL) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	m[3] = w.GetChangeBatch()
	m[5] = w.GetKeyFilters()
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (w *ChangeLogLL) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 3:
		return func(create bool) block.SubBlock {
			v := w.GetChangeBatch()
			if v == nil && create {
				w.ChangeBatch = &WorldChangeLL{}
				v = w.ChangeBatch
			}
			return v
		}
	case 5:
		return func(create bool) block.SubBlock {
			v := w.GetKeyFilters()
			if v == nil && create {
				w.KeyFilters = &filters.KeyFilters{}
				v = w.KeyFilters
			}
			return v
		}
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = (*ChangeLogLL)(nil)
	_ block.BlockWithRefs      = (*ChangeLogLL)(nil)
	_ block.BlockWithSubBlocks = (*ChangeLogLL)(nil)
)
