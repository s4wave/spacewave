package sbset

import (
	"errors"

	"github.com/aperturerobotics/hydra/block"
)

// SubBlockContainer is a sub-block set container.
type SubBlockContainer interface {
	// Get returns the value at the index.
	//
	// Return nil if out of bounds, etc.
	Get(i int) block.SubBlock
	// Len returns the number of elements.
	Len() int
	// Set sets the value at the index.
	Set(i int, r block.SubBlock)
	// Truncate reduces the length to the given len.
	//
	// If nlen >= len, does nothing.
	Truncate(nlen int)
}

// SubBlockSet contains a ordered set of sub-blocks.
type SubBlockSet struct {
	sl  SubBlockContainer
	bcs *block.Cursor
}

// NewSubBlockSet constructs a new SubBlockSet from a slice pointer.
//
// also contains an optional block cursor
func NewSubBlockSet(sl SubBlockContainer, bcs *block.Cursor) *SubBlockSet {
	return &SubBlockSet{sl: sl, bcs: bcs}
}

// GetCursor returns the sub-block cursor located at r, if set.
func (r *SubBlockSet) GetCursor() *block.Cursor {
	return r.bcs
}

// IsNil returns if the object is nil.
func (r *SubBlockSet) IsNil() bool {
	return r == nil
}

// Get gets the sub-block at the index.
//
// returns nil if out of bounds.
func (r *SubBlockSet) Get(idx int) (block.SubBlock, *block.Cursor) {
	if r.sl == nil {
		return nil, nil
	}
	ln := r.sl.Len()
	if idx >= ln {
		return nil, nil
	}
	var nbcs *block.Cursor
	if r.bcs != nil {
		nbcs = r.bcs.FollowSubBlock(uint32(idx)) //nolint:gosec
	}
	return r.sl.Get(idx), nbcs
}

// Len is the number of elements in the collection.
func (r *SubBlockSet) Len() int {
	if r.sl == nil {
		return 0
	}
	return r.sl.Len()
}

// Swap swaps the elements with indexes i and j.
func (r *SubBlockSet) Swap(i, j int) {
	if r.sl == nil {
		return
	}
	ls := r.sl.Len()
	if i >= ls || j >= ls {
		return
	}
	iv := r.sl.Get(i)
	if iv == nil {
		return
	}
	jv := r.sl.Get(j)
	if jv == nil {
		return
	}
	// swap block cursor graph references
	if bcs := r.bcs; bcs != nil {
		ii := uint32(i) //nolint:gosec
		ir := bcs.FollowSubBlock(ii)
		jj := uint32(j) //nolint:gosec
		jr := bcs.FollowSubBlock(jj)
		_ = ir.SetAsSubBlock(jj, bcs)
		_ = jr.SetAsSubBlock(ii, bcs)
	}
	// swap positions in the slice
	r.sl.Set(i, jv)
	r.sl.Set(j, iv)
}

// Delete deletes the given range of elements, shifting the elements following them.
// To remove a single element at index i, call with i, i+1.
func (r *SubBlockSet) Delete(i, j int) {
	ls := r.sl.Len()
	if j > ls {
		j = ls
	}
	if i < 0 {
		i = 0
	}
	nremove := j - i
	if nremove <= 0 || i >= ls {
		return
	}
	for o := range nremove {
		destIdx := uint32(i + o) //nolint:gosec
		srcIdx := uint32(j + o)  //nolint:gosec
		r.bcs.ClearRef(destIdx)
		if int(srcIdx) < ls {
			fromBcs := r.bcs.GetExistingRef(srcIdx)
			if fromBcs != nil {
				r.bcs.ClearRef(srcIdx)
				_ = fromBcs.SetAsSubBlock(destIdx, r.bcs)
			}
		}
	}
	r.sl.Truncate(ls - nremove)
	r.bcs.MarkDirty()
}

// ApplySubBlock applies a sub-block change with a field id.
func (r *SubBlockSet) ApplySubBlock(id uint32, next block.SubBlock) error {
	if r.sl == nil {
		return errors.New("sub-block container is nil")
	}
	l := r.sl.Len()
	if int(id) >= l {
		return errors.New("sub-block reference out of range")
	}
	r.sl.Set(int(id), next)
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (r *SubBlockSet) GetSubBlocks() map[uint32]block.SubBlock {
	if r.sl == nil {
		return nil
	}
	ln := r.sl.Len()
	m := make(map[uint32]block.SubBlock, ln)
	for i := range ln {
		m[uint32(i)] = r.sl.Get(i) //nolint:gosec
	}
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (r *SubBlockSet) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	if r.sl == nil {
		return nil
	}
	idx := int(id)
	return func(create bool) block.SubBlock {
		ln := r.sl.Len()
		if idx >= ln {
			// oob, even if create is set, in this case.
			return nil
		}
		return r.sl.Get(idx)
	}
}

// _ is a type assertion
var (
	_ block.SubBlock           = ((*SubBlockSet)(nil))
	_ block.BlockWithSubBlocks = ((*SubBlockSet)(nil))
)
