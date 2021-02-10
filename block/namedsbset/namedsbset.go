package namedsbset

import (
	"errors"
	"sort"
	"strings"

	"github.com/aperturerobotics/hydra/block"
)

// NamedSubBlock is a named sub-block.
type NamedSubBlock interface {
	// SubBlock indicates this is a sub-block.
	block.SubBlock
	// GetName returns the name of the ref.
	GetName() string
}

// NamedSubBlockContainer is a named sub-block container.
type NamedSubBlockContainer interface {
	// Get returns the value at the index.
	//
	// Return nil if out of bounds, etc.
	Get(i int) NamedSubBlock
	// Len returns the number of elements.
	Len() int
	// Set sets the value at the index.
	Set(i int, r NamedSubBlock)
	// Truncate reduces the length to the given len.
	//
	// If nlen >= len, does nothing.
	Truncate(nlen int)
}

// NamedSubBlockSet contains a set of named sub-blocks.
//
// The list is sorted / keyed by name, unique.
type NamedSubBlockSet struct {
	sl  NamedSubBlockContainer
	bcs *block.Cursor
}

// NewNamedSubBlockSet constructs a new NamedSubBlockSet from a slice pointer.
//
// also contains an optional block cursor
func NewNamedSubBlockSet(sl NamedSubBlockContainer, bcs *block.Cursor) *NamedSubBlockSet {
	return &NamedSubBlockSet{sl: sl, bcs: bcs}
}

// GetCursor returns the sub-block cursor located at r, if set.
func (r *NamedSubBlockSet) GetCursor() *block.Cursor {
	return r.bcs
}

// Len is the number of elements in the collection.
func (r *NamedSubBlockSet) Len() int {
	if r.sl == nil {
		return 0
	}
	return r.sl.Len()
}

// Less reports whether the element with
// index i should sort before the element with index j.
func (r *NamedSubBlockSet) Less(i, j int) bool {
	if r.sl == nil {
		return false
	}
	ls := r.sl.Len()
	if j >= ls {
		return true
	}
	if i >= ls {
		return false
	}
	iv := r.sl.Get(i)
	jv := r.sl.Get(j)
	if iv == nil && jv != nil {
		return false
	}
	if jv == nil && iv != nil {
		return true
	}
	return iv.GetName() < jv.GetName()
}

// Swap swaps the elements with indexes i and j.
func (r *NamedSubBlockSet) Swap(i, j int) {
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
		ii := uint32(i)
		ir := bcs.FollowSubBlock(ii)
		jj := uint32(j)
		jr := bcs.FollowSubBlock(jj)
		bcs.SetRef(jj, ir)
		bcs.SetRef(ii, jr)
	}
	// swap positions in the slice
	r.sl.Set(i, jv)
	r.sl.Set(j, iv)
}

// LookupIndexByName looks up a named sub-block's index by name.
//
// Returns either -1 or len(sl) if not found or nil.
func (r *NamedSubBlockSet) LookupIndexByName(name string) (idx int, sv NamedSubBlock, found bool) {
	if r == nil || r.sl == nil {
		return -1, nil, false
	}
	ls := r.sl.Len()
	idx = sort.Search(ls, func(i int) bool {
		iv := r.sl.Get(i)
		if iv == nil {
			return true
		}
		return strings.Compare(iv.GetName(), name) >= 0
	})
	if idx < 0 || idx >= ls {
		found = false
		idx = -1
	} else {
		sv = r.sl.Get(idx)
		found = sv != nil && sv.GetName() == name
		if !found {
			sv = nil
		}
	}
	return
}

// LookupIndexByNameCaseInsensitive finds the lowest index that matches the name.
//
// Returns either -1 or len(sl) if not found or nil.
func (r *NamedSubBlockSet) LookupIndexByNameCaseInsensitive(name string) (idx int, sv NamedSubBlock, found bool) {
	if r == nil || r.sl == nil {
		return -1, nil, false
	}
	for i := 0; i < r.sl.Len(); i++ {
		v := r.sl.Get(i)
		if v != nil && strings.EqualFold(name, v.GetName()) {
			return i, v, true
		}
	}
	return
}

