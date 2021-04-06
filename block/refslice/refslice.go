package refslice

import (
	"errors"
	"sort"

	"github.com/aperturerobotics/hydra/block"
)

// ErrOutOfBounds indicates a ref was out of bounds.
var ErrOutOfBounds = errors.New("ref out of bounds")

// BlockRefSlice implements block ref slice functions.
type BlockRefSlice struct {
	refs *[]*block.BlockRef
	// bcs is a sub-block cursor located at the slice.
	bcs *block.Cursor
	// blockCtor can be nil, should construct a block at index.
	blockCtor func(idx int) block.Ctor
}

// BlockCtor should construct a block at an index.
type BlockCtor func()

// NewBlockRefSlice builds a new BlockRefSlice from a slice pointer.
// bcs can be nil, should be a cursor located at the slice.
// blockCtor can be nil, should construct a block at index.
// on a object containing []*block.BlockRef, use FollowSubBlock(refID)
func NewBlockRefSlice(
	refs *[]*block.BlockRef,
	bcs *block.Cursor,
	blockCtor func(idx int) block.Ctor,
) *BlockRefSlice {
	return &BlockRefSlice{refs: refs, bcs: bcs, blockCtor: blockCtor}
}

// GetRefs returns the refs slice.
func (d *BlockRefSlice) GetRefs() []*block.BlockRef {
	if d == nil || d.refs == nil {
		return nil
	}
	return *d.refs
}

// Len is the number of elements in the collection.
func (d *BlockRefSlice) Len() int {
	if d.refs == nil {
		return 0
	}
	return len(*d.refs)
}

// Less reports whether the element with
// index i should sort before the element with index j.
// does not do bounds checks
func (d *BlockRefSlice) Less(i, j int) bool {
	if d.refs == nil {
		return false
	}
	refs := *d.refs
	return refs[i].LessThan(refs[j])
}

// Swap swaps the elements with indexes i and j.
// If bcs is set on ref slice, also swaps reference ids.
func (d *BlockRefSlice) Swap(i, j int) {
	if d.refs == nil {
		return
	}
	refs := *d.refs

	if d.bcs != nil {
		iref := d.bcs.FollowRef(uint32(i), refs[i])
		jref := d.bcs.FollowRef(uint32(j), refs[j])
		// swap
		d.bcs.SetRef(uint32(i), jref)
		d.bcs.SetRef(uint32(j), iref)
	}

	// swap slice positions
	p := refs[i]
	refs[i] = refs[j]
	refs[j] = p
}

// BlockPreWriteHook is called when writing the block.
func (d *BlockRefSlice) BlockPreWriteHook() error {
	d.SortBlockRefs()
	return nil
}

// SearchBlockRefs searches a ref slice for a ref.
// If not found returns the index it should be inserted.
func (d *BlockRefSlice) SearchBlockRefs(ref *block.BlockRef) (idx int, match bool) {
	if d.refs == nil {
		return -1, false
	}
	refs := *d.refs
	didx := sort.Search(len(refs), func(idx int) bool {
		// ref <= refs[idx]
		return refs[idx].EqualsRef(ref) || ref.LessThan(refs[idx])
	})
	if didx >= len(refs) || didx < 0 {
		return didx, false
	}
	return didx, refs[didx].EqualsRef(ref)
}

// SortBlockRefs sorts a ref slice.
func (d *BlockRefSlice) SortBlockRefs() {
	sort.Sort(d)
}

// GetBlockRefAtIndex returns a ref at an index.
func (d *BlockRefSlice) GetBlockRefAtIndex(i int) *block.BlockRef {
	if d.refs == nil {
		return nil
	}
	refs := *d.refs
	if i < 0 || i >= len(refs) {
		return nil
	}
	return refs[i]
}

// FollowBlockRefAsCursor follows a index to its node reference.
// bcs must be set on the ref slice
// may return ErrOutOfBounds
func (d *BlockRefSlice) FollowBlockRefAsCursor(idx int) (*block.Cursor, *block.BlockRef, error) {
	if d.refs == nil || d.bcs == nil {
		return nil, nil, ErrOutOfBounds
	}
	refs := *d.refs
	if idx < 0 || idx >= len(refs) {
		return nil, nil, ErrOutOfBounds
	}

	ref := refs[idx]
	subRef := d.bcs.FollowRef(uint32(idx), ref)
	return subRef, ref, nil
}