// SortNamedRefs sorts by name, with any nil entries at the end.
func (r *NamedSubBlockSet) SortNamedRefs() {
	sort.Sort(r)
}

// LookupByName looks up a named sub-block by name.
//
// Returns bcs located at named sub-block.
// Returns nil, false if not found.
// Returns nil block cursor if bcs is not set.
func (r *NamedSubBlockSet) LookupByName(name string) (NamedSubBlock, *block.Cursor, bool) {
	idx, v, ok := r.LookupIndexByName(name)
	if !ok {
		return nil, nil, false
	}
	var nbcs *block.Cursor
	if r.bcs != nil {
		nbcs = r.bcs.FollowSubBlock(uint32(idx))
	}
	return v, nbcs, true
}

// LookupByNameCaseInsensitive finds the lowest index that matches name.
//
// Returns bcs located at named sub-block.
// Returns nil, false if not found.
// Returns nil block cursor if bcs is not set.
func (r *NamedSubBlockSet) LookupByNameCaseInsensitive(name string) (NamedSubBlock, *block.Cursor, bool) {
	idx, v, ok := r.LookupIndexByNameCaseInsensitive(name)
	if !ok {
		return nil, nil, false
	}
	var nbcs *block.Cursor
	if r.bcs != nil {
		nbcs = r.bcs.FollowSubBlock(uint32(idx))
	}
	return v, nbcs, true
}

// DeleteByName deletes a named sub-block by name.
//
// Returns bcs located at old named sub-block.
// Returns nil, nil, false if not found.
// Returns old table header, cursor, true if found
func (r *NamedSubBlockSet) DeleteByName(name string) (NamedSubBlock, *block.Cursor, bool) {
	idx, oldv, ok := r.LookupIndexByName(name)
	if !ok {
		return nil, nil, false
	}
	var ncs *block.Cursor
	if r.bcs != nil {
		ncs = r.bcs.FollowSubBlock(uint32(idx))
	}
	// to delete: swap last index into index, decrement len, re-sort
	slLen := r.sl.Len()
	if slLen > 1 && idx != slLen-1 {
		if r.bcs != nil {
			iecs := r.bcs.FollowSubBlock(uint32(slLen - 1))
			r.bcs.SetRef(uint32(idx), iecs)
			r.bcs.ClearRef(uint32(slLen - 1))
		}
		ie := r.sl.Get(slLen - 1)
		r.sl.Set(idx, ie)
	}
	r.sl.Set(slLen-1, nil)
	r.sl.Truncate(slLen - 1)
	return oldv, ncs, true
}

// ApplySubBlock applies a sub-block change with a field id.
func (r *NamedSubBlockSet) ApplySubBlock(id uint32, next block.SubBlock) error {
	if r.sl == nil {
		return errors.New("sub-block container is nil")
	}
	l := r.sl.Len()
	if int(id) >= l {
		return errors.New("sub-block reference out of range")
	}
	nsb, nsbOk := next.(NamedSubBlock)
	if !nsbOk {
		return errors.New("sub-block was not a namedsubblock")
	}
	r.sl.Set(int(id), nsb)
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (r *NamedSubBlockSet) GetSubBlocks() map[uint32]block.SubBlock {
	if r.sl == nil {
		return nil
	}
	ln := r.sl.Len()
	m := make(map[uint32]block.SubBlock, ln)
	for i := 0; i < ln; i++ {
		m[uint32(i)] = r.sl.Get(i)
	}
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (r *NamedSubBlockSet) GetSubBlockCtor(id uint32) block.SubBlockCtor {
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

// BlockPreWriteHook is called when writing the block.
func (r *NamedSubBlockSet) BlockPreWriteHook() error {
	if r != nil {
		r.SortNamedRefs()
	}
	return nil
}

// _ is a type assertion
var (
	_ block.SubBlock              = ((*NamedSubBlockSet)(nil))
	_ block.BlockWithSubBlocks    = ((*NamedSubBlockSet)(nil))
	_ block.BlockWithPreWriteHook = ((*NamedSubBlockSet)(nil))
	_ sort.Interface              = ((*NamedSubBlockSet)(nil))
)