// RemoveBlockRefs removes one or more directory entries.
// refs must be sorted.
// returns if any were removed.
// after removing all entries be sure to call SortBlockRefs.
func (d *BlockRefSlice) RemoveBlockRefs(rmRefs []*block.BlockRef) (bool, error) {
	if d.refs == nil || len(rmRefs) == 0 {
		return false, nil
	}

	refs := *d.refs
	nextRef := refs[0]
	var any bool
BlockRefLoop:
	for di := 0; di < len(refs); di++ {
		ref := refs[di]
		for nextRef.LessThan(ref) {
			rmRefs = rmRefs[1:]
			if len(rmRefs) == 0 {
				break BlockRefLoop
			}
			nextRef = rmRefs[0]
		}
		if ref.EqualsRef(nextRef) {
			any = true
			rmRefs = rmRefs[1:]
			// clear old reference
			if d.bcs != nil {
				d.bcs.ClearRef(uint32(di))
			}
			if di+1 < len(refs) {
				// update block graph with pre-remove swap
				swapIdx := len(refs) - 1
				if d.bcs != nil {
					sb := d.bcs.FollowSubBlock(uint32(swapIdx))
					d.bcs.SetRef(uint32(di), sb)
				}
				refs[di] = refs[swapIdx]
			}
			// remove ref from slice
			refs = refs[:len(refs)-1]
			if len(refs) == 0 {
				refs = nil
			}
			*d.refs = refs
			if d.bcs != nil {
				d.bcs.SetBlock(d)
			}
			if len(refs) == 0 {
				break BlockRefLoop
			} else {
				nextRef = refs[0]
			}
		}
	}

	// NOTE: call SortBlockRefs if any is true!
	return any, nil
}

// AppendBlockRef appends a entry to the ref slice.
func (d *BlockRefSlice) AppendBlockRef(nent *block.BlockRef) *block.Cursor {
	if d.refs == nil {
		return nil
	}
	nextIdx := len(*d.refs)
	*d.refs = append(*d.refs, nent)
	if d.bcs == nil {
		return nil
	}

	subBlk := d.bcs.FollowRef(uint32(nextIdx), nent)
	subBlk.SetRefAtCursor(nent)
	return subBlk
}

// ApplyBlockRef applies a ref change with a field id.
// The reference may be nil if the child block is nil.
func (d *BlockRefSlice) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	if d.refs == nil {
		return errors.New("nil refs slice reference")
	}
	refSlice := *d.refs
	oldLen := len(refSlice)
	if int(id) >= len(refSlice) {
		if int(id) < cap(refSlice) {
			// extend slice and nil the old positions
			refSlice = refSlice[:int(id)+1]
			for ix := oldLen; ix < len(refSlice); ix++ {
				refSlice[ix] = nil
			}
		} else {
			ds := make([]*block.BlockRef, id+1)
			copy(ds, refSlice)
			refSlice = ds
		}
	}
	refSlice[id] = ptr
	*d.refs = refSlice
	return nil
}

// GetBlockRefs returns all block references by ID.
// May return nil, and values may also be nil.
// Note: this does not include pending references (in a cursor)
func (d *BlockRefSlice) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	refSlice := *d.refs
	if len(refSlice) == 0 {
		return nil, nil
	}

	m := make(map[uint32]*block.BlockRef)
	for i, r := range refSlice {
		if r != nil {
			m[uint32(i)] = r
		}
	}
	if len(m) == 0 {
		m = nil
	}
	return m, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID or unknown.
func (d *BlockRefSlice) GetBlockRefCtor(id uint32) block.Ctor {
	if d.blockCtor == nil {
		return nil
	}
	return d.blockCtor(int(id))
}

// _ is a type assertion
var (
	_ sort.Interface              = ((*BlockRefSlice)(nil))
	_ block.SubBlock              = ((*BlockRefSlice)(nil))
	_ block.BlockWithRefs         = ((*BlockRefSlice)(nil))
	_ block.BlockWithPreWriteHook = ((*BlockRefSlice)(nil))
)
